package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultTaskExecutionTimeout             = 20 * time.Minute
	defaultScriptSegmentExecutionTimeout    = 200 * time.Second
	defaultShotVideoExecutionTimeoutPerShot = 200 * time.Second
	defaultVideoRenderExecutionTimeout      = 30 * time.Minute
)

type Executor interface {
	Execute(
		ctx context.Context,
		job model.Job,
		task model.Task,
		dependencies map[string]model.Task,
	) (model.Task, error)
}

type ExecutorRegistry struct {
	executors map[model.TaskType]Executor
}

func NewExecutorRegistry(executors map[model.TaskType]Executor) *ExecutorRegistry {
	cloned := make(map[model.TaskType]Executor, len(executors))
	for taskType, executor := range executors {
		cloned[taskType] = executor
	}

	return &ExecutorRegistry{executors: cloned}
}

func (r *ExecutorRegistry) Get(taskType model.TaskType) (Executor, bool) {
	executor, ok := r.executors[taskType]
	return executor, ok
}

type DispatchResult struct {
	Tasks               []model.Task
	Dispatched          bool
	ExecutedTaskID      int64
	ExecutedTaskKey     string
	ExecutedTaskIDs     []int64
	ExecutedTaskKeys    []string
	DispatchedTaskCount int
}

func DispatchNextReadyTask(
	ctx context.Context,
	job model.Job,
	tasks []model.Task,
	registry *ExecutorRegistry,
	resources ResourceManager,
) (DispatchResult, error) {
	return DispatchNextReadyTaskWithTimeouts(
		ctx,
		job,
		tasks,
		registry,
		resources,
		defaultScriptSegmentExecutionTimeout,
		defaultShotVideoExecutionTimeoutPerShot,
		defaultVideoRenderExecutionTimeout,
	)
}

func DispatchNextReadyTaskWithTimeouts(
	ctx context.Context,
	job model.Job,
	tasks []model.Task,
	registry *ExecutorRegistry,
	resources ResourceManager,
	scriptTimeoutPerSegment time.Duration,
	shotVideoTimeoutPerShot time.Duration,
	videoRenderTimeout time.Duration,
) (DispatchResult, error) {
	updated := PromoteReadyTasks(tasks)
	selected, err := selectDispatchCandidates(ctx, updated, registry, resources)
	if err != nil {
		return DispatchResult{Tasks: updated}, err
	}
	if len(selected) == 0 {
		return DispatchResult{Tasks: updated}, nil
	}

	outcomes := collectDispatchOutcomes(executeDispatchCandidates(
		ctx,
		job,
		selected,
		resources,
		scriptTimeoutPerSegment,
		shotVideoTimeoutPerShot,
		videoRenderTimeout,
	))
	updated = applyDispatchOutcomes(job, updated, outcomes)
	updated = PromoteReadyTasks(updated)

	return buildDispatchResult(updated, selected), nil
}

type dispatchCandidate struct {
	index        int
	task         model.Task
	taskCtx      context.Context
	executor     Executor
	dependencies map[string]model.Task
}

type dispatchOutcome struct {
	index     int
	task      model.Task
	err       error
	ctxErr    error
	panicInfo any
}

func selectDispatchCandidates(
	ctx context.Context,
	tasks []model.Task,
	registry *ExecutorRegistry,
	resources ResourceManager,
) ([]dispatchCandidate, error) {
	selected := make([]dispatchCandidate, 0, len(tasks))
	for i, task := range tasks {
		if task.Status != model.TaskStatusReady {
			continue
		}
		if !resources.TryAcquire(ctx, task.ResourceKey) {
			continue
		}

		executor, ok := registry.Get(task.Type)
		if !ok {
			releaseSelectedResources(selected, resources)
			resources.Release(task.ResourceKey)
			return nil, fmt.Errorf("executor not found for task type %q", task.Type)
		}

		task.Status = model.TaskStatusRunning
		task.Attempt++
		tasks[i] = task
		selected = append(selected, dispatchCandidate{
			index:        i,
			task:         task,
			executor:     executor,
			dependencies: dependencyTasks(task, tasks),
		})
	}

	return selected, nil
}

func releaseSelectedResources(
	selected []dispatchCandidate,
	resources ResourceManager,
) {
	for _, candidate := range selected {
		resources.Release(candidate.task.ResourceKey)
	}
}

func executeDispatchCandidates(
	ctx context.Context,
	job model.Job,
	selected []dispatchCandidate,
	resources ResourceManager,
	scriptTimeoutPerSegment time.Duration,
	shotVideoTimeoutPerShot time.Duration,
	videoRenderTimeout time.Duration,
) <-chan dispatchOutcome {
	results := make(chan dispatchOutcome, len(selected))

	go func() {
		var wg sync.WaitGroup
		startDispatchCandidates(
			ctx,
			job,
			selected,
			resources,
			scriptTimeoutPerSegment,
			shotVideoTimeoutPerShot,
			videoRenderTimeout,
			results,
			&wg,
		)

		wg.Wait()
		close(results)
	}()

	return results
}

