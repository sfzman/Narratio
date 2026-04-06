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
	imagepipeline "github.com/sfzman/Narratio/backend/internal/pipeline/image"
	scriptpipeline "github.com/sfzman/Narratio/backend/internal/pipeline/script"
	ttspipeline "github.com/sfzman/Narratio/backend/internal/pipeline/tts"
	videopipeline "github.com/sfzman/Narratio/backend/internal/pipeline/video"
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

func TestDispatchOnceExecutesTTSAfterScript(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_tts",
		Token:     "job_token_scheduler_tts",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story", Language: "zh"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "script",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article":  "story",
				"language": "zh",
				"voice_id": "default",
			},
			OutputRef: map[string]any{
				"artifact_type": "script",
				"artifact_path": "jobs/job_public_scheduler_tts/script.json",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "tts",
			Type:        model.TaskTypeTTS,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceTTS,
			DependsOn:   []string{"script"},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
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
		model.TaskTypeTTS: ttspipeline.NewExecutor(),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceTTS: 1,
		}),
	)
	service.clock = fixedSchedulerClock{now: now.Add(time.Minute)}

	result, updatedJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if !result.Dispatched {
		t.Fatal("DispatchOnce() dispatched = false, want true")
	}
	if result.ExecutedTaskKey != "tts" {
		t.Fatalf("DispatchOnce() executed = %q, want %q", result.ExecutedTaskKey, "tts")
	}
	if updatedJob.Status != model.JobStatusCompleted {
		t.Fatalf("job status = %q, want %q", updatedJob.Status, model.JobStatusCompleted)
	}
	if updatedJob.Progress != 100 {
		t.Fatalf("job progress = %d, want 100", updatedJob.Progress)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if len(persistedTasks) != 2 {
		t.Fatalf("task len = %d, want 2", len(persistedTasks))
	}
	if persistedTasks[1].Status != model.TaskStatusSucceeded {
		t.Fatalf("tts status = %q, want %q", persistedTasks[1].Status, model.TaskStatusSucceeded)
	}
	if persistedTasks[1].OutputRef["artifact_type"] != "tts" {
		t.Fatalf("tts output_ref = %#v", persistedTasks[1].OutputRef)
	}
	if persistedTasks[1].OutputRef["script_artifact_ref"] != "jobs/job_public_scheduler_tts/script.json" {
		t.Fatalf("tts script ref = %#v", persistedTasks[1].OutputRef["script_artifact_ref"])
	}
}

func TestDispatchOnceExecutesImageAfterScript(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 6, 10, 30, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_image",
		Token:     "job_token_scheduler_image",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story", Language: "zh"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "script",
			Type:        model.TaskTypeScript,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article":  "story",
				"language": "zh",
				"voice_id": "default",
			},
			OutputRef: map[string]any{
				"artifact_type": "script",
				"artifact_path": "jobs/job_public_scheduler_image/script.json",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "image",
			Type:        model.TaskTypeImage,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceImageGen,
			DependsOn:   []string{"script"},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"image_style": "cinematic",
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
		model.TaskTypeImage: imagepipeline.NewExecutor(),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceImageGen: 1,
		}),
	)
	service.clock = fixedSchedulerClock{now: now.Add(time.Minute)}

	result, updatedJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if !result.Dispatched {
		t.Fatal("DispatchOnce() dispatched = false, want true")
	}
	if result.ExecutedTaskKey != "image" {
		t.Fatalf("DispatchOnce() executed = %q, want %q", result.ExecutedTaskKey, "image")
	}
	if updatedJob.Status != model.JobStatusCompleted {
		t.Fatalf("job status = %q, want %q", updatedJob.Status, model.JobStatusCompleted)
	}
	if updatedJob.Progress != 100 {
		t.Fatalf("job progress = %d, want 100", updatedJob.Progress)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if len(persistedTasks) != 2 {
		t.Fatalf("task len = %d, want 2", len(persistedTasks))
	}
	if persistedTasks[1].Status != model.TaskStatusSucceeded {
		t.Fatalf("image status = %q, want %q", persistedTasks[1].Status, model.TaskStatusSucceeded)
	}
	if persistedTasks[1].OutputRef["artifact_type"] != "image" {
		t.Fatalf("image output_ref = %#v", persistedTasks[1].OutputRef)
	}
	if persistedTasks[1].OutputRef["script_artifact_ref"] != "jobs/job_public_scheduler_image/script.json" {
		t.Fatalf("image script ref = %#v", persistedTasks[1].OutputRef["script_artifact_ref"])
	}
}

