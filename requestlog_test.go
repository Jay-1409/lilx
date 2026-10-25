package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConversationLogRecordsConcurrentCallsWithoutCredentials(t *testing.T) {
	var modelCalls atomic.Int32
	a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "backend.test" {
			return response(`{"status":"shipped"}`), nil
		}
		if modelCalls.Add(1) == 1 {
			return modelResponse(`{"calls":[{"operation_id":"getOrder","parameters":{"id":"123"}},{"operation_id":"getOrder","parameters":{"id":"456"}}]}`), nil
		}
		return modelResponse(`{"status":"shipped"}`), nil
	}))
	a.logDir = t.TempDir()
	input := `{"nlp_data_required":"Order status","response_format":{"type":"object","properties":{"status":{"type":"string"}},"required":["status"]}}`
	r := httptest.NewRequest(http.MethodPost, "/v1/requests?api_key=caller-secret", bytes.NewBufferString(input))
	r.Header.Set("Authorization", "Bearer caller-secret")
	w := httptest.NewRecorder()
	a.authorize(a.logRequest(a.handleRequest))(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	id := w.Header().Get("X-Lilx-Request-ID")
	if id == "" {
		t.Fatal("missing request ID")
	}
	path := filepath.Join(a.logDir, id+".jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("log permissions: %o", info.Mode().Perm())
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"caller-secret", "model-secret", "Bearer backend"} {
		if bytes.Contains(content, []byte(secret)) {
			t.Fatalf("log contains credential %q", secret)
		}
	}
	counts := map[string]int{}
	for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
		var entry logEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("invalid JSONL entry: %v", err)
		}
		counts[entry.Event]++
		if entry.Event == "client_request" && !strings.Contains(string(entry.Body), "Order status") {
			t.Fatal("client request missing")
		}
	}
	for event, want := range map[string]int{"client_request": 1, "client_response": 1, "model_request": 2, "model_response": 2, "backend_request": 2, "backend_response": 2} {
		if counts[event] != want {
			t.Errorf("%s count %d, want %d", event, counts[event], want)
		}
	}
	second := httptest.NewRequest(http.MethodGet, "/v1/operations", nil)
	second.Header.Set("Authorization", "Bearer caller-secret")
	secondResponse := httptest.NewRecorder()
	a.authorize(a.logRequest(a.listOperations))(secondResponse, second)
	if secondResponse.Code != http.StatusOK || secondResponse.Header().Get("X-Lilx-Request-ID") == id {
		t.Fatalf("second request did not get its own log: status %d", secondResponse.Code)
	}
	files, err := filepath.Glob(filepath.Join(a.logDir, "*.jsonl"))
	if err != nil || len(files) != 2 {
		t.Fatalf("log files: %v, %v", files, err)
	}
}

func TestGemmaErrorsAreLoggedExactly(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport http.RoundTripper
		want      string
	}{
		{"HTTP error", roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"quota exceeded"}}`)), Header: make(http.Header)}, nil
		}), "HTTP 429: {\"error\":{\"message\":\"quota exceeded\"}}"},
		{"transport error", roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp: connection refused")
		}), "dial tcp: connection refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, tc.transport)
			a.logDir = t.TempDir()
			r := httptest.NewRequest(http.MethodPost, "/v1/gemma/test", strings.NewReader(`{"prompt":"hello"}`))
			r.Header.Set("Authorization", "Bearer caller-secret")
			w := httptest.NewRecorder()
			a.authorize(a.logRequest(a.testModel))(w, r)
			if w.Code != http.StatusBadGateway {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			content, err := os.ReadFile(filepath.Join(a.logDir, w.Header().Get("X-Lilx-Request-ID")+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(content, []byte("model-secret")) {
				t.Fatal("log contains model key")
			}
			found := false
			for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
				var entry logEntry
				if err := json.Unmarshal(line, &entry); err != nil {
					t.Fatal(err)
				}
				if entry.Event == "model_response" {
					found = true
					if entry.Error != tc.want {
						t.Fatalf("logged error %q, want %q", entry.Error, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("missing model response log")
			}
		})
	}
}
