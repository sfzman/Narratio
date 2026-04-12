package scheduler

import (
	"context"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/store"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

const persistenceTimeout = 5 * time.Second

type Service struct {
	jobStore                store.JobStore
	taskStore               store.TaskStore
	registry                *ExecutorRegistry
	resources               ResourceManager
	clock                   Clock
	scriptTimeoutPerSegment time.Duration
	shotVideoTimeoutPerShot time.Duration
	videoRenderTimeout      time.Duration
	log                     *slog.Logger
}

func NewService(
	jobStore store.JobStore,
	taskStore store.TaskStore,
	registry *ExecutorRegistry,
	resources ResourceManager,
) *Service {
	return &Service{
		jobStore:                jobStore,
		taskStore:               taskStore,
		registry:                registry,
		resources:               resources,
		clock:                   realClock{},
		scriptTimeoutPerSegment: defaultScriptSegmentExecutionTimeout,
		shotVideoTimeoutPerShot: defaultShotVideoExecutionTimeoutPerShot,
		videoRenderTimeout:      defaultVideoRenderExecutionTimeout,
		log:                     slog.Default(),
	}
}

func (s *Service) SetScriptTimeoutPerSegment(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	s.scriptTimeoutPerSegment = timeout
}

func (s *Service) SetVideoRenderTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	s.videoRenderTimeout = timeout
}

func (s *Service) SetShotVideoTimeoutPerShot(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	s.shotVideoTimeoutPerShot = timeout
}

func (s *Service) DispatchOnce(
	ctx context.Context,
	jobID int64,
) (DispatchResult, model.Job, error) {
	s.log.Debug("dispatch once started", "job_id", jobID)
	job, err := s.jobStore.GetJob(ctx, jobID)
	if err != nil {
		s.log.Error("load job for dispatch failed", "job_id", jobID, "error", err)
		return DispatchResult{}, model.Job{}, err
	}

	tasks, err := s.taskStore.ListTasksByJob(ctx, jobID)
	if err != nil {
		s.log.Error("load tasks for dispatch failed", "job_id", jobID, "error", err)
		return DispatchResult{}, model.Job{}, err
	}

	selected, updatedTasks, err := s.prepareDispatch(ctx, job, tasks)
	if err != nil {
		s.log.Error("prepare dispatch failed",
			"job_id", jobID,
			"job_public_id", job.PublicID,
			"error", err,
		)
		return DispatchResult{}, model.Job{}, err
	}
	if err := s.persistDispatchStage(&job, tasks, updatedTasks); err != nil {
		releaseSelectedResources(selected, s.resources)
		s.log.Error("persist pre-execution state failed",
			"job_id", jobID,
			"job_public_id", job.PublicID,
			"error", err,
		)
		return DispatchResult{}, model.Job{}, err
	}
	if len(selected) == 0 {
		result := DispatchResult{Tasks: updatedTasks}
		s.logNoDispatch(result, job)
		return result, job, nil
	}

	result, finalTasks, err := s.executePreparedDispatch(
		ctx,
		&job,
		updatedTasks,
		selected,
	)
	if err != nil {
		return DispatchResult{}, model.Job{}, err
	}
	result.Tasks = finalTasks

	if result.Dispatched {
		if result.DispatchedTaskCount > 1 {
			s.log.Info("tasks dispatched",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"task_ids", result.ExecutedTaskIDs,
				"task_keys", result.ExecutedTaskKeys,
				"task_count", result.DispatchedTaskCount,
				"job_status", job.Status,
				"progress", job.Progress,
			)
			return result, job, nil
		}
		s.log.Info("task dispatched",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", result.ExecutedTaskID,
			"task_key", result.ExecutedTaskKey,
			"job_status", job.Status,
			"progress", job.Progress,
		)
	} else {
		s.logNoDispatch(result, job)
	}

	return result, job, nil
}

func (s *Service) prepareDispatch(
	ctx context.Context,
	job model.Job,
	tasks []model.Task,
) ([]dispatchCandidate, []model.Task, error) {
	updatedTasks := PromoteReadyTasks(tasks)
	selected, err := selectDispatchCandidates(ctx, updatedTasks, s.registry, s.resources)
	if err != nil {
		return nil, nil, err
	}

	return selected, updatedTasks, nil
}

