package image

import (
	"context"
	"encoding/json"
	"image/jpeg"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type fakeClient struct {
	requests  []Request
	response  Response
	responses []Response
	err       error
	errs      []error
}

func (f *fakeClient) Generate(_ context.Context, request Request) (Response, error) {
	f.requests = append(f.requests, request)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return Response{}, err
		}
	}
	if len(f.responses) > 0 {
		response := f.responses[0]
		f.responses = f.responses[1:]
		return response, nil
	}
	if f.err != nil {
		return Response{}, f.err
	}

	return f.response, nil
}

type recordingProgressReporter struct {
	mu       sync.Mutex
	progress []model.TaskProgress
}

func (r *recordingProgressReporter) Report(
	_ context.Context,
	progress model.TaskProgress,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress = append(r.progress, progress)
	return nil
}

func (r *recordingProgressReporter) snapshot() []model.TaskProgress {
	r.mu.Lock()
	defer r.mu.Unlock()

	cloned := make([]model.TaskProgress, len(r.progress))
	copy(cloned, r.progress)
	return cloned
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
				Index: 0,
				Shots: []scriptArtifactShot{
					{
						Index:              0,
						InvolvedCharacters: []string{"Lin Qing"},
						ImagePrompt:        "Lin Qing stands on the bridge in the night rain",
					},
					{
						Index:      1,
						TextPrompt: "Lantern light reflects on the wet stone path",
					},
				},
			},
			{
				Index: 1,
				Shots: []scriptArtifactShot{
					{Index: 0, TextPrompt: "A weary traveler arrives at the inn gate at dusk"},
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
	if updated.OutputRef["shot_image_count"] != 3 {
		t.Fatalf("shot_image_count = %#v, want 3", updated.OutputRef["shot_image_count"])
	}
	if updated.OutputRef["generated_image_count"] != 0 {
		t.Fatalf("generated_image_count = %#v, want 0", updated.OutputRef["generated_image_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 3 {
		t.Fatalf("fallback_image_count = %#v, want 3", updated.OutputRef["fallback_image_count"])
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_123/images/image_manifest.json")
	if len(artifact.Images) != 2 {
		t.Fatalf("len(artifact.Images) = %d, want 2", len(artifact.Images))
	}
	if len(artifact.ShotImages) != 3 {
		t.Fatalf("len(artifact.ShotImages) = %d, want 3", len(artifact.ShotImages))
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
	if !strings.Contains(artifact.Images[0].PromptSourceText, "Lantern light reflects on the wet stone path") {
		t.Fatalf("artifact.Images[0].PromptSourceText = %q, want joined shot prompts", artifact.Images[0].PromptSourceText)
	}
	if got := artifact.Images[1].Prompt; !containsAll(got, []string{"candidate characters: Lin Qing / A-Qing", "character reference details: Lin Qing / A-Qing: Lin Qing, white robe, jade pendant, front view", "A weary traveler arrives at the inn gate at dusk"}) {
		t.Fatalf("artifact.Images[1].Prompt = %q, want fallback candidate prompt", got)
	}
	if artifact.Images[1].PromptSourceType != "shots" {
		t.Fatalf("artifact.Images[1].PromptSourceType = %q, want %q", artifact.Images[1].PromptSourceType, "shots")
	}
	if artifact.ShotImages[0].SegmentIndex != 0 || artifact.ShotImages[0].ShotIndex != 0 {
		t.Fatalf("artifact.ShotImages[0] indexes = (%d,%d), want (0,0)", artifact.ShotImages[0].SegmentIndex, artifact.ShotImages[0].ShotIndex)
	}
	if artifact.ShotImages[0].PromptType != "image_to_image" {
		t.Fatalf("artifact.ShotImages[0].PromptType = %q, want %q", artifact.ShotImages[0].PromptType, "image_to_image")
	}
	if artifact.ShotImages[1].PromptType != "text_to_image" {
		t.Fatalf("artifact.ShotImages[1].PromptType = %q, want %q", artifact.ShotImages[1].PromptType, "text_to_image")
	}
	if artifact.ShotImages[0].FilePath != "jobs/job_image_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("artifact.ShotImages[0].FilePath = %q", artifact.ShotImages[0].FilePath)
	}
	assertJPEGDimensions(t, artifactFullPath(workspaceDir, artifact.ShotImages[0].FilePath), defaultImageWidth, defaultImageHeight)
	assertJPEGDimensions(t, artifactFullPath(workspaceDir, artifact.ShotImages[1].FilePath), defaultImageWidth, defaultImageHeight)
	assertJPEGDimensions(t, artifactFullPath(workspaceDir, artifact.ShotImages[2].FilePath), defaultImageWidth, defaultImageHeight)
	if len(artifact.ShotImages[0].MatchedCharacters) != 1 {
		t.Fatalf("len(artifact.ShotImages[0].MatchedCharacters) = %d, want 1", len(artifact.ShotImages[0].MatchedCharacters))
	}
	if len(artifact.ShotImages[1].MatchedCharacters) != 0 {
		t.Fatalf("len(artifact.ShotImages[1].MatchedCharacters) = %d, want 0", len(artifact.ShotImages[1].MatchedCharacters))
	}
}

func TestExecuteReportsProgress(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{PublicID: "job_image_progress"}
	task := model.Task{
		Key:     "image",
		Payload: map[string]any{"image_style": "realistic"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_progress/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_progress/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_progress/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{
				Index: 0,
				Shots: []scriptArtifactShot{
					{Index: 0, TextPrompt: "雨夜街道上孤灯摇晃"},
					{Index: 1, TextPrompt: "少年停在巷口回头张望"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_progress/character_images/manifest.json", CharacterImageOutput{}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	reporter := &recordingProgressReporter{}
	ctx := model.WithTaskProgressReporter(context.Background(), reporter)

	_, err := executor.Execute(ctx, job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	progress := reporter.snapshot()
	if len(progress) < 3 {
		t.Fatalf("len(progress) = %d, want >= 3", len(progress))
	}
	if progress[0].Phase != "generating_shot" || progress[0].Current != 1 || progress[0].Total != 2 {
		t.Fatalf("progress[0] = %#v, want first shot progress", progress[0])
	}
	if progress[1].Phase != "generating_shot" || progress[1].Current != 2 || progress[1].Total != 2 {
		t.Fatalf("progress[1] = %#v, want second shot progress", progress[1])
	}
	last := progress[len(progress)-1]
	if last.Phase != "writing_artifact" {
		t.Fatalf("last progress phase = %#v, want writing_artifact", last.Phase)
	}
}

func TestExecuteUsesPortraitAspectRatioWhenRequested(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_image_portrait_123",
	}
	task := model.Task{
		ID:  22,
		Key: "image",
		Payload: map[string]any{
			"image_style":  "cinematic",
			"aspect_ratio": "9:16",
		},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_portrait_123/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_portrait_123/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_portrait_123/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{
				Index: 0,
				Shots: []scriptArtifactShot{
					{Index: 0, TextPrompt: "Lantern light falls across the narrow alley"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_portrait_123/character_images/manifest.json", CharacterImageOutput{}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["aspect_ratio"] != "9:16" {
		t.Fatalf("aspect_ratio = %#v, want %q", updated.OutputRef["aspect_ratio"], "9:16")
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_portrait_123/images/image_manifest.json")
	if len(artifact.ShotImages) != 1 {
		t.Fatalf("len(artifact.ShotImages) = %d, want 1", len(artifact.ShotImages))
	}
	if artifact.ShotImages[0].Width != 720 || artifact.ShotImages[0].Height != 1280 {
		t.Fatalf(
			"shot image size = %dx%d, want 720x1280",
			artifact.ShotImages[0].Width,
			artifact.ShotImages[0].Height,
		)
	}
	if !strings.Contains(artifact.ShotImages[0].Prompt, "9:16") {
		t.Fatalf("shot prompt = %q, want 9:16 suffix", artifact.ShotImages[0].Prompt)
	}
	assertJPEGDimensions(
		t,
		artifactFullPath(workspaceDir, artifact.ShotImages[0].FilePath),
		720,
		1280,
	)
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
			{Index: 0, Shots: []scriptArtifactShot{{Index: 0, ImagePrompt: "Lin Qing waits by the river", InvolvedCharacters: []string{"Lin Qing"}}}},
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
	if err := writer.WriteBytes("jobs/job_image_live_fallback_123/character_images/character_000.jpg", []byte("reference-image")); err != nil {
		t.Fatalf("WriteBytes(character_image file) error = %v", err)
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
				Index: 0,
				Shots: []scriptArtifactShot{
					{
						Index:              0,
						InvolvedCharacters: []string{"Lin Qing"},
						ImagePrompt:        "A-Qing enters the courtyard under drifting petals",
					},
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

func TestResolveSegmentPromptSourceTightensSingleImageShotSelection(t *testing.T) {
	t.Parallel()

	source := resolveSegmentPromptSource(
		scriptArtifactSegment{
			Shots: []scriptArtifactShot{
				{Index: 0, TextPrompt: "shot 0 opening scene"},
				{Index: 1, TextPrompt: "shot 1 movement"},
				{Index: 2, TextPrompt: "shot 2 confrontation"},
				{Index: 3, TextPrompt: "shot 3 emotion"},
				{Index: 4, TextPrompt: "shot 4 ending beat"},
			},
		},
		nil,
	)
	if source.Type != "shots" {
		t.Fatalf("source.Type = %q, want %q", source.Type, "shots")
	}
	if len(source.Shots) != 3 {
		t.Fatalf("len(source.Shots) = %d, want 3", len(source.Shots))
	}
	if source.Shots[0] != "shot 0 opening scene" {
		t.Fatalf("source.Shots[0] = %q", source.Shots[0])
	}
	if source.Shots[1] != "shot 2 confrontation" {
		t.Fatalf("source.Shots[1] = %q", source.Shots[1])
	}
	if source.Shots[2] != "shot 4 ending beat" {
		t.Fatalf("source.Shots[2] = %q", source.Shots[2])
	}
}

func TestResolveSegmentPromptSourceReturnsEmptyWithoutShots(t *testing.T) {
	t.Parallel()

	source := resolveSegmentPromptSource(
		scriptArtifactSegment{
			Index: 0,
		},
		nil,
	)
	if source.Type != "empty" {
		t.Fatalf("source.Type = %q, want %q", source.Type, "empty")
	}
}

func TestResolveSegmentPromptSourceIgnoresLegacyPromptField(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
	  "index": 0,
	  "shots": [
	    {
	      "index": 0,
	      "prompt": "legacy prompt should not be consumed"
	    }
	  ]
	}`)

	var segment scriptArtifactSegment
	if err := json.Unmarshal(raw, &segment); err != nil {
		t.Fatalf("Unmarshal(segment) error = %v", err)
	}

	source := resolveSegmentPromptSource(segment, nil)
	if source.Type != "empty" {
		t.Fatalf("source.Type = %q, want %q", source.Type, "empty")
	}
	if source.Text != "" {
		t.Fatalf("source.Text = %q, want empty", source.Text)
	}
	if len(source.Shots) != 0 {
		t.Fatalf("len(source.Shots) = %d, want 0", len(source.Shots))
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
			{Index: 0, Shots: []scriptArtifactShot{{Index: 0, ImagePrompt: "Lin Qing waits by the river", InvolvedCharacters: []string{"Lin Qing"}}}},
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
	if err := writer.WriteBytes("jobs/job_image_live_123/character_images/character_000.jpg", []byte("reference-image")); err != nil {
		t.Fatalf("WriteBytes(character_image file) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("len(client.requests) = %d, want 1", len(client.requests))
	}
	if len(client.requests[0].ReferenceImages) != 1 {
		t.Fatalf("len(client.requests[0].ReferenceImages) = %d, want 1", len(client.requests[0].ReferenceImages))
	}
	if !strings.Contains(client.requests[0].Prompt, "图1中的人物") {
		t.Fatalf("client.requests[0].Prompt = %q, want placeholder text", client.requests[0].Prompt)
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

func TestExecuteReusesLatestSuccessfulShotImageAfterRetriesExhausted(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeClient{
		responses: []Response{
			{
				RequestID: "req_img_1",
				Model:     "qwen-image-2.0",
				ImageURL:  "https://example.com/generated-1.jpg",
				ImageData: []byte("fake-image-bytes-1"),
			},
		},
		errs: []error{
			nil,
			assertiveError("shot generation failed"),
			assertiveError("shot generation failed"),
			assertiveError("shot generation failed"),
		},
	}
	executor := NewExecutorWithClient(client, GenerationConfig{
		Model: "qwen-image-2.0",
	}, workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_image_reuse_123",
	}
	task := model.Task{
		ID:      25,
		Key:     "image",
		Payload: map[string]any{"image_style": "cinematic"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_reuse_123/script.json",
			},
		},
		"character_image": {
			Key: "character_image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_image_reuse_123/character_images/manifest.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_image_reuse_123/script.json", scriptArtifactOutput{
		Segments: []scriptArtifactSegment{
			{
				Index: 0,
				Shots: []scriptArtifactShot{
					{Index: 0, TextPrompt: "first shot at dawn"},
					{Index: 1, TextPrompt: "second shot at dusk"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(script) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_image_reuse_123/character_images/manifest.json", CharacterImageOutput{}); err != nil {
		t.Fatalf("WriteJSON(character_image) error = %v", err)
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["generated_image_count"] != 2 {
		t.Fatalf("generated_image_count = %#v, want 2", updated.OutputRef["generated_image_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 0 {
		t.Fatalf("fallback_image_count = %#v, want 0", updated.OutputRef["fallback_image_count"])
	}

	artifact := readImageArtifact(t, workspaceDir, "jobs/job_image_reuse_123/images/image_manifest.json")
	if len(artifact.ShotImages) != 2 {
		t.Fatalf("len(artifact.ShotImages) = %d, want 2", len(artifact.ShotImages))
	}
	if artifact.ShotImages[1].IsFallback {
		t.Fatal("artifact.ShotImages[1].IsFallback = true, want false")
	}
	if !artifact.ShotImages[1].FilledFromPrevious {
		t.Fatal("artifact.ShotImages[1].FilledFromPrevious = false, want true")
	}
	firstBytes, err := os.ReadFile(artifactFullPath(workspaceDir, artifact.ShotImages[0].FilePath))
	if err != nil {
		t.Fatalf("ReadFile(first shot) error = %v", err)
	}
	secondBytes, err := os.ReadFile(artifactFullPath(workspaceDir, artifact.ShotImages[1].FilePath))
	if err != nil {
		t.Fatalf("ReadFile(second shot) error = %v", err)
	}
	if string(firstBytes) != "fake-image-bytes-1" || string(secondBytes) != "fake-image-bytes-1" {
		t.Fatalf("reused bytes mismatch: first=%q second=%q", string(firstBytes), string(secondBytes))
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
