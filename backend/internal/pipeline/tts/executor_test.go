package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type fakeClient struct {
	requests  []Request
	responses [][]byte
	err       error
	errAfter  int
}

func (f *fakeClient) Synthesize(_ context.Context, request Request) ([]byte, error) {
	f.requests = append(f.requests, request)
	if f.err != nil && (f.errAfter <= 0 || len(f.requests) > f.errAfter) {
		return nil, f.err
	}
	if len(f.responses) == 0 {
		return nil, fmt.Errorf("no fake tts response configured")
	}

	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

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
	if updated.OutputRef["generation_mode"] != "placeholder" {
		t.Fatalf("generation_mode = %#v, want %q", updated.OutputRef["generation_mode"], "placeholder")
	}
	if updated.OutputRef["segmentation_artifact_ref"] != "jobs/job_tts_123/segments.json" {
		t.Fatalf("segmentation_artifact_ref = %#v", updated.OutputRef["segmentation_artifact_ref"])
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

func TestExecuteSynthesizesSegmentsSentenceBySentenceWhenClientInjected(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	firstSentence, err := buildSilentWAV(1.0)
	if err != nil {
		t.Fatalf("buildSilentWAV(firstSentence) error = %v", err)
	}
	secondSentence, err := buildSilentWAV(1.5)
	if err != nil {
		t.Fatalf("buildSilentWAV(secondSentence) error = %v", err)
	}
	thirdSentence, err := buildSilentWAV(0.75)
	if err != nil {
		t.Fatalf("buildSilentWAV(thirdSentence) error = %v", err)
	}
	client := &fakeClient{
		responses: [][]byte{
			firstSentence,
			secondSentence,
			thirdSentence,
		},
	}
	executor := NewExecutorWithClient(client, workspaceDir)
	job := model.Job{
		ID:       2,
		PublicID: "job_tts_live_123",
	}
	task := model.Task{
		ID:      12,
		Key:     "tts",
		Payload: map[string]any{"voice_id": "reader_b"},
	}
	dependencies := map[string]model.Task{
		"segmentation": {
			Key: "segmentation",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_tts_live_123/segments.json",
			},
		},
	}
	writeTTSArtifact(t, workspaceDir, "jobs/job_tts_live_123/segments.json", map[string]any{
		"segments": []map[string]any{
			{"index": 0, "text": "第一句。第二句。"},
			{"index": 1, "text": "第三句"},
		},
	})

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["generation_mode"] != "sentence_serial" {
		t.Fatalf("generation_mode = %#v, want %q", updated.OutputRef["generation_mode"], "sentence_serial")
	}
	if len(client.requests) != 3 {
		t.Fatalf("len(client.requests) = %d, want 3", len(client.requests))
	}
	if client.requests[0].Text != "第一句。" {
		t.Fatalf("client.requests[0].Text = %q", client.requests[0].Text)
	}
	if client.requests[1].Text != "第二句。" {
		t.Fatalf("client.requests[1].Text = %q", client.requests[1].Text)
	}
	if client.requests[2].Text != "第三句" {
		t.Fatalf("client.requests[2].Text = %q", client.requests[2].Text)
	}

	artifact := readTTSArtifact(t, workspaceDir, "jobs/job_tts_live_123/audio/tts_manifest.json")
	if len(artifact.AudioSegments) != 2 {
		t.Fatalf("len(artifact.AudioSegments) = %d, want 2", len(artifact.AudioSegments))
	}
	if artifact.AudioSegments[0].Duration < 2.59 || artifact.AudioSegments[0].Duration > 2.61 {
		t.Fatalf("AudioSegments[0].Duration = %f, want about 2.6", artifact.AudioSegments[0].Duration)
	}
	if artifact.AudioSegments[1].Duration < 0.74 || artifact.AudioSegments[1].Duration > 0.76 {
		t.Fatalf("AudioSegments[1].Duration = %f, want about 0.75", artifact.AudioSegments[1].Duration)
	}
	if artifact.TotalDuration < 3.34 || artifact.TotalDuration > 3.36 {
		t.Fatalf("TotalDuration = %f, want about 3.35", artifact.TotalDuration)
	}
	if artifact.SubtitleItems[0].Text != "第一句。第二句。" {
		t.Fatalf("SubtitleItems[0].Text = %q", artifact.SubtitleItems[0].Text)
	}
	if artifact.SubtitleItems[1].Start < 2.59 || artifact.SubtitleItems[1].Start > 2.61 {
		t.Fatalf("SubtitleItems[1].Start = %f, want about 2.6", artifact.SubtitleItems[1].Start)
	}
	audioBytes := readTTSBytes(t, workspaceDir, "jobs/job_tts_live_123/audio/segment_000.wav")
	if string(audioBytes[:4]) != "RIFF" {
		t.Fatalf("segment_000.wav header = %q, want RIFF", string(audioBytes[:4]))
	}
}

func TestExecutePersistsCompletedSegmentsBeforeLaterSegmentFails(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	firstSentence, err := buildSilentWAV(1.0)
	if err != nil {
		t.Fatalf("buildSilentWAV(firstSentence) error = %v", err)
	}
	client := &fakeClient{
		responses: [][]byte{firstSentence},
		err:       fmt.Errorf("upstream unavailable"),
		errAfter:  1,
	}
	executor := NewExecutorWithClient(client, workspaceDir)
	writeTTSArtifact(t, workspaceDir, "jobs/job_tts_partial/segments.json", map[string]any{
		"segments": []map[string]any{
			{"index": 0, "text": "第一句"},
			{"index": 1, "text": "第二句"},
		},
	})

	_, err = executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_tts_partial"},
		model.Task{Key: "tts", Payload: map[string]any{"voice_id": "default"}},
		map[string]model.Task{
			"segmentation": {Key: "segmentation", OutputRef: map[string]any{"artifact_path": "jobs/job_tts_partial/segments.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "synthesize segment 1") {
		t.Fatalf("err = %v", err)
	}

	artifact := readTTSArtifact(t, workspaceDir, "jobs/job_tts_partial/audio/tts_manifest.json")
	if len(artifact.AudioSegments) != 1 {
		t.Fatalf("len(artifact.AudioSegments) = %d, want 1", len(artifact.AudioSegments))
	}
	if artifact.AudioSegments[0].FilePath != "jobs/job_tts_partial/audio/segment_000.wav" {
		t.Fatalf("AudioSegments[0].FilePath = %q", artifact.AudioSegments[0].FilePath)
	}
	audioBytes := readTTSBytes(t, workspaceDir, "jobs/job_tts_partial/audio/segment_000.wav")
	if string(audioBytes[:4]) != "RIFF" {
		t.Fatalf("segment_000.wav header = %q, want RIFF", string(audioBytes[:4]))
	}
}

func TestSplitSentencesByPeriod(t *testing.T) {
	t.Parallel()

	got := splitSentencesByPeriod("第一句。\n第二句。第三句")
	want := []string{"第一句。", "第二句。", "第三句"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("got[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestExecuteReturnsClientError(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewExecutorWithClient(&fakeClient{err: fmt.Errorf("upstream unavailable")}, workspaceDir)
	writeTTSArtifact(t, workspaceDir, "jobs/job_tts_123/segments.json", map[string]any{
		"segments": []map[string]any{{"index": 0, "text": "第一句。第二句。"}},
	})

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_tts_123"},
		model.Task{Key: "tts", Payload: map[string]any{"voice_id": "reader_a"}},
		map[string]model.Task{
			"segmentation": {Key: "segmentation", OutputRef: map[string]any{"artifact_path": "jobs/job_tts_123/segments.json"}},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "synthesize segment 0") {
		t.Fatalf("err = %v", err)
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
