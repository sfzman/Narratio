package jobs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sfzman/Narratio/backend/internal/model"
	sqlstore "github.com/sfzman/Narratio/backend/internal/store/sql"
)

type fakeJobRunner struct {
	mu     sync.Mutex
	jobIDs []int64
}

func (f *fakeJobRunner) Enqueue(jobID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.jobIDs = append(f.jobIDs, jobID)
}

type fakeJobRunController struct {
	fakeJobRunner
	active       map[int64]bool
	cancelledIDs []int64
}

func (f *fakeJobRunController) Cancel(jobID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.cancelledIDs = append(f.cancelledIDs, jobID)
	return f.active[jobID]
}

func (f *fakeJobRunController) IsActive(jobID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.active[jobID]
}

func (f *fakeJobRunController) IsRunning(jobID int64) bool {
	return f.IsActive(jobID)
}

func TestCreateJobBuildsAndPersistsDefaultWorkflow(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	service := NewService(store)
	service.clock = fixedClock{now: time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)}

	job, tasks, err := service.CreateJob(context.Background(), model.JobSpec{
		Article: "  hello world  ",
		Options: model.RenderOptions{
			VoiceID:    "",
			ImageStyle: "",
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID == 0 {
		t.Fatalf("CreateJob() job id = 0, want auto id")
	}
	if job.Status != model.JobStatusQueued {
		t.Fatalf("CreateJob() status = %q, want %q", job.Status, model.JobStatusQueued)
	}
	if job.Spec.Options.VoiceID != model.DefaultVoicePresetID {
		t.Fatalf("CreateJob() voice_id = %q, want %q", job.Spec.Options.VoiceID, model.DefaultVoicePresetID)
	}
	if job.Spec.Options.ImageStyle != "realistic" {
		t.Fatalf("CreateJob() image_style = %q, want %q", job.Spec.Options.ImageStyle, "realistic")
	}
	if job.Spec.Options.AspectRatio != model.AspectRatioPortrait9x16 {
		t.Fatalf("CreateJob() aspect_ratio = %q, want %q", job.Spec.Options.AspectRatio, model.AspectRatioPortrait9x16)
	}
	if job.Spec.Options.VideoCount == nil || *job.Spec.Options.VideoCount != defaultVideoCount {
		t.Fatalf("CreateJob() video_count = %#v, want %d", job.Spec.Options.VideoCount, defaultVideoCount)
	}

	if len(tasks) != 9 {
		t.Fatalf("CreateJob() tasks len = %d, want 9", len(tasks))
	}
	if tasks[0].Key != "segmentation" {
		t.Fatalf("CreateJob() task[0].Key = %q, want %q", tasks[0].Key, "segmentation")
	}
	if tasks[0].Payload["article"] != "hello world" {
		t.Fatalf("CreateJob() segmentation payload article = %#v, want %#v", tasks[0].Payload["article"], "hello world")
	}
	if tasks[4].Key != "character_image" {
		t.Fatalf("CreateJob() task[4].Key = %q, want %q", tasks[4].Key, "character_image")
	}
	if len(tasks[4].DependsOn) != 1 || tasks[4].DependsOn[0] != "character_sheet" {
		t.Fatalf("CreateJob() character_image depends_on = %#v, want [character_sheet]", tasks[4].DependsOn)
	}
	if tasks[4].Payload["image_style"] != "realistic" {
		t.Fatalf("CreateJob() character_image payload style = %#v, want %#v", tasks[4].Payload["image_style"], "realistic")
	}
	if tasks[6].Payload["image_style"] != "realistic" {
		t.Fatalf("CreateJob() image payload style = %#v, want %#v", tasks[6].Payload["image_style"], "realistic")
	}
	if tasks[6].Payload["aspect_ratio"] != string(model.AspectRatioPortrait9x16) {
		t.Fatalf("CreateJob() image payload aspect_ratio = %#v, want %q", tasks[6].Payload["aspect_ratio"], model.AspectRatioPortrait9x16)
	}

	if tasks[3].Key != "script" {
		t.Fatalf("CreateJob() task[3].Key = %q, want %q", tasks[3].Key, "script")
	}
	if len(tasks[3].DependsOn) != 3 {
		t.Fatalf("CreateJob() script depends_on = %#v, want 3 deps", tasks[3].DependsOn)
	}
	if tasks[6].Key != "image" {
		t.Fatalf("CreateJob() task[6].Key = %q, want %q", tasks[6].Key, "image")
	}
	if len(tasks[6].DependsOn) != 2 {
		t.Fatalf("CreateJob() image depends_on = %#v, want 2 deps", tasks[6].DependsOn)
	}
	if tasks[5].Key != "tts" {
		t.Fatalf("CreateJob() task[5].Key = %q, want %q", tasks[5].Key, "tts")
	}
	if len(tasks[5].DependsOn) != 1 || tasks[5].DependsOn[0] != "segmentation" {
		t.Fatalf("CreateJob() tts depends_on = %#v, want [segmentation]", tasks[5].DependsOn)
	}
	if tasks[7].Key != "shot_video" {
		t.Fatalf("CreateJob() task[7].Key = %q, want %q", tasks[7].Key, "shot_video")
	}
	if len(tasks[7].DependsOn) != 1 {
		t.Fatalf("CreateJob() shot_video depends_on = %#v, want 1 dep", tasks[7].DependsOn)
	}
	if tasks[7].DependsOn[0] != "image" {
		t.Fatalf("CreateJob() shot_video depends_on[0] = %#v, want %q", tasks[7].DependsOn[0], "image")
	}
	if tasks[7].Payload["video_count"] != defaultVideoCount {
		t.Fatalf("CreateJob() shot_video payload video_count = %#v, want %d", tasks[7].Payload["video_count"], defaultVideoCount)
	}
	if tasks[7].Payload["aspect_ratio"] != string(model.AspectRatioPortrait9x16) {
		t.Fatalf("CreateJob() shot_video payload aspect_ratio = %#v, want %q", tasks[7].Payload["aspect_ratio"], model.AspectRatioPortrait9x16)
	}
	if tasks[8].Key != "video" {
		t.Fatalf("CreateJob() task[8].Key = %q, want %q", tasks[8].Key, "video")
	}
	if len(tasks[8].DependsOn) != 2 {
		t.Fatalf("CreateJob() video depends_on = %#v, want 2 deps", tasks[8].DependsOn)
	}
	if tasks[8].DependsOn[0] != "tts" || tasks[8].DependsOn[1] != "shot_video" {
		t.Fatalf("CreateJob() video depends_on = %#v, want [tts shot_video]", tasks[8].DependsOn)
	}
	if tasks[8].Payload["aspect_ratio"] != string(model.AspectRatioPortrait9x16) {
		t.Fatalf("CreateJob() video payload aspect_ratio = %#v, want %q", tasks[8].Payload["aspect_ratio"], model.AspectRatioPortrait9x16)
	}

	persistedJob, err := store.GetJobByPublicID(context.Background(), job.PublicID)
	if err != nil {
		t.Fatalf("GetJobByPublicID() error = %v", err)
	}
	if persistedJob.ID != job.ID {
		t.Fatalf("persisted job id = %d, want %d", persistedJob.ID, job.ID)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if len(persistedTasks) != 9 {
		t.Fatalf("persisted tasks len = %d, want 9", len(persistedTasks))
	}
}

func TestCreateJobEnqueuesBackgroundDispatch(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	runner := &fakeJobRunner{}
	service := NewService(store, runner)
	service.clock = fixedClock{now: time.Date(2026, 4, 6, 20, 0, 0, 0, time.UTC)}

	job, _, err := service.CreateJob(context.Background(), model.JobSpec{
		Article: "hello world",
		Options: model.RenderOptions{
			VoiceID:    "default",
			ImageStyle: "realistic",
			VideoCount: intPtr(5),
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if len(runner.jobIDs) != 1 {
		t.Fatalf("runner enqueued len = %d, want 1", len(runner.jobIDs))
	}
	if runner.jobIDs[0] != job.ID {
		t.Fatalf("runner enqueued job id = %d, want %d", runner.jobIDs[0], job.ID)
	}
}

func TestCreateJobPreservesExplicitVideoCount(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	service := NewService(store)

	job, tasks, err := service.CreateJob(context.Background(), model.JobSpec{
		Article: "hello world",
		Options: model.RenderOptions{
			AspectRatio: model.AspectRatioLandscape16x9,
			VideoCount:  intPtr(5),
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.Spec.Options.VideoCount == nil || *job.Spec.Options.VideoCount != 5 {
		t.Fatalf("job spec video_count = %#v, want 5", job.Spec.Options.VideoCount)
	}
	if job.Spec.Options.AspectRatio != model.AspectRatioLandscape16x9 {
		t.Fatalf("job spec aspect_ratio = %q, want %q", job.Spec.Options.AspectRatio, model.AspectRatioLandscape16x9)
	}
	if tasks[7].Payload["video_count"] != 5 {
		t.Fatalf("shot_video payload video_count = %#v, want 5", tasks[7].Payload["video_count"])
	}
	if tasks[6].Payload["aspect_ratio"] != string(model.AspectRatioLandscape16x9) {
		t.Fatalf("image payload aspect_ratio = %#v, want %q", tasks[6].Payload["aspect_ratio"], model.AspectRatioLandscape16x9)
	}
	if tasks[8].Payload["aspect_ratio"] != string(model.AspectRatioLandscape16x9) {
		t.Fatalf("video payload aspect_ratio = %#v, want %q", tasks[8].Payload["aspect_ratio"], model.AspectRatioLandscape16x9)
	}
}

func TestCreateJobPropagatesExplicitImageStyleToCharacterImage(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	service := NewService(store)

	_, tasks, err := service.CreateJob(context.Background(), model.JobSpec{
		Article: "hello world",
		Options: model.RenderOptions{
			ImageStyle: "现代工笔人物画风",
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if tasks[4].Payload["image_style"] != "现代工笔人物画风" {
		t.Fatalf("character_image payload image_style = %#v, want %q", tasks[4].Payload["image_style"], "现代工笔人物画风")
	}
	if tasks[6].Payload["image_style"] != "现代工笔人物画风" {
		t.Fatalf("image payload image_style = %#v, want %q", tasks[6].Payload["image_style"], "现代工笔人物画风")
	}
}

func TestCancelJobCancelsQueuedJobImmediately(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	service := NewService(store)
	service.clock = fixedClock{now: time.Date(2026, 4, 11, 6, 0, 0, 0, time.UTC)}

	job, tasks, err := service.CreateJob(context.Background(), model.JobSpec{
		Article: "hello world",
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("tasks len = 0")
	}

	outcome, err := service.CancelJob(context.Background(), job.PublicID)
	if err != nil {
		t.Fatalf("CancelJob() error = %v", err)
	}
	if !outcome.Cancelled {
		t.Fatal("Cancelled = false, want true")
	}
	if outcome.Job.Status != model.JobStatusCancelled {
		t.Fatalf("status = %q, want %q", outcome.Job.Status, model.JobStatusCancelled)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	for _, task := range persistedTasks {
		if task.Status != model.TaskStatusCancelled {
			t.Fatalf("task %q status = %q, want cancelled", task.Key, task.Status)
		}
	}
}

func TestCancelJobMarksRunningJobAsCancellingAndCancelsRunner(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	runner := &fakeJobRunController{
		active: make(map[int64]bool),
	}
	service := NewService(store, runner)
	now := time.Date(2026, 4, 11, 7, 0, 0, 0, time.UTC)
	service.clock = fixedClock{now: now}

	job := model.Job{
		PublicID:  "job_running_cancel_123",
		Token:     "job_token_running_cancel_123",
		Status:    model.JobStatusQueued,
		Progress:  22,
		Spec:      model.JobSpec{Article: "story"},
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateJob(context.Background(), &job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	_, err := store.CreateTasks(context.Background(), []model.Task{
		{
			JobID:       job.ID,
			Key:         "segmentation",
			Type:        model.TaskTypeSegmentation,
			Status:      model.TaskStatusSucceeded,
			ResourceKey: model.ResourceLocalCPU,
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			JobID:       job.ID,
			Key:         "tts",
			Type:        model.TaskTypeTTS,
			Status:      model.TaskStatusPending,
			ResourceKey: model.ResourceTTS,
			DependsOn:   []string{"segmentation"},
			MaxAttempts: 1,
			Payload:     map[string]any{},
			OutputRef:   map[string]any{},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error = %v", err)
	}
	runner.active[job.ID] = true

	outcome, err := service.CancelJob(context.Background(), job.PublicID)
	if err != nil {
		t.Fatalf("CancelJob() error = %v", err)
	}
	if outcome.Job.Status != model.JobStatusCancelling {
		t.Fatalf("status = %q, want %q", outcome.Job.Status, model.JobStatusCancelling)
	}
	if len(runner.cancelledIDs) != 1 || runner.cancelledIDs[0] != job.ID {
		t.Fatalf("cancelledIDs = %#v, want [%d]", runner.cancelledIDs, job.ID)
	}

	persistedTasks, err := store.ListTasksByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ListTasksByJob() error = %v", err)
	}
	if persistedTasks[0].Status != model.TaskStatusSucceeded {
		t.Fatalf("task[0] status = %q, want succeeded", persistedTasks[0].Status)
	}
	if persistedTasks[1].Status != model.TaskStatusCancelled {
		t.Fatalf("task[1] status = %q, want cancelled", persistedTasks[1].Status)
	}
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

func newWorkflowTestStore(t *testing.T) *sqlstore.Store {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := applyWorkflowTestMigration(db); err != nil {
		t.Fatalf("applyWorkflowTestMigration() error = %v", err)
	}

	return sqlstore.New(db)
}

func applyWorkflowTestMigration(db *sql.DB) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return os.ErrNotExist
	}

	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "store", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(string(sqlBytes))
	return err
}
