package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestPromoteReadyTasks(t *testing.T) {
	t.Parallel()

	tasks := []model.Task{
		{Key: "outline", Status: model.TaskStatusSucceeded},
		{Key: "character_sheet", Status: model.TaskStatusSucceeded},
		{
			Key:       "script",
			Status:    model.TaskStatusPending,
			DependsOn: []string{"outline", "character_sheet"},
		},
		{
			Key:       "video",
			Status:    model.TaskStatusPending,
			DependsOn: []string{"tts", "image"},
		},
	}

	got := PromoteReadyTasks(tasks)

	if got[2].Status != model.TaskStatusReady {
		t.Fatalf("script status = %q, want %q", got[2].Status, model.TaskStatusReady)
	}
	if got[3].Status != model.TaskStatusPending {
		t.Fatalf("video status = %q, want %q", got[3].Status, model.TaskStatusPending)
	}
}

func TestAggregateJobState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		tasks                 []model.Task
		cancellationRequested bool
		wantStatus            model.JobStatus
		wantProgress          int
	}{
		{
			name: "queued when pending remains",
			tasks: []model.Task{
				{Status: model.TaskStatusPending},
				{Status: model.TaskStatusReady},
			},
			wantStatus:   model.JobStatusQueued,
			wantProgress: 0,
		},
		{
			name: "running when one task is running",
			tasks: []model.Task{
				{Status: model.TaskStatusSucceeded},
				{Status: model.TaskStatusRunning},
			},
			wantStatus:   model.JobStatusRunning,
			wantProgress: 50,
		},
		{
			name: "failed when any task failed",
			tasks: []model.Task{
				{Status: model.TaskStatusSucceeded},
				{Status: model.TaskStatusFailed},
			},
			wantStatus:   model.JobStatusFailed,
			wantProgress: 50,
		},
		{
			name: "completed when all tasks succeeded or skipped",
			tasks: []model.Task{
				{Status: model.TaskStatusSucceeded},
				{Status: model.TaskStatusSkipped},
			},
			wantStatus:   model.JobStatusCompleted,
			wantProgress: 100,
		},
		{
			name: "cancelling when requested and unfinished remain",
			tasks: []model.Task{
				{Status: model.TaskStatusSucceeded},
				{Status: model.TaskStatusReady},
			},
			cancellationRequested: true,
			wantStatus:            model.JobStatusCancelling,
			wantProgress:          50,
		},
		{
			name: "cancelled when all tasks cancelled",
			tasks: []model.Task{
				{Status: model.TaskStatusCancelled},
				{Status: model.TaskStatusCancelled},
			},
			wantStatus:   model.JobStatusCancelled,
			wantProgress: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotStatus, gotProgress, _ := AggregateJobState(tt.tasks, tt.cancellationRequested)
			if gotStatus != tt.wantStatus {
				t.Fatalf("status = %q, want %q", gotStatus, tt.wantStatus)
			}
			if gotProgress != tt.wantProgress {
				t.Fatalf("progress = %d, want %d", gotProgress, tt.wantProgress)
			}
		})
	}
}

func TestMemoryResourceManager(t *testing.T) {
	t.Parallel()

	manager := NewMemoryResourceManager(map[model.ResourceKey]int{
		model.ResourceLLMText: 2,
	})

	if !manager.TryAcquire(context.Background(), model.ResourceLLMText) {
		t.Fatalf("first acquire = false, want true")
	}
	if !manager.TryAcquire(context.Background(), model.ResourceLLMText) {
		t.Fatalf("second acquire = false, want true")
	}
	if manager.TryAcquire(context.Background(), model.ResourceLLMText) {
		t.Fatalf("third acquire = true, want false")
	}

	manager.Release(model.ResourceLLMText)
	if !manager.TryAcquire(context.Background(), model.ResourceLLMText) {
		t.Fatalf("acquire after release = false, want true")
	}
}

func TestDispatchNextReadyTaskExecutesOneTask(t *testing.T) {
	t.Parallel()

	job := model.Job{ID: 1, PublicID: "job_1"}
	tasks := []model.Task{
		{
			ID:          10,
			Key:         "outline",
			Type:        model.TaskTypeOutline,
			Status:      model.TaskStatusReady,
			ResourceKey: model.ResourceLLMText,
		},
		{
			ID:          11,
			Key:         "script",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusReady,
			ResourceKey: model.ResourceLLMText,
		},
	}

	executed := make([]string, 0, 1)
	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeOutline: executorFunc(func(_ context.Context, _ model.Job, task model.Task) error {
			executed = append(executed, task.Key)
			return nil
		}),
		model.TaskTypeScript: executorFunc(func(_ context.Context, _ model.Job, task model.Task) error {
			executed = append(executed, task.Key)
			return nil
		}),
	})

	manager := NewMemoryResourceManager(map[model.ResourceKey]int{
		model.ResourceLLMText: 1,
	})

	result, err := DispatchNextReadyTask(context.Background(), job, tasks, registry, manager)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}

	if !result.Dispatched {
		t.Fatalf("DispatchNextReadyTask() dispatched = false, want true")
	}
	if result.ExecutedTaskKey != "outline" {
		t.Fatalf("DispatchNextReadyTask() executed key = %q, want %q", result.ExecutedTaskKey, "outline")
	}
	if len(executed) != 1 || executed[0] != "outline" {
		t.Fatalf("executor calls = %#v, want %#v", executed, []string{"outline"})
	}
	if result.Tasks[0].Status != model.TaskStatusSucceeded {
		t.Fatalf("outline status = %q, want %q", result.Tasks[0].Status, model.TaskStatusSucceeded)
	}
	if result.Tasks[1].Status != model.TaskStatusReady {
		t.Fatalf("script status = %q, want %q", result.Tasks[1].Status, model.TaskStatusReady)
	}
}

func TestDispatchNextReadyTaskMarksFailure(t *testing.T) {
	t.Parallel()

	job := model.Job{ID: 2, PublicID: "job_2"}
	tasks := []model.Task{
		{
			ID:          20,
			Key:         "image",
			Type:        model.TaskTypeImage,
			Status:      model.TaskStatusReady,
			ResourceKey: model.ResourceImageGen,
		},
	}

	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeImage: executorFunc(func(_ context.Context, _ model.Job, _ model.Task) error {
			return errors.New("boom")
		}),
	})

	manager := NewMemoryResourceManager(map[model.ResourceKey]int{
		model.ResourceImageGen: 1,
	})

	result, err := DispatchNextReadyTask(context.Background(), job, tasks, registry, manager)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}

	if !result.Dispatched {
		t.Fatalf("DispatchNextReadyTask() dispatched = false, want true")
	}
	if result.Tasks[0].Status != model.TaskStatusFailed {
		t.Fatalf("image status = %q, want %q", result.Tasks[0].Status, model.TaskStatusFailed)
	}
	if result.Tasks[0].Error == nil || result.Tasks[0].Error.Code != "task_execution_failed" {
		t.Fatalf("image error = %#v, want task_execution_failed", result.Tasks[0].Error)
	}
}

type executorFunc func(ctx context.Context, job model.Job, task model.Task) error

func (f executorFunc) Execute(ctx context.Context, job model.Job, task model.Task) error {
	return f(ctx, job, task)
}
