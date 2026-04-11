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

type taskStateSnapshot struct {
	ReadyKeys   []string `json:"ready_keys"`
	RunningKeys []string `json:"running_keys"`
	FailedKeys  []string `json:"failed_keys"`
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

	summary := summarizeTasks(tasks)
	snapshot := snapshotTaskState(tasks)

	success(c, http.StatusOK, gin.H{
		"job_id":       job.PublicID,
		"status":       job.Status,
		"progress":     job.Progress,
		"created_at":   job.CreatedAt,
		"updated_at":   job.UpdatedAt,
		"tasks":        summary,
		"task_state":   snapshot,
		"runtime_hint": buildRuntimeHint(summary, snapshot),
		"warnings":     job.Warnings,
		"error":        job.Error,
		"result":       buildJobResult(job),
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

func snapshotTaskState(tasks []model.Task) taskStateSnapshot {
	snapshot := taskStateSnapshot{
		ReadyKeys:   []string{},
		RunningKeys: []string{},
		FailedKeys:  []string{},
	}

	for _, task := range tasks {
		switch task.Status {
		case model.TaskStatusReady:
			snapshot.ReadyKeys = append(snapshot.ReadyKeys, task.Key)
		case model.TaskStatusRunning:
			snapshot.RunningKeys = append(snapshot.RunningKeys, task.Key)
		case model.TaskStatusFailed:
			snapshot.FailedKeys = append(snapshot.FailedKeys, task.Key)
		}
	}

	return snapshot
}

func buildRuntimeHint(summary taskSummary, snapshot taskStateSnapshot) string {
	if len(snapshot.FailedKeys) > 0 {
		return "存在失败 task，请查看 task 明细里的 error 字段。"
	}
	if summary.Running == 0 && len(snapshot.ReadyKeys) > 0 {
		return "当前有 ready task 等待后台调度；若长时间没有变化，请检查日志或手动点击 Dispatch Once。"
	}
	if summary.Running == 0 && summary.Pending > 0 && summary.Succeeded > 0 {
		return "当前没有运行中的 task。若依赖已满足，刷新后查看是否出现 ready task。"
	}
	if summary.Running > 0 {
		return "当前有 task 处于运行中，可继续刷新查看进展。"
	}

	return ""
}
