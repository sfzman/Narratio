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
		300*time.Second,
		200*time.Second,
		30*time.Minute,
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
		300*time.Second,
		200*time.Second,
		30*time.Minute,
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
		300*time.Second,
		200*time.Second,
		30*time.Minute,
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
		300*time.Second,
		200*time.Second,
		30*time.Minute,
	)
	if timeout != 400*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 400*time.Second)
	}
}

func TestTaskExecutionTimeoutUsesSegmentCountForTTS(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeTTS},
		map[string]model.Task{
			"segmentation": {
				Key: "segmentation",
				OutputRef: map[string]any{
					"segment_count": 4,
				},
			},
		},
		200*time.Second,
		300*time.Second,
		200*time.Second,
		30*time.Minute,
	)
	if timeout != 1200*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 1200*time.Second)
	}
}

func TestTaskExecutionTimeoutFallsBackForTTSWithoutSegmentCount(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeTTS},
		map[string]model.Task{},
		200*time.Second,
		300*time.Second,
		200*time.Second,
		30*time.Minute,
	)
	if timeout != defaultTaskExecutionTimeout {
		t.Fatalf("timeout = %s, want %s", timeout, defaultTaskExecutionTimeout)
	}
}

func TestTaskExecutionTimeoutUsesDefaultPerSegmentWhenTTSConfiguredValueInvalid(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeTTS},
		map[string]model.Task{
			"segmentation": {
				OutputRef: map[string]any{"segment_count": 2},
			},
		},
		200*time.Second,
		0,
		200*time.Second,
		30*time.Minute,
	)
	if timeout != 600*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 600*time.Second)
	}
}

func TestTaskExecutionTimeoutUsesConfiguredValueForVideo(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeVideo},
		map[string]model.Task{},
		200*time.Second,
		300*time.Second,
		200*time.Second,
		25*time.Minute,
	)
	if timeout != 25*time.Minute {
		t.Fatalf("timeout = %s, want %s", timeout, 25*time.Minute)
	}
}

func TestTaskExecutionTimeoutUsesDefaultForVideoWhenConfiguredValueInvalid(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{Type: model.TaskTypeVideo},
		map[string]model.Task{},
		200*time.Second,
		300*time.Second,
		200*time.Second,
		0,
	)
	if timeout != defaultVideoRenderExecutionTimeout {
		t.Fatalf("timeout = %s, want %s", timeout, defaultVideoRenderExecutionTimeout)
	}
}

func TestTaskExecutionTimeoutUsesRequestedVideoCountForShotVideo(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{
			Type:    model.TaskTypeShotVideo,
			Payload: map[string]any{"video_count": 3},
		},
		map[string]model.Task{
			"image": {
				OutputRef: map[string]any{"shot_image_count": 10},
			},
		},
		200*time.Second,
		300*time.Second,
		200*time.Second,
		30*time.Minute,
	)
	if timeout != 600*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 600*time.Second)
	}
}

func TestTaskExecutionTimeoutCapsShotVideoByShotImageCount(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{
			Type:    model.TaskTypeShotVideo,
			Payload: map[string]any{"video_count": 12},
		},
		map[string]model.Task{
			"image": {
				OutputRef: map[string]any{"shot_image_count": 4},
			},
		},
		200*time.Second,
		300*time.Second,
		180*time.Second,
		30*time.Minute,
	)
	if timeout != 720*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 720*time.Second)
	}
}

func TestTaskExecutionTimeoutUsesDefaultPerShotWhenConfiguredValueInvalid(t *testing.T) {
	t.Parallel()

	timeout := taskExecutionTimeout(
		model.Task{
			Type:    model.TaskTypeShotVideo,
			Payload: map[string]any{"video_count": 2},
		},
		map[string]model.Task{},
		200*time.Second,
		300*time.Second,
		0,
		30*time.Minute,
	)
	if timeout != 400*time.Second {
		t.Fatalf("timeout = %s, want %s", timeout, 400*time.Second)
	}
}
