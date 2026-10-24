package main

import "context"

type backendJob struct {
	ctx        context.Context
	index      int
	operation  operation
	parameters map[string]string
	out        chan<- backendOutcome
}

type backendOutcome struct {
	index  int
	data   any
	status int
	err    error
}

func (a *app) startWorkers(count int) {
	a.jobs = make(chan backendJob, count*2)
	for range count {
		go func() {
			for job := range a.jobs {
				if err := job.ctx.Err(); err != nil {
					job.out <- backendOutcome{index: job.index, err: err}
					continue
				}
				data, status, err := a.callBackend(job.ctx, job.operation, job.parameters)
				job.out <- backendOutcome{index: job.index, data: data, status: status, err: err}
			}
		}()
	}
}

func (a *app) runCalls(ctx context.Context, calls []plannedCall) ([]callResult, []traceCall, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make(chan backendOutcome, len(calls))
	for index, call := range calls {
		job := backendJob{ctx: ctx, index: index, operation: a.operations[call.OperationID], parameters: call.Parameters, out: out}
		select {
		case a.jobs <- job:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	results := make([]callResult, len(calls))
	traces := make([]traceCall, len(calls))
	seen := make([]bool, len(calls))
	for range calls {
		select {
		case outcome := <-out:
			index := outcome.index
			seen[index] = true
			traces[index] = traceCall{OperationID: calls[index].OperationID, Status: outcome.status}
			if outcome.err != nil {
				return nil, completedTraces(traces, seen), outcome.err
			}
			results[index] = callResult{OperationID: calls[index].OperationID, Data: outcome.data}
		case <-ctx.Done():
			return nil, completedTraces(traces, seen), ctx.Err()
		}
	}
	return results, traces, nil
}

func completedTraces(traces []traceCall, seen []bool) []traceCall {
	completed := make([]traceCall, 0, len(traces))
	for index, ok := range seen {
		if ok {
			completed = append(completed, traces[index])
		}
	}
	return completed
}
