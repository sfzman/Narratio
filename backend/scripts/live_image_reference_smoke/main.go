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
)

const (
	defaultSmokeJobID = "job_live_image_reference_smoke"
)

type recordingClient struct {
	inner    imagepipeline.Client
	requests []imagepipeline.Request
}

func (c *recordingClient) Generate(
	ctx context.Context,
	request imagepipeline.Request,
) (imagepipeline.Response, error) {
	c.requests = append(c.requests, request)
	return c.inner.Generate(ctx, request)
}

type smokeSummary struct {
	WorkspaceDir                  string `json:"workspace_dir"`
	JobID                         string `json:"job_id"`
	CharacterImageManifestPath    string `json:"character_image_manifest_path"`
	ImageManifestPath             string `json:"image_manifest_path"`
	CharacterImageCount           int    `json:"character_image_count"`
	GeneratedCharacterImageCount  int    `json:"generated_character_image_count"`
	ShotImageCount                int    `json:"shot_image_count"`
	ImageToImageShotCount         int    `json:"image_to_image_shot_count"`
	MatchedCharacterShotCount     int    `json:"matched_character_shot_count"`
	RequestsWithReferenceImages   int    `json:"requests_with_reference_images"`
	FirstReferenceImageCount      int    `json:"first_reference_image_count,omitempty"`
	FirstReferencePrompt          string `json:"first_reference_prompt,omitempty"`
	FirstReferenceUsesPlaceholder bool   `json:"first_reference_uses_placeholder"`
	FirstReferenceImagePrefix     string `json:"first_reference_image_prefix,omitempty"`
}

type imageManifest struct {
	Images     []map[string]any `json:"images"`
	ShotImages []struct {
		Prompt            string           `json:"prompt"`
		PromptType        string           `json:"prompt_type"`
		MatchedCharacters []map[string]any `json:"matched_characters"`
	} `json:"shot_images"`
}

type characterManifest struct {
	Images []struct {
		IsFallback bool   `json:"is_fallback"`
		FilePath   string `json:"file_path"`
	} `json:"images"`
}

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	exitOnError("load config", err)
	validateImageConfig(cfg)

	workspaceDir, jobID := prepareSmokeWorkspace()
	client := buildRealImageClient(cfg)
	characterTask := runCharacterImageSmoke(ctx, client, cfg, workspaceDir, jobID)
	summary := runImageReferenceSmoke(ctx, client, cfg, workspaceDir, jobID, characterTask)

	output, err := json.MarshalIndent(summary, "", "  ")
	exitOnError("marshal summary", err)
	fmt.Println(string(output))
}

func validateImageConfig(cfg *config.Config) {
	if strings.TrimSpace(cfg.DashScopeImageAPIKey) == "" {
		exitOnError("validate image config", fmt.Errorf("DASHSCOPE_IMAGE_API_KEY is required"))
	}
	if strings.TrimSpace(cfg.DashScopeImageBaseURL) == "" {
		exitOnError("validate image config", fmt.Errorf("DASHSCOPE_IMAGE_BASE_URL is required"))
	}
	if strings.TrimSpace(cfg.DashScopeImageModel) == "" {
		exitOnError("validate image config", fmt.Errorf("DASHSCOPE_IMAGE_MODEL is required"))
	}
}

func prepareSmokeWorkspace() (string, string) {
	tmpRoot := strings.TrimSpace(os.Getenv("SMOKE_TMP_ROOT"))
	if tmpRoot == "" {
		created, err := os.MkdirTemp("/tmp", "narratio-live-image-reference-smoke.")
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
		jobID = defaultSmokeJobID
	}

	return workspaceDir, jobID
}

func buildRealImageClient(cfg *config.Config) imagepipeline.Client {
	client, err := imagepipeline.NewHTTPClient(
		cfg.DashScopeImageBaseURL,
		cfg.DashScopeImageAPIKey,
		&http.Client{Timeout: 600 * time.Second},
	)
	exitOnError("build image client", err)
	return client
}

