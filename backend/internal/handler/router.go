package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	jobapp "github.com/sfzman/Narratio/backend/internal/app/jobs"
	"github.com/sfzman/Narratio/backend/internal/middleware"
	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/store"
)

const defaultEstimatedSeconds = 120

type JobCreator interface {
	CreateJob(ctx context.Context, spec model.JobSpec) (model.Job, []model.Task, error)
}

type JobCanceler interface {
	CancelJob(ctx context.Context, publicID string) (jobapp.CancelOutcome, error)
}

type JobReader interface {
	ListJobs(ctx context.Context) ([]model.Job, error)
	GetJobByPublicID(ctx context.Context, publicID string) (model.Job, error)
}

type TaskReader interface {
	ListTasksByJobPublicID(ctx context.Context, publicID string) ([]model.Task, error)
}

type JobDispatcher interface {
	DispatchOnce(ctx context.Context, publicID string) (jobapp.DispatchOutcome, error)
}

type HealthStatus struct {
	Version   string
	Services  map[string]string
	Resources map[string]int
}

type Handlers struct {
	jobs       JobCreator
	canceler   JobCanceler
	jobReader  JobReader
	taskReader TaskReader
	dispatcher JobDispatcher
	health     HealthStatus
	workspace  string
}

func NewRouter(
	jobs JobCreator,
	jobReader JobReader,
	taskReader TaskReader,
	dispatcher JobDispatcher,
	health HealthStatus,
	workspaceDir ...string,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	resolvedWorkspaceDir := ""
	if len(workspaceDir) > 0 {
		resolvedWorkspaceDir = workspaceDir[0]
	}

	h := Handlers{
		jobs:       jobs,
		jobReader:  jobReader,
		taskReader: taskReader,
		dispatcher: dispatcher,
		health:     health,
		workspace:  resolvedWorkspaceDir,
	}
	if canceler, ok := jobs.(JobCanceler); ok {
		h.canceler = canceler
	}

	router := gin.New()
	router.Use(middleware.CORS())
	router.Use(gin.Recovery())

	api := router.Group("/api/v1")
	api.GET("/health", h.healthCheck)
	api.GET("/voices", h.listVoices)
	api.POST("/jobs", h.createJob)
	api.GET("/jobs", h.listJobs)
	api.GET("/jobs/:job_id", h.getJob)
	api.GET("/jobs/:job_id/tasks", h.getJobTasks)
	api.GET("/jobs/:job_id/artifact", h.getJobArtifact)
	api.GET("/jobs/:job_id/download", h.downloadJobVideo)
	api.DELETE("/jobs/:job_id", h.cancelJob)
	api.POST("/jobs/:job_id/dispatch-once", h.dispatchOnce)

	return router
}

func requestID(c *gin.Context) string {
	return c.GetString("request_id")
}

func success(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"code": 0,
		"data": data,
	})
}

func failure(c *gin.Context, status int, code int, message string) {
	c.JSON(status, gin.H{
		"code":       code,
		"message":    message,
		"request_id": requestID(c),
	})
}

func estimatedSeconds() int {
	return defaultEstimatedSeconds
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		failure(c, http.StatusBadRequest, 1001, "请求参数格式错误")
		return false
	}

	return true
}

func isJobNotFound(err error) bool {
	return errors.Is(err, store.ErrJobNotFound)
}
