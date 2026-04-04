package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestScriptExecutorExecute(t *testing.T) {
	t.Parallel()

	executor := NewScriptExecutor()
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

	got, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "script" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "script")
	}
	if got.OutputRef["outline_artifact_ref"] != "jobs/job_test_script/outline.json" {
		t.Fatalf("outline_artifact_ref = %#v", got.OutputRef["outline_artifact_ref"])
	}
	if got.OutputRef["character_ref"] != "jobs/job_test_script/character_sheet.json" {
		t.Fatalf("character_ref = %#v", got.OutputRef["character_ref"])
	}
}
