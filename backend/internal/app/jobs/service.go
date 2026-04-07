package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
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

type Service struct {
	store  store.WorkflowStore
	runner JobRunner
	clock  Clock
	log    *slog.Logger
}

func NewService(workflowStore store.WorkflowStore, runner ...JobRunner) *Service {
	var jobRunner JobRunner
	if len(runner) > 0 {
		jobRunner = runner[0]
	}

	return &Service{
		store:  workflowStore,
		runner: jobRunner,
		clock:  realClock{},
		log:    slog.Default(),
	}
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
		"language", normalized.Language,
		"voice_id", normalized.Options.VoiceID,
		"image_style", normalized.Options.ImageStyle,
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

func normalizeSpec(spec model.JobSpec) model.JobSpec {
	spec.Article = strings.TrimSpace(spec.Article)
	spec.Language = strings.TrimSpace(spec.Language)
	spec.Options.VoiceID = strings.TrimSpace(spec.Options.VoiceID)
	spec.Options.ImageStyle = strings.TrimSpace(spec.Options.ImageStyle)

	if spec.Language == "" {
		spec.Language = "zh"
	}
	if spec.Options.VoiceID == "" {
		spec.Options.VoiceID = "default"
	}
	if spec.Options.ImageStyle == "" {
		spec.Options.ImageStyle = "realistic"
	}

	return spec
}

func buildDefaultWorkflow(spec model.JobSpec, now time.Time) []model.Task {
	return []model.Task{
		newTask(
			"segmentation",
			model.TaskTypeSegmentation,
			model.ResourceLocalCPU,
			nil,
			map[string]any{
				"article":  spec.Article,
				"language": spec.Language,
			},
			now,
		),
		newTask(
			"outline",
			model.TaskTypeOutline,
			model.ResourceLLMText,
			nil,
			map[string]any{
				"article":  spec.Article,
				"language": spec.Language,
			},
			now,
		),
		newTask(
			"character_sheet",
			model.TaskTypeCharacterSheet,
			model.ResourceLLMText,
			nil,
			map[string]any{
				"article":  spec.Article,
				"language": spec.Language,
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
				"language": spec.Language,
				"voice_id": spec.Options.VoiceID,
			},
			now,
		),
		newTask(
			"character_image",
			model.TaskTypeCharacterImage,
			model.ResourceImageGen,
			[]string{"character_sheet"},
			map[string]any{},
			now,
		),
		newTask(
			"tts",
			model.TaskTypeTTS,
			model.ResourceTTS,
			[]string{"script"},
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
				"image_style": spec.Options.ImageStyle,
			},
			now,
		),
		newTask(
			"video",
			model.TaskTypeVideo,
			model.ResourceVideoRender,
			[]string{"tts", "image"},
			map[string]any{},
			now,
		),
	}
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
