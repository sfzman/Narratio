package scheduler

import (
	"context"
	"fmt"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type Executor interface {
	Execute(ctx context.Context, job model.Job, task model.Task) error
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
	Tasks           []model.Task
	Dispatched      bool
	ExecutedTaskID  int64
	ExecutedTaskKey string
}

func DispatchNextReadyTask(
	ctx context.Context,
	job model.Job,
	tasks []model.Task,
	registry *ExecutorRegistry,
	resources ResourceManager,
) (DispatchResult, error) {
	updated := PromoteReadyTasks(tasks)

	for i, task := range updated {
		if task.Status != model.TaskStatusReady {
			continue
		}
		if !resources.TryAcquire(ctx, task.ResourceKey) {
			continue
		}

		executor, ok := registry.Get(task.Type)
		if !ok {
			resources.Release(task.ResourceKey)
			return DispatchResult{Tasks: updated}, fmt.Errorf("executor not found for task type %q", task.Type)
		}

		updated[i].Status = model.TaskStatusRunning
		err := executor.Execute(ctx, job, updated[i])
		resources.Release(task.ResourceKey)
		if err != nil {
			updated[i].Status = model.TaskStatusFailed
			updated[i].Error = &model.TaskError{
				Code:    "task_execution_failed",
				Message: err.Error(),
			}
		} else {
			updated[i].Status = model.TaskStatusSucceeded
			updated[i].Error = nil
		}

		return DispatchResult{
			Tasks:           updated,
			Dispatched:      true,
			ExecutedTaskID:  updated[i].ID,
			ExecutedTaskKey: updated[i].Key,
		}, nil
	}

	return DispatchResult{Tasks: updated}, nil
}
