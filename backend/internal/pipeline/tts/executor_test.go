package tts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func writeTTSArtifact(t *testing.T, workspaceDir string, relativePath string, value any) {
	t.Helper()

	fullPath := artifactFullPath(workspaceDir, relativePath)
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

func readTTSArtifact(t *testing.T, workspaceDir string, relativePath string) TTSOutput {
	t.Helper()

	data, err := os.ReadFile(artifactFullPath(workspaceDir, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}

	var value TTSOutput
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", relativePath, err)
	}

	return value
}

func readTTSFile(t *testing.T, workspaceDir string, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(artifactFullPath(workspaceDir, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}

	return string(data)
}

func readTTSBytes(t *testing.T, workspaceDir string, relativePath string) []byte {
	t.Helper()

	data, err := os.ReadFile(artifactFullPath(workspaceDir, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}

	return data
}

func TestExecuteBuildsTTSOutputRef(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
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
		"segmentation": {
			Key: "segmentation",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_tts_123/segments.json",
				"segment_count": 2,
			},
		},
	}
	writeTTSArtifact(t, workspaceDir, "jobs/job_tts_123/segments.json", map[string]any{
		"segments": []map[string]any{
			{"index": 0, "text": "第一段"},
			{"index": 1, "text": "第二段"},
		},
	})

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
	if updated.OutputRef["segmentation_artifact_ref"] != "jobs/job_tts_123/segments.json" {
		t.Fatalf("segmentation_artifact_ref = %#v", updated.OutputRef["segmentation_artifact_ref"])
	}
	if updated.OutputRef["subtitle_artifact_ref"] != "jobs/job_tts_123/audio/subtitles.srt" {
		t.Fatalf("subtitle_artifact_ref = %#v", updated.OutputRef["subtitle_artifact_ref"])
	}
	paths, ok := updated.OutputRef["audio_segment_paths"].([]string)
	if !ok || len(paths) != 2 {
		t.Fatalf("audio_segment_paths = %#v, want 2 paths", updated.OutputRef["audio_segment_paths"])
	}
	artifact := readTTSArtifact(t, workspaceDir, "jobs/job_tts_123/audio/tts_manifest.json")
	if len(artifact.AudioSegments) != 2 {
		t.Fatalf("len(artifact.AudioSegments) = %d, want 2", len(artifact.AudioSegments))
	}
	if artifact.AudioSegments[0].FilePath != "jobs/job_tts_123/audio/segment_000.wav" {
		t.Fatalf("AudioSegments[0].FilePath = %q", artifact.AudioSegments[0].FilePath)
	}
	if artifact.SubtitleItems[1].Text != "第二段" {
		t.Fatalf("SubtitleItems[1].Text = %q", artifact.SubtitleItems[1].Text)
	}
	subtitles := readTTSFile(t, workspaceDir, "jobs/job_tts_123/audio/subtitles.srt")
	if subtitles == "" {
		t.Fatal("subtitles.srt = empty, want non-empty")
	}
	if subtitles != "1\n00:00:00,000 --> 00:00:06,500\n第一段\n\n2\n00:00:06,500 --> 00:00:13,000\n第二段\n\n" {
		t.Fatalf("subtitles.srt = %q", subtitles)
	}
	audioBytes := readTTSBytes(t, workspaceDir, "jobs/job_tts_123/audio/segment_000.wav")
	if len(audioBytes) <= 44 {
		t.Fatalf("segment_000.wav len = %d, want > 44", len(audioBytes))
	}
	if string(audioBytes[:4]) != "RIFF" {
		t.Fatalf("segment_000.wav header = %q, want RIFF", string(audioBytes[:4]))
	}
	if string(audioBytes[8:12]) != "WAVE" {
		t.Fatalf("segment_000.wav format = %q, want WAVE", string(audioBytes[8:12]))
	}
}

func TestExecuteRequiresVoiceID(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutor(workspaceDir)
	writeTTSArtifact(t, workspaceDir, "jobs/job_tts_123/segments.json", map[string]any{
		"segments": []map[string]any{{"index": 0, "text": "第一段"}},
	})
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_tts_123"},
		model.Task{Key: "tts", Payload: map[string]any{}},
		map[string]model.Task{
			"segmentation": {Key: "segmentation", OutputRef: map[string]any{"artifact_path": "jobs/job_tts_123/segments.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresSegmentationDependency(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(t.TempDir())
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

func TestExecuteRequiresSegmentationArtifactFile(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(t.TempDir())
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_tts_123"},
		model.Task{
			Key:     "tts",
			Payload: map[string]any{"voice_id": "reader_a"},
		},
		map[string]model.Task{
			"segmentation": {
				Key: "segmentation",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_tts_123/segments.json",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
