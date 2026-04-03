package store

import (
	"context"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type TaskStore interface {
	CreateTasks(ctx context.Context, tasks []model.Task) ([]model.Task, error)
	GetTask(ctx context.Context, id int64) (model.Task, error)
	ListTasksByJob(ctx context.Context, jobID int64) ([]model.Task, error)
	ListTasksByJobPublicID(ctx context.Context, publicID string) ([]model.Task, error)
	UpdateTask(ctx context.Context, task model.Task) error
}
