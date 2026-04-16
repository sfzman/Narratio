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
	"BACKGROUND_RUNNER_WORKER_COUNT",
	"RESOURCE_LOCAL_CPU_CONCURRENCY",
	"RESOURCE_LLM_TEXT_CONCURRENCY",
	"RESOURCE_TTS_CONCURRENCY",
	"RESOURCE_IMAGE_GEN_CONCURRENCY",
	"RESOURCE_VIDEO_GEN_CONCURRENCY",
	"RESOURCE_VIDEO_RENDER_CONCURRENCY",
	"SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS",
	"SHOT_VIDEO_TIMEOUT_PER_SHOT_SECONDS",
	"VIDEO_RENDER_TIMEOUT_SECONDS",
	"FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS",
	"SHOT_VIDEO_DEFAULT_DURATION_SECONDS",
	"ENABLE_LIVE_TEXT_GENERATION",
	"ENABLE_LIVE_IMAGE_GENERATION",
	"ENABLE_LIVE_VIDEO_GENERATION",
	"DASHSCOPE_TEXT_API_KEY",
	"DASHSCOPE_TEXT_BASE_URL",
	"DASHSCOPE_TEXT_MODEL",
	"DASHSCOPE_TEXT_REQUEST_TIMEOUT_SECONDS",
	"DASHSCOPE_TEXT_MAX_RETRIES",
	"DASHSCOPE_TEXT_RETRY_BACKOFF_SECONDS",
	"DASHSCOPE_IMAGE_API_KEY",
	"DASHSCOPE_IMAGE_BASE_URL",
	"DASHSCOPE_IMAGE_MODEL",
	"DASHSCOPE_VIDEO_API_KEY",
	"DASHSCOPE_VIDEO_BASE_URL",
	"DASHSCOPE_VIDEO_MODEL",
	"DASHSCOPE_VIDEO_SUBMIT_TIMEOUT_SECONDS",
	"DASHSCOPE_VIDEO_POLL_INTERVAL_SECONDS",
	"DASHSCOPE_VIDEO_MAX_WAIT_SECONDS",
	"DASHSCOPE_VIDEO_MAX_REQUEST_BYTES",
	"DASHSCOPE_VIDEO_RESOLUTION",
	"DASHSCOPE_VIDEO_NEGATIVE_PROMPT",
	"DASHSCOPE_VIDEO_IMAGE_JPEG_QUALITY",
	"DASHSCOPE_VIDEO_IMAGE_MIN_JPEG_QUALITY",
	"TTS_API_BASE_URL",
	"TTS_JWT_PRIVATE_KEY",
	"TTS_JWT_EXPIRE_SECONDS",
	"TTS_REQUEST_TIMEOUT_SECONDS",
	"TTS_MAX_RETRIES",
	"TTS_RETRY_BACKOFF_SECONDS",
	"TTS_DEFAULT_VOICE_ID",
	"TTS_EMOTION_PROMPT",
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
	if cfg.BackgroundRunnerWorkerCount != 4 {
		t.Fatalf("BackgroundRunnerWorkerCount = %d", cfg.BackgroundRunnerWorkerCount)
	}
	if cfg.ScriptTimeoutPerSegmentSeconds != 200 {
		t.Fatalf("ScriptTimeoutPerSegmentSeconds = %d", cfg.ScriptTimeoutPerSegmentSeconds)
	}
	if cfg.ResourceLocalCPUConcurrency != 4 {
		t.Fatalf("ResourceLocalCPUConcurrency = %d", cfg.ResourceLocalCPUConcurrency)
	}
	if cfg.ResourceLLMTextConcurrency != 2 {
		t.Fatalf("ResourceLLMTextConcurrency = %d", cfg.ResourceLLMTextConcurrency)
	}
	if cfg.ResourceTTSConcurrency != 3 {
		t.Fatalf("ResourceTTSConcurrency = %d", cfg.ResourceTTSConcurrency)
	}
	if cfg.ResourceImageGenConcurrency != 2 {
		t.Fatalf("ResourceImageGenConcurrency = %d", cfg.ResourceImageGenConcurrency)
	}
	if cfg.ResourceVideoGenConcurrency != 1 {
		t.Fatalf("ResourceVideoGenConcurrency = %d", cfg.ResourceVideoGenConcurrency)
	}
	if cfg.ResourceVideoRenderConcurrency != 1 {
		t.Fatalf("ResourceVideoRenderConcurrency = %d", cfg.ResourceVideoRenderConcurrency)
	}
	if cfg.VideoRenderTimeoutSeconds != 1800 {
		t.Fatalf("VideoRenderTimeoutSeconds = %d", cfg.VideoRenderTimeoutSeconds)
	}
	if cfg.ShotVideoTimeoutPerShotSeconds != 200 {
		t.Fatalf("ShotVideoTimeoutPerShotSeconds = %d", cfg.ShotVideoTimeoutPerShotSeconds)
	}
	if cfg.FFmpegStartupCheckTimeoutSeconds != 10 {
		t.Fatalf("FFmpegStartupCheckTimeoutSeconds = %d", cfg.FFmpegStartupCheckTimeoutSeconds)
	}
	if cfg.ShotVideoDefaultDurationSeconds != 3 {
		t.Fatalf("ShotVideoDefaultDurationSeconds = %d", cfg.ShotVideoDefaultDurationSeconds)
	}
	if cfg.EnableLiveTextGeneration {
		t.Fatal("EnableLiveTextGeneration = true, want false by default")
	}
	if cfg.EnableLiveImageGeneration {
		t.Fatal("EnableLiveImageGeneration = true, want false by default")
	}
	if cfg.EnableLiveVideoGeneration {
		t.Fatal("EnableLiveVideoGeneration = true, want false by default")
	}
	if cfg.DashScopeTextBaseURL != "https://coding.dashscope.aliyuncs.com/v1" {
		t.Fatalf("DashScopeTextBaseURL = %q", cfg.DashScopeTextBaseURL)
	}
	if cfg.DashScopeTextRequestTimeoutSeconds != 600 {
		t.Fatalf("DashScopeTextRequestTimeoutSeconds = %d", cfg.DashScopeTextRequestTimeoutSeconds)
	}
	if cfg.DashScopeTextMaxRetries != 2 {
		t.Fatalf("DashScopeTextMaxRetries = %d", cfg.DashScopeTextMaxRetries)
	}
	if cfg.DashScopeTextRetryBackoffSeconds != 2 {
		t.Fatalf("DashScopeTextRetryBackoffSeconds = %d", cfg.DashScopeTextRetryBackoffSeconds)
	}
	if cfg.DashScopeImageModel != "qwen-image-2.0" {
		t.Fatalf("DashScopeImageModel = %q", cfg.DashScopeImageModel)
	}
	if cfg.DashScopeVideoModel != "wan2.6-i2v-flash" {
		t.Fatalf("DashScopeVideoModel = %q", cfg.DashScopeVideoModel)
	}
	if cfg.DashScopeVideoSubmitTimeoutSeconds != 60 {
		t.Fatalf("DashScopeVideoSubmitTimeoutSeconds = %d", cfg.DashScopeVideoSubmitTimeoutSeconds)
	}
	if cfg.DashScopeVideoPollIntervalSeconds != 10 {
		t.Fatalf("DashScopeVideoPollIntervalSeconds = %d", cfg.DashScopeVideoPollIntervalSeconds)
	}
	if cfg.DashScopeVideoMaxWaitSeconds != 900 {
		t.Fatalf("DashScopeVideoMaxWaitSeconds = %d", cfg.DashScopeVideoMaxWaitSeconds)
	}
	if cfg.DashScopeVideoMaxRequestBytes != 6291456 {
		t.Fatalf("DashScopeVideoMaxRequestBytes = %d", cfg.DashScopeVideoMaxRequestBytes)
	}
	if cfg.DashScopeVideoResolution != "720P" {
		t.Fatalf("DashScopeVideoResolution = %q", cfg.DashScopeVideoResolution)
	}
	if cfg.DashScopeVideoNegativePrompt != "" {
		t.Fatalf("DashScopeVideoNegativePrompt = %q", cfg.DashScopeVideoNegativePrompt)
	}
	if cfg.DashScopeVideoImageJPEGQuality != 80 {
		t.Fatalf("DashScopeVideoImageJPEGQuality = %d", cfg.DashScopeVideoImageJPEGQuality)
	}
	if cfg.DashScopeVideoImageMinJPEGQuality != 45 {
		t.Fatalf("DashScopeVideoImageMinJPEGQuality = %d", cfg.DashScopeVideoImageMinJPEGQuality)
	}
	if cfg.TTSRequestTimeoutSeconds != 300 {
		t.Fatalf("TTSRequestTimeoutSeconds = %d", cfg.TTSRequestTimeoutSeconds)
	}
	if cfg.TTSMaxRetries != 2 {
		t.Fatalf("TTSMaxRetries = %d", cfg.TTSMaxRetries)
	}
	if cfg.TTSRetryBackoffSeconds != 2 {
		t.Fatalf("TTSRetryBackoffSeconds = %d", cfg.TTSRetryBackoffSeconds)
	}
	if cfg.TTSJWTExpireSeconds != 300 {
		t.Fatalf("TTSJWTExpireSeconds = %d", cfg.TTSJWTExpireSeconds)
	}
	if cfg.TTSDefaultVoiceID != "male_calm" {
		t.Fatalf("TTSDefaultVoiceID = %q", cfg.TTSDefaultVoiceID)
	}
	if cfg.TTSEmotionPrompt != "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/male-read-emo.wav" {
		t.Fatalf("TTSEmotionPrompt = %q", cfg.TTSEmotionPrompt)
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

func TestLoadReadsBackgroundRunnerWorkerCount(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("BACKGROUND_RUNNER_WORKER_COUNT", "6")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.BackgroundRunnerWorkerCount != 6 {
		t.Fatalf("BackgroundRunnerWorkerCount = %d, want 6", cfg.BackgroundRunnerWorkerCount)
	}
}

func TestLoadReadsDashScopeTextRequestTimeoutSeconds(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_TEXT_REQUEST_TIMEOUT_SECONDS", "900")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DashScopeTextRequestTimeoutSeconds != 900 {
		t.Fatalf("DashScopeTextRequestTimeoutSeconds = %d, want 900", cfg.DashScopeTextRequestTimeoutSeconds)
	}
}

