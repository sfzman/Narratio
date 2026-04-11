package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
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
		Spec:      model.JobSpec{Article: "story"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	createdTasks, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "segmentation",
			Type:        model.TaskTypeSegmentation,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLocalCPU,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article": "story",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "outline",
			Type:        model.TaskTypeOutline,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article": "story",
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
				"article": "story",
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
			DependsOn:   []string{"segmentation", "outline", "character_sheet"},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article":  "story",
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
		model.TaskTypeSegmentation:   scriptpipeline.NewSegmentationExecutor(""),
		model.TaskTypeOutline:        scriptpipeline.NewOutlineExecutor(),
		model.TaskTypeCharacterSheet: scriptpipeline.NewCharacterSheetExecutor(),
		model.TaskTypeScript:         scriptpipeline.NewScriptExecutor(),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceLocalCPU: 1,
			model.ResourceLLMText:  1,
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
	if firstResult.ExecutedTaskKey != "segmentation" {
		t.Fatalf("first DispatchOnce() executed = %q, want %q", firstResult.ExecutedTaskKey, "segmentation")
	}
	if firstResult.DispatchedTaskCount != 2 {
		t.Fatalf("first DispatchOnce() dispatched task count = %d, want 2", firstResult.DispatchedTaskCount)
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
		t.Fatalf("segmentation status after first dispatch = %q, want %q", persistedAfterFirst[0].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterFirst[0].OutputRef["artifact_type"] != "segmentation" {
		t.Fatalf("segmentation output_ref after first dispatch = %#v, want artifact_type=segmentation", persistedAfterFirst[0].OutputRef)
	}
	if persistedAfterFirst[0].Attempt != 1 {
		t.Fatalf("segmentation attempt after first dispatch = %d, want 1", persistedAfterFirst[0].Attempt)
	}
	if persistedAfterFirst[1].Status != model.TaskStatusSucceeded {
		t.Fatalf("outline status after first dispatch = %q, want %q", persistedAfterFirst[1].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterFirst[1].OutputRef["artifact_type"] != "outline" {
		t.Fatalf("outline output_ref after first dispatch = %#v, want artifact_type=outline", persistedAfterFirst[1].OutputRef)
	}
	if persistedAfterFirst[1].Attempt != 1 {
		t.Fatalf("outline attempt after first dispatch = %d, want 1", persistedAfterFirst[1].Attempt)
	}
	if persistedAfterFirst[2].Status != model.TaskStatusReady {
		t.Fatalf("character_sheet status after first dispatch = %q, want %q", persistedAfterFirst[2].Status, model.TaskStatusReady)
	}
	if persistedAfterFirst[3].Status != model.TaskStatusPending {
		t.Fatalf("script status after first dispatch = %q, want %q", persistedAfterFirst[3].Status, model.TaskStatusPending)
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
	if secondJob.Progress != 75 {
		t.Fatalf("second DispatchOnce() progress = %d, want 75", secondJob.Progress)
	}

	persistedAfterSecond, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after second dispatch error = %v", err)
	}
	if len(persistedAfterSecond) != len(createdTasks) {
		t.Fatalf("persisted task len = %d, want %d", len(persistedAfterSecond), len(createdTasks))
	}
	if persistedAfterSecond[2].Status != model.TaskStatusSucceeded {
		t.Fatalf("character_sheet status after second dispatch = %q, want %q", persistedAfterSecond[2].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterSecond[2].OutputRef["artifact_type"] != "character_sheet" {
		t.Fatalf("character_sheet output_ref after second dispatch = %#v, want artifact_type=character_sheet", persistedAfterSecond[2].OutputRef)
	}
	if persistedAfterSecond[2].Attempt != 1 {
		t.Fatalf("character_sheet attempt after second dispatch = %d, want 1", persistedAfterSecond[2].Attempt)
	}
	if persistedAfterSecond[3].Status != model.TaskStatusReady {
		t.Fatalf("script status after second dispatch = %q, want %q", persistedAfterSecond[3].Status, model.TaskStatusReady)
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
	if persistedAfterThird[3].Status != model.TaskStatusSucceeded {
		t.Fatalf("script status after third dispatch = %q, want %q", persistedAfterThird[3].Status, model.TaskStatusSucceeded)
	}
	if persistedAfterThird[3].OutputRef["artifact_type"] != "script" {
		t.Fatalf("script output_ref after third dispatch = %#v, want artifact_type=script", persistedAfterThird[3].OutputRef)
	}
	if persistedAfterThird[3].OutputRef["segmentation_ref"] != "jobs/job_public_scheduler_1/segments.json" {
		t.Fatalf("script segmentation ref after third dispatch = %#v", persistedAfterThird[3].OutputRef["segmentation_ref"])
	}
	if persistedAfterThird[3].OutputRef["outline_artifact_ref"] != "jobs/job_public_scheduler_1/outline.json" {
		t.Fatalf("script outline ref after third dispatch = %#v", persistedAfterThird[3].OutputRef["outline_artifact_ref"])
	}
	if persistedAfterThird[3].OutputRef["character_ref"] != "jobs/job_public_scheduler_1/character_sheet.json" {
		t.Fatalf("script character ref after third dispatch = %#v", persistedAfterThird[3].OutputRef["character_ref"])
	}
	if persistedAfterThird[3].Attempt != 1 {
		t.Fatalf("script attempt after third dispatch = %d, want 1", persistedAfterThird[3].Attempt)
	}
}

func TestDispatchOnceExecutesTTSAfterSegmentation(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	workspaceDir := t.TempDir()
	if err := writeSchedulerJSONArtifact(
		workspaceDir,
		"jobs/job_public_scheduler_tts/segments.json",
		map[string]any{
			"segments": []map[string]any{
				{"index": 0, "text": "第一段"},
				{"index": 1, "text": "第二段"},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(segmentation) error = %v", err)
	}
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_tts",
		Token:     "job_token_scheduler_tts",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "segmentation",
			Type:        model.TaskTypeSegmentation,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceLocalCPU,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article": "story",
			},
			OutputRef: map[string]any{
				"artifact_type": "segmentation",
				"artifact_path": "jobs/job_public_scheduler_tts/segments.json",
				"segment_count": 2,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "tts",
			Type:        model.TaskTypeTTS,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceTTS,
			DependsOn:   []string{"segmentation"},
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
		model.TaskTypeTTS: ttspipeline.NewExecutor(workspaceDir),
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
	if persistedTasks[1].OutputRef["segmentation_artifact_ref"] != "jobs/job_public_scheduler_tts/segments.json" {
		t.Fatalf("tts segmentation ref = %#v", persistedTasks[1].OutputRef["segmentation_artifact_ref"])
	}
}

func TestDispatchOncePromotesTTSReadyImmediatelyAfterSegmentationSucceeds(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_tts_ready",
		Token:     "job_token_scheduler_tts_ready",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "segmentation",
			Type:        model.TaskTypeSegmentation,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLocalCPU,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article": "第一段。第二段。",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "tts",
			Type:        model.TaskTypeTTS,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceTTS,
			DependsOn:   []string{"segmentation"},
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
		model.TaskTypeSegmentation: scriptpipeline.NewSegmentationExecutor(""),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceLocalCPU: 1,
			model.ResourceTTS:      1,
		}),
	)
	service.clock = fixedSchedulerClock{now: now.Add(time.Minute)}

	_, updatedJob, err := service.DispatchOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if updatedJob.Progress != 50 {
		t.Fatalf("job progress = %d, want 50", updatedJob.Progress)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if persistedTasks[0].Status != model.TaskStatusSucceeded {
		t.Fatalf("segmentation status = %q, want %q", persistedTasks[0].Status, model.TaskStatusSucceeded)
	}
	if persistedTasks[1].Status != model.TaskStatusReady {
		t.Fatalf("tts status = %q, want %q", persistedTasks[1].Status, model.TaskStatusReady)
	}
}

func TestDispatchOncePersistsRunningStateBeforeExecutionCompletes(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 11, 15, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_running_state",
		Token:     "job_token_scheduler_running_state",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
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
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article": "story",
			},
			OutputRef: map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("InitializeJob() error = %v", err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeOutline: executorFunc(func(
			_ context.Context,
			_ model.Job,
			task model.Task,
			_ map[string]model.Task,
		) (model.Task, error) {
			started <- struct{}{}
			<-release
			task.OutputRef = map[string]any{"artifact_type": "outline"}
			return task, nil
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

	resultCh := make(chan struct {
		result DispatchResult
		job    model.Job
		err    error
	}, 1)
	go func() {
		result, updatedJob, err := service.DispatchOnce(context.Background(), job.ID)
		resultCh <- struct {
			result DispatchResult
			job    model.Job
			err    error
		}{result: result, job: updatedJob, err: err}
	}()

	<-started

	persistedJob, err := store.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetJob() during execution error = %v", err)
	}
	if persistedJob.Status != model.JobStatusRunning {
		t.Fatalf("job status during execution = %q, want %q", persistedJob.Status, model.JobStatusRunning)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() during execution error = %v", err)
	}
	if len(persistedTasks) != 1 {
		t.Fatalf("task len during execution = %d, want 1", len(persistedTasks))
	}
	if persistedTasks[0].Status != model.TaskStatusRunning {
		t.Fatalf("task status during execution = %q, want %q", persistedTasks[0].Status, model.TaskStatusRunning)
	}
	if persistedTasks[0].Attempt != 1 {
		t.Fatalf("task attempt during execution = %d, want 1", persistedTasks[0].Attempt)
	}

	close(release)
	outcome := <-resultCh
	if outcome.err != nil {
		t.Fatalf("DispatchOnce() error = %v", outcome.err)
	}
	if !outcome.result.Dispatched {
		t.Fatal("DispatchOnce() dispatched = false, want true")
	}
	if outcome.job.Status != model.JobStatusCompleted {
		t.Fatalf("final job status = %q, want %q", outcome.job.Status, model.JobStatusCompleted)
	}
}

func TestDispatchOncePersistsReportedTaskProgressWhileRunning(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 11, 16, 0, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_task_progress",
		Token:     "job_token_scheduler_task_progress",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
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
			Attempt:     0,
			MaxAttempts: 1,
			Payload:     map[string]any{"article": "story"},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
	if err != nil {
		t.Fatalf("InitializeJob() error = %v", err)
	}

	progressReported := make(chan struct{}, 1)
	release := make(chan struct{})
	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeOutline: executorFunc(func(
			ctx context.Context,
			_ model.Job,
			task model.Task,
			_ map[string]model.Task,
		) (model.Task, error) {
			if err := model.ReportTaskProgress(ctx, model.TaskProgress{
				Phase:   "requesting_text",
				Message: "正在请求大纲生成",
				Current: 1,
				Total:   1,
				Unit:    "step",
			}); err != nil {
				t.Fatalf("ReportTaskProgress() error = %v", err)
			}
			progressReported <- struct{}{}
			<-release
			task.OutputRef = map[string]any{"artifact_type": "outline"}
			return task, nil
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

	errCh := make(chan error, 1)
	go func() {
		_, _, err := service.DispatchOnce(context.Background(), job.ID)
		errCh <- err
	}()

	<-progressReported

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if len(persistedTasks) != 1 {
		t.Fatalf("task len = %d, want 1", len(persistedTasks))
	}
	persistedTask := persistedTasks[0]
	progress, ok := persistedTask.OutputRef["progress"].(map[string]any)
	if !ok {
		t.Fatalf("progress = %#v, want map", persistedTask.OutputRef["progress"])
	}
	if progress["phase"] != "requesting_text" {
		t.Fatalf("progress phase = %#v, want requesting_text", progress["phase"])
	}
	if progress["message"] != "正在请求大纲生成" {
		t.Fatalf("progress message = %#v", progress["message"])
	}

	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}

	finalTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() after completion error = %v", err)
	}
	if _, ok := finalTasks[0].OutputRef["progress"]; ok {
		t.Fatalf("completed task progress = %#v, want cleared", finalTasks[0].OutputRef["progress"])
	}
}

func TestDispatchOncePersistsFastTaskCompletionWhileSlowTaskStillRunning(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	now := time.Date(2026, 4, 11, 15, 30, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_incremental_completion",
		Token:     "job_token_scheduler_incremental_completion",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "segmentation",
			Type:        model.TaskTypeSegmentation,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLocalCPU,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload:     map[string]any{"article": "story"},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			Key:         "outline",
			Type:        model.TaskTypeOutline,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     0,
			MaxAttempts: 1,
			Payload:     map[string]any{"article": "story"},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
	if err != nil {
		t.Fatalf("InitializeJob() error = %v", err)
	}

	segmentationDone := make(chan struct{}, 1)
	releaseOutline := make(chan struct{})
	registry := NewExecutorRegistry(map[model.TaskType]Executor{
		model.TaskTypeSegmentation: executorFunc(func(
			_ context.Context,
			_ model.Job,
			task model.Task,
			_ map[string]model.Task,
		) (model.Task, error) {
			task.OutputRef = map[string]any{"artifact_type": "segmentation"}
			segmentationDone <- struct{}{}
			return task, nil
		}),
		model.TaskTypeOutline: executorFunc(func(
			_ context.Context,
			_ model.Job,
			task model.Task,
			_ map[string]model.Task,
		) (model.Task, error) {
			<-releaseOutline
			task.OutputRef = map[string]any{"artifact_type": "outline"}
			return task, nil
		}),
	})

	service := NewService(
		store,
		store,
		registry,
		NewMemoryResourceManager(map[model.ResourceKey]int{
			model.ResourceLocalCPU: 1,
			model.ResourceLLMText:  1,
		}),
	)
	service.clock = fixedSchedulerClock{now: now.Add(time.Minute)}

	resultCh := make(chan error, 1)
	go func() {
		_, _, err := service.DispatchOnce(context.Background(), job.ID)
		resultCh <- err
	}()

	<-segmentationDone

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if persistedTasks[0].Status != model.TaskStatusSucceeded {
		t.Fatalf("segmentation status during outline run = %q, want %q", persistedTasks[0].Status, model.TaskStatusSucceeded)
	}
	if persistedTasks[1].Status != model.TaskStatusRunning {
		t.Fatalf("outline status during outline run = %q, want %q", persistedTasks[1].Status, model.TaskStatusRunning)
	}

	close(releaseOutline)
	if err := <-resultCh; err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
}

func TestDispatchOnceExecutesImageAfterScript(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	workspaceDir := t.TempDir()
	if err := writeSchedulerJSONArtifact(
		workspaceDir,
		"jobs/job_public_scheduler_image/script.json",
		map[string]any{
			"segments": []map[string]any{
				{
					"index": 0,
					"shots": []map[string]any{
						{
							"index":                0,
							"text_to_image_prompt": "night rain on the bridge",
						},
					},
				},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writeSchedulerJSONArtifact(
		workspaceDir,
		"jobs/job_public_scheduler_image/character_images/manifest.json",
		map[string]any{
			"images": []map[string]any{
				{
					"character_index": 0,
					"character_name":  "Lin Qing",
					"file_path":       "jobs/job_public_scheduler_image/character_images/character_000.jpg",
					"is_fallback":     true,
				},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}
	now := time.Date(2026, 4, 6, 10, 30, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_image",
		Token:     "job_token_scheduler_image",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
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
			Key:         "character_image",
			Type:        model.TaskTypeCharacterImage,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceImageGen,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef: map[string]any{
				"artifact_type": "character_image",
				"artifact_path": "jobs/job_public_scheduler_image/character_images/manifest.json",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "image",
			Type:        model.TaskTypeImage,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceImageGen,
			DependsOn:   []string{"script", "character_image"},
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
		model.TaskTypeImage: imagepipeline.NewExecutor(workspaceDir),
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
	if len(persistedTasks) != 3 {
		t.Fatalf("task len = %d, want 3", len(persistedTasks))
	}
	if persistedTasks[2].Status != model.TaskStatusSucceeded {
		t.Fatalf("image status = %q, want %q", persistedTasks[2].Status, model.TaskStatusSucceeded)
	}
	if persistedTasks[2].OutputRef["artifact_type"] != "image" {
		t.Fatalf("image output_ref = %#v", persistedTasks[2].OutputRef)
	}
	if persistedTasks[2].OutputRef["script_artifact_ref"] != "jobs/job_public_scheduler_image/script.json" {
		t.Fatalf("image script ref = %#v", persistedTasks[2].OutputRef["script_artifact_ref"])
	}
	if persistedTasks[2].OutputRef["character_image_artifact_ref"] != "jobs/job_public_scheduler_image/character_images/manifest.json" {
		t.Fatalf("image character image ref = %#v", persistedTasks[2].OutputRef["character_image_artifact_ref"])
	}
	if got := outputInt(persistedTasks[2].OutputRef, "image_count"); got != 1 {
		t.Fatalf("image_count = %d, want 1", got)
	}
}

func TestDispatchOnceExecutesCharacterImageAfterCharacterSheet(t *testing.T) {
	t.Parallel()

	store := newSchedulerTestStore(t)
	workspaceDir := t.TempDir()
	writer := imagepipeline.NewCharacterImageExecutor(workspaceDir)
	if err := writeSchedulerJSONArtifact(
		workspaceDir,
		"jobs/job_public_scheduler_character_image/character_sheet.json",
		map[string]any{
			"characters": []map[string]any{
				{
					"name":                   "Lin Qing",
					"appearance":             "white robe",
					"visual_signature":       "jade pendant",
					"reference_subject_type": "person",
					"image_prompt_focus":     "front view",
				},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	now := time.Date(2026, 4, 6, 10, 20, 0, 0, time.UTC)
	job := model.Job{
		PublicID:  "job_public_scheduler_character_image",
		Token:     "job_token_scheduler_character_image",
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      model.JobSpec{Article: "story"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := store.InitializeJob(context.Background(), &job, []model.Task{
		{
			Key:         "character_sheet",
			Type:        model.TaskTypeCharacterSheet,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceLLMText,
			DependsOn:   []string{},
			Attempt:     1,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article": "story",
			},
			OutputRef: map[string]any{
				"artifact_type": "character_sheet",
				"artifact_path": "jobs/job_public_scheduler_character_image/character_sheet.json",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "character_image",
			Type:        model.TaskTypeCharacterImage,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceImageGen,
			DependsOn:   []string{"character_sheet"},
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
		model.TaskTypeCharacterImage: writer,
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
	if result.ExecutedTaskKey != "character_image" {
		t.Fatalf("DispatchOnce() executed = %q, want %q", result.ExecutedTaskKey, "character_image")
	}
	if updatedJob.Status != model.JobStatusCompleted {
		t.Fatalf("job status = %q, want %q", updatedJob.Status, model.JobStatusCompleted)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if persistedTasks[1].Status != model.TaskStatusSucceeded {
		t.Fatalf("character_image status = %q, want %q", persistedTasks[1].Status, model.TaskStatusSucceeded)
	}
	if persistedTasks[1].OutputRef["artifact_type"] != "character_image" {
		t.Fatalf("character_image output_ref = %#v", persistedTasks[1].OutputRef)
	}
	if persistedTasks[1].OutputRef["character_sheet_ref"] != "jobs/job_public_scheduler_character_image/character_sheet.json" {
		t.Fatalf("character_image sheet ref = %#v", persistedTasks[1].OutputRef["character_sheet_ref"])
	}
}

func writeSchedulerJSONArtifact(
	workspaceDir string,
	relativePath string,
	value any,
) error {
	fullPath := filepath.Join(workspaceDir, filepath.Clean(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(fullPath, data, 0o644)
}

func outputInt(values map[string]any, key string) int {
	value, ok := values[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
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
		Spec:      model.JobSpec{Article: "story"},
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
				"segment_count":          1,
				"audio_segment_paths":    []string{"jobs/job_public_scheduler_video/audio/segment_000.wav"},
				"total_duration_seconds": 9.5,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "shot_video",
			Type:        model.TaskTypeShotVideo,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceVideoGen,
			DependsOn:   []string{"image"},
			Attempt:     1,
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef: map[string]any{
				"artifact_type":     "shot_video",
				"artifact_path":     "jobs/job_public_scheduler_video/shot_videos/manifest.json",
				"image_source_type": "shot_images",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Key:         "video",
			Type:        model.TaskTypeVideo,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceVideoRender,
			DependsOn:   []string{"tts", "shot_video"},
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
		Spec:      model.JobSpec{Article: "story"},
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
				"article": "story",
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
			DependsOn:   []string{"outline"},
			Attempt:     0,
			MaxAttempts: 1,
			Payload: map[string]any{
				"article":  "story",
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
	if persistedTasks[1].Status != model.TaskStatusSkipped {
		t.Fatalf("downstream task status = %q, want %q", persistedTasks[1].Status, model.TaskStatusSkipped)
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
	db.SetMaxOpenConns(1)
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
