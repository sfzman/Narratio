package video

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const defaultVideoFileSize = 2 * 1024 * 1024

type Executor struct {
	log *slog.Logger
}

func NewExecutor() *Executor {
	return &Executor{
		log: slog.Default().With("executor", "video"),
	}
}

func (e *Executor) Execute(
	_ context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	ttsTask, ok := dependencies["tts"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "tts")
	}

	imageTask, ok := dependencies["image"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "image")
	}

	e.log.Debug("video execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	duration := outputFloat(ttsTask.OutputRef, "total_duration_seconds", 6.5)
	task.OutputRef = map[string]any{
		"artifact_type":      "video",
		"artifact_path":      fmt.Sprintf("jobs/%s/output/final.mp4", job.PublicID),
		"tts_artifact_ref":   ttsTask.OutputRef["artifact_path"],
		"image_artifact_ref": imageTask.OutputRef["artifact_path"],
		"duration_seconds":   duration,
		"file_size_bytes":    int64(defaultVideoFileSize),
	}

	e.log.Info("video execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", task.OutputRef["artifact_path"],
	)

	return task, nil
}

func outputFloat(values map[string]any, key string, fallback float64) float64 {
	value, ok := values[key]
	if !ok {
		return fallback
	}

	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return fallback
	}
}
