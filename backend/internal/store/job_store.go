package store

import (
	"context"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type JobStore interface {
	CreateJob(ctx context.Context, job *model.Job) error
	ListJobs(ctx context.Context) ([]model.Job, error)
	GetJob(ctx context.Context, id int64) (model.Job, error)
	GetJobByPublicID(ctx context.Context, publicID string) (model.Job, error)
	UpdateJob(ctx context.Context, job model.Job) error
	DeleteJob(ctx context.Context, id int64) error
}
