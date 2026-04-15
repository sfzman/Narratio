package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jobapp "github.com/sfzman/Narratio/backend/internal/app/jobs"
)

type renameJobRequest struct {
	Name string `json:"name"`
}

func (h Handlers) renameJob(c *gin.Context) {
	if h.renamer == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务服务未初始化")
		return
	}

	var request renameJobRequest
	if !bindJSON(c, &request) {
		return
	}

	name := strings.TrimSpace(request.Name)
	if name == "" {
		failure(c, http.StatusBadRequest, 1001, "任务名称不能为空")
		return
	}

	outcome, err := h.renamer.RenameJob(c.Request.Context(), c.Param("job_id"), name)
	if err != nil {
		switch {
		case isJobNotFound(err):
			failure(c, http.StatusNotFound, 1002, "任务不存在")
			return
		case errors.Is(err, jobapp.ErrJobNameRequired):
			failure(c, http.StatusBadRequest, 1001, "任务名称不能为空")
			return
		default:
			failure(c, http.StatusInternalServerError, 5003, "重命名任务失败")
			return
		}
	}

	success(c, http.StatusOK, gin.H{
		"job_id":     outcome.Job.PublicID,
		"name":       jobDisplayName(outcome.Job),
		"status":     outcome.Job.Status,
		"progress":   outcome.Job.Progress,
		"created_at": outcome.Job.CreatedAt,
		"updated_at": outcome.Job.UpdatedAt,
		"renamed":    outcome.Renamed,
	})
}