func runCharacterImageSmoke(
	ctx context.Context,
	client imagepipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
) model.Task {
	path := filepath.Join(workspaceDir, "jobs", jobID, "character_sheet.json")
	exitOnError("write character sheet fixture", writeJSON(path, characterSheetFixture()))

	executor := imagepipeline.NewCharacterImageExecutorWithClient(
		client,
		imagepipeline.GenerationConfig{Model: cfg.DashScopeImageModel},
		workspaceDir,
	)
	job := model.Job{PublicID: jobID}
	task := model.Task{Key: "character_image", Type: model.TaskTypeCharacterImage}
	deps := map[string]model.Task{
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "character_sheet.json")),
			},
		},
	}

	updated, err := executor.Execute(ctx, job, task, deps)
	exitOnError("execute character_image smoke", err)
	return updated
}

func runImageReferenceSmoke(
	ctx context.Context,
	client imagepipeline.Client,
	cfg *config.Config,
	workspaceDir string,
	jobID string,
	characterTask model.Task,
) smokeSummary {
	scriptPath := filepath.Join(workspaceDir, "jobs", jobID, "script.json")
	exitOnError("write script fixture", writeJSON(scriptPath, scriptFixture()))

	recording := &recordingClient{inner: client}
	executor := imagepipeline.NewExecutorWithClient(
		recording,
		imagepipeline.GenerationConfig{Model: cfg.DashScopeImageModel},
		workspaceDir,
	)
	job := model.Job{PublicID: jobID}
	task := model.Task{
		Key:     "image",
		Type:    model.TaskTypeImage,
		Payload: map[string]any{"image_style": fallbackString(os.Getenv("SMOKE_IMAGE_STYLE"), "realistic")},
	}
	deps := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "script.json")),
			},
		},
		"character_image": characterTask,
	}

	updated, err := executor.Execute(ctx, job, task, deps)
	exitOnError("execute image smoke", err)
	return buildSmokeSummary(workspaceDir, jobID, updated, recording)
}

func buildSmokeSummary(
	workspaceDir string,
	jobID string,
	imageTask model.Task,
	recording *recordingClient,
) smokeSummary {
	characterManifestPath := filepath.Join(workspaceDir, "jobs", jobID, "character_images", "manifest.json")
	imageManifestPath := filepath.Join(workspaceDir, "jobs", jobID, "images", "image_manifest.json")

	characterManifest := readCharacterManifest(characterManifestPath)
	imageManifest := readImageManifest(imageManifestPath)

	summary := smokeSummary{
		WorkspaceDir:                 workspaceDir,
		JobID:                        jobID,
		CharacterImageManifestPath:   characterManifestPath,
		ImageManifestPath:            imageManifestPath,
		CharacterImageCount:          len(characterManifest.Images),
		GeneratedCharacterImageCount: countGeneratedCharacterImages(characterManifest),
		ShotImageCount:               len(imageManifest.ShotImages),
		ImageToImageShotCount:        countPromptType(imageManifest, "image_to_image"),
		MatchedCharacterShotCount:    countMatchedCharacterShots(imageManifest),
		RequestsWithReferenceImages:  countReferenceRequests(recording.requests),
	}
	fillFirstReferenceRequest(&summary, recording.requests)
	verifySmokeSummary(summary, imageTask)
	return summary
}

func readCharacterManifest(path string) characterManifest {
	var manifest characterManifest
	exitOnError("read character manifest", readJSON(path, &manifest))
	return manifest
}

func readImageManifest(path string) imageManifest {
	var manifest imageManifest
	exitOnError("read image manifest", readJSON(path, &manifest))
	return manifest
}

func countGeneratedCharacterImages(manifest characterManifest) int {
	count := 0
	for _, item := range manifest.Images {
		if !item.IsFallback {
			count++
		}
	}
	return count
}

