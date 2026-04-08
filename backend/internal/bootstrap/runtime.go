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
	ExecutorRegistry *scheduler.ExecutorRegistry
	RunCoordinator   *jobapp.RunCoordinator
	BackgroundRunner *jobapp.BackgroundRunner
	JobsService      *jobapp.Service
	DispatchService  *jobapp.DispatchService
	SchedulerService *scheduler.Service
	ResourceManager  scheduler.ResourceManager
	Router           http.Handler
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
		model.TaskTypeCharacterImage: imagepipeline.NewCharacterImageExecutor(cfg.WorkspaceDir),
		model.TaskTypeTTS:            ttspipeline.NewExecutor(cfg.WorkspaceDir),
		model.TaskTypeImage: imagepipeline.NewExecutorWithClient(
			imageClient,
			imageGenerationConfig,
			cfg.WorkspaceDir,
		),
		model.TaskTypeVideo: videopipeline.NewExecutor(cfg.WorkspaceDir),
	})
	resourceManager := scheduler.NewMemoryResourceManager(defaultResourceLimits())
	schedulerService := scheduler.NewService(store, store, registry, resourceManager)
	runCoordinator := jobapp.NewRunCoordinator()
	backgroundRunner := jobapp.NewBackgroundRunner(schedulerService, runCoordinator)
	jobsService := jobapp.NewService(store, backgroundRunner)
	dispatchService := jobapp.NewDispatchService(store, schedulerService, runCoordinator)
	router := handler.NewRouter(jobsService, store, store, dispatchService, handler.HealthStatus{
		Version: "dev",
		Services: map[string]string{
			"database":        "ok",
			"dashscope_text":  textHealthStatus(cfg),
			"dashscope_image": imageHealthStatus(cfg),
			"tts":             healthStatus(cfg.TTSAPIKey != ""),
		},
	})

	slog.Info("runtime initialized",
		"database_driver", cfg.DatabaseDriver,
		"database_dsn", cfg.DatabaseDSN,
		"live_text_generation", cfg.EnableLiveTextGeneration,
		"live_image_generation", cfg.EnableLiveImageGeneration,
		"dashscope_text_base_url", cfg.DashScopeTextBaseURL,
		"dashscope_text_model", cfg.DashScopeTextModel,
		"dashscope_image_base_url", cfg.DashScopeImageBaseURL,
		"dashscope_image_model", cfg.DashScopeImageModel,
	)

	return &Runtime{
		Config:           cfg,
		DB:               db,
		Store:            store,
		TextClient:       textClient,
		ImageClient:      imageClient,
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

func defaultResourceLimits() map[model.ResourceKey]int {
	return map[model.ResourceKey]int{
		model.ResourceLocalCPU:    4,
		model.ResourceLLMText:     2,
		model.ResourceTTS:         3,
		model.ResourceImageGen:    2,
		model.ResourceVideoRender: 1,
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
		&http.Client{Timeout: 600 * time.Second},
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
