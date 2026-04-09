package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const dispatchOnceTimeout = 2 * time.Hour

func (h Handlers) dispatchOnce(c *gin.Context) {
	if h.dispatcher == nil {
		failure(c, http.StatusInternalServerError, 5003, "调度服务未初始化")
		return
	}

	jobPublicID := c.Param("job_id")
	dispatchCtx, cancel := context.WithTimeout(context.Background(), dispatchOnceTimeout)
	defer cancel()

	result, err := h.dispatcher.DispatchOnce(dispatchCtx, jobPublicID)
	if err != nil {
		if isJobNotFound(err) {
			failure(c, http.StatusNotFound, 1002, "任务不存在")
			return
		}
		failure(c, http.StatusInternalServerError, 5003, "推进任务失败")
		return
	}

	success(c, http.StatusOK, gin.H{
		"job_id":            result.Job.PublicID,
		"status":            result.Job.Status,
		"progress":          result.Job.Progress,
		"dispatched":        result.Dispatched,
		"executed_task_id":  result.ExecutedTaskID,
		"executed_task_key": result.ExecutedTaskKey,
	})
}
