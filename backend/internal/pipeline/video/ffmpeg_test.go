package video

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeFFmpegCheckRunner struct {
	output []byte
	err    error
}

func (r fakeFFmpegCheckRunner) Run(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	if name != "ffmpeg" {
		return nil, fmt.Errorf("unexpected command %q", name)
	}
	if len(args) != 1 || args[0] != "-version" {
		return nil, fmt.Errorf("unexpected args %#v", args)
	}
	if r.err != nil {
		return nil, r.err
	}

	return r.output, nil
}

func TestCheckFFmpegAvailableSucceeds(t *testing.T) {
	t.Parallel()

	err := CheckFFmpegAvailable(
		context.Background(),
		fakeFFmpegCheckRunner{output: []byte("ffmpeg version 8.0.1\n")},
	)
	if err != nil {
		t.Fatalf("CheckFFmpegAvailable() error = %v", err)
	}
}

func TestCheckFFmpegAvailableReturnsErrorWhenCommandFails(t *testing.T) {
	t.Parallel()

	err := CheckFFmpegAvailable(
		context.Background(),
		fakeFFmpegCheckRunner{err: fmt.Errorf("not found")},
	)
	if err == nil {
		t.Fatal("CheckFFmpegAvailable() error = nil, want error")
	}
}

func TestCheckFFmpegAvailableReturnsErrorWhenOutputEmpty(t *testing.T) {
	t.Parallel()

	err := CheckFFmpegAvailable(
		context.Background(),
		fakeFFmpegCheckRunner{output: []byte("   \n")},
	)
	if err == nil {
		t.Fatal("CheckFFmpegAvailable() error = nil, want error")
	}
}

func TestWriteConcatFileUsesAbsoluteMediaPaths(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	listPath := filepath.Join(tempDir, "merged_audio.wav.concat.txt")
	relativeClipPath := filepath.Join("workspace", "jobs", "job_video_123", "audio", "segment_000.wav")

	if err := writeConcatFile(listPath, []string{relativeClipPath}); err != nil {
		t.Fatalf("writeConcatFile() error = %v", err)
	}

	data, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", listPath, err)
	}

	content := strings.TrimSpace(string(data))
	if strings.Contains(content, "file 'workspace/") || strings.Contains(content, "file 'workspace\\") {
		t.Fatalf("concat file kept relative media path: %q", content)
	}

	expectedPath, err := filepath.Abs(relativeClipPath)
	if err != nil {
		t.Fatalf("filepath.Abs(%q) error = %v", relativeClipPath, err)
	}
	expectedLine := fmt.Sprintf("file '%s'", strings.ReplaceAll(expectedPath, "'", "'\\''"))
	if content != expectedLine {
		t.Fatalf("concat file = %q, want %q", content, expectedLine)
	}
}
