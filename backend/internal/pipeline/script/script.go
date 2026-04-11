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

const defaultScriptMaxTokens = 8192

func NewScriptExecutor() *ScriptExecutor {
	return NewScriptExecutorWithClient(nil, TextGenerationConfig{}, "")
}

func NewScriptExecutorWithClient(
	textClient TextClient,
	generationConfig TextGenerationConfig,
	workspaceDir string,
) *ScriptExecutor {
	generationConfig = normalizeScriptGenerationConfig(generationConfig)
	return &ScriptExecutor{
		log:              slog.Default().With("executor", "script"),
		textClient:       textClient,
		generationConfig: generationConfig,
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
	article, voiceID, err := scriptPayload(task)
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
		job.PublicID,
		voiceID,
		segmentation,
		outline,
		characterSheet,
	)
	if err != nil {
		e.logGenerationError("script text generation failed", job, task, err)
		return task, err
	}
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "writing_artifact",
		Message: "正在汇总并写入分镜产物",
	})
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write script artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":        "script",
		"artifact_path":        artifactPath,
		"segment_artifact_dir": scriptSegmentArtifactDir(job.PublicID),
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

func scriptPayload(task model.Task) (string, string, error) {
	article, err := payloadString(task.Payload, "article")
	if err != nil {
		return "", "", err
	}
	voiceID, err := payloadString(task.Payload, "voice_id")
	if err != nil {
		return "", "", err
	}

	return article, voiceID, nil
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
	jobPublicID string,
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

	return e.generateSegmentOutputs(
		ctx,
		jobPublicID,
		voiceID,
		segmentation,
		outline,
		characters,
	)
}

func normalizeScriptGenerationConfig(cfg TextGenerationConfig) TextGenerationConfig {
	cfg = normalizeTextGenerationConfig(cfg)
	if cfg.MaxTokens < defaultScriptMaxTokens {
		cfg.MaxTokens = defaultScriptMaxTokens
	}

	return cfg
}

func (e *ScriptExecutor) generateSegmentOutputs(
	ctx context.Context,
	jobPublicID string,
	voiceID string,
	segmentation SegmentationOutput,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (ScriptOutput, TextResponse, string, error) {
	output := ScriptOutput{
		Segments: make([]Segment, 0, len(segmentation.Segments)),
	}
	var metadata TextResponse
	previews := make([]string, 0, len(segmentation.Segments))

	for _, segment := range segmentation.Segments {
		_ = model.ReportTaskProgress(ctx, model.TaskProgress{
			Phase:   "generating_segment",
			Message: fmt.Sprintf("正在生成第 %d/%d 段分镜", segment.Index+1, len(segmentation.Segments)),
			Current: segment.Index + 1,
			Total:   len(segmentation.Segments),
			Unit:    "segment",
		})
		segmentOutput, response, preview, reused, err := e.loadOrGenerateSegmentOutput(
			ctx,
			jobPublicID,
			voiceID,
			segment,
			outline,
			characters,
		)
		if err != nil {
			return ScriptOutput{}, TextResponse{}, "", err
		}
		output.Segments = append(output.Segments, segmentOutput)

		if metadata.RequestID == "" {
			metadata.RequestID = response.RequestID
		}
		if metadata.Model == "" {
			metadata.Model = response.Model
		}
		if preview != "" {
			previews = append(previews, fmt.Sprintf("segment[%d]: %s", segment.Index, preview))
		} else if reused {
			previews = append(previews, fmt.Sprintf("segment[%d]: reused saved artifact", segment.Index))
		}
	}

	normalizeScriptOutput(&output, segmentation)
	return output, metadata, strings.Join(previews, "\n"), nil
}

func wrapScriptParseError(err error, response TextResponse) error {
	if finishReason := response.FirstFinishReason(); finishReason == "length" {
		return fmt.Errorf(
			"%w; model output stopped with finish_reason=length, consider increasing script output budget",
			err,
		)
	}

	return err
}

func (e *ScriptExecutor) loadOrGenerateSegmentOutput(
	ctx context.Context,
	jobPublicID string,
	voiceID string,
	segment TextSegment,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (Segment, TextResponse, string, bool, error) {
	if saved, ok, err := e.loadSavedSegmentOutput(jobPublicID, segment); err != nil {
		return Segment{}, TextResponse{}, "", false, err
	} else if ok {
		return saved, TextResponse{}, "", true, nil
	}

	response, output, preview, err := e.generateSingleSegmentOutput(
		ctx,
		voiceID,
		segment,
		outline,
		characters,
	)
	if err != nil {
		return Segment{}, TextResponse{}, "", false, err
	}
	if err := e.writeSegmentArtifacts(jobPublicID, output); err != nil {
		return Segment{}, TextResponse{}, "", false, err
	}

	return output, response, preview, false, nil
}

func (e *ScriptExecutor) loadSavedSegmentOutput(
	jobPublicID string,
	source TextSegment,
) (Segment, bool, error) {
	path := scriptSegmentArtifactPath(jobPublicID, source.Index)
	data, err := os.ReadFile(artifactFullPath(e.artifacts.workspaceDir, path))
	if err != nil {
		if os.IsNotExist(err) {
			return Segment{}, false, nil
		}
		return Segment{}, false, fmt.Errorf("read saved script segment artifact: %w", err)
	}

	var segment Segment
	if err := json.Unmarshal(data, &segment); err != nil {
		e.log.Warn("saved script segment artifact invalid, regenerating",
			"segment_index", source.Index,
			"artifact_path", path,
			"error", err,
		)
		return Segment{}, false, nil
	}

	output := ScriptOutput{Segments: []Segment{segment}}
	normalizeScriptOutput(&output, SegmentationOutput{Segments: []TextSegment{source}})
	return output.Segments[0], true, nil
}

func (e *ScriptExecutor) generateSingleSegmentOutput(
	ctx context.Context,
	voiceID string,
	segment TextSegment,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (TextResponse, Segment, string, error) {
	singleSegmentation := SegmentationOutput{Segments: []TextSegment{segment}}
	systemPrompt, userPrompt := buildScriptPrompts(
		singleSegmentation,
		outline,
		characters,
	)
	response, responseText, preview, err := generateTextContent(
		ctx,
		e.textClient,
		e.generationConfig,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return TextResponse{}, Segment{}, "", err
	}

	output, err := buildScriptOutput(singleSegmentation, responseText)
	if err != nil {
		return TextResponse{}, Segment{}, "", wrapScriptParseError(err, response)
	}
	if len(output.Segments) == 0 {
		return TextResponse{}, Segment{}, "", fmt.Errorf("script response returned no segments")
	}

	return response, output.Segments[0], preview, nil
}

func (e *ScriptExecutor) writeSegmentArtifacts(jobPublicID string, segment Segment) error {
	jsonPath := scriptSegmentArtifactPath(jobPublicID, segment.Index)
	if err := e.artifacts.WriteJSON(jsonPath, segment); err != nil {
		return fmt.Errorf("write script segment artifact: %w", err)
	}

	return nil
}

func scriptSegmentArtifactDir(jobPublicID string) string {
	return fmt.Sprintf("jobs/%s/script", jobPublicID)
}

func scriptSegmentArtifactPath(jobPublicID string, index int) string {
	return fmt.Sprintf("%s/segment_%03d.json", scriptSegmentArtifactDir(jobPublicID), index)
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
