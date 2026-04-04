package scheduler

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sfzman/Narratio/backend/internal/model"
	sqlstore "github.com/sfzman/Narratio/backend/internal/store/sql"
)

func TestDispatchOncePersistsTaskAndJobState(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 4, 11, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_1",
		Token:     "job_token_scheduler_1",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story", Language: "zh"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	createdTasks, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "outline",
			Type:        model.TaskTypeOutline,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			Key:         "script",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{"outline"},
			Attempt:     0,
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
	if err != nil {
		t.Fatalf("InitializeJob() error = %v", err)
	}

	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeOutline: executorFunc(func(_ context.Context, _ model.Job, _ model.Task) error {
			return nil
		}),
		model.TaskTypeScript: executorFunc(func(_ context.Context, _ model.Job, _ model.Task) error {
			return nil
		}),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceLLMText: 1,
		}),
	)
	service.clock = fixedSchedulerClock{now: now.Add(time.Minute)}

	firstResult, firstJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("first DispatchOnce() error = %v", err)
	}
	if !firstResult.Dispatched {
		t.Fatalf("first DispatchOnce() dispatched = false, want true")
	}
	if firstResult.ExecutedTaskKey != "outline" {
		t.Fatalf("first DispatchOnce() executed = %q, want %q", firstResult.ExecutedTaskKey, "outline")
	}
	if firstJob.Status != model.JobStatusQueued {
		t.Fatalf("first DispatchOnce() job status = %q, want %q", firstJob.Status, model.JobStatusQueued)
	}
	if firstJob.Progress != 50 {
		t.Fatalf("first DispatchOnce() progress = %d, want 50", firstJob.Progress)
	}

	persistedAfterFirst, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after first dispatch error = %v", err)
	}
	if persistedAfterFirst[0].Status != model.TaskStatusSucceeded {
		t.Fatalf("outline status after first dispatch = %q, want %q", persistedAfterFirst[0].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterFirst[1].Status != model.TaskStatusPending {
		t.Fatalf("script status after first dispatch = %q, want %q", persistedAfterFirst[1].Status, model.TaskStatusPending)
	}

	service.clock = fixedSchedulerClock{now: now.Add(2 * time.Minute)}
	secondResult, secondJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("second DispatchOnce() error = %v", err)
	}
	if !secondResult.Dispatched {
		t.Fatalf("second DispatchOnce() dispatched = false, want true")
	}
	if secondResult.ExecutedTaskKey != "script" {
		t.Fatalf("second DispatchOnce() executed = %q, want %q", secondResult.ExecutedTaskKey, "script")
	}
	if secondJob.Status != model.JobStatusCompleted {
		t.Fatalf("second DispatchOnce() job status = %q, want %q", secondJob.Status, model.JobStatusCompleted)
	}
	if secondJob.Progress != 100 {
		t.Fatalf("second DispatchOnce() progress = %d, want 100", secondJob.Progress)
	}

	persistedAfterSecond, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after second dispatch error = %v", err)
	}
	if len(persistedAfterSecond) != len(createdTasks) {
		t.Fatalf("persisted task len = %d, want %d", len(persistedAfterSecond), len(createdTasks))
	}
	if persistedAfterSecond[1].Status != model.TaskStatusSucceeded {
		t.Fatalf("script status after second dispatch = %q, want %q", persistedAfterSecond[1].Status, model.TaskStatusSucceeded)
	}
}

type fixedSchedulerClock struct {
	now time.Time
}

func (f fixedSchedulerClock) Now() time.Time {
	return f.now
}

func newSchedulerTestStore(t *testing.T) *sqlstore.Store {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := applySchedulerTestMigration(db); err != nil {
		t.Fatalf("applySchedulerTestMigration() error = %v", err)
	}

	return sqlstore.New(db)
}

func applySchedulerTestMigration(db *sql.DB) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return os.ErrNotExist
	}

	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "store", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(string(sqlBytes))
	return err
}
