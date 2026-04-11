package handler

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sfzman/Narratio/backend/internal/handler/streamer"
	"github.com/sfzman/Narratio/backend/internal/model"
)

func (h Handlers) downloadJobVideo(c *gin.Context) {
	if h.jobReader == nil {
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

	videoPath, ok := resolveJobVideoPath(h.workspace, job)
	if !ok {
		failure(c, http.StatusBadRequest, 1003, "任务尚未完成")
		return
	}

	filename := "narratio_" + job.PublicID + ".mp4"
	if err := streamer.ServeFile(c, videoPath, "video/mp4", filename); err != nil {
		failure(c, http.StatusInternalServerError, 5002, "视频文件不可用")
		return
	}
}

func resolveJobVideoPath(workspaceDir string, job model.Job) (string, bool) {
	if job.Status != model.JobStatusCompleted || job.Result == nil {
		return "", false
	}
	if strings.TrimSpace(workspaceDir) == "" {
		return "", false
	}
	if strings.TrimSpace(job.Result.VideoPath) == "" {
		return "", false
	}

	return filepath.Join(workspaceDir, filepath.Clean(job.Result.VideoPath)), true
}
