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
		ID:   41,
		Key:  "character_image",
		Type: model.TaskTypeCharacterImage,
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
	if len(artifact.Images[0].MatchTerms) == 0 {
		t.Fatal("len(artifact.Images[0].MatchTerms) = 0, want non-zero")
	}
	if artifact.Images[0].MatchTerms[0] != "Lin Qing" {
		t.Fatalf("artifact.Images[0].MatchTerms[0] = %q, want %q", artifact.Images[0].MatchTerms[0], "Lin Qing")
	}
	if !artifact.Images[0].IsFallback {
		t.Fatal("artifact.Images[0].IsFallback = false, want true")
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
