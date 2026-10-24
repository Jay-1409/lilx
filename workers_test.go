package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestWorkersRunCallsConcurrentlyAndKeepPlanOrder(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{}, 3)
	defer close(release)
	u, err := url.Parse("http://backend.test")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{backend: u, client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		started <- r.URL.Path
		select {
		case <-release:
			return response(`{"ok":true}`), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})}, operations: map[string]operation{
		"first":  {ID: "first", Path: "/first"},
		"second": {ID: "second", Path: "/second"},
		"third":  {ID: "third", Path: "/third"},
	}}
	a.startWorkers(2)
	defer close(a.jobs)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type result struct {
		data  []callResult
		trace []traceCall
		err   error
	}
	done := make(chan result, 1)
	go func() {
		data, trace, err := a.runCalls(ctx, []plannedCall{{OperationID: "first"}, {OperationID: "second"}, {OperationID: "third"}})
		done <- result{data, trace, err}
	}()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("two backend calls did not start concurrently")
		}
	}
	select {
	case path := <-started:
		t.Fatalf("third call started before a worker was free: %s", path)
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("queued third call did not start")
	}
	release <- struct{}{}
	release <- struct{}{}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		for i, id := range []string{"first", "second", "third"} {
			if got.data[i].OperationID != id || got.trace[i].OperationID != id || got.trace[i].Status != http.StatusOK {
				t.Fatalf("result %d out of plan order: %+v, %+v", i, got.data, got.trace)
			}
		}
	case <-ctx.Done():
		t.Fatal("backend calls did not finish")
	}
}
