package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelServerErrorsHaveBoundedRetries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		statuses  []int
		wantError bool
	}{
		{"recover from server error", []int{500, 200}, false},
		{"stop after two server errors", []int{500, 503}, true},
		{"do not retry client errors", []int{400}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			a := testApp(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				if calls >= len(tc.statuses) {
					t.Fatal("too many model calls")
				}
				status := tc.statuses[calls]
				calls++
				if status == 200 {
					return modelResponse(`{"ok":true}`), nil
				}
				r := response(`{"error":{"message":"provider failure"}}`)
				r.StatusCode = status
				return r, nil
			}))
			err := a.generateValidated(context.Background(), "planning", "return JSON", func(answer string) error {
				var result map[string]any
				return decodeModelJSON(answer, &result)
			})
			if (err != nil) != tc.wantError || calls != len(tc.statuses) {
				t.Fatalf("error %v, calls %d", err, calls)
			}
		})
	}
}

func TestInvalidModelOutputGetsOneCorrectionPerStage(t *testing.T) {
	malformed := "```json\n{\"calls\":[{\"operation_id\":\"getOrder\",\"parameters:{}}]}\n```"
	validPlan := `{"calls":[{"operation_id":"getOrder","parameters":{"id":"123"}}]}`
	for _, tc := range []struct {
		name             string
		answers          []string
		status           int
		backendCalls     int
		validationStages []string
	}{
		{"correct plan and schema", []string{malformed, validPlan, `{"status":123}`, `{"status":"shipped"}`}, 200, 1, []string{"planning", "composition"}},
		{"stop after failed correction", []string{malformed, malformed}, 502, 0, []string{"planning", "planning"}},
		{"reject unauthorized correction", []string{malformed, `{"calls":[{"operation_id":"deleteOrder"}]}`}, 502, 0, []string{"planning"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modelCalls, backendCalls := 0, 0
			a := testApp(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "backend.test" {
					backendCalls++
					return response(`{"status":"shipped"}`), nil
				}
				body, _ := io.ReadAll(r.Body)
				if modelCalls == 1 && !bytes.Contains(body, []byte("Validation error:")) {
					t.Error("correction prompt missing validation error")
				}
				if modelCalls >= len(tc.answers) {
					t.Fatal("unexpected extra model attempt")
				}
				answer := tc.answers[modelCalls]
				modelCalls++
				return modelResponse(answer), nil
			}))
			a.logDir = t.TempDir()
			r := httptest.NewRequest(http.MethodPost, "/v1/requests", strings.NewReader(`{"nlp_data_required":"Order 123 status","response_format":{"type":"object","properties":{"status":{"type":"string"}},"required":["status"]}}`))
			w := httptest.NewRecorder()
			a.logRequest(a.handleRequest)(w, r)
			if w.Code != tc.status || modelCalls != len(tc.answers) || backendCalls != tc.backendCalls {
				t.Fatalf("status %d, model calls %d, backend calls %d: %s", w.Code, modelCalls, backendCalls, w.Body.String())
			}
			if w.Code == 200 && !strings.Contains(w.Body.String(), `"status":"shipped"`) {
				t.Fatalf("unexpected result: %s", w.Body.String())
			}
			logBytes, err := os.ReadFile(filepath.Join(a.logDir, w.Header().Get("X-Lilx-Request-ID")+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			var stages []string
			for _, line := range bytes.Split(bytes.TrimSpace(logBytes), []byte("\n")) {
				var entry logEntry
				if err := json.Unmarshal(line, &entry); err != nil {
					t.Fatal(err)
				}
				if entry.Event == "model_validation_error" {
					stages = append(stages, entry.Stage)
					if entry.Error == "" || entry.Attempt < 1 || entry.Attempt > 2 {
						t.Fatalf("incomplete validation log: %+v", entry)
					}
				}
			}
			if strings.Join(stages, ",") != strings.Join(tc.validationStages, ",") {
				t.Fatalf("validation stages %v, want %v", stages, tc.validationStages)
			}
		})
	}
}
