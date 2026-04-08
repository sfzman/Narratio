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
	if job.Spec.Options.VoiceID != "default" {
		t.Fatalf("CreateJob() voice_id = %q, want %q", job.Spec.Options.VoiceID, "default")
	}
	if job.Spec.Options.ImageStyle != "realistic" {
		t.Fatalf("CreateJob() image_style = %q, want %q", job.Spec.Options.ImageStyle, "realistic")
	}

	if len(tasks) != 8 {
		t.Fatalf("CreateJob() tasks len = %d, want 8", len(tasks))
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
	if tasks[6].Payload["image_style"] != "realistic" {
		t.Fatalf("CreateJob() image payload style = %#v, want %#v", tasks[6].Payload["image_style"], "realistic")
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
	if tasks[7].Key != "video" {
		t.Fatalf("CreateJob() task[7].Key = %q, want %q", tasks[7].Key, "video")
	}
	if len(tasks[7].DependsOn) != 2 {
		t.Fatalf("CreateJob() video depends_on = %#v, want 2 deps", tasks[7].DependsOn)
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
	if len(persistedTasks) != 8 {
		t.Fatalf("persisted tasks len = %d, want 8", len(persistedTasks))
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
