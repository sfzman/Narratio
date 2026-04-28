package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sfzman/Narratio/backend/internal/config"
	"github.com/sfzman/Narratio/backend/internal/model"
	imagepipeline "github.com/sfzman/Narratio/backend/internal/pipeline/image"
	ttspipeline "github.com/sfzman/Narratio/backend/internal/pipeline/tts"
	videopipeline "github.com/sfzman/Narratio/backend/internal/pipeline/video"
)

const defaultVideoRenderSmokeJobID = "job_live_video_render_smoke"

type videoRenderSummary struct {
	WorkspaceDir            string   `json:"workspace_dir"`
	JobID                   string   `json:"job_id"`
	FinalVideoPath          string   `json:"final_video_path"`
	FinalVideoExists        bool     `json:"final_video_exists"`
	FinalVideoFileSizeBytes int64    `json:"final_video_file_size_bytes"`
	ClipStatuses            []string `json:"clip_statuses"`
	GeneratedVideoCount     int      `json:"generated_video_count"`
	FallbackImageCount      int      `json:"fallback_image_count"`
	AudioSegmentCount       int      `json:"audio_segment_count"`
	DurationSeconds         float64  `json:"duration_seconds"`
}

type renderVideoClipManifest struct {
	Clips []struct {
		Status string `json:"status"`
	} `json:"clips"`
}

type recordingVideoClient struct {
	inner videopipeline.Client
}

func (c *recordingVideoClient) Generate(
	ctx context.Context,
	request videopipeline.Request,
) (videopipeline.Response, error) {
	return c.inner.Generate(ctx, request)
}

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	exitOnError("load config", err)
	validateRenderSmokeConfig(cfg)

	workspaceDir, jobID := prepareRenderSmokeWorkspace()
	imageClient := buildRenderSmokeImageClient(cfg)
	videoClient := buildRenderSmokeVideoClient(cfg)

	characterTask := runRenderCharacterImage(ctx, imageClient, cfg, workspaceDir, jobID)
	imageTask := runRenderImage(ctx, imageClient, cfg, workspaceDir, jobID, characterTask)
	shotVideoTask := runRenderShotVideo(ctx, videoClient, cfg, workspaceDir, jobID, imageTask)
	ttsTask := runRenderTTS(ctx, workspaceDir, jobID)
	summary := runRenderVideo(ctx, workspaceDir, jobID, ttsTask, shotVideoTask)

	output, err := json.MarshalIndent(summary, "", "  ")
	exitOnError("marshal render smoke summary", err)
	fmt.Println(string(output))
}

func validateRenderSmokeConfig(cfg *config.Config) {
	if strings.TrimSpace(cfg.DashScopeImageAPIKey) == "" {
		exitOnError("validate render smoke config", fmt.Errorf("DASHSCOPE_IMAGE_API_KEY is required"))
	}
	if strings.TrimSpace(cfg.DashScopeVideoAPIKey) == "" {
		exitOnError("validate render smoke config", fmt.Errorf("DASHSCOPE_VIDEO_API_KEY is required"))
	}
}

func prepareRenderSmokeWorkspace() (string, string) {
	tmpRoot := strings.TrimSpace(os.Getenv("SMOKE_TMP_ROOT"))
	if tmpRoot == "" {
		created, err := os.MkdirTemp("/tmp", "narratio-live-video-render-smoke.")
		exitOnError("create temp dir", err)
		tmpRoot = created
	}

	workspaceDir := strings.TrimSpace(os.Getenv("SMOKE_WORKSPACE_DIR"))
	if workspaceDir == "" {
		workspaceDir = filepath.Join(tmpRoot, "workspace")
	}
	exitOnError("create workspace dir", os.MkdirAll(workspaceDir, 0o755))

	jobID := strings.TrimSpace(os.Getenv("SMOKE_JOB_ID"))
	if jobID == "" {
		jobID = defaultVideoRenderSmokeJobID
	}

	return workspaceDir, jobID
}

func buildRenderSmokeImageClient(cfg *config.Config) imagepipeline.Client {
	client, err := imagepipeline.NewHTTPClient(
		cfg.DashScopeImageBaseURL,
		cfg.DashScopeImageAPIKey,
		&http.Client{Timeout: 600 * time.Second},
	)
	exitOnError("build render smoke image client", err)
	return client
}

