package scheduler

import (
	"context"
	"log/slog"
	"reflect"
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

type Service struct {
	jobStore  store.JobStore
	taskStore store.TaskStore
	registry  *ExecutorRegistry
	resources ResourceManager
	clock     Clock
	log       *slog.Logger
}

func NewService(
	jobStore store.JobStore,
	taskStore store.TaskStore,
	registry *ExecutorRegistry,
	resources ResourceManager,
) *Service {
	return &Service{
		jobStore:  jobStore,
		taskStore: taskStore,
		registry:  registry,
		resources: resources,
		clock:     realClock{},
		log:       slog.Default(),
	}
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

	result, err := DispatchNextReadyTask(ctx, job, tasks, s.registry, s.resources)
	if err != nil {
		s.log.Error("dispatch next ready task failed",
			"job_id", jobID,
			"job_public_id", job.PublicID,
			"error", err,
		)
		return DispatchResult{}, model.Job{}, err
	}

	now := s.clock.Now()
	result.Tasks = applyTaskUpdates(tasks, result.Tasks, now)
	if err := s.persistChangedTasks(ctx, tasks, result.Tasks); err != nil {
		s.log.Error("persist task updates failed",
			"job_id", jobID,
			"job_public_id", job.PublicID,
			"error", err,
		)
		return DispatchResult{}, model.Job{}, err
	}

	job.Status, job.Progress, _ = AggregateJobState(
		result.Tasks,
		job.Status == model.JobStatusCancelling,
	)
	job.Result = buildJobResult(result.Tasks)
	job.UpdatedAt = now
	if err := s.jobStore.UpdateJob(ctx, job); err != nil {
		s.log.Error("persist job state failed",
			"job_id", jobID,
			"job_public_id", job.PublicID,
			"error", err,
		)
		return DispatchResult{}, model.Job{}, err
	}

	if result.Dispatched {
		s.log.Info("task dispatched",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", result.ExecutedTaskID,
			"task_key", result.ExecutedTaskKey,
			"job_status", job.Status,
			"progress", job.Progress,
		)
	} else {
		s.log.Debug("no ready task dispatched",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"job_status", job.Status,
			"progress", job.Progress,
		)
	}

	return result, job, nil
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
