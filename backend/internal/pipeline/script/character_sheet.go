package script

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type CharacterSheetExecutor struct {
	log              *slog.Logger
	textClient       TextClient
	generationConfig TextGenerationConfig
}

func NewCharacterSheetExecutor() *CharacterSheetExecutor {
	return NewCharacterSheetExecutorWithClient(nil, TextGenerationConfig{})
}

func NewCharacterSheetExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
) *CharacterSheetExecutor {
	return &CharacterSheetExecutor{
		log:              slog.Default().With("executor", "character_sheet"),
		textClient:       textClient,
		generationConfig: normalizeTextGenerationConfig(generationConfig),
	}
}

func (e *CharacterSheetExecutor) Type() model.TaskType {
	return model.TaskTypeCharacterSheet
}

func (e *CharacterSheetExecutor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	_ map[string]model.Task,
) (model.Task, error) {
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		e.log.Error("character sheet payload invalid",
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
		e.log.Error("character sheet payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	artifactPath := fmt.Sprintf("jobs/%s/character_sheet.json", job.PublicID)
	systemPrompt, userPrompt := buildCharacterSheetPrompts(article, language)

	e.log.Debug("character sheet execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	response, preview, err := generateTextPreview(
		ctx,
		e.textClient,
		e.generationConfig,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		e.log.Error("character sheet text generation failed",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	task.OutputRef = map[string]any{
		"artifact_type":   "character_sheet",
		"artifact_path":   artifactPath,
		"language":        language,
		"article_length":  len([]rune(article)),
		"character_count": 1,
	}
	appendLLMMetadata(task.OutputRef, response, preview)

	e.log.Info("character sheet execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
	)

	return task, nil
}
