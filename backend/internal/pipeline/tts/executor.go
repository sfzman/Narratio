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
	defaultTotalDuration  = 6.5
	defaultSubtitleFormat = "srt"
)

type Executor struct {
	log *slog.Logger
}

func NewExecutor() *Executor {
	return &Executor{
		log: slog.Default().With("executor", "tts"),
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

	scriptTask, ok := dependencies["script"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "script")
	}

	e.log.Debug("tts execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	task.OutputRef = map[string]any{
		"artifact_type":         "tts",
		"artifact_path":         fmt.Sprintf("jobs/%s/audio/tts_manifest.json", job.PublicID),
		"script_artifact_ref":   scriptTask.OutputRef["artifact_path"],
		"voice_id":              voiceID,
		"segment_count":         defaultSegmentCount,
		"subtitle_format":       defaultSubtitleFormat,
		"subtitle_artifact_ref": fmt.Sprintf("jobs/%s/audio/subtitles.srt", job.PublicID),
		"audio_segment_paths": []string{
			fmt.Sprintf("jobs/%s/audio/segment_000.wav", job.PublicID),
		},
		"total_duration_seconds": defaultTotalDuration,
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
