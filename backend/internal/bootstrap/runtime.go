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
	scriptpipeline "github.com/sfzman/Narratio/backend/internal/pipeline/script"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
	sqlstore "github.com/sfzman/Narratio/backend/internal/store/sql"
)

type Runtime struct {
	Config           *config.Config
	DB               *sql.DB
	Store            *sqlstore.Store
	TextClient       scriptpipeline.TextClient
	ExecutorRegistry *scheduler.ExecutorRegistry
	JobsService      *jobapp.Service
	DispatchService  *jobapp.DispatchService
	SchedulerService *scheduler.Service
	ResourceManager  scheduler.ResourceManager
	Router           http.Handler
}

func (r *Runtime) Close() error {
	if r == nil || r.DB == nil {
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

	textClient, err := scriptpipeline.NewHTTPTextClient(
		cfg.DashScopeTextBaseURL,
		cfg.DashScopeTextAPIKey,
		&http.Client{Timeout: 30 * time.Second},
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("build dashscope text client: %w", err)
	}

	textGenerationConfig := scriptpipeline.TextGenerationConfig{
		Model: cfg.DashScopeTextModel,
	}
	store := sqlstore.New(db)
	registry := scheduler.NewExecutorRegistry(map[model.TaskType]scheduler.Executor{
		model.TaskTypeOutline:        scriptpipeline.NewOutlineExecutorWithClient(textClient, textGenerationConfig),
		model.TaskTypeCharacterSheet: scriptpipeline.NewCharacterSheetExecutorWithClient(textClient, textGenerationConfig),
		model.TaskTypeScript:         scriptpipeline.NewScriptExecutorWithClient(textClient, textGenerationConfig),
	})
	resourceManager := scheduler.NewMemoryResourceManager(defaultResourceLimits())
	jobsService := jobapp.NewService(store)
	schedulerService := scheduler.NewService(store, store, registry, resourceManager)
	dispatchService := jobapp.NewDispatchService(store, schedulerService)
	router := handler.NewRouter(jobsService, store, store, dispatchService, handler.HealthStatus{
		Version: "dev",
		Services: map[string]string{
			"database":       "ok",
			"dashscope_text": healthStatus(cfg.DashScopeTextAPIKey != ""),
			"tts":            healthStatus(cfg.TTSAPIKey != ""),
		},
	})

	slog.Info("runtime initialized",
		"database_driver", cfg.DatabaseDriver,
		"database_dsn", cfg.DatabaseDSN,
		"dashscope_text_base_url", cfg.DashScopeTextBaseURL,
		"dashscope_text_model", cfg.DashScopeTextModel,
	)

	return &Runtime{
		Config:           cfg,
		DB:               db,
		Store:            store,
		TextClient:       textClient,
		ExecutorRegistry: registry,
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
