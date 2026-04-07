package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestScriptExecutorExecute(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewScriptExecutorWithClient(nil, TextGenerationConfig{}, workspaceDir)
	writer := newArtifactWriter(workspaceDir)
	job := model.Job{
		ID:       2,
		PublicID: "job_test_script",
	}
	task := model.Task{
		ID:   20,
		Key:  "script",
		Type: model.TaskTypeScript,
		Payload: map[string]any{
			"article":  "A short article for script generation.",
			"language": "en",
			"voice_id": "default",
		},
		OutputRef: map[string]any{},
	}
	dependencies := map[string]model.Task{
		"segmentation": {
			Key: "segmentation",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_test_script/segments.json",
			},
		},
		"outline": {
			Key: "outline",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_test_script/outline.json",
			},
		},
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_test_script/character_sheet.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_test_script/segments.json", SegmentationOutput{
		Segments: []TextSegment{
			{Index: 0, Text: "A short article for script generation.", CharCount: 32},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(segmentation) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_test_script/outline.json", OutlineOutput{
		Mainline: "一条主线",
		PlotStages: []OutlineStage{
			{Name: "开端", Happened: "故事开始", Goal: "建立局面", Obstacle: "阻碍初现", Outcome: "进入发展"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(outline) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_test_script/character_sheet.json", CharacterSheetOutput{
		Characters: []CharacterProfile{
			{Name: "阿莲", Role: "主角", Appearance: "神情警惕", ReferenceSubjectType: "人"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	got, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "script" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "script")
	}
	if got.OutputRef["segmentation_ref"] != "jobs/job_test_script/segments.json" {
		t.Fatalf("segmentation_ref = %#v", got.OutputRef["segmentation_ref"])
	}
	if got.OutputRef["outline_artifact_ref"] != "jobs/job_test_script/outline.json" {
		t.Fatalf("outline_artifact_ref = %#v", got.OutputRef["outline_artifact_ref"])
	}
	if got.OutputRef["character_ref"] != "jobs/job_test_script/character_sheet.json" {
		t.Fatalf("character_ref = %#v", got.OutputRef["character_ref"])
	}
	if got.OutputRef["segment_count"] != 1 {
		t.Fatalf("segment_count = %#v, want %#v", got.OutputRef["segment_count"], 1)
	}

	artifact := readJSONArtifact[ScriptOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(artifact.Segments))
	}
	if artifact.Segments[0].Script == "" {
		t.Fatal("segments[0].script = empty, want non-empty")
	}
	if artifact.Segments[0].Text != "A short article for script generation." {
		t.Fatalf("segments[0].text = %q", artifact.Segments[0].Text)
	}
}
