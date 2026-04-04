package jobs

import (
	"context"
	"fmt"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
	"github.com/sfzman/Narratio/backend/internal/store"
)

type SchedulerDispatcher interface {
	DispatchOnce(ctx context.Context, jobID int64) (scheduler.DispatchResult, model.Job, error)
}

type DispatchOutcome struct {
	Job             model.Job
	Dispatched      bool
	ExecutedTaskID  int64
	ExecutedTaskKey string
}

type DispatchService struct {
	jobStore  store.JobStore
	scheduler SchedulerDispatcher
}

func NewDispatchService(
	jobStore store.JobStore,
	schedulerDispatcher SchedulerDispatcher,
) *DispatchService {
	return &DispatchService{
		jobStore:  jobStore,
		scheduler: schedulerDispatcher,
	}
}

func (s *DispatchService) DispatchOnce(
	ctx context.Context,
	publicID string,
) (DispatchOutcome, error) {
	job, err := s.jobStore.GetJobByPublicID(ctx, publicID)
	if err != nil {
		return DispatchOutcome{}, fmt.Errorf("get job by public id: %w", err)
	}

	result, updatedJob, err := s.scheduler.DispatchOnce(ctx, job.ID)
	if err != nil {
		return DispatchOutcome{}, fmt.Errorf("dispatch job: %w", err)
	}

	return DispatchOutcome{
		Job:             updatedJob,
		Dispatched:      result.Dispatched,
		ExecutedTaskID:  result.ExecutedTaskID,
		ExecutedTaskKey: result.ExecutedTaskKey,
	}, nil
}
