package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
)

const (
	defaultDispatchTimeout = 2 * time.Hour
	defaultMaxDispatchStep = 32
	defaultQueueSize       = 128
	defaultWorkerCount     = 4
	defaultRetryInterval   = 10 * time.Second
)

type RunCoordinator struct {
	mu     sync.Mutex
	active map[int64]context.CancelFunc
}

func NewRunCoordinator() *RunCoordinator {
	return &RunCoordinator{
		active: make(map[int64]context.CancelFunc),
	}
}

func (c *RunCoordinator) TryAcquire(jobID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.active[jobID]; ok {
		return false
	}

	c.active[jobID] = nil
	return true
}

func (c *RunCoordinator) Release(jobID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.active, jobID)
}

func (c *RunCoordinator) IsActive(jobID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.active[jobID]
	return ok
}

func (c *RunCoordinator) IsRunning(jobID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	cancel, ok := c.active[jobID]
	return ok && cancel != nil
}

func (c *RunCoordinator) SetCancel(jobID int64, cancel context.CancelFunc) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.active[jobID]; !ok {
		return false
	}
	c.active[jobID] = cancel
	return true
}

func (c *RunCoordinator) ClearCancel(jobID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.active[jobID]; ok {
		c.active[jobID] = nil
	}
}

func (c *RunCoordinator) Cancel(jobID int64) bool {
	c.mu.Lock()
	cancel, ok := c.active[jobID]
	c.mu.Unlock()

	if !ok {
		return false
	}
	if cancel != nil {
		cancel()
	}

	return true
}

type BackgroundRunner struct {
	dispatcher      SchedulerDispatcher
	coordinator     *RunCoordinator
	notifier        scheduler.ResourceAvailabilityNotifier
	queue           chan int64
	workerCount     int
	dispatchTimeout time.Duration
	maxDispatchStep int
	retryInterval   time.Duration
	log             *slog.Logger
	wg              sync.WaitGroup
}

func NewBackgroundRunner(
	dispatcher SchedulerDispatcher,
	coordinator *RunCoordinator,
) *BackgroundRunner {
	return NewBackgroundRunnerWithWorkerCount(dispatcher, coordinator, defaultWorkerCount)
}

func NewBackgroundRunnerWithWorkerCount(
	dispatcher SchedulerDispatcher,
	coordinator *RunCoordinator,
	workerCount int,
) *BackgroundRunner {
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}

	runner := &BackgroundRunner{
		dispatcher:      dispatcher,
		coordinator:     coordinator,
		queue:           make(chan int64, defaultQueueSize),
		workerCount:     workerCount,
		dispatchTimeout: defaultDispatchTimeout,
		maxDispatchStep: defaultMaxDispatchStep,
		retryInterval:   defaultRetryInterval,
		log:             slog.Default().With("component", "background_runner"),
	}

	for workerIndex := 0; workerIndex < runner.workerCount; workerIndex++ {
		runner.wg.Add(1)
		go runner.loop(workerIndex)
	}

	return runner
}

