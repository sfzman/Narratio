package video

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
		t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
	}
}

func writeValidVideoTestTTSArtifacts(t *testing.T, workspaceDir string, jobPublicID string, audioPaths ...string) {
	t.Helper()

	writeVideoTestArtifact(t, workspaceDir, filepath.Join("jobs", jobPublicID, "audio", "tts_manifest.json"))
	writeVideoTestArtifact(t, workspaceDir, filepath.Join("jobs", jobPublicID, "audio", "subtitles.srt"))
	for _, audioPath := range audioPaths {
		writeVideoTestArtifact(t, workspaceDir, audioPath)
	}
}

func writeVideoTestImageFiles(t *testing.T, workspaceDir string, imagePaths ...string) {
	t.Helper()

	for _, imagePath := range imagePaths {
		writeVideoTestArtifact(t, workspaceDir, imagePath)
	}
}

func TestExecuteBuildsVideoOutputRef(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
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
				"segment_count":          1,
				"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
				"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
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
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_123/images/segment_000.jpg"},
		},
	})
	writeVideoTestImageFiles(t, workspaceDir, "jobs/job_video_123/images/segment_000.jpg")

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

func TestExecuteAcceptsImageArtifactSummaryFromGeneratedOrFallbackImages(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_video_image_summary_123",
	}
	task := model.Task{
		ID:  32,
		Key: "video",
	}
	dependencies := map[string]model.Task{
		"tts": {
			Key: "tts",
			OutputRef: map[string]any{
				"artifact_path":         "jobs/job_video_image_summary_123/audio/tts_manifest.json",
				"segment_count":         3,
				"subtitle_artifact_ref": "jobs/job_video_image_summary_123/audio/subtitles.srt",
				"audio_segment_paths": []string{
					"jobs/job_video_image_summary_123/audio/segment_000.wav",
					"jobs/job_video_image_summary_123/audio/segment_001.wav",
					"jobs/job_video_image_summary_123/audio/segment_002.wav",
				},
				"total_duration_seconds": 6.5,
			},
		},
		"image": {
			Key: "image",
			OutputRef: map[string]any{
				"artifact_path":         "jobs/job_video_image_summary_123/images/image_manifest.json",
				"generated_image_count": 1,
				"fallback_image_count":  2,
				"images": []map[string]any{
					{"file_path": "jobs/job_video_image_summary_123/images/segment_000.jpg", "is_fallback": false},
					{"file_path": "jobs/job_video_image_summary_123/images/segment_001.jpg", "is_fallback": true},
					{"file_path": "jobs/job_video_image_summary_123/images/segment_002.jpg", "is_fallback": true},
				},
			},
		},
	}
	writeValidVideoTestTTSArtifacts(
		t,
		workspaceDir,
		"job_video_image_summary_123",
		"jobs/job_video_image_summary_123/audio/segment_000.wav",
		"jobs/job_video_image_summary_123/audio/segment_001.wav",
		"jobs/job_video_image_summary_123/audio/segment_002.wav",
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_image_summary_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_image_summary_123/images/segment_000.jpg"},
			{"file_path": "jobs/job_video_image_summary_123/images/segment_001.jpg"},
			{"file_path": "jobs/job_video_image_summary_123/images/segment_002.jpg"},
		},
	})
	writeVideoTestImageFiles(
		t,
		workspaceDir,
		"jobs/job_video_image_summary_123/images/segment_000.jpg",
		"jobs/job_video_image_summary_123/images/segment_001.jpg",
		"jobs/job_video_image_summary_123/images/segment_002.jpg",
	)

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["image_artifact_ref"] != "jobs/job_video_image_summary_123/images/image_manifest.json" {
		t.Fatalf("image_artifact_ref = %#v", updated.OutputRef["image_artifact_ref"])
	}
	if updated.OutputRef["duration_seconds"] != 6.5 {
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

func TestExecuteRequiresTTSArtifactPath(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key:       "tts",
				OutputRef: map[string]any{"total_duration_seconds": 8.25},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresImageArtifactPath(t *testing.T) {
	t.Parallel()

	executor := NewExecutor()
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key:       "tts",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/audio/tts_manifest.json"},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": ""},
			},
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
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_123/images/segment_000.jpg"},
		},
	})

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_video_123"},
		model.Task{Key: "video"},
		map[string]model.Task{
			"tts": {
				Key:       "tts",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/audio/tts_manifest.json"},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingImageArtifactFileWhenWorkspaceConfigured(t *testing.T) {
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
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingImageFileWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_123/images/segment_000.jpg"},
		},
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
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresImageCountMatchingTTSSegmentCountWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_123/images/segment_000.jpg"},
			{"file_path": "jobs/job_video_123/images/segment_001.jpg"},
		},
	})
	writeVideoTestImageFiles(
		t,
		workspaceDir,
		"jobs/job_video_123/images/segment_000.jpg",
		"jobs/job_video_123/images/segment_001.jpg",
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
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresAudioSegmentPathCountMatchingSegmentCount(t *testing.T) {
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
					"segment_count":          2,
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresExistingTTSSubtitleArtifactFileWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeVideoTestArtifact(t, workspaceDir, "jobs/job_video_123/audio/tts_manifest.json")
	writeVideoTestArtifact(t, workspaceDir, "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_123/images/segment_000.jpg"},
		},
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
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
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
	writeVideoTestArtifact(t, workspaceDir, "jobs/job_video_123/audio/subtitles.srt")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{
			{"file_path": "jobs/job_video_123/images/segment_000.jpg"},
		},
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
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresValidImageArtifactStructureWhenWorkspaceConfigured(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeValidVideoTestTTSArtifacts(t, workspaceDir, "job_video_123", "jobs/job_video_123/audio/segment_000.wav")
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{},
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
					"subtitle_artifact_ref":  "jobs/job_video_123/audio/subtitles.srt",
					"audio_segment_paths":    []string{"jobs/job_video_123/audio/segment_000.wav"},
					"total_duration_seconds": 6.5,
				},
			},
			"image": {
				Key:       "image",
				OutputRef: map[string]any{"artifact_path": "jobs/job_video_123/images/image_manifest.json"},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
