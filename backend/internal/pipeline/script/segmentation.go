package script

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type SegmentationExecutor struct {
	log       *slog.Logger
	artifacts artifactWriter
}

func NewSegmentationExecutor(workspaceDir string) *SegmentationExecutor {
	return &SegmentationExecutor{
		log:       slog.Default().With("executor", "segmentation"),
		artifacts: newArtifactWriter(workspaceDir),
	}
}

func (e *SegmentationExecutor) Type() model.TaskType {
	return model.TaskTypeSegmentation
}

func (e *SegmentationExecutor) Execute(
	_ context.Context,
	job model.Job,
	task model.Task,
	_ map[string]model.Task,
) (model.Task, error) {
	article, language, err := outlinePayload(task)
	if err != nil {
		e.log.Error("segmentation payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	artifactPath := fmt.Sprintf("jobs/%s/segments.json", job.PublicID)
	output := buildSegmentationOutput(article)
	normalizeSegmentationOutput(&output)
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write segmentation artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":  "segmentation",
		"artifact_path":  artifactPath,
		"language":       language,
		"article_length": len([]rune(article)),
		"segment_count":  len(output.Segments),
	}

	e.log.Info("segmentation execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
		"segment_count", len(output.Segments),
	)

	return task, nil
}
