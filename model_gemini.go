package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type geminiAdapter struct {
	http modelHTTP
	key  string
}

func (g *geminiAdapter) Generate(ctx context.Context, input modelInput) (string, error) {
	// Gemma uses the schema in the prompt; Go validates every proposed result.
	body := map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": input.Prompt}}}}}
	headers := make(http.Header)
	headers.Set("x-goog-api-key", g.key)
	return g.http.generate(ctx, body, headers, decodeGeminiResponse)
}

func decodeGeminiResponse(body []byte) (string, error) {
	var output struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &output); err != nil {
		return "", err
	}
	if len(output.Candidates) == 0 {
		return "", errors.New("model response has no candidates")
	}
	var parts []string
	for _, part := range output.Candidates[0].Content.Parts {
		if !part.Thought && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	if len(parts) == 0 {
		return "", errors.New("model response has no answer text")
	}
	return strings.TrimSpace(strings.Join(parts, "")), nil
}
