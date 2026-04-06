package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
)

type fakeBackgroundScheduler struct {
	mu      sync.Mutex
	calls   []int64
	results []fakeDispatchStep
}

type fakeDispatchStep struct {
	result scheduler.DispatchResult
	job    model.Job
	err    error
}

func (f *fakeBackgroundScheduler) DispatchOnce(
	_ context.Context,
	jobID int64,
) (scheduler.DispatchResult, model.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, jobID)
	step := f.results[0]
	f.results = f.results[1:]
	return step.result, step.job, step.err
}

func TestBackgroundRunnerDispatchesUntilTerminal(t *testing.T) {
	coord := NewRunCoordinator()
	dispatcher := &fakeBackgroundScheduler{
		results: []fakeDispatchStep{
			{
				result: scheduler.DispatchResult{Dispatched: true, ExecutedTaskKey: "outline"},
				job:    model.Job{ID: 7, PublicID: "job_abc123", Status: model.JobStatusQueued, Progress: 16},
			},
			{
				result: scheduler.DispatchResult{Dispatched: true, ExecutedTaskKey: "video"},
				job:    model.Job{ID: 7, PublicID: "job_abc123", Status: model.JobStatusCompleted, Progress: 100},
			},
		},
	}
	runner := NewBackgroundRunner(dispatcher, coord)
	runner.dispatchTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		_ = runner.Close()
	})

	runner.Enqueue(7)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		dispatcher.mu.Lock()
		callCount := len(dispatcher.calls)
		dispatcher.mu.Unlock()
		if callCount == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.calls) != 2 {
		t.Fatalf("dispatch calls = %d, want 2", len(dispatcher.calls))
	}
	if dispatcher.calls[0] != 7 || dispatcher.calls[1] != 7 {
		t.Fatalf("dispatch calls = %#v", dispatcher.calls)
	}
	if !coord.TryAcquire(7) {
		t.Fatal("job 7 should have been released after terminal state")
	}
	coord.Release(7)
}
