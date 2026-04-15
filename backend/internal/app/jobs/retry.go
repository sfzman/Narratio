package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
	"github.com/sfzman/Narratio/backend/internal/store"
)

var ErrTaskRetryNotAllowed = errors.New("task retry not allowed")

type RetryOutcome struct {
	Job           model.Job
	TaskKey       string
	Retried       bool
	ResetTaskKeys []string
}

func (s *Service) RetryTask(
	ctx context.Context,
	publicID string,
	taskKey string,
) (RetryOutcome, error) {
	job, tasks, err := s.loadJobAndTasks(ctx, publicID)
	if err != nil {
		return RetryOutcome{}, err
	}

	taskKey = strings.TrimSpace(taskKey)
	targetIndex, err := findTaskIndexByKey(tasks, taskKey)
	if err != nil {
		return RetryOutcome{}, err
	}
	if err := validateTaskRetry(job, tasks, tasks[targetIndex]); err != nil {
		return RetryOutcome{}, err
	}

	now := s.clock.Now()
	resetSet := collectRetrySubtree(tasks, taskKey)
	resetKeys := orderedResetKeys(tasks, resetSet)
	updatedTasks := resetTasksForRetry(tasks, resetSet, now)
	updatedTasks = scheduler.PromoteReadyTasks(updatedTasks)
	if err := persistRetriedTasks(ctx, s.store, updatedTasks); err != nil {
		return RetryOutcome{}, err
	}

	job.Status, job.Progress, _ = scheduler.AggregateJobState(updatedTasks, false)
	job.Error = nil
	job.Result = nil
	job.UpdatedAt = now
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return RetryOutcome{}, fmt.Errorf("update retried job: %w", err)
	}

	if s.runner != nil {
		s.runner.Enqueue(job.ID)
		s.log.Info("job re-enqueued after task retry",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_key", taskKey,
			"reset_task_keys", resetKeys,
		)
	}

	return RetryOutcome{
		Job:           job,
		TaskKey:       taskKey,
		Retried:       true,
		ResetTaskKeys: resetKeys,
	}, nil
}

func (s *Service) loadJobAndTasks(
	ctx context.Context,
	publicID string,
) (model.Job, []model.Task, error) {
	job, err := s.store.GetJobByPublicID(ctx, publicID)
	if err != nil {
		return model.Job{}, nil, fmt.Errorf("get job by public id: %w", err)
	}
	tasks, err := s.store.ListTasksByJob(ctx, job.ID)
	if err != nil {
		return model.Job{}, nil, fmt.Errorf("list tasks by job: %w", err)
	}

	return job, tasks, nil
}

func findTaskIndexByKey(tasks []model.Task, taskKey string) (int, error) {
	for index, task := range tasks {
		if task.Key == taskKey {
			return index, nil
		}
	}

	return -1, store.ErrTaskNotFound
}

func validateTaskRetry(job model.Job, tasks []model.Task, task model.Task) error {
	if job.Status == model.JobStatusCancelling || job.Status == model.JobStatusCancelled {
		return ErrTaskRetryNotAllowed
	}
	if hasTaskStatus(tasks, model.TaskStatusRunning) {
		return ErrTaskRetryNotAllowed
	}
	if task.Status != model.TaskStatusFailed {
		return ErrTaskRetryNotAllowed
	}

	return nil
}

func hasTaskStatus(tasks []model.Task, status model.TaskStatus) bool {
	for _, task := range tasks {
		if task.Status == status {
			return true
		}
	}

	return false
}

func collectRetrySubtree(tasks []model.Task, rootKey string) map[string]struct{} {
	children := make(map[string][]string)
	for _, task := range tasks {
		for _, depKey := range task.DependsOn {
			children[depKey] = append(children[depKey], task.Key)
		}
	}

	subtree := map[string]struct{}{rootKey: {}}
	queue := []string{rootKey}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, childKey := range children[current] {
			if _, ok := subtree[childKey]; ok {
				continue
			}
			subtree[childKey] = struct{}{}
			queue = append(queue, childKey)
		}
	}

	return subtree
}

func orderedResetKeys(tasks []model.Task, resetSet map[string]struct{}) []string {
	keys := make([]string, 0, len(resetSet))
	for _, task := range tasks {
		if _, ok := resetSet[task.Key]; ok {
			keys = append(keys, task.Key)
		}
	}

	return keys
}

func resetTasksForRetry(
	tasks []model.Task,
	resetSet map[string]struct{},
	now time.Time,
) []model.Task {
	updated := make([]model.Task, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := resetSet[task.Key]; ok {
			task.Status = model.TaskStatusPending
			task.Error = nil
			task.OutputRef = map[string]any{}
			task.UpdatedAt = now
		}
		updated = append(updated, task)
	}

	return updated
}

func persistRetriedTasks(
	ctx context.Context,
	taskStore store.TaskStore,
	tasks []model.Task,
) error {
	for _, task := range tasks {
		if err := taskStore.UpdateTask(ctx, task); err != nil {
			return fmt.Errorf("update retried task %d: %w", task.ID, err)
		}
	}

	return nil
}
