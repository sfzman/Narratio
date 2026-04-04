package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sfzman/Narratio/backend/internal/model"
)

type taskSummary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Ready     int `json:"ready"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
	Skipped   int `json:"skipped"`
}

func (h Handlers) getJob(c *gin.Context) {
	if h.jobReader == nil || h.taskReader == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务查询服务未初始化")
		return
	}

	jobID := c.Param("job_id")
	job, err := h.jobReader.GetJobByPublicID(c.Request.Context(), jobID)
	if err != nil {
		if isJobNotFound(err) {
			failure(c, http.StatusNotFound, 1002, "任务不存在")
			return
		}
		failure(c, http.StatusInternalServerError, 5003, "查询任务失败")
		return
	}

	tasks, err := h.taskReader.ListTasksByJobPublicID(c.Request.Context(), jobID)
	if err != nil {
		failure(c, http.StatusInternalServerError, 5003, "查询任务失败")
		return
	}

	success(c, http.StatusOK, gin.H{
		"job_id":     job.PublicID,
		"status":     job.Status,
		"progress":   job.Progress,
		"created_at": job.CreatedAt,
		"updated_at": job.UpdatedAt,
		"tasks":      summarizeTasks(tasks),
		"warnings":   job.Warnings,
		"error":      job.Error,
		"result":     buildJobResult(job),
	})
}

func summarizeTasks(tasks []model.Task) taskSummary {
	summary := taskSummary{
		Total: len(tasks),
	}

	for _, task := range tasks {
		switch task.Status {
		case model.TaskStatusPending:
			summary.Pending++
		case model.TaskStatusReady:
			summary.Ready++
		case model.TaskStatusRunning:
			summary.Running++
		case model.TaskStatusSucceeded:
			summary.Succeeded++
		case model.TaskStatusFailed:
			summary.Failed++
		case model.TaskStatusCancelled:
			summary.Cancelled++
		case model.TaskStatusSkipped:
			summary.Skipped++
		}
	}

	return summary
}

func buildJobResult(job model.Job) any {
	if job.Result == nil {
		return nil
	}

	return gin.H{
		"video_url": "/api/v1/jobs/" + job.PublicID + "/download",
		"duration":  job.Result.Duration,
		"file_size": job.Result.FileSize,
	}
}