func TestLoadReadsDashScopeTextRetryConfig(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_TEXT_MAX_RETRIES", "4")
	t.Setenv("DASHSCOPE_TEXT_RETRY_BACKOFF_SECONDS", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DashScopeTextMaxRetries != 4 {
		t.Fatalf("DashScopeTextMaxRetries = %d, want 4", cfg.DashScopeTextMaxRetries)
	}
	if cfg.DashScopeTextRetryBackoffSeconds != 3 {
		t.Fatalf("DashScopeTextRetryBackoffSeconds = %d, want 3", cfg.DashScopeTextRetryBackoffSeconds)
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

func TestLoadReadsResourceConcurrencyConfig(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("RESOURCE_LOCAL_CPU_CONCURRENCY", "6")
	t.Setenv("RESOURCE_LLM_TEXT_CONCURRENCY", "4")
	t.Setenv("RESOURCE_TTS_CONCURRENCY", "5")
	t.Setenv("RESOURCE_IMAGE_GEN_CONCURRENCY", "3")
	t.Setenv("RESOURCE_VIDEO_GEN_CONCURRENCY", "2")
	t.Setenv("RESOURCE_VIDEO_RENDER_CONCURRENCY", "2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ResourceLocalCPUConcurrency != 6 {
		t.Fatalf("ResourceLocalCPUConcurrency = %d, want 6", cfg.ResourceLocalCPUConcurrency)
	}
	if cfg.ResourceLLMTextConcurrency != 4 {
		t.Fatalf("ResourceLLMTextConcurrency = %d, want 4", cfg.ResourceLLMTextConcurrency)
	}
	if cfg.ResourceTTSConcurrency != 5 {
		t.Fatalf("ResourceTTSConcurrency = %d, want 5", cfg.ResourceTTSConcurrency)
	}
	if cfg.ResourceImageGenConcurrency != 3 {
		t.Fatalf("ResourceImageGenConcurrency = %d, want 3", cfg.ResourceImageGenConcurrency)
	}
	if cfg.ResourceVideoGenConcurrency != 2 {
		t.Fatalf("ResourceVideoGenConcurrency = %d, want 2", cfg.ResourceVideoGenConcurrency)
	}
	if cfg.ResourceVideoRenderConcurrency != 2 {
		t.Fatalf("ResourceVideoRenderConcurrency = %d, want 2", cfg.ResourceVideoRenderConcurrency)
	}
}

func TestLoadReadsVideoRenderTimeoutSeconds(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("VIDEO_RENDER_TIMEOUT_SECONDS", "2400")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.VideoRenderTimeoutSeconds != 2400 {
		t.Fatalf("VideoRenderTimeoutSeconds = %d, want 2400", cfg.VideoRenderTimeoutSeconds)
	}
}

func TestLoadReadsShotVideoTimeoutPerShotSeconds(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("SHOT_VIDEO_TIMEOUT_PER_SHOT_SECONDS", "360")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ShotVideoTimeoutPerShotSeconds != 360 {
		t.Fatalf("ShotVideoTimeoutPerShotSeconds = %d, want 360", cfg.ShotVideoTimeoutPerShotSeconds)
	}
}

func TestLoadReadsFFmpegStartupCheckTimeoutSeconds(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS", "15")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.FFmpegStartupCheckTimeoutSeconds != 15 {
		t.Fatalf("FFmpegStartupCheckTimeoutSeconds = %d, want 15", cfg.FFmpegStartupCheckTimeoutSeconds)
	}
}

func TestLoadReadsShotVideoDefaultDurationSeconds(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("SHOT_VIDEO_DEFAULT_DURATION_SECONDS", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ShotVideoDefaultDurationSeconds != 5 {
		t.Fatalf("ShotVideoDefaultDurationSeconds = %d, want 5", cfg.ShotVideoDefaultDurationSeconds)
	}
}

func TestLoadReadsDashScopeVideoLiveClientConfig(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_VIDEO_SUBMIT_TIMEOUT_SECONDS", "75")
	t.Setenv("DASHSCOPE_VIDEO_POLL_INTERVAL_SECONDS", "12")
	t.Setenv("DASHSCOPE_VIDEO_MAX_WAIT_SECONDS", "1200")
	t.Setenv("DASHSCOPE_VIDEO_MAX_REQUEST_BYTES", "7340032")
	t.Setenv("DASHSCOPE_VIDEO_RESOLUTION", "1080P")
	t.Setenv("DASHSCOPE_VIDEO_NEGATIVE_PROMPT", "低清晰度, 文字")
	t.Setenv("DASHSCOPE_VIDEO_IMAGE_JPEG_QUALITY", "78")
	t.Setenv("DASHSCOPE_VIDEO_IMAGE_MIN_JPEG_QUALITY", "40")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DashScopeVideoSubmitTimeoutSeconds != 75 {
		t.Fatalf("DashScopeVideoSubmitTimeoutSeconds = %d, want 75", cfg.DashScopeVideoSubmitTimeoutSeconds)
	}
	if cfg.DashScopeVideoPollIntervalSeconds != 12 {
		t.Fatalf("DashScopeVideoPollIntervalSeconds = %d, want 12", cfg.DashScopeVideoPollIntervalSeconds)
	}
	if cfg.DashScopeVideoMaxWaitSeconds != 1200 {
		t.Fatalf("DashScopeVideoMaxWaitSeconds = %d, want 1200", cfg.DashScopeVideoMaxWaitSeconds)
	}
	if cfg.DashScopeVideoMaxRequestBytes != 7340032 {
		t.Fatalf("DashScopeVideoMaxRequestBytes = %d, want 7340032", cfg.DashScopeVideoMaxRequestBytes)
	}
	if cfg.DashScopeVideoResolution != "1080P" {
		t.Fatalf("DashScopeVideoResolution = %q, want 1080P", cfg.DashScopeVideoResolution)
	}
	if cfg.DashScopeVideoNegativePrompt != "低清晰度, 文字" {
		t.Fatalf("DashScopeVideoNegativePrompt = %q", cfg.DashScopeVideoNegativePrompt)
	}
	if cfg.DashScopeVideoImageJPEGQuality != 78 {
		t.Fatalf("DashScopeVideoImageJPEGQuality = %d, want 78", cfg.DashScopeVideoImageJPEGQuality)
	}
	if cfg.DashScopeVideoImageMinJPEGQuality != 40 {
		t.Fatalf("DashScopeVideoImageMinJPEGQuality = %d, want 40", cfg.DashScopeVideoImageMinJPEGQuality)
	}
}

func TestLoadFallsBackWhenShotVideoDefaultDurationSecondsInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("SHOT_VIDEO_DEFAULT_DURATION_SECONDS", "abc")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ShotVideoDefaultDurationSeconds != 3 {
		t.Fatalf("ShotVideoDefaultDurationSeconds = %d, want 3", cfg.ShotVideoDefaultDurationSeconds)
	}
}

func TestLoadFallsBackWhenDashScopeVideoLiveClientConfigInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_VIDEO_SUBMIT_TIMEOUT_SECONDS", "abc")
	t.Setenv("DASHSCOPE_VIDEO_POLL_INTERVAL_SECONDS", "0")
	t.Setenv("DASHSCOPE_VIDEO_MAX_WAIT_SECONDS", "-1")
	t.Setenv("DASHSCOPE_VIDEO_MAX_REQUEST_BYTES", "bad")
	t.Setenv("DASHSCOPE_VIDEO_IMAGE_JPEG_QUALITY", "")
	t.Setenv("DASHSCOPE_VIDEO_IMAGE_MIN_JPEG_QUALITY", "bad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DashScopeVideoSubmitTimeoutSeconds != 60 {
		t.Fatalf("DashScopeVideoSubmitTimeoutSeconds = %d, want 60", cfg.DashScopeVideoSubmitTimeoutSeconds)
	}
	if cfg.DashScopeVideoPollIntervalSeconds != 10 {
		t.Fatalf("DashScopeVideoPollIntervalSeconds = %d, want 10", cfg.DashScopeVideoPollIntervalSeconds)
	}
	if cfg.DashScopeVideoMaxWaitSeconds != 900 {
		t.Fatalf("DashScopeVideoMaxWaitSeconds = %d, want 900", cfg.DashScopeVideoMaxWaitSeconds)
	}
	if cfg.DashScopeVideoMaxRequestBytes != 6291456 {
		t.Fatalf("DashScopeVideoMaxRequestBytes = %d, want 6291456", cfg.DashScopeVideoMaxRequestBytes)
	}
	if cfg.DashScopeVideoImageJPEGQuality != 80 {
		t.Fatalf("DashScopeVideoImageJPEGQuality = %d, want 80", cfg.DashScopeVideoImageJPEGQuality)
	}
	if cfg.DashScopeVideoImageMinJPEGQuality != 45 {
		t.Fatalf("DashScopeVideoImageMinJPEGQuality = %d, want 45", cfg.DashScopeVideoImageMinJPEGQuality)
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

func TestLoadFallsBackWhenDashScopeTextRequestTimeoutSecondsInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_TEXT_REQUEST_TIMEOUT_SECONDS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DashScopeTextRequestTimeoutSeconds != 600 {
		t.Fatalf("DashScopeTextRequestTimeoutSeconds = %d, want 600", cfg.DashScopeTextRequestTimeoutSeconds)
	}
}

func TestLoadFallsBackWhenDashScopeTextRetryConfigInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("DASHSCOPE_TEXT_MAX_RETRIES", "-1")
	t.Setenv("DASHSCOPE_TEXT_RETRY_BACKOFF_SECONDS", "bad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DashScopeTextMaxRetries != 2 {
		t.Fatalf("DashScopeTextMaxRetries = %d, want 2", cfg.DashScopeTextMaxRetries)
	}
	if cfg.DashScopeTextRetryBackoffSeconds != 2 {
		t.Fatalf("DashScopeTextRetryBackoffSeconds = %d, want 2", cfg.DashScopeTextRetryBackoffSeconds)
	}
}

func TestLoadFallsBackWhenTTSRetryConfigInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("TTS_MAX_RETRIES", "-1")
	t.Setenv("TTS_RETRY_BACKOFF_SECONDS", "bad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.TTSMaxRetries != 2 {
		t.Fatalf("TTSMaxRetries = %d, want 2", cfg.TTSMaxRetries)
	}
	if cfg.TTSRetryBackoffSeconds != 2 {
		t.Fatalf("TTSRetryBackoffSeconds = %d, want 2", cfg.TTSRetryBackoffSeconds)
	}
}

