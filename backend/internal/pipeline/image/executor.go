package image

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultImageWidth  = 1280
	defaultImageHeight = 720
)

type Executor struct {
	log *slog.Logger
}

func NewExecutor() *Executor {
	return &Executor{
		log: slog.Default().With("executor", "image"),
	}
}

func (e *Executor) Execute(
	_ context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	imageStyle, err := payloadString(task.Payload, "image_style")
	if err != nil {
		e.log.Error("image payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	scriptTask, ok := dependencies["script"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "script")
	}

	e.log.Debug("image execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	task.OutputRef = map[string]any{
		"artifact_type":       "image",
		"artifact_path":       fmt.Sprintf("jobs/%s/images/image_manifest.json", job.PublicID),
		"script_artifact_ref": scriptTask.OutputRef["artifact_path"],
		"image_style":         imageStyle,
		"image_count":         1,
		"images": []map[string]any{
			{
				"segment_index": 0,
				"file_path":     fmt.Sprintf("jobs/%s/images/segment_000.jpg", job.PublicID),
				"width":         defaultImageWidth,
				"height":        defaultImageHeight,
				"is_fallback":   true,
			},
		},
	}

	e.log.Info("image execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", task.OutputRef["artifact_path"],
	)

	return task, nil
}

func payloadString(payload map[string]any, key string) (string, error) {
	value, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("missing payload field %q", key)
	}

	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("payload field %q is not a string", key)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("payload field %q is empty", key)
	}

	return s, nil
}
