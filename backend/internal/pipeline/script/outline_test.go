package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestOutlineExecutorExecute(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewOutlineExecutorWithClient(nil, TextGenerationConfig{}, workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_test_outline",
	}
	task := model.Task{
		ID:      10,
		Key:     "outline",
		Type:    model.TaskTypeOutline,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "This is a test article for outline generation.",
			"language": "en",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "outline" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "outline")
	}
	if got.OutputRef["artifact_path"] != "jobs/job_test_outline/outline.json" {
		t.Fatalf("artifact_path = %#v, want %#v", got.OutputRef["artifact_path"], "jobs/job_test_outline/outline.json")
	}
	if got.OutputRef["language"] != "en" {
		t.Fatalf("language = %#v, want %#v", got.OutputRef["language"], "en")
	}
	if got.OutputRef["section_count"] != 5 {
		t.Fatalf("section_count = %#v, want %#v", got.OutputRef["section_count"], 5)
	}

	artifact := readJSONArtifact[OutlineOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.PlotStages) != 5 {
		t.Fatalf("len(plot_stages) = %d, want 5", len(artifact.PlotStages))
	}
	if artifact.PlotStages[0].Name != "开端" {
		t.Fatalf("plot_stages[0].name = %q, want %q", artifact.PlotStages[0].Name, "开端")
	}
	if artifact.Mainline == "" {
		t.Fatal("mainline = empty, want non-empty")
	}
}
