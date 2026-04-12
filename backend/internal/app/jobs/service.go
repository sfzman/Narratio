package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/scheduler"
	"github.com/sfzman/Narratio/backend/internal/store"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type JobRunner interface {
	Enqueue(jobID int64)
}

type JobRunController interface {
	JobRunner
	Cancel(jobID int64) bool
	IsActive(jobID int64) bool
	IsRunning(jobID int64) bool
}

type workflowJobStore interface {
	store.WorkflowStore
	store.JobStore
	store.TaskStore
}

type Service struct {
	store        workflowJobStore
	runner       JobRunner
	clock        Clock
	log          *slog.Logger
	workspaceDir string
}

const defaultVideoCount = 2

func NewService(workflowStore workflowJobStore, runner ...JobRunner) *Service {
	var jobRunner JobRunner
	if len(runner) > 0 {
		jobRunner = runner[0]
	}

	return &Service{
		store:        workflowStore,
		runner:       jobRunner,
		clock:        realClock{},
		log:          slog.Default(),
		workspaceDir: "",
	}
}

func (s *Service) SetWorkspaceDir(workspaceDir string) {
	s.workspaceDir = strings.TrimSpace(workspaceDir)
}

type CancelOutcome struct {
	Job       model.Job
	Cancelled bool
	Deleted   bool
}

func (s *Service) CreateJob(ctx context.Context, spec model.JobSpec) (model.Job, []model.Task, error) {
	normalized := normalizeSpec(spec)
	now := s.clock.Now()
	publicID, err := newPublicID("job")
	if err != nil {
		return model.Job{}, nil, fmt.Errorf("generate job public id: %w", err)
	}
	token, err := newPublicID("token")
	if err != nil {
		return model.Job{}, nil, fmt.Errorf("generate job token: %w", err)
	}

	job := model.Job{
		PublicID:  publicID,
		Token:     token,
		Status:    model.JobStatusQueued,
		Progress:  0,
		Spec:      normalized,
		Warnings:  []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	tasks := buildDefaultWorkflow(normalized, now)
	s.log.Debug("building default workflow",
		"task_count", len(tasks),
		"voice_id", normalized.Options.VoiceID,
		"image_style", normalized.Options.ImageStyle,
		"aspect_ratio", normalized.Options.AspectRatio,
	)
	createdTasks, err := s.store.InitializeJob(ctx, &job, tasks)
	if err != nil {
		s.log.Error("initialize job workflow failed",
			"job_public_id", publicID,
			"error", err,
		)
		return model.Job{}, nil, fmt.Errorf("initialize job workflow: %w", err)
	}

	s.log.Info("job created",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_count", len(createdTasks),
	)
	if s.runner != nil {
		s.runner.Enqueue(job.ID)
		s.log.Info("job enqueued for background dispatch",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
		)
	}

	return job, createdTasks, nil
}

func (s *Service) CancelJob(ctx context.Context, publicID string) (CancelOutcome, error) {
	job, err := s.store.GetJobByPublicID(ctx, publicID)
	if err != nil {
		return CancelOutcome{}, fmt.Errorf("get job by public id: %w", err)
	}
	tasks, err := s.store.ListTasksByJob(ctx, job.ID)
	if err != nil {
		return CancelOutcome{}, fmt.Errorf("list tasks by job: %w", err)
	}

	if isTerminalJobStatus(job.Status) {
		if err := s.store.DeleteJobWorkflow(ctx, job.ID); err != nil {
			return CancelOutcome{}, fmt.Errorf("delete job workflow: %w", err)
		}
		s.deleteWorkspaceArtifacts(job.PublicID)

		return CancelOutcome{
			Job:       job,
			Cancelled: false,
			Deleted:   true,
		}, nil
	}

	running := false
	if controller, ok := s.runner.(JobRunController); ok {
		running = controller.IsRunning(job.ID)
	}

	updatedTasks := make([]model.Task, 0, len(tasks))
	for _, task := range tasks {
		switch task.Status {
		case model.TaskStatusPending, model.TaskStatusReady:
			task.Status = model.TaskStatusCancelled
			task.UpdatedAt = s.clock.Now()
			if err := s.store.UpdateTask(ctx, task); err != nil {
				return CancelOutcome{}, fmt.Errorf("update cancelled task %d: %w", task.ID, err)
			}
		}
		updatedTasks = append(updatedTasks, task)
	}

	if running {
		job.Status = model.JobStatusCancelling
	} else {
		job.Status, job.Progress, _ = scheduler.AggregateJobState(updatedTasks, true)
	}
	job.UpdatedAt = s.clock.Now()
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return CancelOutcome{}, fmt.Errorf("update cancelled job: %w", err)
	}

	if controller, ok := s.runner.(JobRunController); ok && running {
		controller.Cancel(job.ID)
	}

	return CancelOutcome{
		Job:       job,
		Cancelled: true,
		Deleted:   false,
	}, nil
}

