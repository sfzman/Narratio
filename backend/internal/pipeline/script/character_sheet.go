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
	artifacts        artifactWriter
}

func NewCharacterSheetExecutor() *CharacterSheetExecutor {
	return NewCharacterSheetExecutorWithClient(nil, TextGenerationConfig{}, "")
}

func NewCharacterSheetExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
	workspaceDir string,
) *CharacterSheetExecutor {
	return &CharacterSheetExecutor{
		log:              slog.Default().With("executor", "character_sheet"),
		textClient:       textClient,
		generationConfig: normalizeTextGenerationConfig(generationConfig),
		artifacts:        newArtifactWriter(workspaceDir),
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
	article, language, err := characterSheetPayload(task)
	if err != nil {
		e.logPayloadError("character sheet payload invalid", job, task, err)
		return task, err
	}

	artifactPath := fmt.Sprintf("jobs/%s/character_sheet.json", job.PublicID)
	e.logExecutionStart(job, task)

	output, response, preview, err := e.generateOutput(ctx, article, language)
	if err != nil {
		e.logGenerationError("character sheet text generation failed", job, task, err)
		return task, err
	}
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write character sheet artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":   "character_sheet",
		"artifact_path":   artifactPath,
		"language":        language,
		"article_length":  len([]rune(article)),
		"character_count": len(output.Characters),
	}
	appendLLMMetadata(task.OutputRef, response, preview)
	e.logCompletion(job, task, artifactPath)

	return task, nil
}

func characterSheetPayload(task model.Task) (string, string, error) {
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		return "", "", err
	}
	language, err := payloadString(task.Payload, "language")
	if err != nil {
		return "", "", err
	}

	return article, language, nil
}

func (e *CharacterSheetExecutor) generateOutput(
	ctx context.Context,
	article string,
	language string,
) (CharacterSheetOutput, TextResponse, string, error) {
	systemPrompt, userPrompt := buildCharacterSheetPrompts(article, language)
	response, responseText, preview, err := generateTextContent(
		ctx,
		e.textClient,
		e.generationConfig,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return CharacterSheetOutput{}, TextResponse{}, "", err
	}

	output, err := buildCharacterSheetOutput(article, responseText)
	if err != nil {
		return CharacterSheetOutput{}, TextResponse{}, "", err
	}

	return output, response, preview, nil
}

func (e *CharacterSheetExecutor) logPayloadError(
	message string,
	job model.Job,
	task model.Task,
	err error,
) {
	e.log.Error(message,
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"error", err,
	)
}

func (e *CharacterSheetExecutor) logExecutionStart(job model.Job, task model.Task) {
	e.log.Debug("character sheet execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)
}

func (e *CharacterSheetExecutor) logGenerationError(
	message string,
	job model.Job,
	task model.Task,
	err error,
) {
	e.log.Error(message,
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"error", err,
	)
}

func (e *CharacterSheetExecutor) logCompletion(
	job model.Job,
	task model.Task,
	artifactPath string,
) {
	e.log.Info("character sheet execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
	)
}
