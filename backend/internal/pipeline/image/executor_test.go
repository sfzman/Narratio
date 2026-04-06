package image

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestExecuteBuildsImageOutputRef(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
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
}

func TestExecuteRequiresImageStyle(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_image_123"},
		model.Task{Key: "image", Payload: map[string]any{}},
		map[string]model.Task{
			"script": {Key: "script", OutputRef: map[string]any{"artifact_path": "jobs/job_image_123/script.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresScriptDependency(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
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
