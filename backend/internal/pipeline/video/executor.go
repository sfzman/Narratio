package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const defaultVideoFileSize = 2 * 1024 * 1024

type Executor struct {
	log          *slog.Logger
	workspaceDir string
}

func NewExecutor(workspaceDir ...string) *Executor {
	resolvedWorkspaceDir := ""
	if len(workspaceDir) > 0 {
		resolvedWorkspaceDir = strings.TrimSpace(workspaceDir[0])
	}

	return &Executor{
		log:          slog.Default().With("executor", "video"),
		workspaceDir: resolvedWorkspaceDir,
	}
}

func (e *Executor) Execute(
	_ context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	ttsTask, ok := dependencies["tts"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "tts")
	}

	imageTask, ok := dependencies["image"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "image")
	}
	ttsArtifactPath, err := requiredArtifactPath(ttsTask, "tts")
	if err != nil {
		return task, err
	}
	imageArtifactPath, err := requiredArtifactPath(imageTask, "image")
	if err != nil {
		return task, err
	}
	if err := validateTTSOutputRef(ttsTask.OutputRef); err != nil {
		return task, err
	}
	if err := e.validateTTSArtifacts(ttsTask.OutputRef, ttsArtifactPath); err != nil {
		return task, err
	}
	if err := e.validateImageArtifact(imageArtifactPath, outputInt(ttsTask.OutputRef, "segment_count")); err != nil {
		return task, err
	}

	e.log.Debug("video execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	duration := outputFloat(ttsTask.OutputRef, "total_duration_seconds", 6.5)
	task.OutputRef = map[string]any{
		"artifact_type":      "video",
		"artifact_path":      fmt.Sprintf("jobs/%s/output/final.mp4", job.PublicID),
		"tts_artifact_ref":   ttsArtifactPath,
		"image_artifact_ref": imageArtifactPath,
		"duration_seconds":   duration,
		"file_size_bytes":    int64(defaultVideoFileSize),
	}

	e.log.Info("video execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", task.OutputRef["artifact_path"],
	)

	return task, nil
}

func (e *Executor) validateImageArtifact(relativePath string, expectedCount int) error {
	if e == nil || e.workspaceDir == "" {
		return nil
	}

	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing image artifact file")
		}
		return fmt.Errorf("read image artifact file: %w", err)
	}

	var artifact struct {
		Images []struct {
			FilePath string `json:"file_path"`
		} `json:"images"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		return fmt.Errorf("decode image artifact file: %w", err)
	}
	if len(artifact.Images) == 0 {
		return fmt.Errorf("invalid image artifact images")
	}
	if expectedCount > 0 && len(artifact.Images) != expectedCount {
		return fmt.Errorf("image artifact count does not match tts segment_count")
	}
	for _, item := range artifact.Images {
		filePath := strings.TrimSpace(item.FilePath)
		if filePath == "" {
			return fmt.Errorf("invalid image artifact file_path")
		}
		if err := e.requireExistingFile(filePath, "image file"); err != nil {
			return err
		}
	}

	return nil
}

func (e *Executor) validateTTSArtifacts(values map[string]any, artifactPath string) error {
	if e == nil || e.workspaceDir == "" {
		return nil
	}

	if err := e.requireExistingFile(artifactPath, "tts artifact file"); err != nil {
		return err
	}

	subtitlePath := outputString(values, "subtitle_artifact_ref")
	if subtitlePath == "" {
		return fmt.Errorf("invalid tts subtitle_artifact_ref")
	}
	if err := e.requireExistingFile(subtitlePath, "tts subtitle artifact file"); err != nil {
		return err
	}

	for _, audioPath := range outputStringSlice(values, "audio_segment_paths") {
		if err := e.requireExistingFile(audioPath, "tts audio segment file"); err != nil {
			return err
		}
	}

	return nil
}

func (e *Executor) requireExistingFile(relativePath string, label string) error {
	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	_, err := os.Stat(fullPath)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return fmt.Errorf("missing %s", label)
	}
	return fmt.Errorf("stat %s: %w", label, err)
}

func requiredArtifactPath(task model.Task, dependencyKey string) (string, error) {
	value, ok := task.OutputRef["artifact_path"]
	if !ok {
		return "", fmt.Errorf("missing %s artifact_path", dependencyKey)
	}

	path, ok := value.(string)
	if !ok || path == "" {
		return "", fmt.Errorf("invalid %s artifact_path", dependencyKey)
	}

	return path, nil
}

func validateTTSOutputRef(values map[string]any) error {
	segmentCount := outputInt(values, "segment_count")
	if segmentCount <= 0 {
		return fmt.Errorf("invalid tts segment_count")
	}
	if outputFloat(values, "total_duration_seconds", 0) <= 0 {
		return fmt.Errorf("invalid tts total_duration_seconds")
	}
	audioPaths := outputStringSlice(values, "audio_segment_paths")
	if len(audioPaths) == 0 {
		return fmt.Errorf("invalid tts audio_segment_paths")
	}
	if len(audioPaths) != segmentCount {
		return fmt.Errorf("tts audio_segment_paths count does not match segment_count")
	}

	return nil
}

func outputFloat(values map[string]any, key string, fallback float64) float64 {
	value, ok := values[key]
	if !ok {
		return fallback
	}

	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return fallback
	}
}

func outputInt(values map[string]any, key string) int {
	value, ok := values[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func outputString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(text)
}

func outputStringSlice(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok {
		return nil
	}

	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			result = append(result, text)
		}
		return result
	default:
		return nil
	}
}
