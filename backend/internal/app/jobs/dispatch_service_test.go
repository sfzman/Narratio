package jobs

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
)

type fakeDispatchJobStore struct {
	job model.Job
	err error
}

func (f *fakeDispatchJobStore) CreateJob(context.Context, *model.Job) error { return nil }
func (f *fakeDispatchJobStore) GetJob(context.Context, int64) (model.Job, error) {
	return model.Job{}, nil
}
func (f *fakeDispatchJobStore) GetJobByPublicID(_ context.Context, _ string) (model.Job, error) {
	return f.job, f.err
}
func (f *fakeDispatchJobStore) UpdateJob(context.Context, model.Job) error { return nil }
func (f *fakeDispatchJobStore) DeleteJob(context.Context, int64) error     { return nil }

type fakeSchedulerDispatcher struct {
	jobID int64
	job   model.Job
}

func (f *fakeSchedulerDispatcher) DispatchOnce(
	_ context.Context,
	jobID int64,
) (scheduler.DispatchResult, model.Job, error) {
	f.jobID = jobID
	return scheduler.DispatchResult{
		Dispatched:      true,
		ExecutedTaskID:  11,
		ExecutedTaskKey: "outline",
	}, f.job, nil
}

func TestDispatchServiceDispatchOnce(t *testing.T) {
	service := NewDispatchService(
		&fakeDispatchJobStore{
			job: model.Job{ID: 7, PublicID: "job_abc123"},
		},
		&fakeSchedulerDispatcher{
			job: model.Job{PublicID: "job_abc123", Status: model.JobStatusQueued, Progress: 33},
		},
	)

	outcome, err := service.DispatchOnce(context.Background(), "job_abc123")
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}

	if outcome.Job.PublicID != "job_abc123" {
		t.Fatalf("Job.PublicID = %q", outcome.Job.PublicID)
	}
	if !outcome.Dispatched {
		t.Fatal("Dispatched = false")
	}
	if outcome.ExecutedTaskKey != "outline" {
		t.Fatalf("ExecutedTaskKey = %q", outcome.ExecutedTaskKey)
	}
}

func TestDispatchServiceReturnsNoopWhenJobAlreadyRunning(t *testing.T) {
	coord := NewRunCoordinator()
	if !coord.TryAcquire(7) {
		t.Fatal("TryAcquire(7) = false, want true")
	}

	service := NewDispatchService(
		&fakeDispatchJobStore{
			job: model.Job{ID: 7, PublicID: "job_abc123", Status: model.JobStatusRunning},
		},
		&fakeSchedulerDispatcher{},
		coord,
	)

	outcome, err := service.DispatchOnce(context.Background(), "job_abc123")
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if outcome.Dispatched {
		t.Fatal("Dispatched = true, want false")
	}
	if outcome.Job.PublicID != "job_abc123" {
		t.Fatalf("Job.PublicID = %q", outcome.Job.PublicID)
	}
}
