package config

import (
	"os"
	"path/filepath"
	"testing"
)

var narratioEnvKeys = []string{
	"PORT",
	"DATABASE_DRIVER",
	"DATABASE_DSN",
	"WORKSPACE_DIR",
	"SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS",
	"ENABLE_LIVE_TEXT_GENERATION",
	"ENABLE_LIVE_IMAGE_GENERATION",
	"DASHSCOPE_TEXT_API_KEY",
	"DASHSCOPE_TEXT_BASE_URL",
	"DASHSCOPE_TEXT_MODEL",
	"DASHSCOPE_IMAGE_API_KEY",
	"DASHSCOPE_IMAGE_BASE_URL",
	"DASHSCOPE_IMAGE_MODEL",
	"DASHSCOPE_VIDEO_API_KEY",
	"DASHSCOPE_VIDEO_BASE_URL",
	"DASHSCOPE_VIDEO_MODEL",
	"TTS_API_BASE_URL",
	"TTS_API_KEY",
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("Port = %q", cfg.Port)
	}
	if cfg.ScriptTimeoutPerSegmentSeconds != 200 {
		t.Fatalf("ScriptTimeoutPerSegmentSeconds = %d", cfg.ScriptTimeoutPerSegmentSeconds)
	}
	if cfg.EnableLiveTextGeneration {
		t.Fatal("EnableLiveTextGeneration = true, want false by default")
	}
	if cfg.EnableLiveImageGeneration {
		t.Fatal("EnableLiveImageGeneration = true, want false by default")
	}
	if cfg.DashScopeTextBaseURL != "https://coding.dashscope.aliyuncs.com/v1" {
		t.Fatalf("DashScopeTextBaseURL = %q", cfg.DashScopeTextBaseURL)
	}
	if cfg.DashScopeImageModel != "qwen-image-2.0" {
		t.Fatalf("DashScopeImageModel = %q", cfg.DashScopeImageModel)
	}
	if cfg.DashScopeVideoModel != "wan2.6-i2v-flash" {
		t.Fatalf("DashScopeVideoModel = %q", cfg.DashScopeVideoModel)
	}
}

func TestLoadReadsLiveTextGenerationFlag(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("ENABLE_LIVE_TEXT_GENERATION", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.EnableLiveTextGeneration {
		t.Fatal("EnableLiveTextGeneration = false, want true")
	}
}

func TestLoadReadsScriptTimeoutPerSegmentSeconds(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS", "320")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ScriptTimeoutPerSegmentSeconds != 320 {
		t.Fatalf("ScriptTimeoutPerSegmentSeconds = %d, want 320", cfg.ScriptTimeoutPerSegmentSeconds)
	}
}

func TestLoadFallsBackWhenScriptTimeoutPerSegmentSecondsInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS", "abc")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ScriptTimeoutPerSegmentSeconds != 200 {
		t.Fatalf("ScriptTimeoutPerSegmentSeconds = %d, want 200", cfg.ScriptTimeoutPerSegmentSeconds)
	}
}

func TestLoadReadsLiveImageGenerationFlag(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("ENABLE_LIVE_IMAGE_GENERATION", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.EnableLiveImageGeneration {
		t.Fatal("EnableLiveImageGeneration = false, want true")
	}
}

func TestLoadRequiresDatabaseDriver(t *testing.T) {
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing DATABASE_DRIVER")
	}
}

func TestLoadRejectsInvalidDatabaseDriver(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "postgres")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid driver")
	}
}

func TestLoadReadsDotEnvFile(t *testing.T) {
	unsetEnvKeys(t, narratioEnvKeys...)

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte(
		"PORT=9090\n"+
			"DATABASE_DRIVER=sqlite\n"+
			"DATABASE_DSN=./from-dotenv.db\n"+
			"WORKSPACE_DIR=./dotenv-workspace\n"+
			"DASHSCOPE_TEXT_API_KEY=test-key\n",
	), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("Port = %q", cfg.Port)
	}
	if cfg.DatabaseDSN != "./from-dotenv.db" {
		t.Fatalf("DatabaseDSN = %q", cfg.DatabaseDSN)
	}
	if cfg.DashScopeTextAPIKey != "test-key" {
		t.Fatalf("DashScopeTextAPIKey = %q", cfg.DashScopeTextAPIKey)
	}
}

func TestLoadDoesNotOverrideExistingEnv(t *testing.T) {
	unsetEnvKeys(t, narratioEnvKeys...)

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte(
		"DATABASE_DRIVER=sqlite\n"+
			"DATABASE_DSN=./from-dotenv.db\n"+
			"WORKSPACE_DIR=./dotenv-workspace\n",
	), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("DATABASE_DSN", "./from-env.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DatabaseDSN != "./from-env.db" {
		t.Fatalf("DatabaseDSN = %q", cfg.DatabaseDSN)
	}
}

func TestLoadReadsBackendDotEnvFromRepoRoot(t *testing.T) {
	unsetEnvKeys(t, narratioEnvKeys...)

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	err := os.Mkdir(filepath.Join(tempDir, "backend"), 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	err = os.WriteFile(filepath.Join(tempDir, "backend", ".env"), []byte(
		"DATABASE_DRIVER=sqlite\n"+
			"DATABASE_DSN=./repo-root.db\n"+
			"WORKSPACE_DIR=./workspace\n",
	), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DatabaseDSN != "./repo-root.db" {
		t.Fatalf("DatabaseDSN = %q", cfg.DatabaseDSN)
	}
}

func unsetEnvKeys(t *testing.T, keys ...string) {
	t.Helper()

	previous := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			valueCopy := value
			previous[key] = &valueCopy
		} else {
			previous[key] = nil
		}

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q) error = %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			value := previous[key]
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	})
}
