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
	videopipeline "github.com/sfzman/Narratio/backend/internal/pipeline/video"
)

const defaultShotVideoSmokeJobID = "job_live_shot_video_smoke"

type shotVideoSmokeSummary struct {
	WorkspaceDir             string   `json:"workspace_dir"`
	JobID                    string   `json:"job_id"`
	CharacterImageCount      int      `json:"character_image_count"`
	ShotImageCount           int      `json:"shot_image_count"`
	ClipCount                int      `json:"clip_count"`
	GeneratedVideoCount      int      `json:"generated_video_count"`
	FallbackImageCount       int      `json:"fallback_image_count"`
	GenerationMode           string   `json:"generation_mode"`
	Statuses                 []string `json:"statuses"`
	ManifestPath             string   `json:"manifest_path"`
	FirstGeneratedVideoPath  string   `json:"first_generated_video_path,omitempty"`
	FirstGeneratedVideoURL   string   `json:"first_generated_video_url,omitempty"`
	FirstFallbackImagePath   string   `json:"first_fallback_image_path,omitempty"`
	AllMediaFilesPresent     bool     `json:"all_media_files_present"`
	RequestsWithRemoteSource int      `json:"requests_with_remote_source"`
}

type recordingVideoClient struct {
	inner    videopipeline.Client
	requests []videopipeline.Request
}

func (c *recordingVideoClient) Generate(
	ctx context.Context,
	request videopipeline.Request,
) (videopipeline.Response, error) {
	c.requests = append(c.requests, request)
	return c.inner.Generate(ctx, request)
}

type shotVideoManifest struct {
	Clips []struct {
		Status          string  `json:"status"`
		VideoPath       string  `json:"video_path,omitempty"`
		ImagePath       string  `json:"image_path,omitempty"`
		SourceImagePath string  `json:"source_image_path"`
		SourceType      string  `json:"source_type"`
		DurationSeconds float64 `json:"duration_seconds"`
		SourceVideoURL  string  `json:"source_video_url,omitempty"`
	} `json:"clips"`
}

type simpleCharacterManifest struct {
	Images []struct {
		IsFallback bool `json:"is_fallback"`
	} `json:"images"`
}

type simpleImageManifest struct {
	ShotImages []struct {
		SourceImageURL string `json:"source_image_url,omitempty"`
	} `json:"shot_images"`
}

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	exitOnError("load config", err)
	validateRequiredVideoConfig(cfg)

	workspaceDir, jobID := prepareShotVideoSmokeWorkspace()
	imageClient := buildShotVideoSmokeImageClient(cfg)
	videoClient := buildShotVideoSmokeVideoClient(cfg)

	characterTask := runShotVideoCharacterImageFixture(ctx, imageClient, cfg, workspaceDir, jobID)
	imageTask := runShotVideoImageFixture(ctx, imageClient, cfg, workspaceDir, jobID, characterTask)
	summary := runShotVideoFixture(ctx, videoClient, cfg, workspaceDir, jobID, imageTask)

	output, err := json.MarshalIndent(summary, "", "  ")
	exitOnError("marshal shot video smoke summary", err)
	fmt.Println(string(output))
}

func validateRequiredVideoConfig(cfg *config.Config) {
	if strings.TrimSpace(cfg.DashScopeImageAPIKey) == "" {
		exitOnError("validate image config", fmt.Errorf("DASHSCOPE_IMAGE_API_KEY is required"))
	}
	if strings.TrimSpace(cfg.DashScopeVideoAPIKey) == "" {
		exitOnError("validate video config", fmt.Errorf("DASHSCOPE_VIDEO_API_KEY is required"))
	}
}

func prepareShotVideoSmokeWorkspace() (string, string) {
	tmpRoot := strings.TrimSpace(os.Getenv("SMOKE_TMP_ROOT"))
	if tmpRoot == "" {
		created, err := os.MkdirTemp("/tmp", "narratio-live-shot-video-smoke.")
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
		jobID = defaultShotVideoSmokeJobID
	}

	return workspaceDir, jobID
}

