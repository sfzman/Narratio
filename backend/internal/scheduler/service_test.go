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
	scriptpipeline "github.com/sfzman/Narratio/backend/internal/pipeline/script"
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
			Payload: map[string]any{
				"article":  "story",
				"language": "zh",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "character_sheet",
			Type:        model.TaskTypeCharacterSheet,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article":  "story",
				"language": "zh",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "script",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{"outline", "character_sheet"},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article":  "story",
				"language": "zh",
				"voice_id": "default",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("InitializeJob() error = %v", err)
	}

	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeOutline:        scriptpipeline.NewOutlineExecutor(),
		model.TaskTypeCharacterSheet: scriptpipeline.NewCharacterSheetExecutor(),
		model.TaskTypeScript:         scriptpipeline.NewScriptExecutor(),
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
	if firstJob.Progress != 33 {
		t.Fatalf("first DispatchOnce() progress = %d, want 33", firstJob.Progress)
	}

	persistedAfterFirst, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after first dispatch error = %v", err)
	}
	if persistedAfterFirst[0].Status != model.TaskStatusSucceeded {
		t.Fatalf("outline status after first dispatch = %q, want %q", persistedAfterFirst[0].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterFirst[0].OutputRef["artifact_type"] != "outline" {
		t.Fatalf("outline output_ref after first dispatch = %#v, want artifact_type=outline", persistedAfterFirst[0].OutputRef)
	}
	if persistedAfterFirst[0].Attempt != 1 {
		t.Fatalf("outline attempt after first dispatch = %d, want 1", persistedAfterFirst[0].Attempt)
	}
	if persistedAfterFirst[1].Status != model.TaskStatusReady {
		t.Fatalf("character_sheet status after first dispatch = %q, want %q", persistedAfterFirst[1].Status, model.TaskStatusReady)
	}
	if persistedAfterFirst[2].Status != model.TaskStatusPending {
		t.Fatalf("script status after first dispatch = %q, want %q", persistedAfterFirst[2].Status, model.TaskStatusPending)
	}

	service.clock = fixedSchedulerClock{now: now.Add(2 * time.Minute)}
	secondResult, secondJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("second DispatchOnce() error = %v", err)
	}
	if !secondResult.Dispatched {
		t.Fatalf("second DispatchOnce() dispatched = false, want true")
	}
	if secondResult.ExecutedTaskKey != "character_sheet" {
		t.Fatalf("second DispatchOnce() executed = %q, want %q", secondResult.ExecutedTaskKey, "character_sheet")
	}
	if secondJob.Status != model.JobStatusQueued {
		t.Fatalf("second DispatchOnce() job status = %q, want %q", secondJob.Status, model.JobStatusQueued)
	}
	if secondJob.Progress != 66 {
		t.Fatalf("second DispatchOnce() progress = %d, want 66", secondJob.Progress)
	}

	persistedAfterSecond, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after second dispatch error = %v", err)
	}
	if len(persistedAfterSecond) != len(createdTasks) {
		t.Fatalf("persisted task len = %d, want %d", len(persistedAfterSecond), len(createdTasks))
	}
	if persistedAfterSecond[1].Status != model.TaskStatusSucceeded {
		t.Fatalf("character_sheet status after second dispatch = %q, want %q", persistedAfterSecond[1].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterSecond[1].OutputRef["artifact_type"] != "character_sheet" {
		t.Fatalf("character_sheet output_ref after second dispatch = %#v, want artifact_type=character_sheet", persistedAfterSecond[1].OutputRef)
	}
	if persistedAfterSecond[1].Attempt != 1 {
		t.Fatalf("character_sheet attempt after second dispatch = %d, want 1", persistedAfterSecond[1].Attempt)
	}
	if persistedAfterSecond[2].Status != model.TaskStatusPending {
		t.Fatalf("script status after second dispatch = %q, want %q", persistedAfterSecond[2].Status, model.TaskStatusPending)
	}

	service.clock = fixedSchedulerClock{now: now.Add(3 * time.Minute)}
	thirdResult, thirdJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("third DispatchOnce() error = %v", err)
	}
	if !thirdResult.Dispatched {
		t.Fatalf("third DispatchOnce() dispatched = false, want true")
	}
	if thirdResult.ExecutedTaskKey != "script" {
		t.Fatalf("third DispatchOnce() executed = %q, want %q", thirdResult.ExecutedTaskKey, "script")
	}
	if thirdJob.Status != model.JobStatusCompleted {
		t.Fatalf("third DispatchOnce() job status = %q, want %q", thirdJob.Status, model.JobStatusCompleted)
	}
	if thirdJob.Progress != 100 {
		t.Fatalf("third DispatchOnce() progress = %d, want 100", thirdJob.Progress)
	}

	persistedAfterThird, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after third dispatch error = %v", err)
	}
	if persistedAfterThird[2].Status != model.TaskStatusSucceeded {
		t.Fatalf("script status after third dispatch = %q, want %q", persistedAfterThird[2].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterThird[2].OutputRef["artifact_type"] != "script" {
		t.Fatalf("script output_ref after third dispatch = %#v, want artifact_type=script", persistedAfterThird[2].OutputRef)
	}
	if persistedAfterThird[2].OutputRef["outline_artifact_ref"] != "jobs/job_public_scheduler_1/outline.json" {
		t.Fatalf("script outline ref after third dispatch = %#v", persistedAfterThird[2].OutputRef["outline_artifact_ref"])
	}
	if persistedAfterThird[2].OutputRef["character_ref"] != "jobs/job_public_scheduler_1/character_sheet.json" {
		t.Fatalf("script character ref after third dispatch = %#v", persistedAfterThird[2].OutputRef["character_ref"])
	}
	if persistedAfterThird[2].Attempt != 1 {
		t.Fatalf("script attempt after third dispatch = %d, want 1", persistedAfterThird[2].Attempt)
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