func (s *Service) executePreparedDispatch(
	ctx context.Context,
	job *model.Job,
	updatedTasks []model.Task,
	selected []dispatchCandidate,
) (DispatchResult, []model.Task, error) {
	executingTasks := cloneTasks(updatedTasks)
	persistedTasks := cloneTasks(updatedTasks)
	finalTasks := executingTasks
	allSelected := cloneDispatchCandidates(selected)
	results := make(chan dispatchOutcome, len(updatedTasks))
	runningCount := 0
	var wg sync.WaitGroup
	enrichedSelected := s.withProgressReporters(ctx, selected)
	startDispatchCandidates(
		ctx,
		*job,
		enrichedSelected,
		s.resources,
		s.scriptTimeoutPerSegment,
		s.shotVideoTimeoutPerShot,
		s.videoRenderTimeout,
		results,
		&wg,
	)
	runningCount += len(enrichedSelected)

	for runningCount > 0 {
		outcome := <-results
		runningCount--
		finalTasks = applyDispatchOutcome(*job, finalTasks, outcome)
		finalTasks = PromoteReadyTasks(finalTasks)
		nextSelected, err := selectDispatchCandidates(ctx, finalTasks, s.registry, s.resources)
		if err != nil {
			s.log.Error("select next dispatch candidates failed",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"task_id", outcome.task.ID,
				"task_key", outcome.task.Key,
				"error", err,
			)
			return DispatchResult{}, nil, err
		}
		if err := s.persistDispatchStage(job, persistedTasks, finalTasks); err != nil {
			releaseSelectedResources(nextSelected, s.resources)
			s.log.Error("persist incremental execution state failed",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"task_id", outcome.task.ID,
				"task_key", outcome.task.Key,
				"error", err,
			)
			return DispatchResult{}, nil, err
		}
		persistedTasks = cloneTasks(finalTasks)
		if len(nextSelected) == 0 {
			continue
		}

		enrichedNext := s.withProgressReporters(ctx, nextSelected)
		startDispatchCandidates(
			ctx,
			*job,
			enrichedNext,
			s.resources,
			s.scriptTimeoutPerSegment,
			s.shotVideoTimeoutPerShot,
			s.videoRenderTimeout,
			results,
			&wg,
		)
		runningCount += len(enrichedNext)
		allSelected = append(allSelected, cloneDispatchCandidates(enrichedNext)...)
	}
	wg.Wait()
	close(results)

	return buildDispatchResult(finalTasks, allSelected), finalTasks, nil
}

func (s *Service) withProgressReporters(
	parent context.Context,
	selected []dispatchCandidate,
) []dispatchCandidate {
	enriched := make([]dispatchCandidate, 0, len(selected))
	for _, candidate := range selected {
		reporter := &taskProgressReporter{
			task:  candidate.task,
			tasks: s.taskStore,
			log: slog.Default().With(
				"component", "task_progress",
				"task_id", candidate.task.ID,
				"task_key", candidate.task.Key,
			),
		}
		candidate.task.OutputRef = ensureTaskOutputRef(candidate.task.OutputRef)
		candidate.dependencies = cloneTaskMap(candidate.dependencies)
		candidate.taskCtx = model.WithTaskProgressReporter(parent, reporter)
		enriched = append(enriched, candidate)
	}

	return enriched
}

func (s *Service) persistDispatchStage(
	job *model.Job,
	original []model.Task,
	updated []model.Task,
) error {
	persistCtx, cancel := context.WithTimeout(context.Background(), persistenceTimeout)
	defer cancel()

	now := s.clock.Now()
	updatedWithTimestamps := applyTaskUpdates(original, updated, now)
	if err := s.persistChangedTasks(persistCtx, original, updatedWithTimestamps); err != nil {
		return err
	}

	job.Status, job.Progress, _ = AggregateJobState(
		updatedWithTimestamps,
		job.Status == model.JobStatusCancelling,
	)
	job.Result = buildJobResult(updatedWithTimestamps)
	job.UpdatedAt = now
	if err := s.jobStore.UpdateJob(persistCtx, *job); err != nil {
		return err
	}

	copy(updated, updatedWithTimestamps)
	return nil
}

func (s *Service) logNoDispatch(_ DispatchResult, job model.Job) {
	s.log.Debug("no ready task dispatched",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"job_status", job.Status,
		"progress", job.Progress,
	)
}

