package script

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type ScriptExecutor struct {
	log              *slog.Logger
	textClient       TextClient
	generationConfig TextGenerationConfig
}

func NewScriptExecutor() *ScriptExecutor {
	return NewScriptExecutorWithClient(nil, TextGenerationConfig{})
}

func NewScriptExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
) *ScriptExecutor {
	return &ScriptExecutor{
		log:              slog.Default().With("executor", "script"),
		textClient:       textClient,
		generationConfig: normalizeTextGenerationConfig(generationConfig),
	}
}

func (e *ScriptExecutor) Type() model.TaskType {
	return model.TaskTypeScript
}

func (e *ScriptExecutor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		e.log.Error("script payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	voiceID, err := payloadString(task.Payload, "voice_id")
	if err != nil {
		e.log.Error("script payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}
	language, err := payloadString(task.Payload, "language")
	if err != nil {
		e.log.Error("script payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	outline, ok := dependencies["outline"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "outline")
	}
	characterSheet, ok := dependencies["character_sheet"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "character_sheet")
	}

	e.log.Debug("script execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"dependency_count", len(dependencies),
	)

	systemPrompt, userPrompt := buildScriptPrompts(article, language, voiceID, dependencies)
	response, preview, err := generateTextPreview(
		ctx,
		e.textClient,
		e.generationConfig,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		e.log.Error("script text generation failed",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	task.OutputRef = map[string]any{
		"artifact_type":        "script",
		"artifact_path":        fmt.Sprintf("jobs/%s/script.json", job.PublicID),
		"voice_id":             voiceID,
		"article_length":       len([]rune(article)),
		"outline_artifact_ref": outline.OutputRef["artifact_path"],
		"character_ref":        characterSheet.OutputRef["artifact_path"],
		"segment_count":        1,
	}
	appendLLMMetadata(task.OutputRef, response, preview)

	e.log.Info("script execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", task.OutputRef["artifact_path"],
	)

	return task, nil
}
