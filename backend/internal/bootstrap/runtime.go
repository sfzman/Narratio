package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "github.com/mattn/go-sqlite3"
	jobapp "github.com/sfzman/Narratio/backend/internal/app/jobs"
	"github.com/sfzman/Narratio/backend/internal/config"
	"github.com/sfzman/Narratio/backend/internal/handler"
	"github.com/sfzman/Narratio/backend/internal/model"
	imagepipeline "github.com/sfzman/Narratio/backend/internal/pipeline/image"
	scriptpipeline "github.com/sfzman/Narratio/backend/internal/pipeline/script"
	ttspipeline "github.com/sfzman/Narratio/backend/internal/pipeline/tts"
	videopipeline "github.com/sfzman/Narratio/backend/internal/pipeline/video"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
	sqlstore "github.com/sfzman/Narratio/backend/internal/store/sql"
)

type Runtime struct {
	Config           *config.Config
	DB               *sql.DB
	Store            *sqlstore.Store
	TextClient       scriptpipeline.TextClient
	ImageClient      imagepipeline.Client
	TTSClient        ttspipeline.Client
	VideoClient      videopipeline.Client
	ExecutorRegistry *scheduler.ExecutorRegistry
	RunCoordinator   *jobapp.RunCoordinator
	BackgroundRunner *jobapp.BackgroundRunner
	JobsService      *jobapp.Service
	DispatchService  *jobapp.DispatchService
	SchedulerService *scheduler.Service
	ResourceManager  scheduler.ResourceManager
	Router           http.Handler
}

var probeFFmpegAvailability = func(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return videopipeline.CheckFFmpegAvailable(ctx, nil)
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	if r.BackgroundRunner != nil {
		if err := r.BackgroundRunner.Close(); err != nil {
			return err
		}
	}
	if r.DB == nil {
		return nil
	}

	return r.DB.Close()
}