func cloneTasks(tasks []model.Task) []model.Task {
	cloned := make([]model.Task, 0, len(tasks))
	for _, task := range tasks {
		cloned = append(cloned, task)
	}

	return cloned
}

func cloneTaskMap(input map[string]model.Task) map[string]model.Task {
	cloned := make(map[string]model.Task, len(input))
	for key, task := range input {
		cloned[key] = task
	}

	return cloned
}

func cloneDispatchCandidates(input []dispatchCandidate) []dispatchCandidate {
	cloned := make([]dispatchCandidate, 0, len(input))
	for _, candidate := range input {
		candidate.dependencies = cloneTaskMap(candidate.dependencies)
		cloned = append(cloned, candidate)
	}

	return cloned
}

func ensureTaskOutputRef(outputRef map[string]any) map[string]any {
	if outputRef == nil {
		return map[string]any{}
	}

	return outputRef
}

type taskProgressReporter struct {
	task  model.Task
	tasks store.TaskStore
	log   *slog.Logger
}

func (r *taskProgressReporter) Report(
	ctx context.Context,
	progress model.TaskProgress,
) error {
	if r == nil || r.tasks == nil {
		return nil
	}

	loadCtx, cancel := context.WithTimeout(context.Background(), persistenceTimeout)
	defer cancel()

	task, err := r.tasks.GetTask(loadCtx, r.task.ID)
	if err != nil {
		r.log.Warn("load task for progress failed", "error", err)
		return nil
	}
	if task.Status != model.TaskStatusRunning {
		return nil
	}

	task.OutputRef = ensureTaskOutputRef(task.OutputRef)
	task.OutputRef["progress"] = map[string]any{
		"phase":   progress.Phase,
		"message": progress.Message,
		"current": progress.Current,
		"total":   progress.Total,
		"unit":    progress.Unit,
	}

	persistCtx, persistCancel := context.WithTimeout(context.Background(), persistenceTimeout)
	defer persistCancel()
	if err := r.tasks.UpdateTask(persistCtx, task); err != nil {
		r.log.Warn("persist task progress failed",
			"error", err,
			"phase", progress.Phase,
		)
		return nil
	}

	r.log.Debug("task progress updated",
		"phase", progress.Phase,
		"message", progress.Message,
		"current", progress.Current,
		"total", progress.Total,
		"unit", progress.Unit,
	)

	return nil
}

func (s *Service) persistChangedTasks(
	ctx context.Context,
	original []model.Task,
	updated []model.Task,
) error {
	for i := range updated {
		if !taskChanged(original[i], updated[i]) {
			continue
		}
		if err := s.taskStore.UpdateTask(ctx, updated[i]); err != nil {
			return err
		}
	}

	return nil
}

func applyTaskUpdates(
	original []model.Task,
	updated []model.Task,
	now time.Time,
) []model.Task {
	result := make([]model.Task, 0, len(updated))
	for i := range updated {
		task := updated[i]
		if taskChanged(original[i], task) {
			task.UpdatedAt = now
		}
		result = append(result, task)
	}

	return result
}

func taskChanged(before model.Task, after model.Task) bool {
	if before.Status != after.Status {
		return true
	}
	if !taskErrorEqual(before.Error, after.Error) {
		return true
	}
	if !reflect.DeepEqual(before.OutputRef, after.OutputRef) {
		return true
	}
	if before.Attempt != after.Attempt {
		return true
	}

	return false
}

func taskErrorEqual(a *model.TaskError, b *model.TaskError) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Code == b.Code && a.Message == b.Message
}

func buildJobResult(tasks []model.Task) *model.JobResult {
	for _, task := range tasks {
		if task.Type != model.TaskTypeVideo || task.Status != model.TaskStatusSucceeded {
			continue
		}

		videoPath, ok := task.OutputRef["artifact_path"].(string)
		if !ok || videoPath == "" {
			return nil
		}

		return &model.JobResult{
			VideoPath: videoPath,
			Duration:  outputFloat(task.OutputRef, "duration_seconds"),
			FileSize:  outputInt64(task.OutputRef, "file_size_bytes"),
		}
	}

	return nil
}

func outputFloat(values map[string]any, key string) float64 {
	value, ok := values[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func outputInt64(values map[string]any, key string) int64 {
	value, ok := values[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	default:
		return 0
	}
}