func (r *BackgroundRunner) SetResourceAvailabilityNotifier(
	notifier scheduler.ResourceAvailabilityNotifier,
) {
	if r == nil {
		return
	}

	r.notifier = notifier
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

func (r *BackgroundRunner) Cancel(jobID int64) bool {
	if r == nil || r.coordinator == nil {
		return false
	}

	return r.coordinator.Cancel(jobID)
}

func (r *BackgroundRunner) IsActive(jobID int64) bool {
	if r == nil || r.coordinator == nil {
		return false
	}

	return r.coordinator.IsActive(jobID)
}

func (r *BackgroundRunner) IsRunning(jobID int64) bool {
	if r == nil || r.coordinator == nil {
		return false
	}

	return r.coordinator.IsRunning(jobID)
}

func (r *BackgroundRunner) WorkerCount() int {
	if r == nil {
		return 0
	}

	return r.workerCount
}

func (r *BackgroundRunner) Close() error {
	if r == nil {
		return nil
	}

	close(r.queue)
	r.wg.Wait()
	return nil
}

func (r *BackgroundRunner) loop(workerIndex int) {
	defer r.wg.Done()

	for jobID := range r.queue {
		r.log.Info("background worker picked job",
			"worker_index", workerIndex,
			"job_id", jobID,
			"queue_depth", len(r.queue),
		)
		r.runJob(jobID)
		r.log.Info("background worker released job",
			"worker_index", workerIndex,
			"job_id", jobID,
			"queue_depth", len(r.queue),
		)
		r.coordinator.Release(jobID)
	}
}

func (r *BackgroundRunner) runJob(jobID int64) {
	dispatchSteps := 0
	for {
		r.log.Info("background dispatch step started",
			"job_id", jobID,
			"step", dispatchSteps+1,
		)
		dispatchCtx, cancel := context.WithTimeout(context.Background(), r.dispatchTimeout)
		if r.coordinator != nil {
			r.coordinator.SetCancel(jobID, cancel)
		}
		result, job, err := r.dispatcher.DispatchOnce(dispatchCtx, jobID)
		if r.coordinator != nil {
			r.coordinator.ClearCancel(jobID)
		}
		cancel()
		if err != nil {
			r.log.Error("background dispatch failed", "job_id", jobID, "step", dispatchSteps+1, "error", err)
			return
		}
		r.log.Info("background dispatch step completed",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"step", dispatchSteps+1,
			"dispatched", result.Dispatched,
			"dispatched_task_count", result.DispatchedTaskCount,
			"dispatched_task_keys", result.ExecutedTaskKeys,
			"ready_keys", taskKeysByStatus(result.Tasks, model.TaskStatusReady),
			"running_keys", taskKeysByStatus(result.Tasks, model.TaskStatusRunning),
			"failed_keys", taskKeysByStatus(result.Tasks, model.TaskStatusFailed),
			"job_status", job.Status,
			"progress", job.Progress,
		)
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
			readyKeys := taskKeysByStatus(result.Tasks, model.TaskStatusReady)
			runningKeys := taskKeysByStatus(result.Tasks, model.TaskStatusRunning)
			failedKeys := taskKeysByStatus(result.Tasks, model.TaskStatusFailed)
			if len(readyKeys) > 0 {
				woke, wakeReason := r.waitForResourceAvailability(jobID)
				if woke {
					r.log.Info("background dispatch retry after resource wait",
						"job_id", job.ID,
						"job_public_id", job.PublicID,
						"status", job.Status,
						"progress", job.Progress,
						"wake_reason", wakeReason,
						"ready_keys", readyKeys,
						"running_keys", runningKeys,
						"failed_keys", failedKeys,
					)
					continue
				}
				r.log.Info("background dispatch wait aborted",
					"job_id", job.ID,
					"job_public_id", job.PublicID,
					"status", job.Status,
					"progress", job.Progress,
					"wake_reason", wakeReason,
					"ready_keys", readyKeys,
					"running_keys", runningKeys,
					"failed_keys", failedKeys,
				)
			}
			r.log.Debug("background dispatch paused without ready task",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"status", job.Status,
				"progress", job.Progress,
				"ready_keys", readyKeys,
				"running_keys", runningKeys,
				"failed_keys", failedKeys,
			)
			return
		}
		dispatchSteps++
		if dispatchSteps >= r.maxDispatchStep {
			r.log.Warn("background dispatch hit step limit", "job_id", jobID, "step_limit", r.maxDispatchStep)
			return
		}
	}
}

func (r *BackgroundRunner) waitForResourceAvailability(jobID int64) (bool, string) {
	if r == nil {
		return false, "runner_nil"
	}
	if r.retryInterval <= 0 {
		r.retryInterval = defaultRetryInterval
	}

	var availability <-chan struct{}
	cancelSubscription := func() {}
	if r.notifier != nil {
		availability, cancelSubscription = r.notifier.SubscribeAvailability()
	}
	defer cancelSubscription()

	waitCtx, cancel := context.WithCancel(context.Background())
	if r.coordinator != nil {
		r.coordinator.SetCancel(jobID, cancel)
	}
	defer func() {
		if r.coordinator != nil {
			r.coordinator.ClearCancel(jobID)
		}
		cancel()
	}()

	timer := time.NewTimer(r.retryInterval)
	defer timer.Stop()

	r.log.Info("background dispatch waiting for resource availability",
		"job_id", jobID,
		"retry_interval_ms", r.retryInterval.Milliseconds(),
		"has_notifier", r.notifier != nil,
	)

	select {
	case <-waitCtx.Done():
		return false, "cancelled"
	case <-availability:
		return true, "resource_released"
	case <-timer.C:
		return true, "retry_interval_elapsed"
	}
}

func hasReadyTask(tasks []model.Task) bool {
	for _, task := range tasks {
		if task.Status == model.TaskStatusReady {
			return true
		}
	}

	return false
}

func taskKeysByStatus(tasks []model.Task, status model.TaskStatus) []string {
	keys := make([]string, 0)
	for _, task := range tasks {
		if task.Status == status {
			keys = append(keys, task.Key)
		}
	}

	return keys
}

func isTerminalJobStatus(status model.JobStatus) bool {
	switch status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCancelled:
		return true
	default:
		return false
	}
}
