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
}

func NewOutlineExecutor() *OutlineExecutor {
	return NewOutlineExecutorWithClient(nil, TextGenerationConfig{})
}

func NewOutlineExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
) *OutlineExecutor {
	return &OutlineExecutor{
		log:              slog.Default().With("executor", "outline"),
		textClient:       textClient,
		generationConfig: normalizeTextGenerationConfig(generationConfig),
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
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		e.log.Error("outline payload invalid",
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
		e.log.Error("outline payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	summary := summarizeArticle(article, 60)
	artifactPath := fmt.Sprintf("jobs/%s/outline.json", job.PublicID)
	systemPrompt, userPrompt := buildOutlinePrompts(article, language)

	e.log.Debug("outline execution started",
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
		e.log.Error("outline text generation failed",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	task.OutputRef = map[string]any{
		"artifact_type":  "outline",
		"artifact_path":  artifactPath,
		"language":       language,
		"article_length": len([]rune(article)),
		"summary":        summary,
	}
	appendLLMMetadata(task.OutputRef, response, preview)

	e.log.Info("outline execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
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

func summarizeArticle(article string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(article))
	if len(runes) <= maxRunes {
		return string(runes)
	}

	return string(runes[:maxRunes]) + "..."
}