func normalizeSpec(spec model.JobSpec) model.JobSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Article = strings.TrimSpace(spec.Article)
	spec.Options.VoiceID = strings.TrimSpace(spec.Options.VoiceID)
	spec.Options.ImageStyle = strings.TrimSpace(spec.Options.ImageStyle)
	spec.Options.AspectRatio = model.ParseAspectRatio(
		string(spec.Options.AspectRatio),
	).Normalized()

	spec.Options.VoiceID = model.NormalizeVoicePresetID(spec.Options.VoiceID)
	if spec.Name == "" {
		spec.Name = defaultJobName(spec.Article, 10)
	}
	if spec.Options.ImageStyle == "" {
		spec.Options.ImageStyle = "realistic"
	}
	spec.Options.VideoCount = normalizeVideoCount(spec.Options.VideoCount)

	return spec
}

func normalizeVideoCount(value *int) *int {
	if value == nil {
		return intPtr(defaultVideoCount)
	}
	if *value < 0 {
		return intPtr(0)
	}

	return intPtr(*value)
}

func defaultJobName(article string, limit int) string {
	if limit <= 0 || article == "" {
		return ""
	}
	if utf8.RuneCountInString(article) <= limit {
		return article
	}

	runes := []rune(article)
	return string(runes[:limit])
}

func buildDefaultWorkflow(spec model.JobSpec, now time.Time) []model.Task {
	return []model.Task{
		newTask(
			"segmentation",
			model.TaskTypeSegmentation,
			model.ResourceLocalCPU,
			nil,
			map[string]any{
				"article": spec.Article,
			},
			now,
		),
		newTask(
			"outline",
			model.TaskTypeOutline,
			model.ResourceLLMText,
			nil,
			map[string]any{
				"article": spec.Article,
			},
			now,
		),
		newTask(
			"character_sheet",
			model.TaskTypeCharacterSheet,
			model.ResourceLLMText,
			nil,
			map[string]any{
				"article": spec.Article,
			},
			now,
		),
		newTask(
			"script",
			model.TaskTypeScript,
			model.ResourceLLMText,
			[]string{"segmentation", "outline", "character_sheet"},
			map[string]any{
				"article":  spec.Article,
				"voice_id": spec.Options.VoiceID,
			},
			now,
		),
		newTask(
			"character_image",
			model.TaskTypeCharacterImage,
			model.ResourceImageGen,
			[]string{"character_sheet"},
			map[string]any{
				"image_style": spec.Options.ImageStyle,
			},
			now,
		),
		newTask(
			"tts",
			model.TaskTypeTTS,
			model.ResourceTTS,
			[]string{"segmentation"},
			map[string]any{
				"voice_id": spec.Options.VoiceID,
			},
			now,
		),
		newTask(
			"image",
			model.TaskTypeImage,
			model.ResourceImageGen,
			[]string{"script", "character_image"},
			map[string]any{
				"image_style":  spec.Options.ImageStyle,
				"aspect_ratio": string(spec.Options.AspectRatio),
			},
			now,
		),
		newTask(
			"shot_video",
			model.TaskTypeShotVideo,
			model.ResourceVideoGen,
			[]string{"image"},
			map[string]any{
				"video_count":  derefInt(spec.Options.VideoCount),
				"aspect_ratio": string(spec.Options.AspectRatio),
			},
			now,
		),
		newTask(
			"video",
			model.TaskTypeVideo,
			model.ResourceVideoRender,
			[]string{"tts", "shot_video"},
			map[string]any{
				"aspect_ratio": string(spec.Options.AspectRatio),
			},
			now,
		),
	}
}

func (s *Service) deleteWorkspaceArtifacts(publicID string) {
	if strings.TrimSpace(s.workspaceDir) == "" || strings.TrimSpace(publicID) == "" {
		return
	}

	jobDir := filepath.Join(s.workspaceDir, "jobs", publicID)
	if err := os.RemoveAll(jobDir); err != nil {
		s.log.Warn("delete workspace artifacts failed", "job_public_id", publicID, "path", jobDir, "error", err)
	}
}

func intPtr(value int) *int {
	return &value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}

	return *value
}

func newTask(
	key string,
	taskType model.TaskType,
	resourceKey model.ResourceKey,
	dependsOn []string,
	payload map[string]any,
	now time.Time,
) model.Task {
	return model.Task{
		Key:         key,
		Type:        taskType,
		Status:      model.TaskStatusPending,
		ResourceKey: resourceKey,
		DependsOn:   dependsOn,
		Attempt:     0,
		MaxAttempts: 1,
		Payload:     payload,
		OutputRef:   map[string]any{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func newPublicID(prefix string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return prefix + "_" + hex.EncodeToString(buf), nil
}
