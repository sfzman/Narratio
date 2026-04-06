package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sfzman/Narratio/backend/internal/model"
)

type taskDetail struct {
	ID          int64             `json:"id"`
	Key         string            `json:"key"`
	Type        model.TaskType    `json:"type"`
	Status      model.TaskStatus  `json:"status"`
	ResourceKey model.ResourceKey `json:"resource_key"`
	DependsOn   []string          `json:"depends_on"`
	Attempt     int               `json:"attempt"`
	MaxAttempts int               `json:"max_attempts"`
	Payload     map[string]any    `json:"payload"`
	OutputRef   map[string]any    `json:"output_ref"`
	Error       *model.TaskError  `json:"error"`
	CreatedAt   any               `json:"created_at"`
	UpdatedAt   any               `json:"updated_at"`
}

func (h Handlers) getJobTasks(c *gin.Context) {
	if h.jobReader == nil || h.taskReader == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务查询服务未初始化")
		return
	}

	jobID := c.Param("job_id")
	if _, err := h.jobReader.GetJobByPublicID(c.Request.Context(), jobID); err != nil {
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

	items := make([]taskDetail, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, taskDetail{
			ID:          task.ID,
			Key:         task.Key,
			Type:        task.Type,
			Status:      task.Status,
			ResourceKey: task.ResourceKey,
			DependsOn:   nonNilStrings(task.DependsOn),
			Attempt:     task.Attempt,
			MaxAttempts: task.MaxAttempts,
			Payload:     nonNilMap(task.Payload),
			OutputRef:   nonNilMap(task.OutputRef),
			Error:       task.Error,
			CreatedAt:   task.CreatedAt,
			UpdatedAt:   task.UpdatedAt,
		})
	}

	success(c, http.StatusOK, gin.H{
		"job_id": jobID,
		"tasks":  items,
	})
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

func nonNilMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}

	return values
}
