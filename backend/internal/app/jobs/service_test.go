package jobs

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

func TestCreateJobBuildsAndPersistsDefaultWorkflow(t *testing.T) {
	t.Parallel()

	store := newWorkflowTestStore(t)
	service := NewService(store)
	service.clock = fixedClock{now: time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)}

	job, tasks, err := service.CreateJob(context.Background(), model.JobSpec{
		Article:  "  hello world  ",
		Language: "",
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
	if job.Spec.Language != "zh" {
		t.Fatalf("CreateJob() language = %q, want %q", job.Spec.Language, "zh")
	}
	if job.Spec.Options.VoiceID != "default" {
		t.Fatalf("CreateJob() voice_id = %q, want %q", job.Spec.Options.VoiceID, "default")
	}
	if job.Spec.Options.ImageStyle != "realistic" {
		t.Fatalf("CreateJob() image_style = %q, want %q", job.Spec.Options.ImageStyle, "realistic")
	}

	if len(tasks) != 6 {
		t.Fatalf("CreateJob() tasks len = %d, want 6", len(tasks))
	}
	if tasks[0].Payload["article"] != "hello world" {
		t.Fatalf("CreateJob() outline payload article = %#v, want %#v", tasks[0].Payload["article"], "hello world")
	}
	if tasks[0].Payload["language"] != "zh" {
		t.Fatalf("CreateJob() outline payload language = %#v, want %#v", tasks[0].Payload["language"], "zh")
	}
	if tasks[4].Payload["image_style"] != "realistic" {
		t.Fatalf("CreateJob() image payload style = %#v, want %#v", tasks[4].Payload["image_style"], "realistic")
	}

	if tasks[2].Key != "script" {
		t.Fatalf("CreateJob() task[2].Key = %q, want %q", tasks[2].Key, "script")
	}
	if len(tasks[2].DependsOn) != 2 {
		t.Fatalf("CreateJob() script depends_on = %#v, want 2 deps", tasks[2].DependsOn)
	}
	if tasks[5].Key != "video" {
		t.Fatalf("CreateJob() task[5].Key = %q, want %q", tasks[5].Key, "video")
	}
	if len(tasks[5].DependsOn) != 2 {
		t.Fatalf("CreateJob() video depends_on = %#v, want 2 deps", tasks[5].DependsOn)
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
	if len(persistedTasks) != 6 {
		t.Fatalf("persisted tasks len = %d, want 6", len(persistedTasks))
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
