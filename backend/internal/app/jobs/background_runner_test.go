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
	mu           sync.Mutex
	calls        []int64
	resultsByJob map[int64][]fakeDispatchStep
	started      chan int64
	blockByJob   map[int64]chan struct{}
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
	f.calls = append(f.calls, jobID)
	steps := f.resultsByJob[jobID]
	step := steps[0]
	f.resultsByJob[jobID] = steps[1:]
	started := f.started
	block := f.blockByJob[jobID]
	f.mu.Unlock()

	if started != nil {
		started <- jobID
	}
	if block != nil {
		<-block
	}

	return step.result, step.job, step.err
}

func TestBackgroundRunnerDispatchesUntilTerminal(t *testing.T) {
	coord := NewRunCoordinator()
	dispatcher := &fakeBackgroundScheduler{
		resultsByJob: map[int64][]fakeDispatchStep{
			7: {
				{
					result: scheduler.DispatchResult{Dispatched: true, ExecutedTaskKey: "outline"},
					job:    model.Job{ID: 7, PublicID: "job_abc123", Status: model.JobStatusQueued, Progress: 16},
				},
				{
					result: scheduler.DispatchResult{Dispatched: true, ExecutedTaskKey: "video"},
					job:    model.Job{ID: 7, PublicID: "job_abc123", Status: model.JobStatusCompleted, Progress: 100},
				},
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

func TestBackgroundRunnerRunsDifferentJobsConcurrently(t *testing.T) {
	coord := NewRunCoordinator()
	dispatcher := &fakeBackgroundScheduler{
		resultsByJob: map[int64][]fakeDispatchStep{
			7: {
				{
					result: scheduler.DispatchResult{Dispatched: true, ExecutedTaskKey: "outline"},
					job:    model.Job{ID: 7, PublicID: "job_7", Status: model.JobStatusCompleted, Progress: 100},
				},
			},
			8: {
				{
					result: scheduler.DispatchResult{Dispatched: true, ExecutedTaskKey: "outline"},
					job:    model.Job{ID: 8, PublicID: "job_8", Status: model.JobStatusCompleted, Progress: 100},
				},
			},
		},
		started: make(chan int64, 2),
		blockByJob: map[int64]chan struct{}{
			7: make(chan struct{}),
			8: make(chan struct{}),
		},
	}
	runner := NewBackgroundRunnerWithWorkerCount(dispatcher, coord, 2)
	t.Cleanup(func() {
		_ = runner.Close()
	})

	runner.Enqueue(7)
	runner.Enqueue(8)

	started := make(map[int64]struct{}, 2)
	deadline := time.After(500 * time.Millisecond)
	for len(started) < 2 {
		select {
		case jobID := <-dispatcher.started:
			started[jobID] = struct{}{}
		case <-deadline:
			t.Fatalf("started jobs = %#v, want both jobs picked by workers", started)
		}
	}

	close(dispatcher.blockByJob[7])
	close(dispatcher.blockByJob[8])

	waitDeadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(waitDeadline) {
		if coord.TryAcquire(7) {
			coord.Release(7)
			if coord.TryAcquire(8) {
				coord.Release(8)
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("jobs 7 and 8 should have been released after concurrent completion")
}
