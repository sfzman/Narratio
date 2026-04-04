package config

import "testing"

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
