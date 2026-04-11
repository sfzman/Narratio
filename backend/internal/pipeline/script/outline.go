package script

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type OutlineExecutor struct {
	log              *slog.Logger
	textClient       TextClient
	generationConfig TextGenerationConfig
	artifacts        artifactWriter
}

func NewOutlineExecutor() *OutlineExecutor {
	return NewOutlineExecutorWithClient(nil, TextGenerationConfig{}, "")
}

func NewOutlineExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
	workspaceDir string,
) *OutlineExecutor {
	return &OutlineExecutor{
		log:              slog.Default().With("executor", "outline"),
		textClient:       textClient,
		generationConfig: normalizeTextGenerationConfig(generationConfig),
		artifacts:        newArtifactWriter(workspaceDir),
	}
}

func (e *OutlineExecutor) Type() model.TaskType {
	return model.TaskTypeOutline
}

func (e *OutlineExecutor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	_ map[string]model.Task,
) (model.Task, error) {
	article, err := articlePayload(task)
	if err != nil {
		e.logPayloadError("outline payload invalid", job, task, err)
		return task, err
	}

	artifactPath := fmt.Sprintf("jobs/%s/outline.json", job.PublicID)
	e.logExecutionStart(job, task)
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "requesting_text",
		Message: "正在请求大纲生成",
	})

	output, response, preview, err := e.generateOutput(ctx, article)
	if err != nil {
		e.logGenerationError("outline text generation failed", job, task, err)
		return task, err
	}
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "writing_artifact",
		Message: "正在写入大纲产物",
	})
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write outline artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":  "outline",
		"artifact_path":  artifactPath,
		"article_length": len([]rune(article)),
		"summary":        summarizeArticle(article, 60),
		"section_count":  len(output.PlotStages),
	}
	appendLLMMetadata(task.OutputRef, response, preview)
	e.logCompletion(job, task, artifactPath)

	return task, nil
}

func articlePayload(task model.Task) (string, error) {
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		return "", err
	}

	return article, nil
}

func (e *OutlineExecutor) generateOutput(
	ctx context.Context,
	article string,
) (OutlineOutput, TextResponse, string, error) {
	systemPrompt, userPrompt := buildOutlinePrompts(article)
	response, responseText, preview, err := generateTextContent(
		ctx,
		e.textClient,
		e.generationConfig,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return OutlineOutput{}, TextResponse{}, "", err
	}

	output, err := buildOutlineOutput(article, responseText)
	if err != nil {
		return OutlineOutput{}, TextResponse{}, "", err
	}

	return output, response, preview, nil
}

func (e *OutlineExecutor) logPayloadError(
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

func (e *OutlineExecutor) logExecutionStart(job model.Job, task model.Task) {
	e.log.Debug("outline execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)
}

func (e *OutlineExecutor) logGenerationError(
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

func (e *OutlineExecutor) logCompletion(job model.Job, task model.Task, artifactPath string) {
	e.log.Info("outline execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
	)
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

func summarizeArticle(article string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(article))
	if len(runes) <= maxRunes {
		return string(runes)
	}

	return string(runes[:maxRunes]) + "..."
}
