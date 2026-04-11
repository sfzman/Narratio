package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h Handlers) cancelJob(c *gin.Context) {
	if h.canceler == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务服务未初始化")
		return
	}

	jobID := c.Param("job_id")
	outcome, err := h.canceler.CancelJob(c.Request.Context(), jobID)
	if err != nil {
		if isJobNotFound(err) {
			failure(c, http.StatusNotFound, 1002, "任务不存在")
			return
		}
		failure(c, http.StatusInternalServerError, 5003, "取消任务失败")
		return
	}

	success(c, http.StatusOK, gin.H{
		"cancelled": outcome.Cancelled,
		"deleted":   outcome.Deleted,
		"status":    outcome.Job.Status,
	})
}