func countPromptType(manifest imageManifest, target string) int {
	count := 0
	for _, shot := range manifest.ShotImages {
		if shot.PromptType == target {
			count++
		}
	}
	return count
}

func countMatchedCharacterShots(manifest imageManifest) int {
	count := 0
	for _, shot := range manifest.ShotImages {
		if len(shot.MatchedCharacters) > 0 {
			count++
		}
	}
	return count
}

func countReferenceRequests(requests []imagepipeline.Request) int {
	count := 0
	for _, request := range requests {
		if len(request.ReferenceImages) > 0 {
			count++
		}
	}
	return count
}

func fillFirstReferenceRequest(summary *smokeSummary, requests []imagepipeline.Request) {
	for _, request := range requests {
		if len(request.ReferenceImages) == 0 {
			continue
		}
		summary.FirstReferenceImageCount = len(request.ReferenceImages)
		summary.FirstReferencePrompt = request.Prompt
		summary.FirstReferenceUsesPlaceholder = strings.Contains(request.Prompt, "图1中的人物")
		summary.FirstReferenceImagePrefix = firstReferencePrefix(request.ReferenceImages[0])
		return
	}
}

func firstReferencePrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(trimmed, "data:image/"):
		return "data_url"
	case strings.HasPrefix(trimmed, "http://"), strings.HasPrefix(trimmed, "https://"):
		return "remote_url"
	default:
		return "unknown"
	}
}

func verifySmokeSummary(summary smokeSummary, imageTask model.Task) {
	if summary.GeneratedCharacterImageCount == 0 {
		exitOnError("verify smoke", fmt.Errorf("character_image did not generate a live reference image"))
	}
	if summary.ImageToImageShotCount == 0 {
		exitOnError("verify smoke", fmt.Errorf("script fixture did not produce any image_to_image shot"))
	}
	if summary.MatchedCharacterShotCount == 0 {
		exitOnError("verify smoke", fmt.Errorf("image manifest did not record any matched_characters"))
	}
	if summary.RequestsWithReferenceImages == 0 {
		exitOnError("verify smoke", fmt.Errorf("no live image request carried reference_images"))
	}
	if !summary.FirstReferenceUsesPlaceholder {
		exitOnError("verify smoke", fmt.Errorf("reference image prompt did not replace character name with placeholder"))
	}
	if imageTask.OutputRef["generated_image_count"] == nil {
		exitOnError("verify smoke", fmt.Errorf("image output_ref missing generated_image_count"))
	}
}

func characterSheetFixture() map[string]any {
	return map[string]any{
		"characters": []map[string]any{
			{
				"name":                   fallbackString(os.Getenv("SMOKE_CHARACTER_NAME"), "林夏"),
				"appearance":             "黑发，浅色风衣，手持黑伞",
				"visual_signature":       "雨夜里黑伞与微湿发梢",
				"reference_subject_type": "人",
				"image_prompt_focus":     "平视角、正面、单人、半身或全身可见，画面干净，关键特征完整露出。",
			},
		},
	}
}

func scriptFixture() map[string]any {
	name := fallbackString(os.Getenv("SMOKE_CHARACTER_NAME"), "林夏")
	return map[string]any{
		"segments": []map[string]any{
			{
				"index": 0,
				"shots": []map[string]any{
					{
						"index":                 0,
						"involved_characters":   []string{name},
						"image_to_image_prompt": name + "站在暴雨中的旧城巷口，撑着黑伞，侧身回望，暖黄路灯映在湿漉漉的青石板上，中景，平视，固定镜头",
					},
					{
						"index":                1,
						"text_to_image_prompt": "雨夜旧城巷口的空镜，潮湿青石板路面反光，巷子尽头透出暖黄书店灯光，中景，平视，固定镜头",
					},
				},
			},
		},
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

func exitOnError(step string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
	os.Exit(1)
}
