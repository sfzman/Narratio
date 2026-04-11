package scheduler

import "github.com/sfzman/Narratio/backend/internal/model"

type TaskCounts struct {
	Total     int
	Pending   int
	Ready     int
	Running   int
	Succeeded int
	Failed    int
	Cancelled int
	Skipped   int
}

func PromoteReadyTasks(tasks []model.Task) []model.Task {
	updated := cloneSchedulingTasks(tasks)
	for {
		index := make(map[string]model.Task, len(updated))
		for _, task := range updated {
			index[task.Key] = task
		}

		changed := false
		for i, task := range updated {
			if shouldSkipTask(task, index) {
				task.Status = model.TaskStatusSkipped
				task.Error = nil
				updated[i] = task
				changed = true
				continue
			}
			if task.Status == model.TaskStatusPending && dependenciesSatisfied(task, index) {
				task.Status = model.TaskStatusReady
				updated[i] = task
				changed = true
			}
		}
		if !changed {
			return updated
		}
	}
}

func AggregateJobState(tasks []model.Task, cancellationRequested bool) (model.JobStatus, int, TaskCounts) {
	counts := countTasks(tasks)
	if counts.Total == 0 {
		return model.JobStatusQueued, 0, counts
	}

	progress := completedProgress(counts)

	if cancellationRequested && hasUnfinished(counts) {
		return model.JobStatusCancelling, progress, counts
	}
	if cancellationRequested {
		return model.JobStatusCancelled, progress, counts
	}
	if counts.Running > 0 {
		return model.JobStatusRunning, progress, counts
	}
	if counts.Failed > 0 {
		return model.JobStatusFailed, progress, counts
	}
	if counts.Cancelled > 0 && !hasUnfinished(counts) {
		return model.JobStatusCancelled, progress, counts
	}
	if counts.Cancelled == counts.Total {
		return model.JobStatusCancelled, progress, counts
	}
	if counts.Succeeded+counts.Skipped == counts.Total {
		return model.JobStatusCompleted, 100, counts
	}
	if counts.Ready > 0 || counts.Pending > 0 {
		return model.JobStatusQueued, progress, counts
	}

	return model.JobStatusQueued, progress, counts
}

func countTasks(tasks []model.Task) TaskCounts {
	var counts TaskCounts
	counts.Total = len(tasks)

	for _, task := range tasks {
		switch task.Status {
		case model.TaskStatusPending:
			counts.Pending++
		case model.TaskStatusReady:
			counts.Ready++
		case model.TaskStatusRunning:
			counts.Running++
		case model.TaskStatusSucceeded:
			counts.Succeeded++
		case model.TaskStatusFailed:
			counts.Failed++
		case model.TaskStatusCancelled:
			counts.Cancelled++
		case model.TaskStatusSkipped:
			counts.Skipped++
		}
	}

	return counts
}

func completedProgress(counts TaskCounts) int {
	if counts.Total == 0 {
		return 0
	}

	done := counts.Succeeded + counts.Skipped
	return done * 100 / counts.Total
}

func hasUnfinished(counts TaskCounts) bool {
	return counts.Pending > 0 || counts.Ready > 0 || counts.Running > 0
}

func dependenciesSatisfied(task model.Task, index map[string]model.Task) bool {
	for _, depKey := range task.DependsOn {
		dep, ok := index[depKey]
		if !ok {
			return false
		}
		if dep.Status != model.TaskStatusSucceeded {
			return false
		}
	}

	return true
}

func shouldSkipTask(task model.Task, index map[string]model.Task) bool {
	if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusReady {
		return false
	}

	for _, depKey := range task.DependsOn {
		dep, ok := index[depKey]
		if !ok {
			return false
		}
		if dep.Status == model.TaskStatusFailed || dep.Status == model.TaskStatusSkipped {
			return true
		}
	}

	return false
}

func cloneSchedulingTasks(tasks []model.Task) []model.Task {
	cloned := make([]model.Task, 0, len(tasks))
	for _, task := range tasks {
		cloned = append(cloned, task)
	}

	return cloned
}
