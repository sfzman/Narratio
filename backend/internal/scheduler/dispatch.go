package scheduler

import (
	"context"
	"fmt"

	"github.com/sfzman/Narratio/backend/internal/model"
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
		updated[i].Attempt++
		dependencies := dependencyTasks(updated[i], updated)
		executedTask, err := executor.Execute(ctx, job, updated[i], dependencies)
		resources.Release(task.ResourceKey)
		executedTask = mergeExecutedTask(updated[i], executedTask)
		if err != nil {
			executedTask.Status = model.TaskStatusFailed
			executedTask.Error = &model.TaskError{
				Code:    "task_execution_failed",
				Message: err.Error(),
			}
		} else {
			executedTask.Status = model.TaskStatusSucceeded
			executedTask.Error = nil
		}
		updated[i] = executedTask

		return DispatchResult{
			Tasks:           updated,
			Dispatched:      true,
			ExecutedTaskID:  updated[i].ID,
			ExecutedTaskKey: updated[i].Key,
		}, nil
	}

	return DispatchResult{Tasks: updated}, nil
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
