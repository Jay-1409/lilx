package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type parameter struct {
	Name        string `json:"name" yaml:"name"`
	In          string `json:"in" yaml:"in"`
	Required    bool   `json:"required" yaml:"required"`
	Description string `json:"description,omitempty" yaml:"description"`
	Schema      any    `json:"schema,omitempty" yaml:"schema"`
}

type operation struct {
	ID          string      `json:"operation_id"`
	Path        string      `json:"path"`
	Summary     string      `json:"summary,omitempty"`
	Description string      `json:"description,omitempty"`
	Parameters  []parameter `json:"parameters,omitempty"`
}

type specOperation struct {
	OperationID string      `yaml:"operationId"`
	Summary     string      `yaml:"summary"`
	Description string      `yaml:"description"`
	Parameters  []parameter `yaml:"parameters"`
}

type pathItem struct {
	Parameters []parameter    `yaml:"parameters"`
	Get        *specOperation `yaml:"get"`
}

type apiSpec struct {
	OpenAPI string              `yaml:"openapi"`
	Paths   map[string]pathItem `yaml:"paths"`
}

func loadOperations(ctx context.Context, client *http.Client, specURL, authorization string) (map[string]operation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
	if err != nil {
		return nil, err
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAPI spec returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("OpenAPI spec exceeds 4 MiB")
	}
	var spec apiSpec
	if err := yaml.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		return nil, errors.New("OpenAPI 3.x spec required")
	}
	ops := make(map[string]operation)
	for path, item := range spec.Paths {
		if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "..") {
			continue
		}
		get := item.Get
		if get == nil {
			continue
		}
		if get.OperationID == "" {
			get.OperationID = "GET " + path
		}
		params := append(append([]parameter{}, item.Parameters...), get.Parameters...)
		valid := true
		for _, p := range params {
			if p.Name == "" || (p.In != "path" && p.In != "query") {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if _, exists := ops[get.OperationID]; exists {
			return nil, fmt.Errorf("duplicate operationId %q", get.OperationID)
		}
		ops[get.OperationID] = operation{ID: get.OperationID, Path: path, Summary: get.Summary, Description: get.Description, Parameters: params}
	}
	if len(ops) == 0 {
		return nil, errors.New("OpenAPI spec has no supported GET operations")
	}
	return ops, nil
}

func sortedOperations(operations map[string]operation) []operation {
	result := make([]operation, 0, len(operations))
	for _, op := range operations {
		result = append(result, op)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
