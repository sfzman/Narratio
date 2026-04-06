package tts

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestExecuteBuildsTTSOutputRef(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	job := model.Job{
		ID:       1,
		PublicID: "job_tts_123",
	}
	task := model.Task{
		ID:      11,
		Key:     "tts",
		Payload: map[string]any{"voice_id": "reader_a"},
	}
	dependencies := map[string]model.Task{
		"script": {
			Key: "script",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_tts_123/script.json",
			},
		},
	}

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["artifact_type"] != "tts" {
		t.Fatalf("artifact_type = %#v, want %q", updated.OutputRef["artifact_type"], "tts")
	}
	if updated.OutputRef["voice_id"] != "reader_a" {
		t.Fatalf("voice_id = %#v, want %q", updated.OutputRef["voice_id"], "reader_a")
	}
	if updated.OutputRef["script_artifact_ref"] != "jobs/job_tts_123/script.json" {
		t.Fatalf("script_artifact_ref = %#v", updated.OutputRef["script_artifact_ref"])
	}
	if updated.OutputRef["subtitle_artifact_ref"] != "jobs/job_tts_123/audio/subtitles.srt" {
		t.Fatalf("subtitle_artifact_ref = %#v", updated.OutputRef["subtitle_artifact_ref"])
	}
}

func TestExecuteRequiresVoiceID(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_tts_123"},
		model.Task{Key: "tts", Payload: map[string]any{}},
		map[string]model.Task{
			"script": {Key: "script", OutputRef: map[string]any{"artifact_path": "jobs/job_tts_123/script.json"}},
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
		model.Job{PublicID: "job_tts_123"},
		model.Task{
			Key:     "tts",
			Payload: map[string]any{"voice_id": "reader_a"},
		},
		map[string]model.Task{},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