func TestLoadFallsBackWhenResourceConcurrencyInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("RESOURCE_LOCAL_CPU_CONCURRENCY", "bad")
	t.Setenv("RESOURCE_LLM_TEXT_CONCURRENCY", "0")
	t.Setenv("RESOURCE_TTS_CONCURRENCY", "-1")
	t.Setenv("RESOURCE_IMAGE_GEN_CONCURRENCY", "")
	t.Setenv("RESOURCE_VIDEO_GEN_CONCURRENCY", "bad")
	t.Setenv("RESOURCE_VIDEO_RENDER_CONCURRENCY", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ResourceLocalCPUConcurrency != 4 {
		t.Fatalf("ResourceLocalCPUConcurrency = %d, want 4", cfg.ResourceLocalCPUConcurrency)
	}
	if cfg.ResourceLLMTextConcurrency != 2 {
		t.Fatalf("ResourceLLMTextConcurrency = %d, want 2", cfg.ResourceLLMTextConcurrency)
	}
	if cfg.ResourceTTSConcurrency != 3 {
		t.Fatalf("ResourceTTSConcurrency = %d, want 3", cfg.ResourceTTSConcurrency)
	}
	if cfg.ResourceImageGenConcurrency != 2 {
		t.Fatalf("ResourceImageGenConcurrency = %d, want 2", cfg.ResourceImageGenConcurrency)
	}
	if cfg.ResourceVideoGenConcurrency != 1 {
		t.Fatalf("ResourceVideoGenConcurrency = %d, want 1", cfg.ResourceVideoGenConcurrency)
	}
	if cfg.ResourceVideoRenderConcurrency != 1 {
		t.Fatalf("ResourceVideoRenderConcurrency = %d, want 1", cfg.ResourceVideoRenderConcurrency)
	}
}

