package main

import (
	"context"
	"net/http"
	"testing"
)

func TestLoadOperationsOnlyExposesGET(t *testing.T) {
	spec := `openapi: 3.1.0
paths:
  /orders/{id}:
    parameters:
      - name: id
        in: path
        required: true
    get:
      operationId: getOrder
      summary: Read an order
    post:
      operationId: createOrder
  /status:
    get:
      summary: Read status
`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer spec" {
			t.Error("missing spec authorization")
		}
		return response(spec), nil
	})}
	ops, err := loadOperations(context.Background(), client, "https://spec.test/openapi.yaml", "Bearer spec")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops["getOrder"].Path != "/orders/{id}" || ops["GET /status"].Path != "/status" {
		t.Fatalf("unexpected catalog: %+v", ops)
	}
	if _, ok := ops["createOrder"]; ok {
		t.Fatal("POST operation was exposed")
	}
}
