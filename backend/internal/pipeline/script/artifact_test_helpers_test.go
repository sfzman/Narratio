package script

import (
	"encoding/json"
	"os"
	"testing"
)

func readJSONArtifact[T any](t *testing.T, workspaceDir string, artifactPath string) T {
	t.Helper()

	data, err := os.ReadFile(artifactFullPath(workspaceDir, artifactPath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", artifactPath, err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", artifactPath, err)
	}

	return value
}
