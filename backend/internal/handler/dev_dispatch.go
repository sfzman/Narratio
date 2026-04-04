package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h Handlers) dispatchOnce(c *gin.Context) {
	if h.dispatcher == nil {
		failure(c, http.StatusInternalServerError, 5003, "调度服务未初始化")
		return
	}

	jobPublicID := c.Param("job_id")
	result, err := h.dispatcher.DispatchOnce(c.Request.Context(), jobPublicID)
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
