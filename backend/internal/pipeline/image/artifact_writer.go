package image

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
