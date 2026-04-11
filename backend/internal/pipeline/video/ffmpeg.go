package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultFinalVideoMaxEdge = 1280
	defaultFinalVideoFPS     = 24
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type realCommandRunner struct{}

func (r realCommandRunner) Run(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func CheckFFmpegAvailable(
	ctx context.Context,
	runner CommandRunner,
) error {
	if runner == nil {
		runner = realCommandRunner{}
	}
	output, err := runner.Run(ctx, "ffmpeg", "-version")
	if err != nil {
		return fmt.Errorf("run ffmpeg -version: %w", err)
	}
	if strings.TrimSpace(string(output)) == "" {
		return fmt.Errorf("empty ffmpeg -version output")
	}

	return nil
}

func commandError(label string, err error, output []byte) error {
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("%s: %w", label, err)
	}

	return fmt.Errorf("%s: %w: %s", label, err, trimmed)
}

func writeConcatFile(listPath string, clipPaths []string) error {
	lines := make([]string, 0, len(clipPaths))
	for _, clipPath := range clipPaths {
		absolutePath, err := filepath.Abs(filepath.Clean(clipPath))
		if err != nil {
			return fmt.Errorf("resolve concat media path: %w", err)
		}
		escaped := strings.ReplaceAll(absolutePath, "'", "'\\''")
		lines = append(lines, fmt.Sprintf("file '%s'", escaped))
	}
	return os.WriteFile(listPath, []byte(strings.Join(lines, "\n")), 0o644)
}

func concatVideoClips(
	ctx context.Context,
	runner CommandRunner,
	outputPath string,
	clipPaths []string,
) error {
	listPath := outputPath + ".concat.txt"
	if err := writeConcatFile(listPath, clipPaths); err != nil {
		return fmt.Errorf("write video concat file: %w", err)
	}
	if _, err := runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-f",
		"concat",
		"-safe",
		"0",
		"-i",
		listPath,
		"-c",
		"copy",
		outputPath,
	); err == nil {
		return nil
	}

	output, err := runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-f",
		"concat",
		"-safe",
		"0",
		"-i",
		listPath,
		"-an",
		"-c:v",
		"libx264",
		"-preset",
		"veryfast",
		"-crf",
		"18",
		"-pix_fmt",
		"yuv420p",
		outputPath,
	)
	if err != nil {
		return commandError("concat video clips", err, output)
	}

	return nil
}

func concatAudioSegments(
	ctx context.Context,
	runner CommandRunner,
	outputPath string,
	audioPaths []string,
) error {
	listPath := outputPath + ".concat.txt"
	if err := writeConcatFile(listPath, audioPaths); err != nil {
		return fmt.Errorf("write audio concat file: %w", err)
	}
	output, err := runner.Run(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel",
		"error",
		"-f",
		"concat",
		"-safe",
		"0",
		"-i",
		listPath,
		"-c:a",
		"pcm_s16le",
		outputPath,
	)
	if err != nil {
		return commandError("concat audio segments", err, output)
	}

	return nil
}

func probeMediaDuration(
	ctx context.Context,
	runner CommandRunner,
	path string,
) (float64, error) {
	output, err := runner.Run(
		ctx,
		"ffprobe",
		"-v",
		"error",
		"-show_entries",
		"format=duration",
		"-of",
		"default=noprint_wrappers=1:nokey=1",
		path,
	)
	if err != nil {
		return 0, commandError("probe media duration", err, output)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid media duration for %s", filepath.Base(path))
	}

	return duration, nil
}
