package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestOutlineExecutorExecute(t *testing.T) {
	t.Parallel()

	executor := NewOutlineExecutor()
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
}
