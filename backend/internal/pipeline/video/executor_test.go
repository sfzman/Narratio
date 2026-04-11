package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func writeVideoTestArtifact(t *testing.T, workspaceDir string, relativePath string) {
	t.Helper()

	fullPath := filepath.Join(workspaceDir, filepath.Clean(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
	}
}

func writeVideoTestJSONArtifact(t *testing.T, workspaceDir string, relativePath string, value any) {
	t.Helper()

	fullPath := filepath.Join(workspaceDir, filepath.Clean(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", fullPath, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(%q) error = %v", relativePath, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", relativePath, err)
	}
}

func writeValidVideoTestTTSArtifacts(t *testing.T, workspaceDir string, jobPublicID string, audioPaths ...string) {
	t.Helper()

	writeVideoTestArtifact(t, workspaceDir, filepath.Join("jobs", jobPublicID, "audio", "tts_manifest.json"))
	for _, audioPath := range audioPaths {
		writeVideoTestArtifact(t, workspaceDir, audioPath)
	}
}

func writeRenderableVideoTestTTSArtifacts(
	t *testing.T,
	workspaceDir string,
	jobPublicID string,
	segments []map[string]any,
	totalDuration float64,
) {
	t.Helper()

	artifactPath := filepath.Join("jobs", jobPublicID, "audio", "tts_manifest.json")
	writeVideoTestJSONArtifact(t, workspaceDir, artifactPath, map[string]any{
		"audio_segments":         segments,
		"total_duration_seconds": totalDuration,
	})
	for _, segment := range segments {
		audioPath, _ := segment["file_path"].(string)
		if strings.TrimSpace(audioPath) == "" {
			t.Fatalf("segment file_path is empty: %#v", segment)
		}
		writeVideoTestArtifact(t, workspaceDir, audioPath)
	}
}

type fakeCommandRunner struct {
	commands          [][]string
	failuresByMatch   map[string]error
	skipWriteByMatch  map[string]bool
	emptyWriteByMatch map[string]bool
}

type recordingProgressReporter struct {
	mu       sync.Mutex
	progress []model.TaskProgress
}

func (r *recordingProgressReporter) Report(
	_ context.Context,
	progress model.TaskProgress,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress = append(r.progress, progress)
	return nil
}

func (r *recordingProgressReporter) snapshot() []model.TaskProgress {
	r.mu.Lock()
	defer r.mu.Unlock()

	cloned := make([]model.TaskProgress, len(r.progress))
	copy(cloned, r.progress)
	return cloned
}

func (r *fakeCommandRunner) Run(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	command := append([]string{name}, args...)
	r.commands = append(r.commands, command)
	joined := strings.Join(command, " ")
	for match, err := range r.failuresByMatch {
		if strings.Contains(joined, match) {
			return nil, err
		}
	}

	switch name {
	case "ffprobe":
		return []byte("4.000\n"), nil
	case "ffmpeg":
		if len(args) == 0 {
			return nil, nil
		}
		outputPath := args[len(args)-1]
		for match := range r.skipWriteByMatch {
			if strings.Contains(joined, match) {
				return []byte("ok"), nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, err
		}
		content := []byte("fake-media")
		for match := range r.emptyWriteByMatch {
			if strings.Contains(joined, match) {
				content = []byte{}
				break
			}
		}
		if err := os.WriteFile(outputPath, content, 0o644); err != nil {
			return nil, err
		}
		return []byte("ok"), nil
	default:
		return nil, nil
	}
}

func (r *fakeCommandRunner) hasCommandContaining(parts ...string) bool {
	for _, command := range r.commands {
		joined := strings.Join(command, " ")
		matched := true
		for _, part := range parts {
			if !strings.Contains(joined, part) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}

	return false
}

func (r *fakeCommandRunner) failOn(match string, err error) {
	if r.failuresByMatch == nil {
		r.failuresByMatch = make(map[string]error)
	}
	r.failuresByMatch[match] = err
}

func (r *fakeCommandRunner) skipWriteOn(match string) {
	if r.skipWriteByMatch == nil {
		r.skipWriteByMatch = make(map[string]bool)
	}
	r.skipWriteByMatch[match] = true
}

func (r *fakeCommandRunner) writeEmptyOn(match string) {
	if r.emptyWriteByMatch == nil {
		r.emptyWriteByMatch = make(map[string]bool)
	}
	r.emptyWriteByMatch[match] = true
}

func writeVideoTestMediaFiles(t *testing.T, workspaceDir string, relativePaths ...string) {
	t.Helper()

	for _, relativePath := range relativePaths {
		writeVideoTestArtifact(t, workspaceDir, relativePath)
	}
}

func TestExecuteBuildsVideoOutputRef(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	job := model.Job{ID: 1, PublicID: "job_video_123"}
	task := model.Task{ID: 31, Key: "video"}
	dependencies := map[string]model.Task{
		"tts": {
			Key: "tts",
			OutputRef: map[string]any{
				"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
				"segment_count":          1,
				"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
				"total_duration_seconds": 8.25,
			},
		},
		"shot_video": {
			Key: "shot_video",
			OutputRef: map[string]any{
				"artifact_path":     "jobs/job_video_123/shot_videos/manifest.json",
				"image_source_type": "shot_images",
			},
		},
	}
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "image_fallback",
				"duration_seconds": 3.0,
				"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
				"source_type":      "image_fallback",
				"is_fallback":      true,
			},
		},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

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
	if updated.OutputRef["shot_video_artifact_ref"] != "jobs/job_video_123/shot_videos/manifest.json" {
		t.Fatalf("shot_video_artifact_ref = %#v", updated.OutputRef["shot_video_artifact_ref"])
	}
	if updated.OutputRef["image_source_type"] != "shot_images" {
		t.Fatalf("image_source_type = %#v, want %q", updated.OutputRef["image_source_type"], "shot_images")
	}
	if updated.OutputRef["duration_seconds"] != 3.0 {
		t.Fatalf("duration_seconds = %#v", updated.OutputRef["duration_seconds"])
	}
	if updated.OutputRef["narration_duration_seconds"] != 8.25 {
		t.Fatalf("narration_duration_seconds = %#v", updated.OutputRef["narration_duration_seconds"])
	}
	if updated.OutputRef["visual_duration_seconds"] != 3.0 {
		t.Fatalf("visual_duration_seconds = %#v", updated.OutputRef["visual_duration_seconds"])
	}
}

func TestExecuteAcceptsGeneratedShotVideoClips(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_generated_123", "jobs/job_video_generated_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_generated_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "generated_video",
				"duration_seconds": 3.2,
				"video_path":       "jobs/job_video_generated_123/shot_videos/segment_000_shot_000.mp4",
				"source_type":      "generated_video",
				"is_fallback":      false,
			},
		},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_generated_123/shot_videos/segment_000_shot_000.mp4")

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_generated_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_generated_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_generated_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     "jobs/job_video_generated_123/shot_videos/manifest.json",
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["artifact_type"] != "video" {
		t.Fatalf("artifact_type = %#v, want %q", updated.OutputRef["artifact_type"], "video")
	}
	if updated.OutputRef["duration_seconds"] != 3.2 {
		t.Fatalf("duration_seconds = %#v, want 3.2", updated.OutputRef["duration_seconds"])
	}
}

