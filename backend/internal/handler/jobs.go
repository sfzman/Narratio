package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sfzman/Narratio/backend/internal/model"
)

type createJobRequest struct {
	Article string              `json:"article"`
	Options createRenderOptions `json:"options"`
}

type createRenderOptions struct {
	VoiceID    string `json:"voice_id"`
	ImageStyle string `json:"image_style"`
}

func (h Handlers) createJob(c *gin.Context) {
	if h.jobs == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务服务未初始化")
		return
	}

	var request createJobRequest
	if !bindJSON(c, &request) {
		return
	}

	spec, err := validateCreateJobRequest(request)
	if err != nil {
		failure(c, http.StatusBadRequest, 1001, err.Error())
		return
	}

	job, _, err := h.jobs.CreateJob(c.Request.Context(), spec)
	if err != nil {
		failure(c, http.StatusInternalServerError, 5003, "创建任务失败")
		return
	}

	success(c, http.StatusAccepted, gin.H{
		"job_id":            job.PublicID,
		"status":            job.Status,
		"created_at":        job.CreatedAt,
		"estimated_seconds": estimatedSeconds(),
	})
}

func validateCreateJobRequest(request createJobRequest) (model.JobSpec, error) {
	article := strings.TrimSpace(request.Article)
	if article == "" {
		return model.JobSpec{}, errInvalidArticle("文章内容不能为空")
	}
	if len([]rune(article)) > 10000 {
		return model.JobSpec{}, errInvalidArticle("文章内容不能超过10000字")
	}

	return model.JobSpec{
		Article: article,
		Options: model.RenderOptions{
			VoiceID:    strings.TrimSpace(request.Options.VoiceID),
			ImageStyle: strings.TrimSpace(request.Options.ImageStyle),
		},
	}, nil
}

type invalidArticleError string

func (e invalidArticleError) Error() string {
	return string(e)
}

func errInvalidArticle(message string) error {
	return invalidArticleError(message)
}
