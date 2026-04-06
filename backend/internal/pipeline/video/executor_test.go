package video

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestExecuteBuildsVideoOutputRef(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	job := model.Job{
		ID:       1,
		PublicID: "job_video_123",
	}
	task := model.Task{
		ID:  31,
		Key: "video",
	}
	dependencies := map[string]model.Task{
		"tts": {
			Key: "tts",
			OutputRef: map[string]any{
				"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
				"total_duration_seconds": 8.25,
			},
		},
		"image": {
			Key: "image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_video_123/images/image_manifest.json",
			},
		},
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["artifact_type"] != "video" {
		t.Fatalf("artifact_type = %#v, want %q", updated.OutputRef["artifact_type"], "video")
	}
	if updated.OutputRef["artifact_path"] != "jobs/job_video_123/output/final.mp4" {
		t.Fatalf("artifact_path = %#v", updated.OutputRef["artifact_path"])
	}
	if updated.OutputRef["duration_seconds"] != 8.25 {
		t.Fatalf("duration_seconds = %#v", updated.OutputRef["duration_seconds"])
	}
}

func TestExecuteRequiresTTSDependency(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"image": {Key: "image", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresImageDependency(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {Key: "tts", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/audio/tts_manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