func TestExecuteSumsShotVideoClipDurations(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(
		t,
		workspaceDir,
		"job_video_sum_123",
		"jobs/job_video_sum_123/audio/segment_000.wav",
		"jobs/job_video_sum_123/audio/segment_001.wav",
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_sum_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "image_fallback",
				"duration_seconds": 1.5,
				"image_path":       "jobs/job_video_sum_123/images/segment_000_shot_000.jpg",
				"source_type":      "image_fallback",
			},
			{
				"segment_index":    0,
				"shot_index":       1,
				"status":           "image_fallback",
				"duration_seconds": 2.0,
				"image_path":       "jobs/job_video_sum_123/images/segment_000_shot_001.jpg",
				"source_type":      "image_fallback",
			},
			{
				"segment_index":    1,
				"shot_index":       0,
				"status":           "generated_video",
				"duration_seconds": 3.0,
				"video_path":       "jobs/job_video_sum_123/shot_videos/segment_001_shot_000.mp4",
				"source_type":      "generated_video",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_video_sum_123/images/segment_000_shot_000.jpg",
		"jobs/job_video_sum_123/images/segment_000_shot_001.jpg",
		"jobs/job_video_sum_123/shot_videos/segment_001_shot_000.mp4",
	)

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_sum_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_sum_123/audio/tts_manifest.json",
					"segment_count":          2,
					"audio_segment_paths":    []string{"jobs/job_video_sum_123/audio/segment_000.wav", "jobs/job_video_sum_123/audio/segment_001.wav"},
					"total_duration_seconds": 9.5,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     "jobs/job_video_sum_123/shot_videos/manifest.json",
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["duration_seconds"] != 6.5 {
		t.Fatalf("duration_seconds = %#v, want 6.5", updated.OutputRef["duration_seconds"])
	}
	if updated.OutputRef["narration_duration_seconds"] != 9.5 {
		t.Fatalf("narration_duration_seconds = %#v, want 9.5", updated.OutputRef["narration_duration_seconds"])
	}
	if updated.OutputRef["visual_duration_seconds"] != 6.5 {
		t.Fatalf("visual_duration_seconds = %#v, want 6.5", updated.OutputRef["visual_duration_seconds"])
	}
}

