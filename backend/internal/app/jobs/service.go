package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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

type Service struct {
	store store.WorkflowStore
	clock Clock
}

func NewService(workflowStore store.WorkflowStore) *Service {
	return &Service{
		store: workflowStore,
		clock: realClock{},
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

	tasks := buildDefaultWorkflow(now)
	createdTasks, err := s.store.InitializeJob(ctx, &job, tasks)
	if err != nil {
		return model.Job{}, nil, fmt.Errorf("initialize job workflow: %w", err)
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

func buildDefaultWorkflow(now time.Time) []model.Task {
	return []model.Task{
		newTask("outline", model.TaskTypeOutline, model.ResourceLLMText, nil, now),
		newTask("character_sheet", model.TaskTypeCharacterSheet, model.ResourceLLMText, nil, now),
		newTask(
			"script",
			model.TaskTypeScript,
			model.ResourceLLMText,
			[]string{"outline", "character_sheet"},
			now,
		),
		newTask("tts", model.TaskTypeTTS, model.ResourceTTS, []string{"script"}, now),
		newTask("image", model.TaskTypeImage, model.ResourceImageGen, []string{"script"}, now),
		newTask("video", model.TaskTypeVideo, model.ResourceVideoRender, []string{"tts", "image"}, now),
	}
}

func newTask(
	key string,
	taskType model.TaskType,
	resourceKey model.ResourceKey,
	dependsOn []string,
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
		Payload:     map[string]any{},
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