func startDispatchCandidates(
	ctx context.Context,
	job model.Job,
	selected []dispatchCandidate,
	resources ResourceManager,
	scriptTimeoutPerSegment time.Duration,
	shotVideoTimeoutPerShot time.Duration,
	videoRenderTimeout time.Duration,
	results chan<- dispatchOutcome,
	wg *sync.WaitGroup,
) {
	for _, candidate := range selected {
		wg.Add(1)
		go func(candidate dispatchCandidate) {
			defer wg.Done()
			defer resources.Release(candidate.task.ResourceKey)
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- dispatchOutcome{
						index:     candidate.index,
						task:      candidate.task,
						panicInfo: recovered,
					}
				}
			}()

			slog.Info("executor triggered",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"task_id", candidate.task.ID,
				"task_key", candidate.task.Key,
				"task_type", candidate.task.Type,
				"resource_key", candidate.task.ResourceKey,
				"attempt", candidate.task.Attempt,
			)
			parentCtx := ctx
			if candidate.taskCtx != nil {
				parentCtx = candidate.taskCtx
			}
			executionCtx, cancel := withTaskExecutionTimeout(
				parentCtx,
				candidate.task,
				candidate.dependencies,
				scriptTimeoutPerSegment,
				shotVideoTimeoutPerShot,
				videoRenderTimeout,
			)
			executedTask, err := candidate.executor.Execute(
				executionCtx,
				job,
				candidate.task,
				candidate.dependencies,
			)
			ctxErr := executionCtx.Err()
			cancel()
			results <- dispatchOutcome{
				index:  candidate.index,
				task:   mergeExecutedTask(candidate.task, executedTask),
				err:    err,
				ctxErr: ctxErr,
			}
		}(candidate)
	}
}

func collectDispatchOutcomes(results <-chan dispatchOutcome) []dispatchOutcome {
	outcomes := make([]dispatchOutcome, 0)
	for outcome := range results {
		outcomes = append(outcomes, outcome)
	}

	return outcomes
}

func applyDispatchOutcomes(
	job model.Job,
	tasks []model.Task,
	outcomes []dispatchOutcome,
) []model.Task {
	for _, outcome := range outcomes {
		tasks = applyDispatchOutcome(job, tasks, outcome)
	}

	return tasks
}

func applyDispatchOutcome(
	job model.Job,
	tasks []model.Task,
	outcome dispatchOutcome,
) []model.Task {
	if tasks[outcome.index].Status == model.TaskStatusCancelled {
		return tasks
	}

	executedTask := outcome.task
	switch {
	case outcome.panicInfo != nil:
		executedTask.Status = model.TaskStatusFailed
		executedTask.Error = &model.TaskError{
			Code:    "task_execution_failed",
			Message: fmt.Sprintf("panic: %v", outcome.panicInfo),
		}
		slog.Error("task execution panicked",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", executedTask.ID,
			"task_key", executedTask.Key,
			"task_type", executedTask.Type,
			"resource_key", executedTask.ResourceKey,
			"panic", outcome.panicInfo,
		)
	case outcome.err == nil:
		executedTask.Status = model.TaskStatusSucceeded
		executedTask.Error = nil
	case outcome.ctxErr == context.Canceled:
		executedTask.Status = model.TaskStatusCancelled
		executedTask.Error = nil
		slog.Warn("task execution cancelled",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", executedTask.ID,
			"task_key", executedTask.Key,
			"task_type", executedTask.Type,
		)
	default:
		executedTask.Status = model.TaskStatusFailed
		executedTask.Error = &model.TaskError{
			Code:    "task_execution_failed",
			Message: outcome.err.Error(),
		}
		slog.Error("task execution failed",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", executedTask.ID,
			"task_key", executedTask.Key,
			"task_type", executedTask.Type,
			"resource_key", executedTask.ResourceKey,
			"error", outcome.err,
		)
	}
	clearTaskProgress(executedTask.OutputRef)
	tasks[outcome.index] = executedTask
	if outcome.ctxErr == context.Canceled {
		tasks = cancelUnfinishedTasks(tasks)
	}

	return tasks
}

func clearTaskProgress(outputRef map[string]any) {
	if outputRef == nil {
		return
	}

	delete(outputRef, "progress")
}

func buildDispatchResult(
	tasks []model.Task,
	selected []dispatchCandidate,
) DispatchResult {
	result := DispatchResult{
		Tasks:               tasks,
		Dispatched:          len(selected) > 0,
		ExecutedTaskIDs:     make([]int64, 0, len(selected)),
		ExecutedTaskKeys:    make([]string, 0, len(selected)),
		DispatchedTaskCount: len(selected),
	}
	for _, candidate := range selected {
		result.ExecutedTaskIDs = append(result.ExecutedTaskIDs, candidate.task.ID)
		result.ExecutedTaskKeys = append(result.ExecutedTaskKeys, candidate.task.Key)
	}
	if len(selected) > 0 {
		result.ExecutedTaskID = selected[0].task.ID
		result.ExecutedTaskKey = selected[0].task.Key
	}

	return result
}

