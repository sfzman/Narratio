package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port                     string
	DatabaseDriver           string
	DatabaseDSN              string
	EnableLiveTextGeneration bool
	DashScopeTextBaseURL     string
	DashScopeTextModel       string
	DashScopeTextAPIKey      string
	DashScopeImageBaseURL    string
	DashScopeImageModel      string
	DashScopeImageAPIKey     string
	DashScopeVideoBaseURL    string
	DashScopeVideoModel      string
	DashScopeVideoAPIKey     string
	TTSBaseURL               string
	TTSAPIKey                string
	WorkspaceDir             string
}

func Load() (*Config, error) {
	if err := loadDotEnvCandidates(".env", filepath.Join("backend", ".env")); err != nil {
		return nil, err
	}

	databaseDriver, err := requiredEnv("DATABASE_DRIVER")
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseDriver(databaseDriver); err != nil {
		return nil, err
	}

	databaseDSN, err := requiredEnv("DATABASE_DSN")
	if err != nil {
		return nil, err
	}
	workspaceDir, err := requiredEnv("WORKSPACE_DIR")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:                     envOrDefault("PORT", "8080"),
		DatabaseDriver:           databaseDriver,
		DatabaseDSN:              databaseDSN,
		EnableLiveTextGeneration: envAsBool("ENABLE_LIVE_TEXT_GENERATION"),
		DashScopeTextBaseURL:     envOrDefault("DASHSCOPE_TEXT_BASE_URL", "https://coding.dashscope.aliyuncs.com/v1"),
		DashScopeTextModel:       envOrDefault("DASHSCOPE_TEXT_MODEL", "qwen-max"),
		DashScopeTextAPIKey:      env("DASHSCOPE_TEXT_API_KEY"),
		DashScopeImageBaseURL:    envOrDefault("DASHSCOPE_IMAGE_BASE_URL", "https://dashscope.aliyuncs.com/api/v1"),
		DashScopeImageModel:      envOrDefault("DASHSCOPE_IMAGE_MODEL", "qwen-image-2.0"),
		DashScopeImageAPIKey:     env("DASHSCOPE_IMAGE_API_KEY"),
		DashScopeVideoBaseURL:    envOrDefault("DASHSCOPE_VIDEO_BASE_URL", "https://dashscope.aliyuncs.com"),
		DashScopeVideoModel:      envOrDefault("DASHSCOPE_VIDEO_MODEL", "wan2.6-i2v-flash"),
		DashScopeVideoAPIKey:     env("DASHSCOPE_VIDEO_API_KEY"),
		TTSBaseURL:               env("TTS_API_BASE_URL"),
		TTSAPIKey:                env("TTS_API_KEY"),
		WorkspaceDir:             workspaceDir,
	}

	return cfg, nil
}

func validateDatabaseDriver(driver string) error {
	switch driver {
	case "sqlite", "mysql":
		return nil
	default:
		return fmt.Errorf("DATABASE_DRIVER must be sqlite or mysql")
	}
}

func requiredEnv(key string) (string, error) {
	value := env(key)
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}

	return value, nil
}

func envOrDefault(key, fallback string) string {
	value := env(key)
	if value == "" {
		return fallback
	}

	return value
}

func env(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envAsBool(key string) bool {
	value := strings.ToLower(env(key))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func loadDotEnvCandidates(paths ...string) error {
	for _, path := range paths {
		if err := loadDotEnv(path); err != nil {
			return err
		}
	}

	return nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("parse %s:%d: missing '='", path, lineNumber)
		}

		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		value = trimEnvValue(value)
		if key == "" {
			return fmt.Errorf("parse %s:%d: empty key", path, lineNumber)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from %s:%d: %w", key, path, lineNumber, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}

	return nil
}

func trimEnvValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
			(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
			return trimmed[1 : len(trimmed)-1]
		}
	}

	return trimmed
}
