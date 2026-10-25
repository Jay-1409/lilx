package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type modelHTTP struct {
	client               *http.Client
	provider, model, url string
}

func (m modelHTTP) generate(ctx context.Context, payload any, headers http.Header, decode func([]byte) (string, error)) (string, error) {
	entry := logEntry{Event: "model_response", Provider: m.provider, Model: m.model}
	defer func() { logExchange(ctx, entry) }()
	body, err := json.Marshal(payload)
	if err != nil {
		entry.Error = err.Error()
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(body))
	if err != nil {
		entry.Error = err.Error()
		return "", err
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	req.Header.Set("Content-Type", "application/json")
	logExchange(ctx, logEntry{Event: "model_request", Provider: m.provider, Model: m.model, Method: req.Method, URL: safeLogURL(req.URL), Body: rawLogBody(body)})
	resp, err := m.client.Do(req)
	if err != nil {
		entry.Error = err.Error()
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			entry.Error = urlErr.Err.Error()
		}
		return "", err
	}
	defer resp.Body.Close()
	limited, err := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
	entry.Status = resp.StatusCode
	entry.Body = rawLogBody(limited)
	if err != nil {
		entry.Error = err.Error()
		return "", errors.New("invalid model response")
	}
	if len(limited) > 128<<10 {
		entry.Body = rawLogBody(limited[:128<<10])
		entry.Error = "model response too large; body truncated at 128 KiB"
		return "", errors.New("invalid model response")
	}
	if resp.StatusCode != http.StatusOK {
		entry.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
		return "", modelHTTPError(resp.StatusCode)
	}
	answer, err := decode(limited)
	if err != nil {
		entry.Error = err.Error()
	}
	return answer, err
}
