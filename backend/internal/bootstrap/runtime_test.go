package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/sfzman/Narratio/backend/internal/config"
	"github.com/sfzman/Narratio/backend/internal/model"
)

func stubFFmpegProbe(t *testing.T, err error, expectedTimeout time.Duration) {
	t.Helper()

	previous := probeFFmpegAvailability
	probeFFmpegAvailability = func(timeout time.Duration) error {
		if expectedTimeout > 0 && timeout != expectedTimeout {
			t.Fatalf("probe timeout = %s, want %s", timeout, expectedTimeout)
		}
		return err
	}
	t.Cleanup(func() {
		probeFFmpegAvailability = previous
	})
}

func TestLoadRuntime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "narratio.db")
	stubFFmpegProbe(t, nil, 10*time.Second)

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
	if runtime.ImageClient != nil {
		t.Fatal("ImageClient != nil, want skeleton default")
	}
	if runtime.VideoClient != nil {
		t.Fatal("VideoClient != nil, want skeleton default")
	}
	if runtime.ExecutorRegistry == nil {
		t.Fatal("ExecutorRegistry = nil")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeSegmentation); !ok {
		t.Fatal("segmentation executor not registered")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeCharacterImage); !ok {
		t.Fatal("character_image executor not registered")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeTTS); !ok {
		t.Fatal("TTS executor not registered")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeImage); !ok {
		t.Fatal("image executor not registered")
	}
	if _, ok := runtime.ExecutorRegistry.Get(model.TaskTypeShotVideo); !ok {
		t.Fatal("shot_video executor not registered")
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
	stubFFmpegProbe(t, nil, 10*time.Second)

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

func TestLoadRuntimeBuildsImageClientWhenEnabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "narratio-image.db")
	stubFFmpegProbe(t, nil, 10*time.Second)

	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", dbPath)
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("ENABLE_LIVE_IMAGE_GENERATION", "true")
	t.Setenv("DASHSCOPE_IMAGE_BASE_URL", "https://dashscope.aliyuncs.com/api/v1")
	t.Setenv("DASHSCOPE_IMAGE_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_IMAGE_MODEL", "qwen-image-2.0")

	runtime, err := LoadRuntime()
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if runtime.ImageClient == nil {
		t.Fatal("ImageClient = nil, want live client when enabled")
	}
}

func TestLoadRuntimeBuildsVideoClientWhenEnabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "narratio-video.db")
	stubFFmpegProbe(t, nil, 10*time.Second)

	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", dbPath)
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("ENABLE_LIVE_VIDEO_GENERATION", "true")
	t.Setenv("DASHSCOPE_VIDEO_BASE_URL", "https://dashscope.aliyuncs.com")
	t.Setenv("DASHSCOPE_VIDEO_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_VIDEO_MODEL", "wan2.6-i2v-flash")

	runtime, err := LoadRuntime()
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if runtime.VideoClient == nil {
		t.Fatal("VideoClient = nil, want live client when enabled")
	}
}

func TestLoadRuntimeReturnsErrorWhenFFmpegStartupCheckFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "narratio-ffmpeg-fail.db")
	stubFFmpegProbe(t, fmt.Errorf("ffmpeg missing"), 15*time.Second)

	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", dbPath)
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS", "15")

	_, err := LoadRuntime()
	if err == nil {
		t.Fatal("LoadRuntime() error = nil, want ffmpeg startup check error")
	}
}

func TestDefaultResourceLimitsUsesConfig(t *testing.T) {
	limits := defaultResourceLimits(&config.Config{
		ResourceLocalCPUConcurrency:    6,
		ResourceLLMTextConcurrency:     4,
		ResourceTTSConcurrency:         5,
		ResourceImageGenConcurrency:    3,
		ResourceVideoGenConcurrency:    2,
		ResourceVideoRenderConcurrency: 2,
	})

	if limits[model.ResourceLocalCPU] != 6 {
		t.Fatalf("local_cpu limit = %d, want 6", limits[model.ResourceLocalCPU])
	}
	if limits[model.ResourceLLMText] != 4 {
		t.Fatalf("llm_text limit = %d, want 4", limits[model.ResourceLLMText])
	}
	if limits[model.ResourceTTS] != 5 {
		t.Fatalf("tts limit = %d, want 5", limits[model.ResourceTTS])
	}
	if limits[model.ResourceImageGen] != 3 {
		t.Fatalf("image_gen limit = %d, want 3", limits[model.ResourceImageGen])
	}
	if limits[model.ResourceVideoGen] != 2 {
		t.Fatalf("video_gen limit = %d, want 2", limits[model.ResourceVideoGen])
	}
	if limits[model.ResourceVideoRender] != 2 {
		t.Fatalf("video_render limit = %d, want 2", limits[model.ResourceVideoRender])
	}
}

func TestHealthResourceLimitsUsesConfig(t *testing.T) {
	resources := healthResourceLimits(&config.Config{
		ResourceLocalCPUConcurrency:    6,
		ResourceLLMTextConcurrency:     4,
		ResourceTTSConcurrency:         5,
		ResourceImageGenConcurrency:    3,
		ResourceVideoGenConcurrency:    2,
		ResourceVideoRenderConcurrency: 2,
	})

	if resources["local_cpu"] != 6 {
		t.Fatalf("local_cpu health limit = %d, want 6", resources["local_cpu"])
	}
	if resources["llm_text"] != 4 {
		t.Fatalf("llm_text health limit = %d, want 4", resources["llm_text"])
	}
	if resources["tts"] != 5 {
		t.Fatalf("tts health limit = %d, want 5", resources["tts"])
	}
	if resources["image_gen"] != 3 {
		t.Fatalf("image_gen health limit = %d, want 3", resources["image_gen"])
	}
	if resources["video_gen"] != 2 {
		t.Fatalf("video_gen health limit = %d, want 2", resources["video_gen"])
	}
	if resources["video_render"] != 2 {
		t.Fatalf("video_render health limit = %d, want 2", resources["video_render"])
	}
}
