package image

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestCharacterImageExecutorWritesArtifact(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewCharacterImageExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	if err := writer.WriteJSON(
		"jobs/job_character_image_123/character_sheet.json",
		characterSheetArtifact{
			Characters: []characterProfileArtifact{
				{
					Name:                 "Lin Qing",
					Appearance:           "white robe, long hair",
					VisualSignature:      "jade pendant",
					ReferenceSubjectType: "person",
					ImagePromptFocus:     "front view, full body",
				},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	job := model.Job{
		ID:       1,
		PublicID: "job_character_image_123",
	}
	task := model.Task{
		ID:      41,
		Key:     "character_image",
		Type:    model.TaskTypeCharacterImage,
		Payload: map[string]any{"image_style": "modern_gongbi"},
	}
	dependencies := map[string]model.Task{
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_character_image_123/character_sheet.json",
			},
		},
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["artifact_type"] != "character_image" {
		t.Fatalf("artifact_type = %#v, want %q", updated.OutputRef["artifact_type"], "character_image")
	}
	if updated.OutputRef["character_sheet_ref"] != "jobs/job_character_image_123/character_sheet.json" {
		t.Fatalf("character_sheet_ref = %#v", updated.OutputRef["character_sheet_ref"])
	}

	artifact := readCharacterImageArtifact(t, workspaceDir, "jobs/job_character_image_123/character_images/manifest.json")
	if len(artifact.Images) != 1 {
		t.Fatalf("len(artifact.Images) = %d, want 1", len(artifact.Images))
	}
	if artifact.Images[0].CharacterName != "Lin Qing" {
		t.Fatalf("artifact.Images[0].CharacterName = %q, want %q", artifact.Images[0].CharacterName, "Lin Qing")
	}
	if artifact.Images[0].FilePath != "jobs/job_character_image_123/character_images/character_000.jpg" {
		t.Fatalf("artifact.Images[0].FilePath = %q", artifact.Images[0].FilePath)
	}
	assertJPEGDimensions(
		t,
		artifactFullPath(workspaceDir, artifact.Images[0].FilePath),
		characterReferenceImageWidth,
		characterReferenceImageHeight,
	)
	if len(artifact.Images[0].MatchTerms) == 0 {
		t.Fatal("len(artifact.Images[0].MatchTerms) = 0, want non-zero")
	}
	if artifact.Images[0].MatchTerms[0] != "Lin Qing" {
		t.Fatalf("artifact.Images[0].MatchTerms[0] = %q, want %q", artifact.Images[0].MatchTerms[0], "Lin Qing")
	}
	if artifact.Images[0].Prompt == "" {
		t.Fatal("artifact.Images[0].Prompt = empty, want non-empty")
	}
	if artifact.Images[0].Prompt != "" &&
		!containsAll(artifact.Images[0].Prompt, []string{"单人", "正面", "全身", "居中站姿", "现代工笔人物画风"}) {
		t.Fatalf("artifact.Images[0].Prompt = %q, want fixed character reference framing", artifact.Images[0].Prompt)
	}
	if !artifact.Images[0].IsFallback {
		t.Fatal("artifact.Images[0].IsFallback = false, want true")
	}
	if updated.OutputRef["image_style"] != modernGongbiImageStyle {
		t.Fatalf("image_style = %#v, want %q", updated.OutputRef["image_style"], modernGongbiImageStyle)
	}
	if updated.OutputRef["generated_character_image_count"] != 0 {
		t.Fatalf("generated_character_image_count = %#v, want 0", updated.OutputRef["generated_character_image_count"])
	}
	if updated.OutputRef["fallback_character_image_count"] != 1 {
		t.Fatalf("fallback_character_image_count = %#v, want 1", updated.OutputRef["fallback_character_image_count"])
	}
}

func TestCharacterImageExecutorRequiresCharacterSheetDependency(t *testing.T) {
	t.Parallel()

	executor := NewCharacterImageExecutor(t.TempDir())
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_character_image_123"},
		model.Task{Key: "character_image", Type: model.TaskTypeCharacterImage},
		map[string]model.Task{},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestCharacterImageExecutorWritesLiveImageWhenClientInjected(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeClient{
		response: Response{
			RequestID: "req_character_1",
			Model:     "qwen-image-2.0",
			ImageURL:  "https://example.com/character.jpg",
			ImageData: []byte("character-image-bytes"),
		},
	}
	executor := NewCharacterImageExecutorWithClient(client, GenerationConfig{
		Model: "qwen-image-2.0",
	}, workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	if err := writer.WriteJSON(
		"jobs/job_character_image_live_123/character_sheet.json",
		characterSheetArtifact{
			Characters: []characterProfileArtifact{
				{
					Name:                 "Lin Qing",
					Appearance:           "white robe, long hair",
					VisualSignature:      "jade pendant",
					ReferenceSubjectType: "person",
					ImagePromptFocus:     "front view, full body",
				},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_character_image_live_123"},
		model.Task{
			Key:     "character_image",
			Type:    model.TaskTypeCharacterImage,
			Payload: map[string]any{"image_style": "realistic"},
		},
		map[string]model.Task{
			"character_sheet": {
				Key: "character_sheet",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_character_image_live_123/character_sheet.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("len(client.requests) = %d, want 1", len(client.requests))
	}
	if client.requests[0].Size != characterReferenceImageSize() {
		t.Fatalf("client.requests[0].Size = %q, want %q", client.requests[0].Size, characterReferenceImageSize())
	}
	if !containsAll(client.requests[0].Prompt, []string{"单人", "正面", "全身", "居中站姿", "写实风格"}) {
		t.Fatalf("client.requests[0].Prompt = %q, want fixed character reference framing", client.requests[0].Prompt)
	}
	if updated.OutputRef["generated_character_image_count"] != 1 {
		t.Fatalf("generated_character_image_count = %#v, want 1", updated.OutputRef["generated_character_image_count"])
	}
	if updated.OutputRef["fallback_character_image_count"] != 0 {
		t.Fatalf("fallback_character_image_count = %#v, want 0", updated.OutputRef["fallback_character_image_count"])
	}

	artifact := readCharacterImageArtifact(
		t,
		workspaceDir,
		"jobs/job_character_image_live_123/character_images/manifest.json",
	)
	if artifact.Images[0].IsFallback {
		t.Fatal("artifact.Images[0].IsFallback = true, want false")
	}
	if artifact.Images[0].GenerationRequestID != "req_character_1" {
		t.Fatalf("artifact.Images[0].GenerationRequestID = %q", artifact.Images[0].GenerationRequestID)
	}
	if artifact.Images[0].GenerationModel != "qwen-image-2.0" {
		t.Fatalf("artifact.Images[0].GenerationModel = %q", artifact.Images[0].GenerationModel)
	}
	if artifact.Images[0].SourceImageURL != "https://example.com/character.jpg" {
		t.Fatalf("artifact.Images[0].SourceImageURL = %q", artifact.Images[0].SourceImageURL)
	}

	data, err := os.ReadFile(artifactFullPath(workspaceDir, artifact.Images[0].FilePath))
	if err != nil {
		t.Fatalf("ReadFile(generated character image) error = %v", err)
	}
	if string(data) != "character-image-bytes" {
		t.Fatalf("generated character image bytes = %q", string(data))
	}
}

func TestCharacterImageExecutorDefaultsToRealisticStyle(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewCharacterImageExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	if err := writer.WriteJSON(
		"jobs/job_character_image_default_style/character_sheet.json",
		characterSheetArtifact{
			Characters: []characterProfileArtifact{
				{
					Name:             "Lin Qing",
					Appearance:       "white robe, long hair",
					VisualSignature:  "jade pendant",
					ImagePromptFocus: "front view, full body",
				},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_character_image_default_style"},
		model.Task{Key: "character_image", Type: model.TaskTypeCharacterImage},
		map[string]model.Task{
			"character_sheet": {
				Key: "character_sheet",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_character_image_default_style/character_sheet.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["image_style"] != defaultCharacterImageStyle {
		t.Fatalf("image_style = %#v, want %q", updated.OutputRef["image_style"], defaultCharacterImageStyle)
	}

	artifact := readCharacterImageArtifact(
		t,
		workspaceDir,
		"jobs/job_character_image_default_style/character_images/manifest.json",
	)
	if !containsAll(artifact.Images[0].Prompt, []string{"写实风格", "单人", "正面", "全身"}) {
		t.Fatalf("artifact.Images[0].Prompt = %q, want realistic style prompt", artifact.Images[0].Prompt)
	}
}

func TestCharacterImageExecutorReportsProgress(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewCharacterImageExecutor(workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	if err := writer.WriteJSON(
		"jobs/job_character_image_progress/character_sheet.json",
		characterSheetArtifact{
			Characters: []characterProfileArtifact{
				{Name: "Lin Qing", ImagePromptFocus: "front view"},
				{Name: "A-Xuan", ImagePromptFocus: "front view"},
			},
		},
	); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	reporter := &recordingProgressReporter{}
	ctx := model.WithTaskProgressReporter(context.Background(), reporter)

	_, err := executor.Execute(
		ctx,
		model.Job{PublicID: "job_character_image_progress"},
		model.Task{Key: "character_image", Type: model.TaskTypeCharacterImage},
		map[string]model.Task{
			"character_sheet": {
				Key: "character_sheet",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_character_image_progress/character_sheet.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	progress := reporter.snapshot()
	if len(progress) < 3 {
		t.Fatalf("len(progress) = %d, want >= 3", len(progress))
	}
	if progress[0].Phase != "generating_character" || progress[0].Current != 1 || progress[0].Total != 2 {
		t.Fatalf("progress[0] = %#v, want first character progress", progress[0])
	}
	if progress[1].Phase != "generating_character" || progress[1].Current != 2 || progress[1].Total != 2 {
		t.Fatalf("progress[1] = %#v, want second character progress", progress[1])
	}
	last := progress[len(progress)-1]
	if last.Phase != "writing_artifact" {
		t.Fatalf("last progress phase = %#v, want writing_artifact", last.Phase)
	}
}

func readCharacterImageArtifact(
	t *testing.T,
	workspaceDir string,
	artifactPath string,
) CharacterImageOutput {
	t.Helper()

	data, err := os.ReadFile(artifactFullPath(workspaceDir, artifactPath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", artifactPath, err)
	}

	var value CharacterImageOutput
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", artifactPath, err)
	}

	return value
}
