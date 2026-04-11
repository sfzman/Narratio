package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type ShotVideoExecutor struct {
	log                    *slog.Logger
	client                 Client
	generationConfig       GenerationConfig
	workspaceDir           string
	defaultDurationSeconds float64
}

type ShotVideoStatus string

const (
	ShotVideoStatusGeneratedVideo ShotVideoStatus = "generated_video"
	ShotVideoStatusImageFallback  ShotVideoStatus = "image_fallback"
	defaultVideoGenerationCount   int             = 2
)

type ShotVideoOutput struct {
	Clips []GeneratedShotVideo `json:"clips"`
}

type GeneratedShotVideo struct {
	SegmentIndex        int             `json:"segment_index"`
	ShotIndex           int             `json:"shot_index"`
	Status              ShotVideoStatus `json:"status"`
	DurationSeconds     float64         `json:"duration_seconds"`
	VideoPath           string          `json:"video_path,omitempty"`
	ImagePath           string          `json:"image_path,omitempty"`
	SourceImagePath     string          `json:"source_image_path"`
	SourceType          string          `json:"source_type"`
	IsFallback          bool            `json:"is_fallback"`
	GenerationRequestID string          `json:"generation_request_id,omitempty"`
	GenerationModel     string          `json:"generation_model,omitempty"`
	SourceVideoURL      string          `json:"source_video_url,omitempty"`
}

func NewShotVideoExecutor(
	workspaceDir string,
	defaultDurationSeconds float64,
) *ShotVideoExecutor {
	return NewShotVideoExecutorWithClient(
		nil,
		GenerationConfig{},
		workspaceDir,
		defaultDurationSeconds,
	)
}

func NewShotVideoExecutorWithClient(
	client Client,
	generationConfig GenerationConfig,
	workspaceDir string,
	defaultDurationSeconds float64,
) *ShotVideoExecutor {
	if defaultDurationSeconds <= 0 {
		defaultDurationSeconds = 3
	}

	return &ShotVideoExecutor{
		log:                    slog.Default().With("executor", "shot_video"),
		client:                 client,
		generationConfig:       normalizeGenerationConfig(generationConfig),
		workspaceDir:           strings.TrimSpace(workspaceDir),
		defaultDurationSeconds: defaultDurationSeconds,
	}
}

func (e *ShotVideoExecutor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	imageTask, ok := dependencies["image"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "image")
	}

	imageArtifactPath, err := requiredArtifactPath(imageTask, "image")
	if err != nil {
		return task, err
	}

	shotImages, imageSourceType, err := e.loadShotImages(imageArtifactPath)
	if err != nil {
		return task, err
	}

	artifactPath := fmt.Sprintf("jobs/%s/shot_videos/manifest.json", job.PublicID)
	requestedVideoCount := resolveRequestedVideoCount(task.Payload)
	aspectRatio := resolveTaskAspectRatio(task.Payload)
	output, generationMode, err := e.buildShotVideoOutput(
		ctx,
		job.PublicID,
		shotImages,
		aspectRatio,
		requestedVideoCount,
	)
	if err != nil {
		return task, err
	}
	if err := e.writeShotVideoArtifact(artifactPath, output); err != nil {
		return task, err
	}
	generatedVideoCount := countGeneratedShotVideos(output.Clips)
	fallbackImageCount := len(output.Clips) - generatedVideoCount

	task.OutputRef = map[string]any{
		"artifact_type":         "shot_video",
		"artifact_path":         artifactPath,
		"image_artifact_ref":    imageArtifactPath,
		"image_source_type":     imageSourceType,
		"clip_count":            len(output.Clips),
		"generated_video_count": generatedVideoCount,
		"fallback_image_count":  fallbackImageCount,
		"aspect_ratio":          string(aspectRatio),
		"requested_video_count": requestedVideoCount,
		"selected_video_count":  minInt(requestedVideoCount, len(output.Clips)),
		"generation_mode":       generationMode,
	}

	e.log.Info("shot video execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
		"clip_count", len(output.Clips),
	)

	return task, nil
}

