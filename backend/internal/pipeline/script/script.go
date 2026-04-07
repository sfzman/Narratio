package script

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type ScriptExecutor struct {
	log              *slog.Logger
	textClient       TextClient
	generationConfig TextGenerationConfig
	artifacts        artifactWriter
}

func NewScriptExecutor() *ScriptExecutor {
	return NewScriptExecutorWithClient(nil, TextGenerationConfig{}, "")
}

func NewScriptExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
	workspaceDir string,
) *ScriptExecutor {
	return &ScriptExecutor{
		log:              slog.Default().With("executor", "script"),
		textClient:       textClient,
		generationConfig: normalizeTextGenerationConfig(generationConfig),
		artifacts:        newArtifactWriter(workspaceDir),
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
	article, language, voiceID, err := scriptPayload(task)
	if err != nil {
		e.logPayloadError("script payload invalid", job, task, err)
		return task, err
	}

	outline, err := requiredDependency(dependencies, "outline")
	if err != nil {
		return task, err
	}
	segmentation, err := requiredDependency(dependencies, "segmentation")
	if err != nil {
		return task, err
	}
	characterSheet, err := requiredDependency(dependencies, "character_sheet")
	if err != nil {
		return task, err
	}

	artifactPath := fmt.Sprintf("jobs/%s/script.json", job.PublicID)
	e.logExecutionStart(job, task, len(dependencies))

	output, response, preview, err := e.generateOutput(
		ctx,
		language,
		voiceID,
		segmentation,
		outline,
		characterSheet,
	)
	if err != nil {
		e.logGenerationError("script text generation failed", job, task, err)
		return task, err
	}
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write script artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":        "script",
		"artifact_path":        artifactPath,
		"voice_id":             voiceID,
		"article_length":       len([]rune(article)),
		"segmentation_ref":     segmentation.OutputRef["artifact_path"],
		"outline_artifact_ref": outline.OutputRef["artifact_path"],
		"character_ref":        characterSheet.OutputRef["artifact_path"],
		"segment_count":        len(output.Segments),
	}
	appendLLMMetadata(task.OutputRef, response, preview)
	e.logCompletion(job, task, artifactPath)

	return task, nil
}

func scriptPayload(task model.Task) (string, string, string, error) {
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		return "", "", "", err
	}
	language, err := payloadString(task.Payload, "language")
	if err != nil {
		return "", "", "", err
	}
	voiceID, err := payloadString(task.Payload, "voice_id")
	if err != nil {
		return "", "", "", err
	}

	return article, language, voiceID, nil
}

func requiredDependency(
	dependencies map[string]model.Task,
	key string,
) (model.Task, error) {
	dependency, ok := dependencies[key]
	if !ok {
		return model.Task{}, fmt.Errorf("missing dependency %q", key)
	}

	return dependency, nil
}

func (e *ScriptExecutor) generateOutput(
	ctx context.Context,
	language string,
	voiceID string,
	segmentationTask model.Task,
	outlineTask model.Task,
	characterTask model.Task,
) (ScriptOutput, TextResponse, string, error) {
	segmentation, err := loadArtifactJSON[SegmentationOutput](
		e.artifacts.workspaceDir,
		segmentationTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return ScriptOutput{}, TextResponse{}, "", fmt.Errorf("load segmentation artifact: %w", err)
	}
	outline, err := loadArtifactJSON[OutlineOutput](
		e.artifacts.workspaceDir,
		outlineTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return ScriptOutput{}, TextResponse{}, "", fmt.Errorf("load outline artifact: %w", err)
	}
	characters, err := loadArtifactJSON[CharacterSheetOutput](
		e.artifacts.workspaceDir,
		characterTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return ScriptOutput{}, TextResponse{}, "", fmt.Errorf("load character sheet artifact: %w", err)
	}

	systemPrompt, userPrompt := buildScriptPrompts(language, voiceID, segmentation, outline, characters)
	response, responseText, preview, err := generateTextContent(
		ctx,
		e.textClient,
		e.generationConfig,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return ScriptOutput{}, TextResponse{}, "", err
	}

	output, err := buildScriptOutput(segmentation, responseText)
	if err != nil {
		return ScriptOutput{}, TextResponse{}, "", err
	}

	return output, response, preview, nil
}

func loadArtifactJSON[T any](workspaceDir string, ref any) (T, error) {
	var zero T

	path, ok := ref.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return zero, fmt.Errorf("artifact ref is invalid: %v", ref)
	}

	data, err := os.ReadFile(artifactFullPath(workspaceDir, path))
	if err != nil {
		return zero, fmt.Errorf("read artifact file: %w", err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return zero, fmt.Errorf("decode artifact json: %w", err)
	}

	return value, nil
}

func (e *ScriptExecutor) logPayloadError(
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

func (e *ScriptExecutor) logExecutionStart(
	job model.Job,
	task model.Task,
	dependencyCount int,
) {
	e.log.Debug("script execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"dependency_count", dependencyCount,
	)
}

func (e *ScriptExecutor) logGenerationError(
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

func (e *ScriptExecutor) logCompletion(job model.Job, task model.Task, artifactPath string) {
	e.log.Info("script execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
	)
}
