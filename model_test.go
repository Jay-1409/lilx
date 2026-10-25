package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModelConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, provider, model, key, url string
		invalid                         bool
	}{
		{name: "default Gemini", key: "secret"},
		{name: "Gemini requires key", invalid: true},
		{name: "Gemini rejects local model", model: "qwen3.5:4b", key: "secret", invalid: true},
		{name: "Ollama needs no key", provider: "ollama"},
		{name: "Ollama custom model and URL", provider: "ollama", model: "qwen3.5:9b", url: "http://localhost:11434/proxy/"},
		{name: "unknown provider", provider: "other", invalid: true},
		{name: "bad local URL", provider: "ollama", url: "file:///tmp/model", invalid: true},
		{name: "URL credentials", provider: "ollama", url: "http://secret@localhost:11434", invalid: true},
		{name: "URL query", provider: "ollama", url: "http://localhost:11434?token=secret", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := newModelAdapter(tc.provider, tc.model, tc.key, tc.url, nil)
			if (err != nil) != tc.invalid {
				t.Fatalf("configuration error: %v", err)
			}
			if err != nil {
				return
			}
			switch m := adapter.(type) {
			case *geminiAdapter:
				if m.http.model != "gemma-4-26b-a4b-it" {
					t.Fatalf("default model: %s", m.http.model)
				}
			case *ollamaAdapter:
				wantURL, wantModel := "http://127.0.0.1:11434/api/chat", "qwen3.5:4b"
				if tc.url != "" {
					wantURL, wantModel = "http://localhost:11434/proxy/api/chat", tc.model
				}
				if m.http.url != wantURL || m.http.model != wantModel {
					t.Fatalf("Ollama configuration: %+v", m.http)
				}
			}
		})
	}
}

func TestOllamaRequestPipeline(t *testing.T) {
	for _, scenario := range []string{"success", "schema correction", "unapproved operation"} {
		t.Run(scenario, func(t *testing.T) {
			modelCalls, backendCalls := 0, 0
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "backend.test" {
					backendCalls++
					return response(`{"status":"shipped"}`), nil
				}
				modelCalls++
				if r.Method != "POST" || r.URL.String() != "http://ollama.test/api/chat" || r.Header.Get("x-goog-api-key") != "" || r.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected Ollama request or leaked credentials: %s %s", r.Method, r.URL)
				}
				var payload struct {
					Model    string                           `json:"model"`
					Stream   *bool                            `json:"stream"`
					Think    *bool                            `json:"think"`
					Messages []struct{ Role, Content string } `json:"messages"`
					Format   json.RawMessage                  `json:"format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Model != "qwen3.5:4b" || payload.Stream == nil || *payload.Stream || payload.Think == nil || *payload.Think || len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
					t.Fatalf("bad Ollama payload: %+v", payload)
				}
				answer := `{"status":"shipped"}`
				if modelCalls == 1 {
					if !bytes.Equal(payload.Format, planningSchema) {
						t.Fatal("planning schema missing")
					}
					answer = `{"calls":[{"operation_id":"getOrder","parameters":{"id":"123"}}]}`
					if scenario == "unapproved operation" {
						answer = `{"calls":[{"operation_id":"deleteOrder","parameters":{}}]}`
					}
				} else {
					if !bytes.Contains(payload.Format, []byte(`"status"`)) || !strings.Contains(payload.Messages[0].Content, "shipped") {
						t.Fatal("composition schema or backend data missing")
					}
					if scenario == "schema correction" && modelCalls == 2 {
						answer = `{"status":42}`
					}
					if modelCalls == 3 && !strings.Contains(payload.Messages[0].Content, "Validation error:") {
						t.Fatal("correction prompt missing")
					}
				}
				body, _ := json.Marshal(map[string]any{"message": map[string]string{"content": answer, "thinking": "ignore me"}, "done": true})
				return response(string(body)), nil
			})
			a := testApp(t, transport)
			var err error
			a.model, err = newModelAdapter("ollama", "", "must-not-be-sent", "http://ollama.test", transport)
			if err != nil {
				t.Fatal(err)
			}
			a.logDir = t.TempDir()
			r := httptest.NewRequest("POST", "/v1/requests", strings.NewReader(`{"nlp_data_required":"Order 123 status","response_format":{"type":"object","properties":{"status":{"type":"string"}},"required":["status"],"additionalProperties":false}}`))
			w := httptest.NewRecorder()
			a.logRequest(a.handleRequest)(w, r)
			if scenario == "unapproved operation" {
				if w.Code != 502 || backendCalls != 0 || modelCalls != 1 {
					t.Fatalf("unapproved plan executed: %d %s", w.Code, w.Body)
				}
				return
			}
			wantCalls := 2
			if scenario == "schema correction" {
				wantCalls = 3
			}
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"shipped"`) || backendCalls != 1 || modelCalls != wantCalls {
				t.Fatalf("bad response: %d %s; model calls %d, backend calls %d", w.Code, w.Body, modelCalls, backendCalls)
			}
			logBytes, err := os.ReadFile(filepath.Join(a.logDir, w.Header().Get("X-Lilx-Request-ID")+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(logBytes, []byte(`"provider":"ollama"`)) || !bytes.Contains(logBytes, []byte(`"model":"qwen3.5:4b"`)) || bytes.Contains(logBytes, []byte("must-not-be-sent")) {
				t.Fatal("missing provider log metadata or leaked key")
			}
		})
	}
}

func TestOllamaResponseErrorsAndPlainText(t *testing.T) {
	for _, body := range []string{`{`, `{"done":false,"message":{"content":"partial"}}`, `{"done":true,"message":{"thinking":"no answer"}}`, `{"error":"model not found"}`} {
		if _, err := decodeOllamaResponse([]byte(body)); err == nil {
			t.Errorf("accepted invalid response: %s", body)
		}
	}
	adapter, err := newModelAdapter("ollama", "", "", "", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(`"format"`)) {
			t.Fatal("plain text test should not request JSON output")
		}
		return response(`{"done":true,"message":{"content":" hello "}}`), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	answer, err := adapter.Generate(context.Background(), modelInput{Prompt: "hello"})
	if err != nil || answer != "hello" {
		t.Fatalf("plain text: %q, %v", answer, err)
	}
}

func TestModelTimeoutAndResponseLimit(t *testing.T) {
	for _, provider := range []string{"gemini", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				<-r.Context().Done()
				return nil, r.Context().Err()
			})
			adapter, err := newModelAdapter(provider, "", "secret", "", transport)
			if err != nil {
				t.Fatal(err)
			}
			a := &app{model: adapter, modelTimeout: 5 * time.Millisecond}
			if _, err := a.generate(context.Background(), modelInput{Prompt: "hello"}); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("deadline not enforced: %v", err)
			}
			adapter, err = newModelAdapter(provider, "", "secret", "", roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(strings.Repeat("x", (128<<10)+1)), nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Generate(context.Background(), modelInput{Prompt: "hello"}); err == nil {
				t.Fatal("oversized response accepted")
			}
		})
	}
}