func cancelUnfinishedTasks(tasks []model.Task) []model.Task {
	updated := make([]model.Task, 0, len(tasks))
	for _, task := range tasks {
		switch task.Status {
		case model.TaskStatusPending, model.TaskStatusReady, model.TaskStatusRunning:
			task.Status = model.TaskStatusCancelled
			task.Error = nil
			clearTaskProgress(task.OutputRef)
		}
		updated = append(updated, task)
	}

	return updated
}

func withTaskExecutionTimeout(
	parent context.Context,
	task model.Task,
	dependencies map[string]model.Task,
	scriptTimeoutPerSegment time.Duration,
	shotVideoTimeoutPerShot time.Duration,
	videoRenderTimeout time.Duration,
) (context.Context, context.CancelFunc) {
	timeout := taskExecutionTimeout(
		task,
		dependencies,
		scriptTimeoutPerSegment,
		shotVideoTimeoutPerShot,
		videoRenderTimeout,
	)
	if timeout <= 0 {
		return context.WithCancel(parent)
	}

	return context.WithTimeout(parent, timeout)
}

func taskExecutionTimeout(
	task model.Task,
	dependencies map[string]model.Task,
	scriptTimeoutPerSegment time.Duration,
	shotVideoTimeoutPerShot time.Duration,
	videoRenderTimeout time.Duration,
) time.Duration {
	if task.Type == model.TaskTypeVideo {
		if videoRenderTimeout <= 0 {
			return defaultVideoRenderExecutionTimeout
		}

		return videoRenderTimeout
	}
	if task.Type == model.TaskTypeShotVideo {
		return shotVideoExecutionTimeout(task, dependencies, shotVideoTimeoutPerShot)
	}
	if task.Type != model.TaskTypeScript {
		return defaultTaskExecutionTimeout
	}

	if scriptTimeoutPerSegment <= 0 {
		scriptTimeoutPerSegment = defaultScriptSegmentExecutionTimeout
	}
	segmentCount := scriptSegmentCount(dependencies["segmentation"])
	if segmentCount <= 0 {
		return defaultTaskExecutionTimeout
	}

	return time.Duration(segmentCount) * scriptTimeoutPerSegment
}

func shotVideoExecutionTimeout(
	task model.Task,
	dependencies map[string]model.Task,
	shotVideoTimeoutPerShot time.Duration,
) time.Duration {
	if shotVideoTimeoutPerShot <= 0 {
		shotVideoTimeoutPerShot = defaultShotVideoExecutionTimeoutPerShot
	}

	requestedCount := shotVideoRequestedCount(task)
	if requestedCount == 0 {
		return defaultTaskExecutionTimeout
	}

	shotImageCount := taskOutputInt(dependencies["image"].OutputRef, "shot_image_count")
	if shotImageCount > 0 && requestedCount > shotImageCount {
		requestedCount = shotImageCount
	}
	if requestedCount <= 0 {
		return defaultTaskExecutionTimeout
	}

	return time.Duration(requestedCount) * shotVideoTimeoutPerShot
}

func shotVideoRequestedCount(task model.Task) int {
	if task.Payload == nil {
		return 0
	}

	return taskOutputInt(task.Payload, "video_count")
}

func scriptSegmentCount(task model.Task) int {
	if task.OutputRef == nil {
		return 0
	}

	return taskOutputInt(task.OutputRef, "segment_count")
}

func taskOutputInt(values map[string]any, key string) int {
	value, ok := values[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func mergeExecutedTask(before model.Task, after model.Task) model.Task {
	if after.ID == 0 {
		after.ID = before.ID
	}
	if after.JobID == 0 {
		after.JobID = before.JobID
	}
	if after.Key == "" {
		after.Key = before.Key
	}
	if after.Type == "" {
		after.Type = before.Type
	}
	if after.ResourceKey == "" {
		after.ResourceKey = before.ResourceKey
	}
	if after.MaxAttempts == 0 {
		after.MaxAttempts = before.MaxAttempts
	}
	if after.Attempt == 0 {
		after.Attempt = before.Attempt
	}
	if after.Payload == nil {
		after.Payload = before.Payload
	}
	if after.OutputRef == nil {
		after.OutputRef = before.OutputRef
	}
	if after.DependsOn == nil {
		after.DependsOn = before.DependsOn
	}
	if after.CreatedAt.IsZero() {
		after.CreatedAt = before.CreatedAt
	}
	if after.UpdatedAt.IsZero() {
		after.UpdatedAt = before.UpdatedAt
	}

	return after
}

func dependencyTasks(task model.Task, tasks []model.Task) map[string]model.Task {
	if len(task.DependsOn) == 0 {
		return map[string]model.Task{}
	}

	index := make(map[string]model.Task, len(tasks))
	for _, item := range tasks {
		index[item.Key] = item
	}

	dependencies := make(map[string]model.Task, len(task.DependsOn))
	for _, key := range task.DependsOn {
		if dep, ok := index[key]; ok {
			dependencies[key] = dep
		}
	}

	return dependencies
}
