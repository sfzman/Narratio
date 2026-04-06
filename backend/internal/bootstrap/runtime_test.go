package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestLoadRuntime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "narratio.db")

	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", dbPath)
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_TEXT_BASE_URL", "https://coding.dashscope.aliyuncs.com/v1")
	t.Setenv("DASHSCOPE_TEXT_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_TEXT_MODEL", "qwen-max")

	runtime, err := LoadRuntime()
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if runtime.Config.DashScopeTextModel != "qwen-max" {
		t.Fatalf("DashScopeTextModel = %q", runtime.Config.DashScopeTextModel)
	}
	if runtime.TextClient != nil {
		t.Fatal("TextClient != nil, want skeleton default")
	}
	if runtime.ExecutorRegistry == nil {
		t.Fatal("ExecutorRegistry = nil")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeTTS); !ok {
		t.Fatal("TTS executor not registered")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeImage); !ok {
		t.Fatal("image executor not registered")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeVideo); !ok {
		t.Fatal("video executor not registered")
	}
	if runtime.Store == nil {
		t.Fatal("Store = nil")
	}
	if runtime.JobsService == nil {
		t.Fatal("JobsService = nil")
	}
	if runtime.BackgroundRunner == nil {
		t.Fatal("BackgroundRunner = nil")
	}
	if runtime.RunCoordinator == nil {
		t.Fatal("RunCoordinator = nil")
	}
	if runtime.SchedulerService == nil {
		t.Fatal("SchedulerService = nil")
	}
	if runtime.Router == nil {
		t.Fatal("Router = nil")
	}
	if _, err := runtime.Store.GetJobByPublicID(context.Background(), "missing"); err == nil {
		t.Fatal("GetJobByPublicID() error = nil, want missing row")
	}
}

func TestLoadRuntimeBuildsTextClientWhenEnabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "narratio-live.db")

	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", dbPath)
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("ENABLE_LIVE_TEXT_GENERATION", "true")
	t.Setenv("DASHSCOPE_TEXT_BASE_URL", "https://coding.dashscope.aliyuncs.com/v1")
	t.Setenv("DASHSCOPE_TEXT_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_TEXT_MODEL", "qwen-max")

	runtime, err := LoadRuntime()
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if runtime.TextClient == nil {
		t.Fatal("TextClient = nil, want live client when enabled")
	}
}
