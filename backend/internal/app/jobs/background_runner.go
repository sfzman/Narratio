package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultDispatchTimeout = 12 * time.Minute
	defaultMaxDispatchStep = 32
	defaultQueueSize       = 128
)

type RunCoordinator struct {
	mu     sync.Mutex
	active map[int64]struct{}
}

func NewRunCoordinator() *RunCoordinator {
	return &RunCoordinator{
		active: make(map[int64]struct{}),
	}
}

func (c *RunCoordinator) TryAcquire(jobID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.active[jobID]; ok {
		return false
	}

	c.active[jobID] = struct{}{}
	return true
}

func (c *RunCoordinator) Release(jobID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.active, jobID)
}

type BackgroundRunner struct {
	dispatcher      SchedulerDispatcher
	coordinator     *RunCoordinator
	queue           chan int64
	dispatchTimeout time.Duration
	maxDispatchStep int
	log             *slog.Logger
	wg              sync.WaitGroup
}

func NewBackgroundRunner(
	dispatcher SchedulerDispatcher,
	coordinator *RunCoordinator,
) *BackgroundRunner {
	runner := &BackgroundRunner{
		dispatcher:      dispatcher,
		coordinator:     coordinator,
		queue:           make(chan int64, defaultQueueSize),
		dispatchTimeout: defaultDispatchTimeout,
		maxDispatchStep: defaultMaxDispatchStep,
		log:             slog.Default().With("component", "background_runner"),
	}

	runner.wg.Add(1)
	go runner.loop()

	return runner
}

func (r *BackgroundRunner) Enqueue(jobID int64) {
	if r == nil || r.dispatcher == nil || r.coordinator == nil {
		return
	}
	if !r.coordinator.TryAcquire(jobID) {
		r.log.Debug("job already scheduled or running", "job_id", jobID)
		return
	}

	select {
	case r.queue <- jobID:
		r.log.Debug("job enqueued", "job_id", jobID)
	default:
		r.coordinator.Release(jobID)
		r.log.Warn("background queue is full, skip enqueue", "job_id", jobID)
	}
}

func (r *BackgroundRunner) Close() error {
	if r == nil {
		return nil
	}

	close(r.queue)
	r.wg.Wait()
	return nil
}

func (r *BackgroundRunner) loop() {
	defer r.wg.Done()

	for jobID := range r.queue {
		r.runJob(jobID)
		r.coordinator.Release(jobID)
	}
}

func (r *BackgroundRunner) runJob(jobID int64) {
	for step := 0; step < r.maxDispatchStep; step++ {
		dispatchCtx, cancel := context.WithTimeout(context.Background(), r.dispatchTimeout)
		result, job, err := r.dispatcher.DispatchOnce(dispatchCtx, jobID)
		cancel()
		if err != nil {
			r.log.Error("background dispatch failed", "job_id", jobID, "step", step+1, "error", err)
			return
		}
		if isTerminalJobStatus(job.Status) {
			r.log.Info("background job reached terminal state",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"status", job.Status,
				"progress", job.Progress,
			)
			return
		}
		if !result.Dispatched {
			r.log.Debug("background dispatch paused without ready task",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"status", job.Status,
				"progress", job.Progress,
			)
			return
		}
	}

	r.log.Warn("background dispatch hit step limit", "job_id", jobID, "step_limit", r.maxDispatchStep)
}

func isTerminalJobStatus(status model.JobStatus) bool {
	switch status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCancelled:
		return true
	default:
		return false
	}
}