func TestExecuteRendersFinalVideoWithMixedGeneratedAndFallbackClips(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	runner := &fakeCommandRunner{}
	executor := NewExecutorWithRunner(runner, workspaceDir)
	jobID := "job_video_render_mixed_123"

	writeRenderableVideoTestTTSArtifacts(
		t,
		workspaceDir,
		jobID,
		[]map[string]any{
			{
				"segment_index": 0,
				"file_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
				"duration":      6.0,
			},
		},
		6.0,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")), map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "generated_video",
				"duration_seconds": 2.0,
				"video_path":       filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "segment_000_shot_000.mp4")),
				"source_type":      "generated_video",
				"is_fallback":      false,
			},
			{
				"segment_index":     0,
				"shot_index":        1,
				"status":            "image_fallback",
				"duration_seconds":  2.0,
				"image_path":        filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_001.jpg")),
				"source_image_path": filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_001.jpg")),
				"source_type":       "image_fallback",
				"is_fallback":       true,
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "segment_000_shot_000.mp4")),
		filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_001.jpg")),
	)

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: jobID},
		model.Task{
			Key: "video",
			Payload: map[string]any{
				"aspect_ratio": "9:16",
			},
		},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "tts_manifest.json")),
					"segment_count": 1,
					"audio_segment_paths": []string{
						filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
					},
					"total_duration_seconds": 6.0,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")),
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if updated.OutputRef["artifact_path"] != filepath.ToSlash(filepath.Join("jobs", jobID, "output", "final.mp4")) {
		t.Fatalf("artifact_path = %#v", updated.OutputRef["artifact_path"])
	}
	if updated.OutputRef["duration_seconds"] != 4.0 {
		t.Fatalf("duration_seconds = %#v, want 4.0", updated.OutputRef["duration_seconds"])
	}
	if updated.OutputRef["aspect_ratio"] != "9:16" {
		t.Fatalf("aspect_ratio = %#v, want %q", updated.OutputRef["aspect_ratio"], "9:16")
	}
	fileSize, ok := updated.OutputRef["file_size_bytes"].(int64)
	if !ok || fileSize <= 0 {
		t.Fatalf("file_size_bytes = %#v, want positive int64", updated.OutputRef["file_size_bytes"])
	}

	finalPath := filepath.Join(workspaceDir, "jobs", jobID, "output", "final.mp4")
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("Stat(%q) error = %v", finalPath, err)
	}
	if !runner.hasCommandContaining("ffmpeg", "-loop", "segment_000_shot_001.jpg") {
		t.Fatalf("expected fallback image ffmpeg command, got %#v", runner.commands)
	}
	if !runner.hasCommandContaining("ffmpeg", "segment_000_shot_000.mp4", "segment_000_shot_000.mp4") {
		t.Fatalf("expected generated video normalize command, got %#v", runner.commands)
	}
	if !runner.hasCommandContaining("ffprobe", "segment_000_base.mp4") {
		t.Fatalf("expected ffprobe command, got %#v", runner.commands)
	}
	if !runner.hasCommandContaining("crop=720:1280") {
		t.Fatalf("expected portrait crop ffmpeg command, got %#v", runner.commands)
	}
	if !runner.hasCommandContaining("crop=720:1280:x='(in_w-out_w)*(1-min(t/2.000,1))'") {
		t.Fatalf("expected animated portrait fallback crop ffmpeg command, got %#v", runner.commands)
	}
	if !runner.hasCommandContaining("ffmpeg", "merged_audio.wav", "final.mp4") {
		t.Fatalf("expected final mux command, got %#v", runner.commands)
	}
}

