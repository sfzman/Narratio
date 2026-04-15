package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	jobapp "github.com/sfzman/Narratio/backend/internal/app/jobs"
)

func (h Handlers) retryTask(c *gin.Context) {
	if h.retrier == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务服务未初始化")
		return
	}

	jobID := c.Param("job_id")
	taskKey := c.Param("task_key")
	outcome, err := h.retrier.RetryTask(c.Request.Context(), jobID, taskKey)
	if err != nil {
		switch {
		case isJobNotFound(err), isTaskNotFound(err):
			failure(c, http.StatusNotFound, 1002, "任务不存在")
			return
		case errors.Is(err, jobapp.ErrTaskRetryNotAllowed):
			failure(c, http.StatusConflict, 1004, "任务当前状态不允许该操作")
			return
		default:
			failure(c, http.StatusInternalServerError, 5003, "重试任务失败")
			return
		}
	}

	success(c, http.StatusOK, gin.H{
		"job_id":          outcome.Job.PublicID,
		"task_key":        outcome.TaskKey,
		"status":          outcome.Job.Status,
		"progress":        outcome.Job.Progress,
		"retried":         outcome.Retried,
		"reset_task_keys": outcome.ResetTaskKeys,
	})
}
