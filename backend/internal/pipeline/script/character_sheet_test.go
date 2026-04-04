package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestCharacterSheetExecutorExecute(t *testing.T) {
	t.Parallel()

	executor := NewCharacterSheetExecutor()
	job := model.Job{
		ID:       3,
		PublicID: "job_test_character_sheet",
	}
	task := model.Task{
		ID:      30,
		Key:     "character_sheet",
		Type:    model.TaskTypeCharacterSheet,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "Alice meets Bob in a test story.",
			"language": "en",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "character_sheet" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "character_sheet")
	}
	if got.OutputRef["artifact_path"] != "jobs/job_test_character_sheet/character_sheet.json" {
		t.Fatalf("artifact_path = %#v, want %#v", got.OutputRef["artifact_path"], "jobs/job_test_character_sheet/character_sheet.json")
	}
	if got.OutputRef["character_count"] != 1 {
		t.Fatalf("character_count = %#v, want %#v", got.OutputRef["character_count"], 1)
	}
}