func buildRenderSmokeVideoClient(cfg *config.Config) videopipeline.Client {
	client, err := videopipeline.NewHTTPClient(
		cfg.DashScopeVideoBaseURL,
		cfg.DashScopeVideoAPIKey,
		videopipeline.GenerationConfig{
			Model:               cfg.DashScopeVideoModel,
			Resolution:          cfg.DashScopeVideoResolution,
			NegativePrompt:      cfg.DashScopeVideoNegativePrompt,
			PollInterval:        time.Duration(cfg.DashScopeVideoPollIntervalSeconds) * time.Second,
			MaxWait:             time.Duration(cfg.DashScopeVideoMaxWaitSeconds) * time.Second,
			MaxRequestBytes:     cfg.DashScopeVideoMaxRequestBytes,
			ImageJPEGQuality:    cfg.DashScopeVideoImageJPEGQuality,
			ImageMinJPEGQuality: cfg.DashScopeVideoImageMinJPEGQuality,
		},
		&http.Client{Timeout: time.Duration(cfg.DashScopeVideoSubmitTimeoutSeconds) * time.Second},
	)
	exitOnError("build render smoke video client", err)
	return &recordingVideoClient{inner: client}
}

func runRenderCharacterImage(
	ctx context.Context,
	client imagepipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
) model.Task {
	path := filepath.Join(workspaceDir, "jobs", jobID, "character_sheet.json")
	exitOnError("write character sheet fixture", writeJSON(path, map[string]any{
		"characters": []map[string]any{
			{
				"name":                   fallbackString(os.Getenv("SMOKE_CHARACTER_NAME"), "林夏"),
				"appearance":             "黑发，浅色风衣，手持黑伞",
				"visual_signature":       "雨夜里黑伞与微湿发梢",
				"reference_subject_type": "人",
				"image_prompt_focus":     "平视角、正面、单人、半身或全身可见，画面干净，关键特征完整露出。",
			},
		},
	}))

	executor := imagepipeline.NewCharacterImageExecutorWithClient(
		client,
		imagepipeline.GenerationConfig{Model: cfg.DashScopeImageModel},
		workspaceDir,
	)
	task, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{Key: "character_image", Type: model.TaskTypeCharacterImage},
		map[string]model.Task{
			"character_sheet": {
				Key: "character_sheet",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "character_sheet.json")),
				},
			},
		},
	)
	exitOnError("execute character_image fixture", err)
	return task
}

func runRenderImage(
	ctx context.Context,
	client imagepipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
	characterTask model.Task,
) model.Task {
	name := fallbackString(os.Getenv("SMOKE_CHARACTER_NAME"), "林夏")
	scriptPath := filepath.Join(workspaceDir, "jobs", jobID, "script.json")
	exitOnError("write script fixture", writeJSON(scriptPath, map[string]any{
		"segments": []map[string]any{
			{
				"index": 0,
				"shots": []map[string]any{
					{
						"index":                 0,
						"involved_characters":   []string{name},
						"image_to_image_prompt": name + "站在暴雨中的旧城巷口，撑着黑伞，缓慢转身望向巷子尽头的书店灯光，中景，平视，固定镜头",
					},
				},
			},
		},
	}))

	executor := imagepipeline.NewExecutorWithClient(
		client,
		imagepipeline.GenerationConfig{Model: cfg.DashScopeImageModel},
		workspaceDir,
	)
	task, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{
			Key:     "image",
			Type:    model.TaskTypeImage,
			Payload: map[string]any{"image_style": fallbackString(os.Getenv("SMOKE_IMAGE_STYLE"), "realistic")},
		},
		map[string]model.Task{
			"script": {
				Key: "script",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "script.json")),
				},
			},
			"character_image": characterTask,
		},
	)
	exitOnError("execute image fixture", err)
	return task
}

