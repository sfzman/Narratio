package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const defaultVideoFileSize = 2 * 1024 * 1024

type Executor struct {
	log          *slog.Logger
	workspaceDir string
	runner       CommandRunner
}

func NewExecutor(workspaceDir ...string) *Executor {
	return NewExecutorWithRunner(nil, workspaceDir...)
}

func NewRealExecutor(workspaceDir ...string) *Executor {
	return NewExecutorWithRunner(realCommandRunner{}, workspaceDir...)
}

func NewExecutorWithRunner(
	runner CommandRunner,
	workspaceDir ...string,
) *Executor {
	resolvedWorkspaceDir := ""
	if len(workspaceDir) > 0 {
		resolvedWorkspaceDir = strings.TrimSpace(workspaceDir[0])
	}

	return &Executor{
		log:          slog.Default().With("executor", "video"),
		workspaceDir: resolvedWorkspaceDir,
		runner:       runner,
	}
}

func (e *Executor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "validating_dependencies",
		Message: "正在校验音频与分镜视频产物",
	})
	ttsTask, ok := dependencies["tts"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "tts")
	}

	shotVideoTask, ok := dependencies["shot_video"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "shot_video")
	}
	ttsArtifactPath, err := requiredArtifactPath(ttsTask, "tts")
	if err != nil {
		return task, err
	}
	shotVideoArtifactPath, err := requiredArtifactPath(shotVideoTask, "shot_video")
	if err != nil {
		return task, err
	}
	if err := validateTTSOutputRef(ttsTask.OutputRef); err != nil {
		return task, err
	}
	if err := e.validateTTSArtifacts(ttsTask.OutputRef, ttsArtifactPath); err != nil {
		return task, err
	}
	shotVideoClips, err := e.loadAndValidateShotVideoArtifact(
		shotVideoArtifactPath,
		outputInt(ttsTask.OutputRef, "segment_count"),
	)
	if err != nil {
		return task, err
	}
	var ttsArtifact ttsArtifact
	if e.runner != nil && e.workspaceDir != "" {
		ttsArtifact, err = e.loadAndValidateTTSArtifact(
			ttsArtifactPath,
			outputInt(ttsTask.OutputRef, "segment_count"),
		)
		if err != nil {
			return task, err
		}
	}

	e.log.Debug("video execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	aspectRatio := resolveTaskAspectRatio(task.Payload)
	narrationDuration := outputFloat(ttsTask.OutputRef, "total_duration_seconds", 6.5)
	visualDuration, err := resolveVideoDuration(narrationDuration, shotVideoClips)
	if err != nil {
		return task, err
	}
	artifactPath := fmt.Sprintf("jobs/%s/output/final.mp4", job.PublicID)
	fileSize := int64(defaultVideoFileSize)
	if e.runner != nil && e.workspaceDir != "" {
		_ = model.ReportTaskProgress(ctx, model.TaskProgress{
			Phase:   "rendering_video",
			Message: "正在渲染最终视频",
		})
		fileSize, err = e.renderFinalVideo(
			ctx,
			artifactPath,
			aspectRatio,
			ttsArtifact,
			shotVideoClips,
		)
		if err != nil {
			e.log.Error("video render failed",
				"job_id", job.ID,
				"job_public_id", job.PublicID,
				"task_id", task.ID,
				"task_key", task.Key,
				"tts_artifact_path", ttsArtifactPath,
				"shot_video_artifact_path", shotVideoArtifactPath,
				"error", err,
			)
			return task, err
		}
	}
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "finalizing_output",
		Message: "正在整理最终视频产物",
	})
	task.OutputRef = map[string]any{
		"artifact_type":              "video",
		"artifact_path":              artifactPath,
		"tts_artifact_ref":           ttsArtifactPath,
		"shot_video_artifact_ref":    shotVideoArtifactPath,
		"image_source_type":          outputString(shotVideoTask.OutputRef, "image_source_type"),
		"aspect_ratio":               string(aspectRatio),
		"duration_seconds":           visualDuration,
		"narration_duration_seconds": narrationDuration,
		"visual_duration_seconds":    visualDuration,
		"file_size_bytes":            fileSize,
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

type ttsArtifact struct {
	AudioSegments []ttsArtifactAudioSegment `json:"audio_segments"`
	TotalDuration float64                   `json:"total_duration_seconds"`
}

type ttsArtifactAudioSegment struct {
	SegmentIndex int     `json:"segment_index"`
	FilePath     string  `json:"file_path"`
	Duration     float64 `json:"duration"`
}

type imageArtifactShotEntry struct {
	SegmentIndex   int    `json:"segment_index"`
	ShotIndex      int    `json:"shot_index"`
	FilePath       string `json:"file_path"`
	Prompt         string `json:"prompt,omitempty"`
	SourceImageURL string `json:"source_image_url,omitempty"`
}

type shotVideoArtifactClip struct {
	SegmentIndex        int             `json:"segment_index"`
	ShotIndex           int             `json:"shot_index"`
	Status              ShotVideoStatus `json:"status"`
	DurationSeconds     float64         `json:"duration_seconds"`
	VideoPath           string          `json:"video_path,omitempty"`
	ImagePath           string          `json:"image_path,omitempty"`
	SourceImagePath     string          `json:"source_image_path"`
	SourceType          string          `json:"source_type"`
	GenerationRequestID string          `json:"generation_request_id,omitempty"`
	GenerationModel     string          `json:"generation_model,omitempty"`
	SourceVideoURL      string          `json:"source_video_url,omitempty"`
}

func (e *Executor) loadAndValidateShotVideoArtifact(
	relativePath string,
	expectedCount int,
) ([]shotVideoArtifactClip, error) {
	if e == nil || e.workspaceDir == "" {
		return nil, nil
	}

	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing shot video artifact file")
		}
		return nil, fmt.Errorf("read shot video artifact file: %w", err)
	}

	var artifact struct {
		Clips []shotVideoArtifactClip `json:"clips"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("decode shot video artifact file: %w", err)
	}

	if err := e.validateShotVideoClips(artifact.Clips, expectedCount); err != nil {
		return nil, err
	}

	return artifact.Clips, nil
}

func (e *Executor) validateShotImageEntries(
	shots []imageArtifactShotEntry,
	expectedCount int,
) error {
	if len(shots) == 0 {
		return fmt.Errorf("invalid image artifact shot_images")
	}

	segments := make(map[int]struct{}, len(shots))
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
		segments[item.SegmentIndex] = struct{}{}
	}
	if expectedCount > 0 && len(segments) != expectedCount {
		return fmt.Errorf("image artifact shot_images segment coverage does not match tts segment_count")
	}

	return nil
}

func (e *Executor) validateShotVideoClips(
	clips []shotVideoArtifactClip,
	expectedCount int,
) error {
	if len(clips) == 0 {
		return fmt.Errorf("invalid shot video artifact clips")
	}

	segments := make(map[int]struct{}, len(clips))
	seen := make(map[string]struct{}, len(clips))
	previousSegmentIndex := -1
	previousShotIndex := -1
	for _, item := range clips {
		if item.SegmentIndex < 0 {
			return fmt.Errorf("invalid shot video clip segment_index")
		}
		if item.ShotIndex < 0 {
			return fmt.Errorf("invalid shot video clip shot_index")
		}
		if !isValidShotVideoStatus(item.Status) {
			return fmt.Errorf("invalid shot video clip status")
		}
		if item.DurationSeconds <= 0 {
			return fmt.Errorf("invalid shot video clip duration_seconds")
		}
		clipKey := fmt.Sprintf("%d:%d", item.SegmentIndex, item.ShotIndex)
		if _, ok := seen[clipKey]; ok {
			return fmt.Errorf("duplicate shot video clip")
		}
		seen[clipKey] = struct{}{}
		if previousSegmentIndex >= 0 {
			if item.SegmentIndex < previousSegmentIndex ||
				(item.SegmentIndex == previousSegmentIndex &&
					item.ShotIndex < previousShotIndex) {
				return fmt.Errorf("shot video artifact clips are not sorted")
			}
		}
		if strings.TrimSpace(item.VideoPath) == "" && strings.TrimSpace(item.ImagePath) == "" {
			return fmt.Errorf("invalid shot video clip media path")
		}
		if item.VideoPath != "" {
			if err := e.requireExistingFile(item.VideoPath, "shot video file"); err != nil {
				return err
			}
		}
		if item.ImagePath != "" {
			if err := e.requireExistingFile(item.ImagePath, "shot fallback image file"); err != nil {
				return err
			}
		}
		segments[item.SegmentIndex] = struct{}{}
		previousSegmentIndex = item.SegmentIndex
		previousShotIndex = item.ShotIndex
	}
	if expectedCount > 0 && len(segments) != expectedCount {
		return fmt.Errorf("shot video artifact segment coverage does not match tts segment_count")
	}

	return nil
}

func resolveVideoDuration(
	narrationDuration float64,
	clips []shotVideoArtifactClip,
) (float64, error) {
	if len(clips) == 0 {
		return narrationDuration, nil
	}

	total := 0.0
	for _, clip := range clips {
		total += clip.DurationSeconds
	}
	if total <= 0 {
		return 0, fmt.Errorf("invalid shot video total_duration_seconds")
	}

	return total, nil
}

func (e *Executor) loadAndValidateTTSArtifact(
	relativePath string,
	expectedCount int,
) (ttsArtifact, error) {
	if e == nil || e.workspaceDir == "" {
		return ttsArtifact{}, nil
	}

	fullPath := filepath.Join(e.workspaceDir, filepath.Clean(relativePath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ttsArtifact{}, fmt.Errorf("read tts artifact file: %w", err)
	}

	var artifact ttsArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return ttsArtifact{}, fmt.Errorf("decode tts artifact file: %w", err)
	}
	if len(artifact.AudioSegments) == 0 {
		return ttsArtifact{}, fmt.Errorf("invalid tts artifact audio_segments")
	}
	if expectedCount > 0 && len(artifact.AudioSegments) != expectedCount {
		return ttsArtifact{}, fmt.Errorf("tts artifact audio_segments count does not match tts segment_count")
	}

	return artifact, nil
}

func (e *Executor) validateTTSArtifacts(values map[string]any, artifactPath string) error {
	if e == nil || e.workspaceDir == "" {
		return nil
	}

	if err := e.requireExistingFile(artifactPath, "tts artifact file"); err != nil {
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
	info, err := os.Stat(fullPath)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("invalid %s", label)
		}
		if info.Size() <= 0 {
			return fmt.Errorf("empty %s", label)
		}
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

func (e *Executor) renderFinalVideo(
	ctx context.Context,
	artifactPath string,
	aspectRatio model.AspectRatio,
	tts ttsArtifact,
	clips []shotVideoArtifactClip,
) (int64, error) {
	outputDir := filepath.Join(e.workspaceDir, filepath.Dir(filepath.Clean(artifactPath)))
	renderDir := filepath.Join(outputDir, "render")
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir video render dir: %w", err)
	}

	mergedAudioPath, err := e.renderMergedAudio(ctx, renderDir, tts)
	if err != nil {
		return 0, err
	}
	segmentPaths, err := e.renderAdjustedSegments(
		ctx,
		renderDir,
		aspectRatio,
		tts,
		clips,
	)
	if err != nil {
		return 0, err
	}
	concatenatedPath, err := e.renderConcatenatedVideo(ctx, renderDir, segmentPaths)
	if err != nil {
		return 0, err
	}
	fullOutputPath := filepath.Join(e.workspaceDir, filepath.Clean(artifactPath))
	if err := e.muxFinalVideo(ctx, concatenatedPath, mergedAudioPath, fullOutputPath); err != nil {
		return 0, err
	}

	info, err := os.Stat(fullOutputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("missing final video file")
		}
		return 0, fmt.Errorf("stat final video file: %w", err)
	}
	if info.Size() <= 0 {
		return 0, fmt.Errorf("empty final video file")
	}

	return info.Size(), nil
}

func (e *Executor) renderMergedAudio(
	ctx context.Context,
	renderDir string,
	tts ttsArtifact,
) (string, error) {
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "rendering_audio",
		Message: "正在拼接旁白音频",
	})
	audioPaths := make([]string, 0, len(tts.AudioSegments))
	for _, segment := range tts.AudioSegments {
		audioPaths = append(
			audioPaths,
			filepath.Join(e.workspaceDir, filepath.Clean(segment.FilePath)),
		)
	}
	mergedAudioPath := filepath.Join(renderDir, "merged_audio.wav")
	if err := concatAudioSegments(ctx, e.runner, mergedAudioPath, audioPaths); err != nil {
		return "", err
	}

	return mergedAudioPath, nil
}

func (e *Executor) renderAdjustedSegments(
	ctx context.Context,
	renderDir string,
	aspectRatio model.AspectRatio,
	tts ttsArtifact,
	clips []shotVideoArtifactClip,
) ([]string, error) {
	grouped := groupClipsBySegment(clips)
	segmentPaths := make([]string, 0, len(tts.AudioSegments))
	for index, audio := range tts.AudioSegments {
		_ = model.ReportTaskProgress(ctx, model.TaskProgress{
			Phase:   "rendering_segment",
			Message: fmt.Sprintf("正在渲染第 %d/%d 段视频", index+1, len(tts.AudioSegments)),
			Current: index + 1,
			Total:   len(tts.AudioSegments),
			Unit:    "segment",
		})
		segmentClips := grouped[audio.SegmentIndex]
		if len(segmentClips) == 0 {
			return nil, fmt.Errorf("missing shot video clips for segment %d", audio.SegmentIndex)
		}

		basePath, err := e.renderSegmentBaseVideo(
			ctx,
			renderDir,
			aspectRatio,
			audio.SegmentIndex,
			segmentClips,
		)
		if err != nil {
			return nil, err
		}
		adjustedPath, err := e.adjustSegmentVideoDuration(
			ctx,
			renderDir,
			audio.SegmentIndex,
			basePath,
			audio.Duration,
		)
		if err != nil {
			return nil, err
		}
		segmentPaths = append(segmentPaths, adjustedPath)
	}

	return segmentPaths, nil
}

func groupClipsBySegment(
	clips []shotVideoArtifactClip,
) map[int][]shotVideoArtifactClip {
	grouped := make(map[int][]shotVideoArtifactClip, len(clips))
	for _, clip := range clips {
		grouped[clip.SegmentIndex] = append(grouped[clip.SegmentIndex], clip)
	}
	for segmentIndex := range grouped {
		sort.Slice(grouped[segmentIndex], func(i, j int) bool {
			return grouped[segmentIndex][i].ShotIndex < grouped[segmentIndex][j].ShotIndex
		})
	}

	return grouped
}

func (e *Executor) renderSegmentBaseVideo(
	ctx context.Context,
	renderDir string,
	aspectRatio model.AspectRatio,
	segmentIndex int,
	clips []shotVideoArtifactClip,
) (string, error) {
	clipPaths := make([]string, 0, len(clips))
	for _, clip := range clips {
		renderedPath, err := e.renderClipMedia(ctx, renderDir, aspectRatio, clip)
		if err != nil {
			return "", err
		}
		clipPaths = append(clipPaths, renderedPath)
	}

	outputPath := filepath.Join(renderDir, fmt.Sprintf("segment_%03d_base.mp4", segmentIndex))
	if err := concatVideoClips(ctx, e.runner, outputPath, clipPaths); err != nil {
		return "", err
	}

	return outputPath, nil
}

func (e *Executor) renderClipMedia(
	ctx context.Context,
	renderDir string,
	aspectRatio model.AspectRatio,
	clip shotVideoArtifactClip,
) (string, error) {
	outputPath := filepath.Join(
		renderDir,
		fmt.Sprintf("segment_%03d_shot_%03d.mp4", clip.SegmentIndex, clip.ShotIndex),
	)
	if clip.Status == ShotVideoStatusGeneratedVideo && strings.TrimSpace(clip.VideoPath) != "" {
		return outputPath, e.normalizeVideoClip(ctx, aspectRatio, clip.VideoPath, outputPath)
	}

	imagePath := clip.ImagePath
	if strings.TrimSpace(imagePath) == "" {
		imagePath = clip.SourceImagePath
	}
	return outputPath, e.renderImagePlaceholderClip(
		ctx,
		aspectRatio,
		clip,
		imagePath,
		outputPath,
		clip.DurationSeconds,
	)
}

func (e *Executor) normalizeVideoClip(
	ctx context.Context,
	aspectRatio model.AspectRatio,
	relativeInputPath string,
	outputPath string,
) error {
	inputPath := filepath.Join(e.workspaceDir, filepath.Clean(relativeInputPath))
	output, err := e.runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-i",
		inputPath,
		"-map",
		"0:v:0",
		"-vf",
		buildCoverScaleFilter(aspectRatio),
		"-an",
		"-c:v",
		"libx264",
		"-preset",
		"veryfast",
		"-crf",
		"20",
		"-pix_fmt",
		"yuv420p",
		outputPath,
	)
	if err != nil {
		return commandError("normalize shot video clip", err, output)
	}

	return nil
}

func (e *Executor) renderImagePlaceholderClip(
	ctx context.Context,
	aspectRatio model.AspectRatio,
	clip shotVideoArtifactClip,
	relativeImagePath string,
	outputPath string,
	duration float64,
) error {
	imagePath := filepath.Join(e.workspaceDir, filepath.Clean(relativeImagePath))
	if duration <= 0 {
		duration = 3
	}
	output, err := e.runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-loop",
		"1",
		"-framerate",
		fmt.Sprintf("%d", defaultFinalVideoFPS),
		"-t",
		fmt.Sprintf("%.3f", duration),
		"-i",
		imagePath,
		"-vf",
		buildAnimatedCoverScaleFilter(
			aspectRatio,
			duration,
			clip.SegmentIndex+clip.ShotIndex,
		),
		"-an",
		"-c:v",
		"libx264",
		"-preset",
		"veryfast",
		"-crf",
		"20",
		"-pix_fmt",
		"yuv420p",
		outputPath,
	)
	if err != nil {
		return commandError("render image fallback clip", err, output)
	}

	return nil
}

func (e *Executor) adjustSegmentVideoDuration(
	ctx context.Context,
	renderDir string,
	segmentIndex int,
	basePath string,
	targetDuration float64,
) (string, error) {
	if targetDuration <= 0 {
		return "", fmt.Errorf("invalid tts segment duration")
	}
	baseDuration, err := probeMediaDuration(ctx, e.runner, basePath)
	if err != nil {
		return "", err
	}
	outputPath := filepath.Join(renderDir, fmt.Sprintf("segment_%03d.mp4", segmentIndex))
	output, err := e.runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-i",
		basePath,
		"-filter:v",
		fmt.Sprintf("setpts=%.10f*PTS", targetDuration/baseDuration),
		"-an",
		"-c:v",
		"libx264",
		"-preset",
		"veryfast",
		"-crf",
		"18",
		"-pix_fmt",
		"yuv420p",
		outputPath,
	)
	if err != nil {
		return "", commandError("adjust segment video duration", err, output)
	}

	return outputPath, nil
}

func (e *Executor) renderConcatenatedVideo(
	ctx context.Context,
	renderDir string,
	segmentPaths []string,
) (string, error) {
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "concatenating_segments",
		Message: "正在拼接分段视频",
	})
	outputPath := filepath.Join(renderDir, "concatenated.mp4")
	if err := concatVideoClips(ctx, e.runner, outputPath, segmentPaths); err != nil {
		return "", err
	}

	return outputPath, nil
}

func (e *Executor) muxFinalVideo(
	ctx context.Context,
	videoPath string,
	audioPath string,
	outputPath string,
) error {
	_ = model.ReportTaskProgress(ctx, model.TaskProgress{
		Phase:   "muxing_video",
		Message: "正在合成最终视频与音频",
	})
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("mkdir final video dir: %w", err)
	}
	output, err := e.runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-i",
		videoPath,
		"-i",
		audioPath,
		"-map",
		"0:v:0",
		"-map",
		"1:a:0",
		"-c:v",
		"libx264",
		"-preset",
		"veryfast",
		"-crf",
		"18",
		"-pix_fmt",
		"yuv420p",
		"-c:a",
		"aac",
		"-b:a",
		"192k",
		"-shortest",
		"-movflags",
		"+faststart",
		outputPath,
	)
	if err != nil {
		return commandError("mux final video", err, output)
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
