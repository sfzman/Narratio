package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	jobapp "github.com/sfzman/Narratio/backend/internal/app/jobs"
	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/store"
)

type fakeJobCreator struct {
	job           model.Job
	spec          model.JobSpec
	err           error
	cancelOutcome jobapp.CancelOutcome
	cancelErr     error
}

func (f *fakeJobCreator) CreateJob(_ context.Context, spec model.JobSpec) (model.Job, []model.Task, error) {
	f.spec = spec
	if f.err != nil {
		return model.Job{}, nil, f.err
	}

	return f.job, nil, nil
}

func (f *fakeJobCreator) CancelJob(_ context.Context, _ string) (jobapp.CancelOutcome, error) {
	if f.cancelErr != nil {
		return jobapp.CancelOutcome{}, f.cancelErr
	}

	return f.cancelOutcome, nil
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
		Resources: map[string]int{
			"local_cpu":    4,
			"llm_text":     2,
			"tts":          3,
			"image_gen":    2,
			"video_gen":    1,
			"video_render": 1,
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
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"resources":{"image_gen":2,"llm_text":2,"local_cpu":4,"tts":3,"video_gen":1,"video_render":1}`)) {
		t.Fatalf("body = %s", recorder.Body.String())
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

func TestListVoices(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/voices", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}

	var response struct {
		Code int `json:"code"`
		Data struct {
			DefaultVoiceID string `json:"default_voice_id"`
			Voices         []struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				ReferenceAudio string `json:"reference_audio"`
				PreviewURL     string `json:"preview_url"`
			} `json:"voices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.Data.DefaultVoiceID != model.DefaultVoicePresetID {
		t.Fatalf("default_voice_id = %q, want %q", response.Data.DefaultVoiceID, model.DefaultVoicePresetID)
	}
	if len(response.Data.Voices) != len(model.DefaultVoicePresets()) {
		t.Fatalf("voices len = %d, want %d", len(response.Data.Voices), len(model.DefaultVoicePresets()))
	}
	if response.Data.Voices[0].ID == "" || response.Data.Voices[0].Name == "" {
		t.Fatalf("first voice = %#v", response.Data.Voices[0])
	}
	if response.Data.Voices[0].ReferenceAudio == "" || response.Data.Voices[0].PreviewURL == "" {
		t.Fatalf("first voice preview/reference = %#v", response.Data.Voices[0])
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
			"voice_id":     "male_calm",
			"image_style":  "realistic",
			"aspect_ratio": "16:9",
			"video_count":  5,
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
	if service.spec.Options.VoiceID != "male_calm" {
		t.Fatalf("voice_id = %q", service.spec.Options.VoiceID)
	}
	if service.spec.Options.AspectRatio != model.AspectRatioLandscape16x9 {
		t.Fatalf("aspect_ratio = %q", service.spec.Options.AspectRatio)
	}
	if service.spec.Options.VideoCount == nil || *service.spec.Options.VideoCount != 5 {
		t.Fatalf("video_count = %#v", service.spec.Options.VideoCount)
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

func TestCreateJobRejectsNegativeVideoCount(t *testing.T) {
	router := NewRouter(&fakeJobCreator{}, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/jobs",
		bytes.NewBufferString(`{"article":"hello","options":{"video_count":-1}}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestCreateJobRejectsInvalidAspectRatio(t *testing.T) {
	router := NewRouter(&fakeJobCreator{}, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/jobs",
		bytes.NewBufferString(`{"article":"hello","options":{"aspect_ratio":"1:1"}}`),
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

func TestDownloadJobVideo(t *testing.T) {
	workspaceDir := t.TempDir()
	videoRelativePath := filepath.ToSlash(filepath.Join("jobs", "job_abc123", "output", "final.mp4"))
	videoPath := filepath.Join(workspaceDir, filepath.Clean(videoRelativePath))
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	videoData := []byte("fake-mp4-binary")
	if err := os.WriteFile(videoPath, videoData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	router := NewRouter(
		nil,
		&fakeJobReader{
			job: model.Job{
				PublicID: "job_abc123",
				Status:   model.JobStatusCompleted,
				Result: &model.JobResult{
					VideoPath: videoRelativePath,
					FileSize:  int64(len(videoData)),
				},
			},
		},
		nil,
		nil,
		HealthStatus{},
		workspaceDir,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_abc123/download", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="narratio_job_abc123.mp4"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), videoData) {
		t.Fatalf("body = %q", recorder.Body.Bytes())
	}
}

func TestDownloadJobVideoSupportsRangeRequests(t *testing.T) {
	workspaceDir := t.TempDir()
	videoRelativePath := filepath.ToSlash(filepath.Join("jobs", "job_range", "output", "final.mp4"))
	videoPath := filepath.Join(workspaceDir, filepath.Clean(videoRelativePath))
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	videoData := []byte("0123456789")
	if err := os.WriteFile(videoPath, videoData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	router := NewRouter(
		nil,
		&fakeJobReader{
			job: model.Job{
				PublicID: "job_range",
				Status:   model.JobStatusCompleted,
				Result: &model.JobResult{
					VideoPath: videoRelativePath,
					FileSize:  int64(len(videoData)),
				},
			},
		},
		nil,
		nil,
		HealthStatus{},
		workspaceDir,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_range/download", nil)
	request.Header.Set("Range", "bytes=0-3")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q", got)
	}
	if got := recorder.Header().Get("Content-Range"); got != fmt.Sprintf("bytes 0-3/%d", len(videoData)) {
		t.Fatalf("Content-Range = %q", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), []byte("0123")) {
		t.Fatalf("body = %q", recorder.Body.Bytes())
	}
}

func TestDownloadJobVideoRejectsIncompleteJob(t *testing.T) {
	router := NewRouter(
		nil,
		&fakeJobReader{
			job: model.Job{PublicID: "job_running", Status: model.JobStatusRunning},
		},
		nil,
		nil,
		HealthStatus{},
		t.TempDir(),
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_running/download", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":1003`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestDownloadJobVideoReturnsInternalErrorWhenFileMissing(t *testing.T) {
	workspaceDir := t.TempDir()
	router := NewRouter(
		nil,
		&fakeJobReader{
			job: model.Job{
				PublicID: "job_missing_video",
				Status:   model.JobStatusCompleted,
				Result: &model.JobResult{
					VideoPath: "jobs/job_missing_video/output/final.mp4",
				},
			},
		},
		nil,
		nil,
		HealthStatus{},
		workspaceDir,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job_missing_video/download", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":5002`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestCancelJob(t *testing.T) {
	service := &fakeJobCreator{
		cancelOutcome: jobapp.CancelOutcome{
			Job: model.Job{
				PublicID: "job_cancel_123",
				Status:   model.JobStatusCancelling,
			},
			Cancelled: true,
			Deleted:   false,
		},
	}
	router := NewRouter(service, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job_cancel_123", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"cancelled":true`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"status":"cancelling"`)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestCancelJobNotFound(t *testing.T) {
	service := &fakeJobCreator{cancelErr: store.ErrJobNotFound}
	router := NewRouter(service, nil, nil, nil, HealthStatus{})

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job_missing", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
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
				Dispatched:          true,
				ExecutedTaskID:      11,
				ExecutedTaskKey:     "outline",
				ExecutedTaskIDs:     []int64{11, 12},
				ExecutedTaskKeys:    []string{"outline", "character_sheet"},
				DispatchedTaskCount: 2,
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
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"dispatched_task_count":2`)) {
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
				Dispatched:          true,
				ExecutedTaskID:      14,
				ExecutedTaskKey:     "character_sheet",
				ExecutedTaskIDs:     []int64{14},
				ExecutedTaskKeys:    []string{"character_sheet"},
				DispatchedTaskCount: 1,
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