func runRenderShotVideo(
	ctx context.Context,
	client videopipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
	imageTask model.Task,
) model.Task {
	executor := videopipeline.NewShotVideoExecutorWithClient(
		client,
		videopipeline.GenerationConfig{
			Model:               cfg.DashScopeVideoModel,
			Resolution:          cfg.DashScopeVideoResolution,
			NegativePrompt:      cfg.DashScopeVideoNegativePrompt,
			PollInterval:        time.Duration(cfg.DashScopeVideoPollIntervalSeconds) * time.Second,
			MaxWait:             time.Duration(cfg.DashScopeVideoMaxWaitSeconds) * time.Second,
			MaxRequestBytes:     cfg.DashScopeVideoMaxRequestBytes,
			ImageJPEGQuality:    cfg.DashScopeVideoImageJPEGQuality,
			ImageMinJPEGQuality: cfg.DashScopeVideoImageMinJPEGQuality,
		},
		workspaceDir,
		float64(cfg.ShotVideoDefaultDurationSeconds),
	)
	task, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{"image": imageTask},
	)
	exitOnError("execute shot_video fixture", err)
	return task
}

func runRenderTTS(
	ctx context.Context,
	workspaceDir string,
	jobID string,
) model.Task {
	segmentationPath := filepath.Join(workspaceDir, "jobs", jobID, "segments.json")
	exitOnError("write segmentation fixture", writeJSON(segmentationPath, map[string]any{
		"segments": []map[string]any{
			{
				"index": 0,
				"text":  fallbackString(os.Getenv("SMOKE_SEGMENT_TEXT"), "暴雨夜里，林夏撑着黑伞站在旧城巷口。她望向书店方向，慢慢向前走去。"),
			},
		},
	}))

	executor := ttspipeline.NewExecutor(workspaceDir)
	task, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{
			Key:     "tts",
			Type:    model.TaskTypeTTS,
			Payload: map[string]any{"voice_id": fallbackString(os.Getenv("SMOKE_VOICE_ID"), "default")},
		},
		map[string]model.Task{
			"segmentation": {
				Key: "segmentation",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "segments.json")),
				},
			},
		},
	)
	exitOnError("execute tts fixture", err)
	return task
}

func runRenderVideo(
	ctx context.Context,
	workspaceDir string,
	jobID string,
	ttsTask model.Task,
	shotVideoTask model.Task,
) videoRenderSummary {
	executor := videopipeline.NewRealExecutor(workspaceDir)
	task, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{Key: "video", Type: model.TaskTypeVideo},
		map[string]model.Task{
			"tts":        ttsTask,
			"shot_video": shotVideoTask,
		},
	)
	exitOnError("execute video render fixture", err)

	manifestPath := filepath.Join(workspaceDir, "jobs", jobID, "shot_videos", "manifest.json")
	var manifest renderVideoClipManifest
	exitOnError("read shot video manifest", readJSON(manifestPath, &manifest))

	finalVideoPath := filepath.Join(
		workspaceDir,
		filepath.Clean(outputString(task.OutputRef, "artifact_path")),
	)
	info, err := os.Stat(finalVideoPath)
	exitOnError("stat final video", err)

	summary := videoRenderSummary{
		WorkspaceDir:            workspaceDir,
		JobID:                   jobID,
		FinalVideoPath:          finalVideoPath,
		FinalVideoExists:        true,
		FinalVideoFileSizeBytes: info.Size(),
		GeneratedVideoCount:     countStatus(manifest.Clips, "generated_video"),
		FallbackImageCount:      countStatus(manifest.Clips, "image_fallback"),
		AudioSegmentCount:       len(outputStringSlice(ttsTask.OutputRef, "audio_segment_paths")),
		DurationSeconds:         outputFloat(task.OutputRef, "duration_seconds"),
	}
	for _, clip := range manifest.Clips {
		summary.ClipStatuses = append(summary.ClipStatuses, clip.Status)
	}

	if summary.FinalVideoFileSizeBytes <= 0 {
		exitOnError("verify render smoke", fmt.Errorf("final video file is empty"))
	}

	return summary
}

func countStatus(
	clips []struct {
		Status string `json:"status"`
	},
	target string,
) int {
	count := 0
	for _, clip := range clips {
		if clip.Status == target {
			count++
		}
	}
	return count
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func outputString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func outputStringSlice(values map[string]any, key string) []string {
	raw, ok := values[key]
	if !ok {
		return nil
	}
	items, ok := raw.([]string)
	if ok {
		return items
	}
	valuesAny, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(valuesAny))
	for _, item := range valuesAny {
		value, ok := item.(string)
		if ok && strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func outputFloat(values map[string]any, key string) float64 {
	raw, ok := values[key]
	if !ok {
		return 0
	}
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func fallbackString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func exitOnError(step string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
	os.Exit(1)
}
