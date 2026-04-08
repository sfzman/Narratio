package image

import (
	"context"
	"encoding/json"
	"image/jpeg"
	"os"
	"strings"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type fakeClient struct {
	requests []Request
	response Response
	err      error
}

func (f *fakeClient) Generate(_ context.Context, request Request) (Response, error) {
	f.requests = append(f.requests, request)
	if f.err != nil {
		return Response{}, f.err
	}

	return f.response, nil
}

func TestExecuteBuildsImageOutputRef(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_image_123",
	}
	task := model.Task{
		ID:      21,
		Key:     "image",
		Payload: map[string]any{"image_style": "cinematic"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_123/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_123/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_123/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{
				Index:   0,
				Text:    "Lin Qing steps onto the bridge",
				Script:  "Lin Qing walks through the rain",
				Summary: "legacy bridge summary",
				Shots: []scriptArtifactShot{
					{Index: 0, Prompt: "Lin Qing stands on the bridge in the night rain"},
					{Index: 1, Prompt: "Lantern light reflects on the wet stone path"},
				},
			},
			{
				Index:   1,
				Text:    "scene text 2",
				Script:  "voice over 2",
				Summary: "legacy inn summary",
				Shots: []scriptArtifactShot{
					{Index: 0, Prompt: "A weary traveler arrives at the inn gate at dusk"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_123/character_images/manifest.json", CharacterImageOutput{
		Images: []CharacterReferenceImage{
			{
				CharacterIndex: 0,
				CharacterName:  "Lin Qing / A-Qing",
				FilePath:       "jobs/job_image_123/character_images/character_000.jpg",
				Prompt:         "Lin Qing, white robe, jade pendant, front view",
				MatchTerms:     []string{"Lin Qing", "A-Qing"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["artifact_type"] != "image" {
		t.Fatalf("artifact_type = %#v, want %q", updated.OutputRef["artifact_type"], "image")
	}
	if updated.OutputRef["image_style"] != "cinematic" {
		t.Fatalf("image_style = %#v, want %q", updated.OutputRef["image_style"], "cinematic")
	}
	if updated.OutputRef["script_artifact_ref"] != "jobs/job_image_123/script.json" {
		t.Fatalf("script_artifact_ref = %#v", updated.OutputRef["script_artifact_ref"])
	}
	if updated.OutputRef["character_image_artifact_ref"] != "jobs/job_image_123/character_images/manifest.json" {
		t.Fatalf("character_image_artifact_ref = %#v", updated.OutputRef["character_image_artifact_ref"])
	}
	if updated.OutputRef["image_count"] != 2 {
		t.Fatalf("image_count = %#v, want 2", updated.OutputRef["image_count"])
	}
	if updated.OutputRef["generated_image_count"] != 0 {
		t.Fatalf("generated_image_count = %#v, want 0", updated.OutputRef["generated_image_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 2 {
		t.Fatalf("fallback_image_count = %#v, want 2", updated.OutputRef["fallback_image_count"])
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_123/images/image_manifest.json")
	if len(artifact.Images) != 2 {
		t.Fatalf("len(artifact.Images) = %d, want 2", len(artifact.Images))
	}
	assertJPEGDimensions(t, artifactFullPath(workspaceDir, artifact.Images[0].FilePath), defaultImageWidth, defaultImageHeight)
	assertJPEGDimensions(t, artifactFullPath(workspaceDir, artifact.Images[1].FilePath), defaultImageWidth, defaultImageHeight)
	if artifact.Images[0].Prompt == "" {
		t.Fatal("artifact.Images[0].Prompt = empty, want non-empty")
	}
	if artifact.Images[0].PromptSourceType != "shots" {
		t.Fatalf("artifact.Images[0].PromptSourceType = %q, want %q", artifact.Images[0].PromptSourceType, "shots")
	}
	if len(artifact.Images[0].PromptSourceShots) != 2 {
		t.Fatalf("len(artifact.Images[0].PromptSourceShots) = %d, want 2", len(artifact.Images[0].PromptSourceShots))
	}
	if artifact.Images[0].PromptSourceShots[0] != "Lin Qing stands on the bridge in the night rain" {
		t.Fatalf("artifact.Images[0].PromptSourceShots[0] = %q", artifact.Images[0].PromptSourceShots[0])
	}
	if len(artifact.Images[0].CharacterReferences) != 1 {
		t.Fatalf("len(artifact.Images[0].CharacterReferences) = %d, want 1", len(artifact.Images[0].CharacterReferences))
	}
	if len(artifact.Images[0].CharacterReferences[0].MatchTerms) != 2 {
		t.Fatalf("len(artifact.Images[0].CharacterReferences[0].MatchTerms) = %d, want 2", len(artifact.Images[0].CharacterReferences[0].MatchTerms))
	}
	if artifact.Images[0].CharacterReferences[0].Prompt == "" {
		t.Fatal("artifact.Images[0].CharacterReferences[0].Prompt = empty, want non-empty")
	}
	if len(artifact.Images[0].MatchedCharacters) != 1 {
		t.Fatalf("len(artifact.Images[0].MatchedCharacters) = %d, want 1", len(artifact.Images[0].MatchedCharacters))
	}
	if artifact.Images[0].MatchedCharacters[0].CharacterName != "Lin Qing / A-Qing" {
		t.Fatalf("artifact.Images[0].MatchedCharacters[0].CharacterName = %q, want %q", artifact.Images[0].MatchedCharacters[0].CharacterName, "Lin Qing / A-Qing")
	}
	if len(artifact.Images[1].MatchedCharacters) != 0 {
		t.Fatalf("len(artifact.Images[1].MatchedCharacters) = %d, want 0", len(artifact.Images[1].MatchedCharacters))
	}
	if got := artifact.Images[0].Prompt; !containsAll(got, []string{"matched characters: Lin Qing / A-Qing", "character reference details: Lin Qing / A-Qing: Lin Qing, white robe, jade pendant, front view", "style: cinematic"}) {
		t.Fatalf("artifact.Images[0].Prompt = %q, want matched character prompt", got)
	}
	if strings.Contains(artifact.Images[0].Prompt, "legacy bridge summary") {
		t.Fatalf("artifact.Images[0].Prompt = %q, want shots to win over summary", artifact.Images[0].Prompt)
	}
	if !strings.Contains(artifact.Images[0].PromptSourceText, "Lantern light reflects on the wet stone path") {
		t.Fatalf("artifact.Images[0].PromptSourceText = %q, want joined shot prompts", artifact.Images[0].PromptSourceText)
	}
	if got := artifact.Images[1].Prompt; !containsAll(got, []string{"candidate characters: Lin Qing / A-Qing", "character reference details: Lin Qing / A-Qing: Lin Qing, white robe, jade pendant, front view", "A weary traveler arrives at the inn gate at dusk"}) {
		t.Fatalf("artifact.Images[1].Prompt = %q, want fallback candidate prompt", got)
	}
	if strings.Contains(artifact.Images[1].Prompt, "legacy inn summary") {
		t.Fatalf("artifact.Images[1].Prompt = %q, want shots to win over summary", artifact.Images[1].Prompt)
	}
	if artifact.Images[1].PromptSourceType != "shots" {
		t.Fatalf("artifact.Images[1].PromptSourceType = %q, want %q", artifact.Images[1].PromptSourceType, "shots")
	}
}

func TestExecuteWritesFallbackImageWhenLiveGenerationFails(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeClient{err: assertiveError("upstream failed")}
	executor := NewExecutorWithClient(client, GenerationConfig{
		Model: "qwen-image-2.0",
	}, workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_image_live_fallback_123",
	}
	task := model.Task{
		ID:      24,
		Key:     "image",
		Payload: map[string]any{"image_style": "cinematic"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_live_fallback_123/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_live_fallback_123/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_live_fallback_123/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{Index: 0, Text: "Lin Qing waits by the river", Script: "Lin Qing stays still", Summary: "Lin Qing by the river"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_live_fallback_123/character_images/manifest.json", CharacterImageOutput{
		Images: []CharacterReferenceImage{
			{
				CharacterIndex: 0,
				CharacterName:  "Lin Qing",
				FilePath:       "jobs/job_image_live_fallback_123/character_images/character_000.jpg",
				Prompt:         "Lin Qing, white robe, front view",
				MatchTerms:     []string{"Lin Qing"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["generated_image_count"] != 0 {
		t.Fatalf("generated_image_count = %#v, want 0", updated.OutputRef["generated_image_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 1 {
		t.Fatalf("fallback_image_count = %#v, want 1", updated.OutputRef["fallback_image_count"])
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_live_fallback_123/images/image_manifest.json")
	if !artifact.Images[0].IsFallback {
		t.Fatal("artifact.Images[0].IsFallback = false, want true")
	}
	assertJPEGDimensions(t, artifactFullPath(workspaceDir, artifact.Images[0].FilePath), defaultImageWidth, defaultImageHeight)
}

func TestExecuteMatchesCharacterByAliasTerm(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_image_alias_123",
	}
	task := model.Task{
		ID:      22,
		Key:     "image",
		Payload: map[string]any{"image_style": "cinematic"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_alias_123/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_alias_123/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_alias_123/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{
				Index:   0,
				Text:    "A young swordswoman enters the courtyard",
				Script:  "She keeps silent and studies the room",
				Summary: "A lone traveler in the courtyard",
				Shots: []scriptArtifactShot{
					{Index: 0, Prompt: "A-Qing enters the courtyard under drifting petals"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_alias_123/character_images/manifest.json", CharacterImageOutput{
		Images: []CharacterReferenceImage{
			{
				CharacterIndex: 0,
				CharacterName:  "Lin Qing",
				FilePath:       "jobs/job_image_alias_123/character_images/character_000.jpg",
				Prompt:         "Lin Qing, white robe, front view",
				MatchTerms:     []string{"Lin Qing", "A-Qing"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["image_count"] != 1 {
		t.Fatalf("image_count = %#v, want 1", updated.OutputRef["image_count"])
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_alias_123/images/image_manifest.json")
	if len(artifact.Images[0].MatchedCharacters) != 1 {
		t.Fatalf("len(artifact.Images[0].MatchedCharacters) = %d, want 1", len(artifact.Images[0].MatchedCharacters))
	}
	if artifact.Images[0].MatchedCharacters[0].CharacterName != "Lin Qing" {
		t.Fatalf("artifact.Images[0].MatchedCharacters[0].CharacterName = %q, want %q", artifact.Images[0].MatchedCharacters[0].CharacterName, "Lin Qing")
	}
	if artifact.Images[0].PromptSourceType != "shots" {
		t.Fatalf("artifact.Images[0].PromptSourceType = %q, want %q", artifact.Images[0].PromptSourceType, "shots")
	}
}

func TestBuildSegmentImagePromptFallsBackToSummaryWhenShotsMissing(t *testing.T) {
	t.Parallel()

	source := resolveSegmentPromptSource(
		scriptArtifactSegment{
			Index:   0,
			Text:    "source text",
			Script:  "narration text",
			Summary: "summary fallback prompt",
		},
	)
	if source.Type != "summary" {
		t.Fatalf("source.Type = %q, want %q", source.Type, "summary")
	}

	prompt := buildSegmentImagePrompt(
		source.Text,
		"cinematic",
		nil,
		false,
	)
	if !strings.Contains(prompt, "summary fallback prompt") {
		t.Fatalf("prompt = %q, want summary fallback", prompt)
	}
}

func TestExecuteWritesLiveImageWhenClientInjected(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeClient{
		response: Response{
			RequestID: "req_img_1",
			Model:     "qwen-image-2.0",
			ImageURL:  "https://example.com/generated.jpg",
			ImageData: []byte("fake-image-bytes"),
		},
	}
	executor := NewExecutorWithClient(client, GenerationConfig{
		Model: "qwen-image-2.0",
	}, workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_image_live_123",
	}
	task := model.Task{
		ID:      23,
		Key:     "image",
		Payload: map[string]any{"image_style": "cinematic"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_live_123/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_live_123/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_live_123/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{Index: 0, Text: "Lin Qing waits by the river", Script: "Lin Qing stays still", Summary: "Lin Qing by the river"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_live_123/character_images/manifest.json", CharacterImageOutput{
		Images: []CharacterReferenceImage{
			{
				CharacterIndex: 0,
				CharacterName:  "Lin Qing",
				FilePath:       "jobs/job_image_live_123/character_images/character_000.jpg",
				Prompt:         "Lin Qing, white robe, front view",
				MatchTerms:     []string{"Lin Qing"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("len(client.requests) = %d, want 1", len(client.requests))
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_live_123/images/image_manifest.json")
	if artifact.Images[0].IsFallback {
		t.Fatal("artifact.Images[0].IsFallback = true, want false")
	}
	if artifact.Images[0].GenerationRequestID != "req_img_1" {
		t.Fatalf("artifact.Images[0].GenerationRequestID = %q, want %q", artifact.Images[0].GenerationRequestID, "req_img_1")
	}
	if artifact.Images[0].GenerationModel != "qwen-image-2.0" {
		t.Fatalf("artifact.Images[0].GenerationModel = %q, want %q", artifact.Images[0].GenerationModel, "qwen-image-2.0")
	}
	if artifact.Images[0].SourceImageURL != "https://example.com/generated.jpg" {
		t.Fatalf("artifact.Images[0].SourceImageURL = %q, want %q", artifact.Images[0].SourceImageURL, "https://example.com/generated.jpg")
	}
	imageBytes, err := os.ReadFile(artifactFullPath(workspaceDir, artifact.Images[0].FilePath))
	if err != nil {
		t.Fatalf("ReadFile(generated image) error = %v", err)
	}
	if string(imageBytes) != "fake-image-bytes" {
		t.Fatalf("generated image bytes = %q", string(imageBytes))
	}
	if updated.OutputRef["image_count"] != 1 {
		t.Fatalf("image_count = %#v, want 1", updated.OutputRef["image_count"])
	}
	if updated.OutputRef["generated_image_count"] != 1 {
		t.Fatalf("generated_image_count = %#v, want 1", updated.OutputRef["generated_image_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 0 {
		t.Fatalf("fallback_image_count = %#v, want 0", updated.OutputRef["fallback_image_count"])
	}
}

func TestExecuteRequiresImageStyle(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(t.TempDir())
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_image_123"},
		model.Task{Key: "image", Payload: map[string]any{}},
		map[string]model.Task{
			"script": {Key: "script", OutputRef: map[string]any{"artifact_path": "jobs/job_image_123/script.json"}},
			"character_image": {
				Key: "character_image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_image_123/character_images/manifest.json",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresScriptDependency(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(t.TempDir())
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_image_123"},
		model.Task{
			Key:     "image",
			Payload: map[string]any{"image_style": "cinematic"},
		},
		map[string]model.Task{},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresCharacterImageDependency(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(t.TempDir())
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_image_123"},
		model.Task{
			Key:     "image",
			Payload: map[string]any{"image_style": "cinematic"},
		},
		map[string]model.Task{
			"script": {
				Key: "script",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_image_123/script.json",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func readImageArtifact(t *testing.T, workspaceDir string, artifactPath string) ImageOutput {
	t.Helper()

	data, err := os.ReadFile(artifactFullPath(workspaceDir, artifactPath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", artifactPath, err)
	}

	var value ImageOutput
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", artifactPath, err)
	}

	return value
}

func containsAll(value string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}

	return true
}

func assertJPEGDimensions(t *testing.T, path string, width int, height int) {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer file.Close()

	config, err := jpeg.DecodeConfig(file)
	if err != nil {
		t.Fatalf("DecodeConfig(%q) error = %v", path, err)
	}
	if config.Width != width {
		t.Fatalf("jpeg width = %d, want %d", config.Width, width)
	}
	if config.Height != height {
		t.Fatalf("jpeg height = %d, want %d", config.Height, height)
	}
}

type assertiveError string

func (e assertiveError) Error() string {
	return string(e)
}
