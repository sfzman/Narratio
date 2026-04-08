package tts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type artifactWriter struct {
	workspaceDir string
}

func newArtifactWriter(workspaceDir string) artifactWriter {
	return artifactWriter{workspaceDir: defaultWorkspaceDir(workspaceDir)}
}

func (w artifactWriter) WriteJSON(relativePath string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal artifact json: %w", err)
	}
	data = append(data, '\n')

	return w.WriteBytes(relativePath, data)
}

func (w artifactWriter) WriteBytes(relativePath string, data []byte) error {
	fullPath := artifactFullPath(w.workspaceDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("write artifact file: %w", err)
	}

	return nil
}

func loadArtifactJSON[T any](workspaceDir string, ref any) (T, error) {
	var zero T

	path, ok := ref.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return zero, fmt.Errorf("artifact ref is invalid: %v", ref)
	}

	data, err := os.ReadFile(artifactFullPath(workspaceDir, path))
	if err != nil {
		return zero, fmt.Errorf("read artifact file: %w", err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return zero, fmt.Errorf("decode artifact json: %w", err)
	}

	return value, nil
}

func artifactFullPath(workspaceDir string, relativePath string) string {
	cleanRelative := filepath.Clean(strings.TrimSpace(relativePath))
	return filepath.Join(defaultWorkspaceDir(workspaceDir), cleanRelative)
}

func defaultWorkspaceDir(workspaceDir string) string {
	trimmed := strings.TrimSpace(workspaceDir)
	if trimmed != "" {
		return trimmed
	}

	return filepath.Join(os.TempDir(), "narratio-workspace")
}
