package tts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultSegmentCount   = 1
	defaultSubtitleFormat = "srt"
)

type Executor struct {
	log       *slog.Logger
	artifacts artifactWriter
}

func NewExecutor(workspaceDir ...string) *Executor {
	resolvedWorkspaceDir := ""
	if len(workspaceDir) > 0 {
		resolvedWorkspaceDir = strings.TrimSpace(workspaceDir[0])
	}

	return &Executor{
		log:       slog.Default().With("executor", "tts"),
		artifacts: newArtifactWriter(resolvedWorkspaceDir),
	}
}

func (e *Executor) Execute(
	_ context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	voiceID, err := payloadString(task.Payload, "voice_id")
	if err != nil {
		e.log.Error("tts payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	segmentationTask, ok := dependencies["segmentation"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "segmentation")
	}
	segmentation, err := loadArtifactJSON[segmentationArtifact](
		e.artifacts.workspaceDir,
		segmentationTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return task, fmt.Errorf("load segmentation artifact: %w", err)
	}
	output := buildTTSOutput(job.PublicID, segmentation)
	segmentCount := len(output.AudioSegments)
	if segmentCount == 0 {
		segmentCount = defaultSegmentCount
	}
	subtitleArtifactPath := fmt.Sprintf("jobs/%s/audio/subtitles.srt", job.PublicID)

	e.log.Debug("tts execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	artifactPath := fmt.Sprintf("jobs/%s/audio/tts_manifest.json", job.PublicID)
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write tts artifact: %w", err)
	}
	if err := e.artifacts.WriteBytes(subtitleArtifactPath, []byte(buildSRT(output.SubtitleItems))); err != nil {
		return task, fmt.Errorf("write subtitle artifact: %w", err)
	}
	if err := writePlaceholderAudioSegments(e.artifacts, output.AudioSegments); err != nil {
		return task, fmt.Errorf("write placeholder audio segments: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":             "tts",
		"artifact_path":             artifactPath,
		"segmentation_artifact_ref": segmentationTask.OutputRef["artifact_path"],
		"voice_id":                  voiceID,
		"segment_count":             segmentCount,
		"subtitle_format":           defaultSubtitleFormat,
		"subtitle_artifact_ref":     subtitleArtifactPath,
		"audio_segment_paths":       collectAudioSegmentPaths(output.AudioSegments),
		"total_duration_seconds":    output.TotalDuration,
	}

	e.log.Info("tts execution completed",
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