func buildShotVideoSmokeImageClient(cfg *config.Config) imagepipeline.Client {
	client, err := imagepipeline.NewHTTPClient(
		cfg.DashScopeImageBaseURL,
		cfg.DashScopeImageAPIKey,
		&http.Client{Timeout: 600 * time.Second},
	)
	exitOnError("build image client", err)
	return client
}

func buildShotVideoSmokeVideoClient(cfg *config.Config) videopipeline.Client {
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
	exitOnError("build video client", err)
	return &recordingVideoClient{inner: client}
}

func runShotVideoCharacterImageFixture(
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
	updated, err := executor.Execute(
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
	return updated
}

func runShotVideoImageFixture(
	ctx context.Context,
	client imagepipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
	characterTask model.Task,
) model.Task {
	scriptPath := filepath.Join(workspaceDir, "jobs", jobID, "script.json")
	name := fallbackString(os.Getenv("SMOKE_CHARACTER_NAME"), "林夏")
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
	updated, err := executor.Execute(
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
	return updated
}

func runShotVideoFixture(
	ctx context.Context,
	videoClient videopipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
	imageTask model.Task,
) shotVideoSmokeSummary {
	executor := videopipeline.NewShotVideoExecutorWithClient(
		videoClient,
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

	updated, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{
			"image": imageTask,
		},
	)
	exitOnError("execute shot_video fixture", err)

	recording, ok := videoClient.(*recordingVideoClient)
	if !ok {
		exitOnError("build shot video summary", fmt.Errorf("recording video client is not available"))
	}
	return buildShotVideoSmokeSummary(workspaceDir, jobID, updated, recording.requests)
}

func buildShotVideoSmokeSummary(
	workspaceDir string,
	jobID string,
	shotVideoTask model.Task,
	requests []videopipeline.Request,
) shotVideoSmokeSummary {
	characterManifestPath := filepath.Join(workspaceDir, "jobs", jobID, "character_images", "manifest.json")
	imageManifestPath := filepath.Join(workspaceDir, "jobs", jobID, "images", "image_manifest.json")
	shotVideoManifestPath := filepath.Join(workspaceDir, "jobs", jobID, "shot_videos", "manifest.json")

	var characterManifest simpleCharacterManifest
	var imageManifest simpleImageManifest
	var manifest shotVideoManifest
	exitOnError("read character manifest", readJSON(characterManifestPath, &characterManifest))
	exitOnError("read image manifest", readJSON(imageManifestPath, &imageManifest))
	exitOnError("read shot video manifest", readJSON(shotVideoManifestPath, &manifest))

	summary := shotVideoSmokeSummary{
		WorkspaceDir:             workspaceDir,
		JobID:                    jobID,
		CharacterImageCount:      len(characterManifest.Images),
		ShotImageCount:           len(imageManifest.ShotImages),
		ClipCount:                len(manifest.Clips),
		GeneratedVideoCount:      countGeneratedVideoClips(manifest.Clips),
		FallbackImageCount:       countFallbackVideoClips(manifest.Clips),
		GenerationMode:           outputString(shotVideoTask.OutputRef, "generation_mode"),
		ManifestPath:             shotVideoManifestPath,
		AllMediaFilesPresent:     true,
		RequestsWithRemoteSource: countRequestsWithRemoteSource(requests),
	}
	for _, clip := range manifest.Clips {
		summary.Statuses = append(summary.Statuses, clip.Status)
		if clip.Status == "generated_video" && summary.FirstGeneratedVideoPath == "" {
			summary.FirstGeneratedVideoPath = filepath.Join(workspaceDir, filepath.Clean(clip.VideoPath))
			summary.FirstGeneratedVideoURL = clip.SourceVideoURL
		}
		if clip.Status == "image_fallback" && summary.FirstFallbackImagePath == "" {
			summary.FirstFallbackImagePath = filepath.Join(workspaceDir, filepath.Clean(clip.ImagePath))
		}
		if err := ensureClipMediaExists(workspaceDir, clip); err != nil {
			summary.AllMediaFilesPresent = false
			exitOnError("verify shot video media", err)
		}
	}

	verifyShotVideoSummary(summary)
	return summary
}

func countGeneratedVideoClips(clips []struct {
	Status          string  `json:"status"`
	VideoPath       string  `json:"video_path,omitempty"`
	ImagePath       string  `json:"image_path,omitempty"`
	SourceImagePath string  `json:"source_image_path"`
	SourceType      string  `json:"source_type"`
	DurationSeconds float64 `json:"duration_seconds"`
	SourceVideoURL  string  `json:"source_video_url,omitempty"`
}) int {
	count := 0
	for _, clip := range clips {
		if clip.Status == "generated_video" {
			count++
		}
	}
	return count
}

func countFallbackVideoClips(clips []struct {
	Status          string  `json:"status"`
	VideoPath       string  `json:"video_path,omitempty"`
	ImagePath       string  `json:"image_path,omitempty"`
	SourceImagePath string  `json:"source_image_path"`
	SourceType      string  `json:"source_type"`
	DurationSeconds float64 `json:"duration_seconds"`
	SourceVideoURL  string  `json:"source_video_url,omitempty"`
}) int {
	count := 0
	for _, clip := range clips {
		if clip.Status == "image_fallback" {
			count++
		}
	}
	return count
}

func countRequestsWithRemoteSource(requests []videopipeline.Request) int {
	count := 0
	for _, request := range requests {
		if strings.TrimSpace(request.SourceImageURL) != "" {
			count++
		}
	}
	return count
}

func ensureClipMediaExists(
	workspaceDir string,
	clip struct {
		Status          string  `json:"status"`
		VideoPath       string  `json:"video_path,omitempty"`
		ImagePath       string  `json:"image_path,omitempty"`
		SourceImagePath string  `json:"source_image_path"`
		SourceType      string  `json:"source_type"`
		DurationSeconds float64 `json:"duration_seconds"`
		SourceVideoURL  string  `json:"source_video_url,omitempty"`
	},
) error {
	if strings.TrimSpace(clip.SourceImagePath) == "" {
		return fmt.Errorf("shot video clip missing source_image_path")
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, filepath.Clean(clip.SourceImagePath))); err != nil {
		return fmt.Errorf("missing source image file: %w", err)
	}
	if clip.VideoPath != "" {
		if _, err := os.Stat(filepath.Join(workspaceDir, filepath.Clean(clip.VideoPath))); err != nil {
			return fmt.Errorf("missing generated video file: %w", err)
		}
	}
	if clip.ImagePath != "" {
		if _, err := os.Stat(filepath.Join(workspaceDir, filepath.Clean(clip.ImagePath))); err != nil {
			return fmt.Errorf("missing fallback image file: %w", err)
		}
	}
	return nil
}

func verifyShotVideoSummary(summary shotVideoSmokeSummary) {
	if summary.CharacterImageCount == 0 {
		exitOnError("verify shot video smoke", fmt.Errorf("character_image fixture did not produce any reference image"))
	}
	if summary.ShotImageCount == 0 {
		exitOnError("verify shot video smoke", fmt.Errorf("image fixture did not produce any shot image"))
	}
	if summary.ClipCount == 0 {
		exitOnError("verify shot video smoke", fmt.Errorf("shot_video did not produce any clip"))
	}
	if summary.GeneratedVideoCount == 0 {
		exitOnError("verify shot video smoke", fmt.Errorf("shot_video did not produce any generated_video clip"))
	}
	if summary.FirstGeneratedVideoPath == "" {
		exitOnError("verify shot video smoke", fmt.Errorf("generated_video clip missing video_path"))
	}
	if !summary.AllMediaFilesPresent {
		exitOnError("verify shot video smoke", fmt.Errorf("some shot video media files are missing"))
	}
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

func fallbackString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
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

func exitOnError(step string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
	os.Exit(1)
}
