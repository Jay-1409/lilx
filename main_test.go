package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func modelResponse(text string) *http.Response {
	data, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]string{"text": text}}}}}})
	return response(string(data))
}

func TestRequestCallsApprovedBackendAndValidatesShape(t *testing.T) {
	backendCalls, modelCalls := 0, 0
	a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "backend.test":
			backendCalls++
			if r.URL.Path != "/orders/123" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer backend" {
				t.Errorf("unexpected backend request: %s %s", r.Method, r.URL.Path)
			}
			return response(`{"status":"shipped"}`), nil
		case "model.test":
			modelCalls++
			if r.Header.Get("x-goog-api-key") != "model-secret" {
				t.Error("missing model key")
			}
			var payload struct {
				Contents []struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"contents"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if modelCalls == 2 {
				if !strings.Contains(payload.Contents[0].Parts[0].Text, `"shipped"`) {
					t.Error("backend data missing from composition prompt")
				}
				return modelResponse(`{"status":"shipped"}`), nil
			}
			return modelResponse(`{"calls":[{"operation_id":"getOrder","parameters":{"id":"123"}}]}`), nil
		}
		t.Fatalf("unexpected host %s", r.URL.Host)
		return nil, nil
	}))
	body := `{"nlp_data_required":"Order 123 status","response_format":{"type":"object","properties":{"status":{"type":"string"}},"required":["status"],"additionalProperties":false}}`
	r := httptest.NewRequest(http.MethodPost, "/v1/requests", bytes.NewBufferString(body))
	r.Header.Set("Authorization", "Bearer caller-secret")
	w := httptest.NewRecorder()
	a.authorize(a.handleRequest)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Data  map[string]any `json:"data"`
		Trace []traceCall    `json:"trace"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data["status"] != "shipped" || len(result.Trace) != 1 || result.Trace[0].OperationID != "getOrder" || backendCalls != 1 || modelCalls != 2 {
		t.Fatalf("unexpected result: %+v, backend calls %d, model calls %d", result, backendCalls, modelCalls)
	}
}

func TestRequestAcceptsGemmaThoughtsAndFencedJSON(t *testing.T) {
	modelCalls := 0
	a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "backend.test" {
			return response(`{"status":"shipped"}`), nil
		}
		modelCalls++
		parts := []map[string]any{{"text": "```json\n{\"status\":\"shipped\"}\n```"}}
		if modelCalls == 1 {
			parts = []map[string]any{{"text": "I should get the order first.", "thought": true}, {"text": "```", "thought": true}, {"text": "```json\n{\"calls\":[{\"operation_id\":\"getOrder\",\"parameters\":{\"id\":\"123\"}}]}\n```"}}
		}
		payload, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": parts}}}})
		return response(string(payload)), nil
	}))
	r := httptest.NewRequest(http.MethodPost, "/v1/requests", strings.NewReader(`{"nlp_data_required":"Order 123 status","response_format":{"type":"object","properties":{"status":{"type":"string"}},"required":["status"]}}`))
	r.Header.Set("Authorization", "Bearer caller-secret")
	w := httptest.NewRecorder()
	a.authorize(a.handleRequest)(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"shipped"`) || modelCalls != 2 {
		t.Fatalf("status %d, model calls %d: %s", w.Code, modelCalls, w.Body.String())
	}
}

func TestUnapprovedPlanNeverCallsBackend(t *testing.T) {
	backendCalls := 0
	a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "backend.test" {
			backendCalls++
			return response(`{}`), nil
		}
		return modelResponse(`{"calls":[{"operation_id":"deleteOrder"}]}`), nil
	}))
	r := httptest.NewRequest(http.MethodPost, "/v1/requests", strings.NewReader(`{"nlp_data_required":"delete orders","response_format":{"type":"object"}}`))
	r.Header.Set("Authorization", "Bearer caller-secret")
	w := httptest.NewRecorder()
	a.authorize(a.handleRequest)(w, r)
	if w.Code != http.StatusBadGateway || backendCalls != 0 {
		t.Fatalf("status %d, backend calls %d: %s", w.Code, backendCalls, w.Body.String())
	}
}

func TestGemmaTestEndpoint(t *testing.T) {
	modelCalls := 0
	a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		modelCalls++
		if r.URL.Host != "model.test" || r.Header.Get("x-goog-api-key") != "model-secret" {
			t.Errorf("unexpected model request: %s", r.URL)
		}
		return modelResponse("Hello from Gemma"), nil
	}))
	for _, tc := range []struct {
		name, token, body string
		status            int
	}{
		{"success", "caller-secret", `{"prompt":"Say hello"}`, http.StatusOK},
		{"unauthorized", "wrong", `{"prompt":"Say hello"}`, http.StatusUnauthorized},
		{"empty prompt", "caller-secret", `{"prompt":" "}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/gemma/test", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			a.authorize(a.testGemma)(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == http.StatusOK && !strings.Contains(w.Body.String(), "Hello from Gemma") {
				t.Fatalf("unexpected response: %s", w.Body.String())
			}
		})
	}
	if modelCalls != 1 {
		t.Fatalf("model called %d times, want 1", modelCalls)
	}
}

func TestApprovedOperationsWildcard(t *testing.T) {
	available := map[string]operation{"getOrder": {ID: "getOrder"}, "listOrders": {ID: "listOrders"}}
	approved, err := approvedOperations(available, "*")
	if err != nil || len(approved) != 2 {
		t.Fatalf("wildcard selection: %v, %+v", err, approved)
	}
	approved, err = approvedOperations(available, "getOrder")
	if err != nil || len(approved) != 1 {
		t.Fatalf("explicit selection: %v, %+v", err, approved)
	}
	if _, err := approvedOperations(available, "missing"); err == nil {
		t.Fatal("unknown operation was accepted")
	}
}

func TestCallerAuthCanBeDisabled(t *testing.T) {
	a := &app{}
	r := httptest.NewRequest(http.MethodGet, "/v1/operations", nil)
	w := httptest.NewRecorder()
	a.authorize(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("blank API key rejected request: %d", w.Code)
	}
	a.apiKey = "secret"
	w = httptest.NewRecorder()
	a.authorize(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("configured API key accepted request without token: %d", w.Code)
	}
}

func TestGemmaUsesItsOwnTimeout(t *testing.T) {
	a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-time.After(25 * time.Millisecond):
			return modelResponse("slow but successful"), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}))
	a.client.Timeout = 5 * time.Millisecond
	a.modelTimeout = 100 * time.Millisecond
	answer, err := a.generate(context.Background(), "hello")
	if err != nil || answer != "slow but successful" {
		t.Fatalf("model call was cut off by backend timeout: %q, %v", answer, err)
	}
	if a.client.Timeout != 5*time.Millisecond {
		t.Fatalf("backend timeout changed to %s", a.client.Timeout)
	}
}

func testApp(t *testing.T, transport http.RoundTripper) *app {
	t.Helper()
	u, err := url.Parse("http://backend.test")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{client: &http.Client{Transport: transport}, backend: u, modelURL: "http://model.test", modelKey: "model-secret", backendAuth: "Bearer backend", apiKey: "caller-secret", operations: map[string]operation{"getOrder": {ID: "getOrder", Path: "/orders/{id}", Parameters: []parameter{{Name: "id", In: "path", Required: true}}}}}
	a.startWorkers(2)
	t.Cleanup(func() { close(a.jobs) })
	return a
}