func TestExecuteReportsProgress(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	runner := &fakeCommandRunner{}
	executor := NewExecutorWithRunner(runner, workspaceDir)
	jobID := "job_video_progress_123"

	writeRenderableVideoTestTTSArtifacts(
		t,
		workspaceDir,
		jobID,
		[]map[string]any{
			{
				"segment_index": 0,
				"file_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
				"duration":      4.0,
			},
		},
		4.0,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")), map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":     0,
				"shot_index":        0,
				"status":            "image_fallback",
				"duration_seconds":  2.0,
				"image_path":        filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
				"source_image_path": filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
				"source_type":       "image_fallback",
				"is_fallback":       true,
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
	)

	reporter := &recordingProgressReporter{}
	ctx := model.WithTaskProgressReporter(context.Background(), reporter)

	_, err := executor.Execute(
		ctx,
		model.Job{PublicID: jobID},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "tts_manifest.json")),
					"segment_count": 1,
					"audio_segment_paths": []string{
						filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
					},
					"total_duration_seconds": 4.0,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")),
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	progress := reporter.snapshot()
	if len(progress) == 0 {
		t.Fatal("len(progress) = 0, want non-zero")
	}
	assertHasPhase := func(phase string) {
		t.Helper()
		for _, item := range progress {
			if item.Phase == phase {
				return
			}
		}
		t.Fatalf("progress phases = %#v, want phase %q", progress, phase)
	}

	assertHasPhase("validating_dependencies")
	assertHasPhase("rendering_video")
	assertHasPhase("rendering_audio")
	assertHasPhase("rendering_segment")
	assertHasPhase("concatenating_segments")
	assertHasPhase("muxing_video")
	assertHasPhase("finalizing_output")
}

func TestExecuteReturnsErrorWhenProbeSegmentDurationFails(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	runner := &fakeCommandRunner{}
	runner.failOn("ffprobe", fmt.Errorf("ffprobe failed"))
	executor := NewExecutorWithRunner(runner, workspaceDir)
	jobID := "job_video_probe_failure_123"

	writeRenderableVideoTestTTSArtifacts(
		t,
		workspaceDir,
		jobID,
		[]map[string]any{
			{
				"segment_index": 0,
				"file_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
				"duration":      6.0,
			},
		},
		6.0,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")), map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":     0,
				"shot_index":        0,
				"status":            "image_fallback",
				"duration_seconds":  2.0,
				"image_path":        filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
				"source_image_path": filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
				"source_type":       "image_fallback",
				"is_fallback":       true,
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
	)

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: jobID},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "tts_manifest.json")),
					"segment_count": 1,
					"audio_segment_paths": []string{
						filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
					},
					"total_duration_seconds": 6.0,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")),
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "probe media duration") {
		t.Fatalf("error = %v, want probe media duration", err)
	}
}

func TestExecuteReturnsErrorWhenMuxFinalVideoFails(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	runner := &fakeCommandRunner{}
	runner.failOn("+faststart", fmt.Errorf("mux failed"))
	executor := NewExecutorWithRunner(runner, workspaceDir)
	jobID := "job_video_mux_failure_123"

	writeRenderableVideoTestTTSArtifacts(
		t,
		workspaceDir,
		jobID,
		[]map[string]any{
			{
				"segment_index": 0,
				"file_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
				"duration":      6.0,
			},
		},
		6.0,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")), map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "generated_video",
				"duration_seconds": 2.0,
				"video_path":       filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "segment_000_shot_000.mp4")),
				"source_type":      "generated_video",
				"is_fallback":      false,
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "segment_000_shot_000.mp4")),
	)

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: jobID},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "tts_manifest.json")),
					"segment_count": 1,
					"audio_segment_paths": []string{
						filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
					},
					"total_duration_seconds": 6.0,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")),
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "mux final video") {
		t.Fatalf("error = %v, want mux final video", err)
	}
}

func TestExecuteReturnsErrorWhenFinalVideoFileMissing(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	runner := &fakeCommandRunner{}
	runner.skipWriteOn("+faststart")
	executor := NewExecutorWithRunner(runner, workspaceDir)
	jobID := "job_video_missing_output_123"

	writeRenderableVideoTestTTSArtifacts(
		t,
		workspaceDir,
		jobID,
		[]map[string]any{
			{
				"segment_index": 0,
				"file_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
				"duration":      6.0,
			},
		},
		6.0,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")), map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":     0,
				"shot_index":        0,
				"status":            "image_fallback",
				"duration_seconds":  2.0,
				"image_path":        filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
				"source_image_path": filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
				"source_type":       "image_fallback",
				"is_fallback":       true,
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		filepath.ToSlash(filepath.Join("jobs", jobID, "images", "segment_000_shot_000.jpg")),
	)

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: jobID},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "tts_manifest.json")),
					"segment_count": 1,
					"audio_segment_paths": []string{
						filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
					},
					"total_duration_seconds": 6.0,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")),
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing final video file") {
		t.Fatalf("error = %v, want missing final video file", err)
	}
}

