package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jobapp "github.com/sfzman/Narratio/backend/internal/app/jobs"
	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/store"
)

type fakeJobCreator struct {
	job  model.Job
	spec model.JobSpec
	err  error
}

func (f *fakeJobCreator) CreateJob(_ context.Context, spec model.JobSpec) (model.Job, []model.Task, error) {
	f.spec = spec
	if f.err != nil {
		return model.Job{}, nil, f.err
	}

	return f.job, nil, nil
}

type fakeJobReader struct {
	job model.Job
	err error
}

func (f *fakeJobReader) GetJobByPublicID(_ context.Context, _ string) (model.Job, error) {
	if f.err != nil {
		return model.Job{}, f.err
	}

	return f.job, nil
}

type fakeTaskReader struct {
	tasks []model.Task
	err   error
}

func (f *fakeTaskReader) ListTasksByJobPublicID(_ context.Context, _ string) ([]model.Task, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.tasks, nil
}

type fakeDispatcher struct {
	outcome jobapp.DispatchOutcome
	err     error
	call    func(context.Context, string)
}

func (f *fakeDispatcher) DispatchOnce(ctx context.Context, publicID string) (jobapp.DispatchOutcome, error) {
	if f.call != nil {
		f.call(ctx, publicID)
	}

	if f.err != nil {
		return jobapp.DispatchOutcome{}, f.err
	}

	return f.outcome, nil
}

func TestHealthCheck(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, HealthStatus{
		Version: "dev",
		Services: map[string]string{
			"database":       "ok",
			"dashscope_text": "configured",
			"tts":            "not_configured",
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := recorder.Body.String(); got == "" {
		t.Fatal("body is empty")
	}
}

func TestCORSPreflight(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/jobs", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("Access-Control-Allow-Methods is empty")
	}
}

func TestCreateJob(t *testing.T) {
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	service := &fakeJobCreator{
		job: model.Job{
			PublicID:  "job_abc123",
			Status:    model.JobStatusQueued,
			CreatedAt: now,
		},
	}
	router := NewRouter(service, nil, nil, nil, HealthStatus{})

	body, err := json.Marshal(map[string]any{
		"article": "hello world",
		"options": map[string]any{
			"voice_id":    "default",
			"image_style": "realistic",
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d", recorder.Code)
	}
	if service.spec.Article != "hello world" {
		t.Fatalf("article = %q", service.spec.Article)
	}
	if service.spec.Options.VoiceID != "default" {
		t.Fatalf("voice_id = %q", service.spec.Options.VoiceID)
	}
}

func TestCreateJobRejectsEmptyArticle(t *testing.T) {
	router := NewRouter(&fakeJobCreator{}, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/jobs",
		bytes.NewBufferString(`{"article":"   "}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGetJob(t *testing.T) {
	now := time.Date(2026, 4, 4, 13, 0, 0, 0, time.UTC)
	router := NewRouter(
		nil,
		&fakeJobReader{
			job: model.Job{
				PublicID:  "job_abc123",
				Status:    model.JobStatusRunning,
				Progress:  66,
				Warnings:  []string{"image fallback"},
				CreatedAt: now,
				UpdatedAt: now.Add(time.Minute),
			},
		},
		&fakeTaskReader{
			tasks: []model.Task{
				{Status: model.TaskStatusSucceeded},
				{Status: model.TaskStatusSucceeded},
				{Status: model.TaskStatusRunning},
			},
		},
		nil,
		HealthStatus{},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_abc123", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"job_id":"job_abc123"`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"running":1`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"runtime_hint":"当前有 task 处于运行中，可继续刷新查看进展。"`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestGetJobNotFound(t *testing.T) {
	router := NewRouter(
		nil,
		&fakeJobReader{err: store.ErrJobNotFound},
		&fakeTaskReader{},
		nil,
		HealthStatus{},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_missing", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGetJobTasks(t *testing.T) {
	now := time.Date(2026, 4, 4, 14, 0, 0, 0, time.UTC)
	router := NewRouter(
		nil,
		&fakeJobReader{
			job: model.Job{PublicID: "job_abc123"},
		},
		&fakeTaskReader{
			tasks: []model.Task{
				{
					ID:          11,
					Key:         "outline",
					Type:        model.TaskTypeOutline,
					Status:      model.TaskStatusSucceeded,
					ResourceKey: model.ResourceLLMText,
					DependsOn:   []string{},
					Attempt:     1,
					MaxAttempts: 1,
					Payload: map[string]any{
						"article": "hello",
					},
					OutputRef: map[string]any{
						"artifact_path": "jobs/job_abc123/outline.json",
					},
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
		},
		nil,
		HealthStatus{},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_abc123/tasks", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"key":"outline"`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"artifact_path":"jobs/job_abc123/outline.json"`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestGetJobReturnsInternalError(t *testing.T) {
	router := NewRouter(
		nil,
		&fakeJobReader{err: errors.New("db down")},
		&fakeTaskReader{},
		nil,
		HealthStatus{},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_broken", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestDispatchOnce(t *testing.T) {
	router := NewRouter(
		nil,
		nil,
		nil,
		&fakeDispatcher{
			outcome: jobapp.DispatchOutcome{
				Job: model.Job{
					PublicID: "job_abc123",
					Status:   model.JobStatusQueued,
					Progress: 33,
				},
				Dispatched:      true,
				ExecutedTaskID:  11,
				ExecutedTaskKey: "outline",
			},
		},
		HealthStatus{},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job_abc123/dispatch-once", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"executed_task_key":"outline"`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestDispatchOnceUsesDetachedContext(t *testing.T) {
	router := NewRouter(
		nil,
		nil,
		nil,
		&fakeDispatcher{
			outcome: jobapp.DispatchOutcome{
				Job: model.Job{
					PublicID: "job_abc123",
					Status:   model.JobStatusRunning,
					Progress: 50,
				},
				Dispatched:      true,
				ExecutedTaskID:  14,
				ExecutedTaskKey: "character_sheet",
			},
			call: func(ctx context.Context, publicID string) {
				if publicID != "job_abc123" {
					t.Fatalf("publicID = %q", publicID)
				}
				if err := ctx.Err(); err != nil {
					t.Fatalf("dispatch context should not inherit request cancellation, got %v", err)
				}
			},
		},
		HealthStatus{},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job_abc123/dispatch-once", nil)
	requestCtx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(requestCtx)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
}
