package sqlstore

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
)

func TestCreateJobAndGetByPublicID(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	now := time.Date(2026, 4, 3, 16, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID: "job_public_1",
		Token:    "job_token_1",
		Status:   model.JobStatusQueued,
		Progress: 5,
		Spec: model.JobSpec{
			Article: "test article",
			Options: model.RenderOptions{
				VoiceID:    "default",
				ImageStyle: "realistic",
			},
		},
		Warnings:  []string{"trimmed"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.CreateJob(context.Background(), &job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID == 0 {
		t.Fatalf("CreateJob() did not write back auto id")
	}

	got, err := store.GetJobByPublicID(context.Background(), "job_public_1")
	if err != nil {
		t.Fatalf("GetJobByPublicID() error = %v", err)
	}

	if got.ID != job.ID {
		t.Fatalf("GetJobByPublicID() id = %d, want %d", got.ID, job.ID)
	}

	if got.Spec.Options.VoiceID != "default" {
		t.Fatalf("GetJobByPublicID() voice_id = %q, want %q", got.Spec.Options.VoiceID, "default")
	}

	if len(got.Warnings) != 1 || got.Warnings[0] != "trimmed" {
		t.Fatalf("GetJobByPublicID() warnings = %#v, want %#v", got.Warnings, []string{"trimmed"})
	}
}

func TestCreateTasksAndListByJob(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	now := time.Date(2026, 4, 3, 16, 30, 0, 0, time.UTC)
	job := model.Job{
		PublicID: "job_public_2",
		Token:    "job_token_2",
		Status:   model.JobStatusQueued,
		Spec: model.JobSpec{
			Article: "story",
		},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.CreateJob(context.Background(), &job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	tasks, err := store.CreateTasks(context.Background(), []model.Task{
		{
			JobID:       job.ID,
			Key:         "outline",
			Type:        model.TaskTypeOutline,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 2,
			Payload: map[string]any{
				"article": "story",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			JobID:       job.ID,
			Key:         "script",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{"outline"},
			Attempt:     0,
			MaxAttempts: 2,
			Payload: map[string]any{
				"voice_id": "default",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error = %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("CreateTasks() len = %d, want 2", len(tasks))
	}

	if tasks[0].ID == 0 || tasks[1].ID == 0 {
		t.Fatalf("CreateTasks() did not write back auto ids: %#v", tasks)
	}

	got, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ListTasksByJob() len = %d, want 2", len(got))
	}

	if got[1].Key != "script" {
		t.Fatalf("ListTasksByJob() second key = %q, want %q", got[1].Key, "script")
	}

	if len(got[1].DependsOn) != 1 || got[1].DependsOn[0] != "outline" {
		t.Fatalf("ListTasksByJob() depends_on = %#v, want %#v", got[1].DependsOn, []string{"outline"})
	}
}

func TestInitializeJobRollsBackWhenTaskInsertFails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	now := time.Date(2026, 4, 3, 17, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID: "job_public_3",
		Token:    "job_token_3",
		Status:   model.JobStatusQueued,
		Spec: model.JobSpec{
			Article: "story",
		},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "outline",
			Type:        model.TaskTypeOutline,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			Key:         "outline",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
	if err == nil {
		t.Fatalf("InitializeJob() error = nil, want duplicate task key error")
	}

	if _, err := store.GetJobByPublicID(context.Background(), "job_public_3"); err == nil {
		t.Fatalf("GetJobByPublicID() error = nil, want rolled back job to be absent")
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := applyTestMigration(db); err != nil {
		t.Fatalf("applyTestMigration() error = %v", err)
	}

	return New(db)
}

func applyTestMigration(db *sql.DB) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return os.ErrNotExist
	}

	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(string(sqlBytes))
	return err
}