func LoadRuntime() (*Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := probeFFmpegAvailability(
		time.Duration(cfg.FFmpegStartupCheckTimeoutSeconds) * time.Second,
	); err != nil {
		return nil, fmt.Errorf("ffmpeg startup check: %w", err)
	}

	db, err := openDatabase(cfg)
	if err != nil {
		return nil, err
	}

	textClient, err := buildTextClient(cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	imageClient, err := buildImageClient(cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	ttsClient, err := buildTTSClient(cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	videoClient, err := buildVideoClient(cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	textGenerationConfig := scriptpipeline.TextGenerationConfig{
		Model: cfg.DashScopeTextModel,
	}
	imageGenerationConfig := imagepipeline.GenerationConfig{
		Model: cfg.DashScopeImageModel,
	}
	store := sqlstore.New(db)
	registry := scheduler.NewExecutorRegistry(map[model.TaskType]scheduler.Executor{
		model.TaskTypeSegmentation: scriptpipeline.NewSegmentationExecutor(cfg.WorkspaceDir),
		model.TaskTypeOutline: scriptpipeline.NewOutlineExecutorWithClient(
			textClient,
			textGenerationConfig,
			cfg.WorkspaceDir,
		),
		model.TaskTypeCharacterSheet: scriptpipeline.NewCharacterSheetExecutorWithClient(
			textClient,
			textGenerationConfig,
			cfg.WorkspaceDir,
		),
		model.TaskTypeScript: scriptpipeline.NewScriptExecutorWithClient(
			textClient,
			textGenerationConfig,
			cfg.WorkspaceDir,
		),
		model.TaskTypeCharacterImage: imagepipeline.NewCharacterImageExecutorWithClient(
			imageClient,
			imageGenerationConfig,
			cfg.WorkspaceDir,
		),
		model.TaskTypeTTS: ttspipeline.NewExecutorWithClient(ttsClient, cfg.WorkspaceDir),
		model.TaskTypeImage: imagepipeline.NewExecutorWithClient(
			imageClient,
			imageGenerationConfig,
			cfg.WorkspaceDir,
		),
		model.TaskTypeShotVideo: videopipeline.NewShotVideoExecutorWithClient(
			videoClient,
			videopipeline.GenerationConfig{
				Model:               cfg.DashScopeVideoModel,
				Resolution:          cfg.DashScopeVideoResolution,
				NegativePrompt:      cfg.DashScopeVideoNegativePrompt,
				PollInterval:        time.Duration(cfg.DashScopeVideoPollIntervalSeconds) * time.Second,
				MaxWait:             time.Duration(cfg.DashScopeVideoMaxWaitSeconds) * time.Second,
				MaxRequestBytes:     cfg.DashScopeVideoMaxRequestBytes,
				ImageJPEGQuality:    cfg.DashScopeVideoImageJPEGQuality,
				ImageMinJPEGQuality: cfg.DashScopeVideoImageMinJPEGQuality,
			},
			cfg.WorkspaceDir,
			float64(cfg.ShotVideoDefaultDurationSeconds),
		),
		model.TaskTypeVideo: videopipeline.NewRealExecutor(cfg.WorkspaceDir),
	})
	resourceManager := scheduler.NewMemoryResourceManager(defaultResourceLimits(cfg))
	schedulerService := scheduler.NewService(store, store, registry, resourceManager)
	schedulerService.SetScriptTimeoutPerSegment(
		time.Duration(cfg.ScriptTimeoutPerSegmentSeconds) * time.Second,
	)
	schedulerService.SetTTSTimeoutPerSegment(
		time.Duration(cfg.TTSTimeoutPerSegmentSeconds) * time.Second,
	)
	schedulerService.SetShotVideoTimeoutPerShot(
		time.Duration(cfg.ShotVideoTimeoutPerShotSeconds) * time.Second,
	)
	schedulerService.SetVideoRenderTimeout(
		time.Duration(cfg.VideoRenderTimeoutSeconds) * time.Second,
	)
	runCoordinator := jobapp.NewRunCoordinator()
	backgroundRunner := jobapp.NewBackgroundRunnerWithWorkerCount(
		schedulerService,
		runCoordinator,
		cfg.BackgroundRunnerWorkerCount,
	)
	backgroundRunner.SetResourceAvailabilityNotifier(resourceManager)
	jobsService := jobapp.NewService(store, backgroundRunner)
	jobsService.SetWorkspaceDir(cfg.WorkspaceDir)
	dispatchService := jobapp.NewDispatchService(store, schedulerService, runCoordinator)
	router := handler.NewRouter(jobsService, store, store, dispatchService, handler.HealthStatus{
		Version: "dev",
		Services: map[string]string{
			"database":        "ok",
			"dashscope_text":  textHealthStatus(cfg),
			"dashscope_image": imageHealthStatus(cfg),
			"dashscope_video": videoHealthStatus(cfg),
			"tts":             healthStatus(cfg.TTSBaseURL != "" && cfg.TTSJWTPrivateKey != ""),
		},
		Resources: healthResourceLimits(cfg),
	}, cfg.WorkspaceDir)

	slog.Info("runtime initialized",
		"database_driver", cfg.DatabaseDriver,
		"database_dsn", cfg.DatabaseDSN,
		"background_runner_worker_count", cfg.BackgroundRunnerWorkerCount,
		"resource_local_cpu_concurrency", cfg.ResourceLocalCPUConcurrency,
		"resource_llm_text_concurrency", cfg.ResourceLLMTextConcurrency,
		"resource_tts_concurrency", cfg.ResourceTTSConcurrency,
		"resource_image_gen_concurrency", cfg.ResourceImageGenConcurrency,
		"resource_video_gen_concurrency", cfg.ResourceVideoGenConcurrency,
		"resource_video_render_concurrency", cfg.ResourceVideoRenderConcurrency,
		"script_timeout_per_segment_seconds", cfg.ScriptTimeoutPerSegmentSeconds,
		"tts_timeout_per_segment_seconds", cfg.TTSTimeoutPerSegmentSeconds,
		"shot_video_timeout_per_shot_seconds", cfg.ShotVideoTimeoutPerShotSeconds,
		"video_render_timeout_seconds", cfg.VideoRenderTimeoutSeconds,
		"ffmpeg_startup_check_timeout_seconds", cfg.FFmpegStartupCheckTimeoutSeconds,
		"shot_video_default_duration_seconds", cfg.ShotVideoDefaultDurationSeconds,
		"live_text_generation", cfg.EnableLiveTextGeneration,
		"live_image_generation", cfg.EnableLiveImageGeneration,
		"live_video_generation", cfg.EnableLiveVideoGeneration,
		"dashscope_text_base_url", cfg.DashScopeTextBaseURL,
		"dashscope_text_model", cfg.DashScopeTextModel,
		"dashscope_text_request_timeout_seconds", cfg.DashScopeTextRequestTimeoutSeconds,
		"dashscope_text_max_retries", cfg.DashScopeTextMaxRetries,
		"dashscope_text_retry_backoff_seconds", cfg.DashScopeTextRetryBackoffSeconds,
		"dashscope_image_base_url", cfg.DashScopeImageBaseURL,
		"dashscope_image_model", cfg.DashScopeImageModel,
		"dashscope_video_base_url", cfg.DashScopeVideoBaseURL,
		"dashscope_video_model", cfg.DashScopeVideoModel,
		"tts_base_url", cfg.TTSBaseURL,
		"tts_request_timeout_seconds", cfg.TTSRequestTimeoutSeconds,
		"tts_max_retries", cfg.TTSMaxRetries,
		"tts_retry_backoff_seconds", cfg.TTSRetryBackoffSeconds,
	)

	return &Runtime{
		Config:           cfg,
		DB:               db,
		Store:            store,
		TextClient:       textClient,
		ImageClient:      imageClient,
		TTSClient:        ttsClient,
		VideoClient:      videoClient,
		ExecutorRegistry: registry,
		RunCoordinator:   runCoordinator,
		BackgroundRunner: backgroundRunner,
		JobsService:      jobsService,
		DispatchService:  dispatchService,
		SchedulerService: schedulerService,
		ResourceManager:  resourceManager,
		Router:           router,
	}, nil
}

func openDatabase(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open(sqlDriverName(cfg.DatabaseDriver), cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if cfg.DatabaseDriver == "sqlite" {
		if err := applySQLiteMigrations(context.Background(), db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return db, nil
}

func sqlDriverName(driver string) string {
	switch driver {
	case "sqlite":
		return "sqlite3"
	default:
		return driver
	}
}

func applySQLiteMigrations(ctx context.Context, db *sql.DB) error {
	migrationSQL, err := loadInitialMigration()
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, migrationSQL); err != nil {
		return fmt.Errorf("apply sqlite migrations: %w", err)
	}

	return nil
}

func loadInitialMigration() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}

	migrationPath := filepath.Join(
		filepath.Dir(currentFile),
		"..",
		"store",
		"migrations",
		"001_init.sql",
	)
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		return "", fmt.Errorf("read migration file: %w", err)
	}

	return string(sqlBytes), nil
}

func defaultResourceLimits(cfg *config.Config) map[model.ResourceKey]int {
	if cfg == nil {
		return map[model.ResourceKey]int{
			model.ResourceLocalCPU:    4,
			model.ResourceLLMText:     2,
			model.ResourceTTS:         3,
			model.ResourceImageGen:    2,
			model.ResourceVideoGen:    1,
			model.ResourceVideoRender: 1,
		}
	}

	return map[model.ResourceKey]int{
		model.ResourceLocalCPU:    cfg.ResourceLocalCPUConcurrency,
		model.ResourceLLMText:     cfg.ResourceLLMTextConcurrency,
		model.ResourceTTS:         cfg.ResourceTTSConcurrency,
		model.ResourceImageGen:    cfg.ResourceImageGenConcurrency,
		model.ResourceVideoGen:    cfg.ResourceVideoGenConcurrency,
		model.ResourceVideoRender: cfg.ResourceVideoRenderConcurrency,
	}
}

func healthResourceLimits(cfg *config.Config) map[string]int {
	limits := defaultResourceLimits(cfg)
	return map[string]int{
		string(model.ResourceLocalCPU):    limits[model.ResourceLocalCPU],
		string(model.ResourceLLMText):     limits[model.ResourceLLMText],
		string(model.ResourceTTS):         limits[model.ResourceTTS],
		string(model.ResourceImageGen):    limits[model.ResourceImageGen],
		string(model.ResourceVideoGen):    limits[model.ResourceVideoGen],
		string(model.ResourceVideoRender): limits[model.ResourceVideoRender],
	}
}

func healthStatus(ok bool) string {
	if ok {
		return "configured"
	}

	return "not_configured"
}

func textHealthStatus(cfg *config.Config) string {
	if cfg.DashScopeTextAPIKey == "" {
		return "not_configured"
	}
	if !cfg.EnableLiveTextGeneration {
		return "configured_but_disabled"
	}

	return "configured"
}

func imageHealthStatus(cfg *config.Config) string {
	if cfg.DashScopeImageAPIKey == "" {
		return "not_configured"
	}
	if !cfg.EnableLiveImageGeneration {
		return "configured_but_disabled"
	}

	return "configured"
}

func videoHealthStatus(cfg *config.Config) string {
	if cfg.DashScopeVideoAPIKey == "" {
		return "not_configured"
	}
	if !cfg.EnableLiveVideoGeneration {
		return "configured_but_disabled"
	}

	return "configured"
}

func buildTextClient(cfg *config.Config) (scriptpipeline.TextClient, error) {
	if !cfg.EnableLiveTextGeneration {
		return nil, nil
	}
	if cfg.DashScopeTextAPIKey == "" {
		return nil, nil
	}

	client, err := scriptpipeline.NewHTTPTextClient(
		cfg.DashScopeTextBaseURL,
		cfg.DashScopeTextAPIKey,
		&http.Client{Timeout: time.Duration(cfg.DashScopeTextRequestTimeoutSeconds) * time.Second},
		scriptpipeline.HTTPTextClientOptions{
			MaxRetries: cfg.DashScopeTextMaxRetries,
			Backoff:    time.Duration(cfg.DashScopeTextRetryBackoffSeconds) * time.Second,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("build dashscope text client: %w", err)
	}

	return client, nil
}

func buildImageClient(cfg *config.Config) (imagepipeline.Client, error) {
	if !cfg.EnableLiveImageGeneration {
		return nil, nil
	}
	if cfg.DashScopeImageAPIKey == "" {
		return nil, nil
	}

	client, err := imagepipeline.NewHTTPClient(
		cfg.DashScopeImageBaseURL,
		cfg.DashScopeImageAPIKey,
		&http.Client{Timeout: 600 * time.Second},
	)
	if err != nil {
		return nil, fmt.Errorf("build dashscope image client: %w", err)
	}

	return client, nil
}

func buildTTSClient(cfg *config.Config) (ttspipeline.Client, error) {
	if cfg.TTSBaseURL == "" {
		return nil, nil
	}

	client, err := ttspipeline.NewHTTPClient(
		cfg.TTSBaseURL,
		cfg.TTSJWTPrivateKey,
		cfg.TTSJWTExpireSeconds,
		cfg.TTSDefaultVoiceID,
		cfg.TTSEmotionPrompt,
		&http.Client{Timeout: time.Duration(cfg.TTSRequestTimeoutSeconds) * time.Second},
		ttspipeline.HTTPClientOptions{
			MaxRetries: cfg.TTSMaxRetries,
			Backoff:    time.Duration(cfg.TTSRetryBackoffSeconds) * time.Second,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("build tts client: %w", err)
	}

	return client, nil
}

func buildVideoClient(cfg *config.Config) (videopipeline.Client, error) {
	if !cfg.EnableLiveVideoGeneration {
		return nil, nil
	}
	if cfg.DashScopeVideoAPIKey == "" {
		return nil, nil
	}

	client, err := videopipeline.NewHTTPClient(
		cfg.DashScopeVideoBaseURL,
		cfg.DashScopeVideoAPIKey,
		videopipeline.GenerationConfig{
			Model:               cfg.DashScopeVideoModel,
			Resolution:          cfg.DashScopeVideoResolution,
			NegativePrompt:      cfg.DashScopeVideoNegativePrompt,
			PollInterval:        time.Duration(cfg.DashScopeVideoPollIntervalSeconds) * time.Second,
			MaxWait:             time.Duration(cfg.DashScopeVideoMaxWaitSeconds) * time.Second,
			MaxRequestBytes:     cfg.DashScopeVideoMaxRequestBytes,
			ImageJPEGQuality:    cfg.DashScopeVideoImageJPEGQuality,
			ImageMinJPEGQuality: cfg.DashScopeVideoImageMinJPEGQuality,
		},
		&http.Client{Timeout: time.Duration(cfg.DashScopeVideoSubmitTimeoutSeconds) * time.Second},
	)
	if err != nil {
		return nil, fmt.Errorf("build dashscope video client: %w", err)
	}

	return client, nil
}