func (e *ShotVideoExecutor) loadShotImages(
	relativePath string,
) ([]imageArtifactShotEntry, string, error) {
	if strings.TrimSpace(e.workspaceDir) == "" {
		return nil, "", fmt.Errorf("workspace_dir is required for shot_video")
	}

	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("missing image artifact file")
		}
		return nil, "", fmt.Errorf("read image artifact file: %w", err)
	}

	var artifact struct {
		ShotImages []imageArtifactShotEntry `json:"shot_images"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, "", fmt.Errorf("decode image artifact file: %w", err)
	}
	if err := e.validateShotImageEntries(artifact.ShotImages); err != nil {
		return nil, "", err
	}

	return artifact.ShotImages, "shot_images", nil
}

func (e *ShotVideoExecutor) buildShotVideoOutput(
	ctx context.Context,
	jobPublicID string,
	shotImages []imageArtifactShotEntry,
	aspectRatio model.AspectRatio,
	requestedVideoCount int,
) (ShotVideoOutput, string, error) {
	sortShotImages(shotImages)
	clips := make([]GeneratedShotVideo, 0, len(shotImages))
	for index, shot := range shotImages {
		shouldGenerateVideo := index < requestedVideoCount
		clip, err := e.buildShotVideoClip(
			ctx,
			jobPublicID,
			shot,
			aspectRatio,
			shouldGenerateVideo,
		)
		if err != nil {
			return ShotVideoOutput{}, "", err
		}
		clips = append(clips, clip)
	}

	return ShotVideoOutput{
		Clips: clips,
	}, detectShotVideoGenerationMode(clips), nil
}

func (e *ShotVideoExecutor) buildShotVideoClip(
	ctx context.Context,
	jobPublicID string,
	shot imageArtifactShotEntry,
	aspectRatio model.AspectRatio,
	shouldGenerateVideo bool,
) (GeneratedShotVideo, error) {
	fallback := GeneratedShotVideo{
		SegmentIndex:    shot.SegmentIndex,
		ShotIndex:       shot.ShotIndex,
		Status:          ShotVideoStatusImageFallback,
		DurationSeconds: e.defaultDurationSeconds,
		ImagePath:       shot.FilePath,
		SourceImagePath: shot.FilePath,
		SourceType:      "image_fallback",
		IsFallback:      true,
	}
	if !shouldGenerateVideo || e.client == nil {
		return fallback, nil
	}

	response, err := e.client.Generate(ctx, Request{
		Model:           e.generationConfig.Model,
		Prompt:          strings.TrimSpace(shot.Prompt),
		AspectRatio:     aspectRatio,
		SegmentIndex:    shot.SegmentIndex,
		ShotIndex:       shot.ShotIndex,
		SourceImagePath: shot.FilePath,
		SourceImageURL:  strings.TrimSpace(shot.SourceImageURL),
		DurationSeconds: e.defaultDurationSeconds,
	})
	if err != nil {
		e.log.Warn("shot video generation failed, fallback to image",
			"segment_index", shot.SegmentIndex,
			"shot_index", shot.ShotIndex,
			"source_image_path", shot.FilePath,
			"error", err,
		)
		return fallback, nil
	}

	videoPath, err := e.writeGeneratedVideo(
		jobPublicID,
		shot.SegmentIndex,
		shot.ShotIndex,
		response.VideoData,
	)
	if err != nil {
		return GeneratedShotVideo{}, err
	}

	duration := response.DurationSeconds
	if duration <= 0 {
		duration = e.defaultDurationSeconds
	}
	modelName := strings.TrimSpace(response.Model)
	if modelName == "" {
		modelName = e.generationConfig.Model
	}

	return GeneratedShotVideo{
		SegmentIndex:        shot.SegmentIndex,
		ShotIndex:           shot.ShotIndex,
		Status:              ShotVideoStatusGeneratedVideo,
		DurationSeconds:     duration,
		VideoPath:           videoPath,
		SourceImagePath:     shot.FilePath,
		SourceType:          "generated_video",
		IsFallback:          false,
		GenerationRequestID: strings.TrimSpace(response.RequestID),
		GenerationModel:     modelName,
		SourceVideoURL:      strings.TrimSpace(response.VideoURL),
	}, nil
}

func resolveRequestedVideoCount(payload map[string]any) int {
	if payload == nil {
		return defaultVideoGenerationCount
	}

	value, ok := payload["video_count"]
	if !ok {
		return defaultVideoGenerationCount
	}

	count, ok := convertVideoCount(value)
	if !ok || count < 0 {
		return defaultVideoGenerationCount
	}

	return count
}

func convertVideoCount(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func sortShotImages(items []imageArtifactShotEntry) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SegmentIndex != items[j].SegmentIndex {
			return items[i].SegmentIndex < items[j].SegmentIndex
		}
		return items[i].ShotIndex < items[j].ShotIndex
	})
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func (e *ShotVideoExecutor) writeGeneratedVideo(
	jobPublicID string,
	segmentIndex int,
	shotIndex int,
	videoData []byte,
) (string, error) {
	if len(videoData) == 0 {
		return "", fmt.Errorf("shot video response is empty")
	}

	relativePath := fmt.Sprintf(
		"jobs/%s/shot_videos/segment_%03d_shot_%03d.mp4",
		jobPublicID,
		segmentIndex,
		shotIndex,
	)
	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("mkdir shot video dir: %w", err)
	}
	if err := os.WriteFile(fullPath, videoData, 0o644); err != nil {
		return "", fmt.Errorf("write shot video file: %w", err)
	}

	return relativePath, nil
}

func (e *ShotVideoExecutor) writeShotVideoArtifact(
	relativePath string,
	output ShotVideoOutput,
) error {
	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("mkdir shot video artifact dir: %w", err)
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal shot video artifact: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("write shot video artifact: %w", err)
	}

	return nil
}

func (e *ShotVideoExecutor) validateShotImageEntries(
	shots []imageArtifactShotEntry,
) error {
	if len(shots) == 0 {
		return fmt.Errorf("invalid image artifact shot_images")
	}

	for _, item := range shots {
		filePath := strings.TrimSpace(item.FilePath)
		if filePath == "" {
			return fmt.Errorf("invalid image artifact shot_images file_path")
		}
		if item.SegmentIndex < 0 {
			return fmt.Errorf("invalid image artifact shot_images segment_index")
		}
		if item.ShotIndex < 0 {
			return fmt.Errorf("invalid image artifact shot_images shot_index")
		}
		if err := e.requireExistingFile(filePath, "image shot file"); err != nil {
			return err
		}
	}

	return nil
}

func (e *ShotVideoExecutor) requireExistingFile(
	relativePath string,
	label string,
) error {
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

func isValidShotVideoStatus(status ShotVideoStatus) bool {
	switch status {
	case ShotVideoStatusGeneratedVideo, ShotVideoStatusImageFallback:
		return true
	default:
		return false
	}
}

func detectShotVideoGenerationMode(clips []GeneratedShotVideo) string {
	generatedCount := countGeneratedShotVideos(clips)
	switch {
	case generatedCount == 0:
		return "image_fallback"
	case generatedCount == len(clips):
		return "generated_video"
	default:
		return "mixed"
	}
}

func countGeneratedShotVideos(clips []GeneratedShotVideo) int {
	count := 0
	for _, clip := range clips {
		if clip.Status == ShotVideoStatusGeneratedVideo {
			count++
		}
	}

	return count
}
