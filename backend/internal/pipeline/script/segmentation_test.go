package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestSegmentationExecutorExecute(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewSegmentationExecutor(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_test_segmentation",
	}
	task := model.Task{
		ID:   10,
		Key:  "segmentation",
		Type: model.TaskTypeSegmentation,
		Payload: map[string]any{
			"article":  "第一句。第二句。第三句。",
			"language": "zh",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "segmentation" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "segmentation")
	}
	if got.OutputRef["artifact_path"] != "jobs/job_test_segmentation/segments.json" {
		t.Fatalf("artifact_path = %#v", got.OutputRef["artifact_path"])
	}
	if got.OutputRef["segment_count"] != 1 {
		t.Fatalf("segment_count = %#v, want %#v", got.OutputRef["segment_count"], 1)
	}

	artifact := readJSONArtifact[SegmentationOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(artifact.Segments))
	}
	if artifact.Segments[0].Text != "第一句。第二句。第三句。" {
		t.Fatalf("segments[0].text = %q", artifact.Segments[0].Text)
	}
	if artifact.Segments[0].CharCount != 9 {
		t.Fatalf("segments[0].char_count = %d, want 9", artifact.Segments[0].CharCount)
	}
}
