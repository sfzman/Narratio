package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h Handlers) listJobs(c *gin.Context) {
	if h.jobReader == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务查询服务未初始化")
		return
	}

	jobs, err := h.jobReader.ListJobs(c.Request.Context())
	if err != nil {
		failure(c, http.StatusInternalServerError, 5003, "查询任务列表失败")
		return
	}

	items := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, gin.H{
			"job_id":     job.PublicID,
			"name":       jobDisplayName(job),
			"status":     job.Status,
			"progress":   job.Progress,
			"created_at": job.CreatedAt,
			"updated_at": job.UpdatedAt,
		})
	}

	success(c, http.StatusOK, gin.H{
		"jobs": items,
	})
}
