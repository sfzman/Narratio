package bootstrap

import (
	"context"
	"path/filepath"
	"testing"
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
	if runtime.TextClient == nil {
		t.Fatal("TextClient = nil")
	}
	if runtime.ExecutorRegistry == nil {
		t.Fatal("ExecutorRegistry = nil")
	}
	if runtime.Store == nil {
		t.Fatal("Store = nil")
	}
	if runtime.JobsService == nil {
		t.Fatal("JobsService = nil")
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