func TestDispatchOnceExecutesVideoAndPersistsJobResult(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 6, 11, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_video",
		Token:     "job_token_scheduler_video",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story", Language: "zh"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "tts",
			Type:        model.TaskTypeTTS,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceTTS,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload:     map[string]any{"voice_id": "default"},
			OutputRef: map[string]any{
				"artifact_type":          "tts",
				"artifact_path":          "jobs/job_public_scheduler_video/audio/tts_manifest.json",
				"total_duration_seconds": 9.5,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "image",
			Type:        model.TaskTypeImage,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceImageGen,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload:     map[string]any{"image_style": "cinematic"},
			OutputRef: map[string]any{
				"artifact_type": "image",
				"artifact_path": "jobs/job_public_scheduler_video/images/image_manifest.json",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "video",
			Type:        model.TaskTypeVideo,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceVideoRender,
			DependsOn:   []string{"tts", "image"},
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
		model.TaskTypeVideo: videopipeline.NewExecutor(),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceVideoRender: 1,
		}),
	)
	service.clock = fixedSchedulerClock{now: now.Add(time.Minute)}

	result, updatedJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if !result.Dispatched {
		t.Fatal("DispatchOnce() dispatched = false, want true")
	}
	if result.ExecutedTaskKey != "video" {
		t.Fatalf("DispatchOnce() executed = %q, want %q", result.ExecutedTaskKey, "video")
	}
	if updatedJob.Status != model.JobStatusCompleted {
		t.Fatalf("job status = %q, want %q", updatedJob.Status, model.JobStatusCompleted)
	}
	if updatedJob.Result == nil {
		t.Fatal("job result = nil, want video result")
	}
	if updatedJob.Result.VideoPath != "jobs/job_public_scheduler_video/output/final.mp4" {
		t.Fatalf("video path = %q", updatedJob.Result.VideoPath)
	}
	if updatedJob.Result.Duration != 9.5 {
		t.Fatalf("duration = %v, want 9.5", updatedJob.Result.Duration)
	}

	persistedJob, err := store.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if persistedJob.Result == nil {
		t.Fatal("persisted job result = nil, want video result")
	}
	if persistedJob.Result.FileSize == 0 {
		t.Fatal("persisted job file size = 0, want non-zero")
	}
}

func TestDispatchOncePersistsFailedTaskAfterExecutionContextTimeout(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 6, 11, 30, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_timeout",
		Token:     "job_token_scheduler_timeout",
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
	})
	if err != nil {
		t.Fatalf("InitializeJob() error = %v", err)
	}

	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeOutline: timeoutExecutor{},
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

	_, updatedJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if updatedJob.Status != model.JobStatusFailed {
		t.Fatalf("job status = %q, want %q", updatedJob.Status, model.JobStatusFailed)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if len(persistedTasks) != len(createdTasks) {
		t.Fatalf("task len = %d, want %d", len(persistedTasks), len(createdTasks))
	}
	if persistedTasks[0].Status != model.TaskStatusFailed {
		t.Fatalf("task status = %q, want %q", persistedTasks[0].Status, model.TaskStatusFailed)
	}
	if persistedTasks[0].Error == nil {
		t.Fatal("task error = nil, want persisted error")
	}
}

type timeoutExecutor struct{}

func (timeoutExecutor) Execute(
	_ context.Context,
	_ model.Job,
	task model.Task,
	_ map[string]model.Task,
) (model.Task, error) {
	return task, context.DeadlineExceeded
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
