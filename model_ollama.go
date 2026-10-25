package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type ollamaAdapter struct{ http modelHTTP }

func (o *ollamaAdapter) Generate(ctx context.Context, input modelInput) (string, error) {
	body := map[string]any{
		"model":      o.http.model,
		"messages":   []any{map[string]string{"role": "user", "content": input.Prompt}},
		"stream":     false,
		"think":      false,
		"keep_alive": "30m",
		"options":    map[string]any{"temperature": 0},
	}
	if len(input.Schema) > 0 {
		body["format"] = input.Schema
	}
	return o.http.generate(ctx, body, nil, decodeOllamaResponse)
}

func decodeOllamaResponse(body []byte) (string, error) {
	var output struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done  bool   `json:"done"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &output); err != nil {
		return "", err
	}
	if output.Error != "" {
		return "", errors.New(output.Error)
	}
	if !output.Done {
		return "", errors.New("model response is incomplete")
	}
	answer := strings.TrimSpace(output.Message.Content)
	if answer == "" {
		return "", errors.New("model response has no answer text")
	}
	return answer, nil
}
