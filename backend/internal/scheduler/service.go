package scheduler

import (
	"context"
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
	}
}

func (s *Service) DispatchOnce(
	ctx context.Context,
	jobID int64,
) (DispatchResult, model.Job, error) {
	job, err := s.jobStore.GetJob(ctx, jobID)
	if err != nil {
		return DispatchResult{}, model.Job{}, err
	}

	tasks, err := s.taskStore.ListTasksByJob(ctx, jobID)
	if err != nil {
		return DispatchResult{}, model.Job{}, err
	}

	result, err := DispatchNextReadyTask(ctx, job, tasks, s.registry, s.resources)
	if err != nil {
		return DispatchResult{}, model.Job{}, err
	}

	now := s.clock.Now()
	result.Tasks = applyTaskUpdates(tasks, result.Tasks, now)
	if err := s.persistChangedTasks(ctx, tasks, result.Tasks); err != nil {
		return DispatchResult{}, model.Job{}, err
	}

	job.Status, job.Progress, _ = AggregateJobState(
		result.Tasks,
		job.Status == model.JobStatusCancelling,
	)
	job.UpdatedAt = now
	if err := s.jobStore.UpdateJob(ctx, job); err != nil {
		return DispatchResult{}, model.Job{}, err
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
