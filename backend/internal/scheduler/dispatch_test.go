package scheduler

import (
	"testing"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestTaskExecutionTimeoutUsesSegmentCountForScript(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeScript},
		map[string]model.Task{
			"segmentation": {
				Key: "segmentation",
				OutputRef: map[string]any{
					"segment_count": 3,
				},
			},
		},
		200*time.Second,
	)
	if timeout != 600*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 600*time.Second)
	}
}

func TestTaskExecutionTimeoutFallsBackForScriptWithoutSegmentCount(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeScript},
		map[string]model.Task{},
		200*time.Second,
	)
	if timeout != defaultTaskExecutionTimeout {
		t.Fatalf("timeout = %s, want %s", timeout, defaultTaskExecutionTimeout)
	}
}

func TestTaskExecutionTimeoutUsesDefaultForNonScript(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeOutline},
		map[string]model.Task{},
		200*time.Second,
	)
	if timeout != defaultTaskExecutionTimeout {
		t.Fatalf("timeout = %s, want %s", timeout, defaultTaskExecutionTimeout)
	}
}

func TestTaskExecutionTimeoutUsesDefaultPerSegmentWhenConfiguredValueInvalid(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeScript},
		map[string]model.Task{
			"segmentation": {
				OutputRef: map[string]any{"segment_count": 2},
			},
		},
		0,
	)
	if timeout != 400*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 400*time.Second)
	}
}
