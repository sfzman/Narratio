package store

import (
	"context"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type WorkflowStore interface {
	InitializeJob(ctx context.Context, job *model.Job, tasks []model.Task) ([]model.Task, error)
}