func TestExecuteReturnsErrorWhenFinalVideoFileEmpty(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	runner := &fakeCommandRunner{}
	runner.writeEmptyOn("+faststart")
	executor := NewExecutorWithRunner(runner, workspaceDir)
	jobID := "job_video_empty_output_123"

	writeRenderableVideoTestTTSArtifacts(
		t,
		workspaceDir,
		jobID,
		[]map[string]any{
			{
				"segment_index": 0,
				"file_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
				"duration":      6.0,
			},
		},
		6.0,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")), map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "generated_video",
				"duration_seconds": 2.0,
				"video_path":       filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "segment_000_shot_000.mp4")),
				"source_type":      "generated_video",
				"is_fallback":      false,
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "segment_000_shot_000.mp4")),
	)

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: jobID},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path": filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "tts_manifest.json")),
					"segment_count": 1,
					"audio_segment_paths": []string{
						filepath.ToSlash(filepath.Join("jobs", jobID, "audio", "segment_000.wav")),
					},
					"total_duration_seconds": 6.0,
				},
			},
			"shot_video": {
				Key: "shot_video",
				OutputRef: map[string]any{
					"artifact_path":     filepath.ToSlash(filepath.Join("jobs", jobID, "shot_videos", "manifest.json")),
					"image_source_type": "shot_images",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "empty final video file") {
		t.Fatalf("error = %v, want empty final video file", err)
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
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresShotVideoDependency(t *testing.T) {
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

func TestExecuteRequiresTTSArtifactPath(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts":        {Key: "tts", OutputRef: map[string]any{"total_duration_seconds": 8.25}},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresShotVideoArtifactPath(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": ""}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresValidTTSOutputRef(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "image_fallback",
			"duration_seconds": 3.0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts":        {Key: "tts", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/audio/tts_manifest.json"}},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingShotVideoArtifactFileWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingShotFallbackImageFileWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "image_fallback",
			"duration_seconds": 3.0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresClipCoverageMatchingTTSSegmentCount(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(
		t,
		workspaceDir,
		"job_video_123",
		"jobs/job_video_123/audio/segment_000.wav",
		"jobs/job_video_123/audio/segment_001.wav",
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "image_fallback",
			"duration_seconds": 3.0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          2,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav", "jobs/job_video_123/audio/segment_001.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresValidShotVideoArtifactStructureWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"source_type":      "image_fallback",
			"duration_seconds": 3.0,
		}},
	})

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRejectsNonPositiveShotVideoDurationWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "image_fallback",
			"duration_seconds": 0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRejectsUnsortedShotVideoClipsWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{
			{
				"segment_index":    0,
				"shot_index":       1,
				"status":           "image_fallback",
				"duration_seconds": 3.0,
				"image_path":       "jobs/job_video_123/images/segment_000_shot_001.jpg",
				"source_type":      "image_fallback",
			},
			{
				"segment_index":    0,
				"shot_index":       0,
				"status":           "image_fallback",
				"duration_seconds": 3.0,
				"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
				"source_type":      "image_fallback",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_video_123/images/segment_000_shot_001.jpg",
		"jobs/job_video_123/images/segment_000_shot_000.jpg",
	)

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRejectsUnknownShotVideoStatusWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "failed",
			"duration_seconds": 3.0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingTTSArtifactFileWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeVideoTestArtifact(t, workspaceDir, "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "image_fallback",
			"duration_seconds": 3.0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingTTSAudioSegmentFileWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeVideoTestArtifact(t, workspaceDir, "jobs/job_video_123/audio/tts_manifest.json")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/shot_videos/manifest.json", map[string]any{
		"clips": []map[string]any{{
			"segment_index":    0,
			"shot_index":       0,
			"status":           "image_fallback",
			"duration_seconds": 3.0,
			"image_path":       "jobs/job_video_123/images/segment_000_shot_000.jpg",
			"source_type":      "image_fallback",
		}},
	})
	writeVideoTestMediaFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000_shot_000.jpg")

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key: "tts",
				OutputRef: map[string]any{
					"artifact_path":          "jobs/job_video_123/audio/tts_manifest.json",
					"segment_count":          1,
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"shot_video": {Key: "shot_video", OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/shot_videos/manifest.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