func TestLoadFallsBackWhenVideoRenderTimeoutSecondsInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("VIDEO_RENDER_TIMEOUT_SECONDS", "bad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.VideoRenderTimeoutSeconds != 1800 {
		t.Fatalf("VideoRenderTimeoutSeconds = %d, want 1800", cfg.VideoRenderTimeoutSeconds)
	}
}

func TestLoadFallsBackWhenShotVideoTimeoutPerShotSecondsInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("SHOT_VIDEO_TIMEOUT_PER_SHOT_SECONDS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ShotVideoTimeoutPerShotSeconds != 200 {
		t.Fatalf("ShotVideoTimeoutPerShotSeconds = %d, want 200", cfg.ShotVideoTimeoutPerShotSeconds)
	}
}

func TestLoadFallsBackWhenFFmpegStartupCheckTimeoutSecondsInvalid(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.FFmpegStartupCheckTimeoutSeconds != 10 {
		t.Fatalf("FFmpegStartupCheckTimeoutSeconds = %d, want 10", cfg.FFmpegStartupCheckTimeoutSeconds)
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

func TestLoadReadsTTSEmotionPrompt(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("TTS_EMOTION_PROMPT", "https://example.com/custom-emotion.wav")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.TTSEmotionPrompt != "https://example.com/custom-emotion.wav" {
		t.Fatalf("TTSEmotionPrompt = %q", cfg.TTSEmotionPrompt)
	}
}

func TestLoadReadsTTSRetryConfig(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("TTS_MAX_RETRIES", "4")
	t.Setenv("TTS_RETRY_BACKOFF_SECONDS", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.TTSMaxRetries != 4 {
		t.Fatalf("TTSMaxRetries = %d, want 4", cfg.TTSMaxRetries)
	}
	if cfg.TTSRetryBackoffSeconds != 5 {
		t.Fatalf("TTSRetryBackoffSeconds = %d, want 5", cfg.TTSRetryBackoffSeconds)
	}
}

func TestLoadReadsLiveVideoGenerationFlag(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "./narratio.db")
	t.Setenv("WORKSPACE_DIR", "./workspace")
	t.Setenv("ENABLE_LIVE_VIDEO_GENERATION", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.EnableLiveVideoGeneration {
		t.Fatal("EnableLiveVideoGeneration = false, want true")
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
