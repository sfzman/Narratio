package video

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeVideoClientTestPNG(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%q) error = %v", path, err)
	}
	defer file.Close()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("png.Encode(%q) error = %v", path, err)
	}
}

func TestHTTPClientGenerateSubmitPollAndDownload(t *testing.T) {
	t.Parallel()

	var submitCalls int32
	var queryCalls int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/aigc/video-generation/video-synthesis":
			atomic.AddInt32(&submitCalls, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer test-video-key" {
				t.Fatalf("Authorization = %q", got)
			}
			if got := r.Header.Get("X-DashScope-Async"); got != "enable" {
				t.Fatalf("X-DashScope-Async = %q", got)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(submit payload) error = %v", err)
			}
			input := payload["input"].(map[string]any)
			params := payload["parameters"].(map[string]any)
			if input["img_url"] != "https://example.com/source.jpg" {
				t.Fatalf("img_url = %#v", input["img_url"])
			}
			if input["prompt"] != "镜头保持稳定推进" {
				t.Fatalf("prompt = %#v", input["prompt"])
			}
			if params["resolution"] != "720P" {
				t.Fatalf("resolution = %#v", params["resolution"])
			}
			if params["shot_type"] != "multi" {
				t.Fatalf("shot_type = %#v", params["shot_type"])
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"request_id": "req-submit-123",
				"output": map[string]any{
					"task_id":     "task-123",
					"task_status": "PENDING",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/task-123":
			call := atomic.AddInt32(&queryCalls, 1)
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"request_id": "req-query-1",
					"output": map[string]any{
						"task_id":     "task-123",
						"task_status": "RUNNING",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"request_id": "req-query-2",
				"output": map[string]any{
					"task_id":       "task-123",
					"task_status":   "SUCCEEDED",
					"video_url":     server.URL + "/download/generated.mp4",
					"actual_prompt": "扩写后的提示词",
					"orig_prompt":   "镜头保持稳定推进",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/download/generated.mp4":
			_, _ = w.Write([]byte("fake-video-data"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-video-key", GenerationConfig{
		Model:        "wan2.6-i2v-flash",
		PollInterval: 5 * time.Millisecond,
		MaxWait:      time.Second,
	}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	client.wait = func(context.Context, time.Duration) error { return nil }

	response, err := client.Generate(context.Background(), Request{
		Model:           "wan2.6-i2v-flash",
		Prompt:          "镜头保持稳定推进",
		SegmentIndex:    0,
		ShotIndex:       0,
		SourceImagePath: "/tmp/unused.jpg",
		SourceImageURL:  "https://example.com/source.jpg",
		DurationSeconds: 6,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.ProviderTaskID != "task-123" {
		t.Fatalf("ProviderTaskID = %q", response.ProviderTaskID)
	}
	if response.ProviderStatus != "SUCCEEDED" {
		t.Fatalf("ProviderStatus = %q", response.ProviderStatus)
	}
	if response.RequestID != "req-query-2" {
		t.Fatalf("RequestID = %q", response.RequestID)
	}
	if response.VideoURL != server.URL+"/download/generated.mp4" {
		t.Fatalf("VideoURL = %q", response.VideoURL)
	}
	if string(response.VideoData) != "fake-video-data" {
		t.Fatalf("VideoData = %q", string(response.VideoData))
	}
	if response.ActualPrompt != "扩写后的提示词" {
		t.Fatalf("ActualPrompt = %q", response.ActualPrompt)
	}
	if response.OriginalPrompt != "镜头保持稳定推进" {
		t.Fatalf("OriginalPrompt = %q", response.OriginalPrompt)
	}
}

func TestHTTPClientGenerateFallsBackFromRemoteURLToLocalDataURL(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	imagePath := filepath.Join(tempDir, "source.png")
	writeVideoClientTestPNG(t, imagePath)

	var submitCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/aigc/video-generation/video-synthesis":
			call := atomic.AddInt32(&submitCalls, 1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(submit payload) error = %v", err)
			}
			input := payload["input"].(map[string]any)
			imgURL := input["img_url"].(string)

			if call == 1 {
				if imgURL != "https://example.com/remote.png" {
					t.Fatalf("first img_url = %q", imgURL)
				}
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    "BadImageURL",
					"message": "remote image rejected",
				})
				return
			}
			if !strings.HasPrefix(imgURL, "data:image/") {
				t.Fatalf("second img_url = %q, want data url", imgURL)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"request_id": "req-submit-local",
				"output": map[string]any{
					"task_id":     "task-local",
					"task_status": "PENDING",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/task-local":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"request_id": "req-query-local",
				"output": map[string]any{
					"task_id":     "task-local",
					"task_status": "SUCCEEDED",
					"video_url":   server.URL + "/download/local.mp4",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/download/local.mp4":
			_, _ = w.Write([]byte("local-video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-video-key", GenerationConfig{
		Model:        "wan2.6-i2v-flash",
		PollInterval: 5 * time.Millisecond,
		MaxWait:      time.Second,
	}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	client.wait = func(context.Context, time.Duration) error { return nil }

	response, err := client.Generate(context.Background(), Request{
		Model:           "wan2.6-i2v-flash",
		Prompt:          "镜头向前推进",
		SegmentIndex:    0,
		ShotIndex:       1,
		SourceImagePath: imagePath,
		SourceImageURL:  "https://example.com/remote.png",
		DurationSeconds: 6,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if atomic.LoadInt32(&submitCalls) != 2 {
		t.Fatalf("submitCalls = %d, want 2", atomic.LoadInt32(&submitCalls))
	}
	if string(response.VideoData) != "local-video" {
		t.Fatalf("VideoData = %q", string(response.VideoData))
	}
}

func TestHTTPClientGenerateReturnsErrorWhenTaskFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/aigc/video-generation/video-synthesis":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"request_id": "req-submit-failed",
				"output": map[string]any{
					"task_id":     "task-failed",
					"task_status": "PENDING",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/task-failed":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"request_id": "req-query-failed",
				"output": map[string]any{
					"task_id":     "task-failed",
					"task_status": "FAILED",
					"message":     "provider failed",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-video-key", GenerationConfig{
		Model:        "wan2.6-i2v-flash",
		PollInterval: 5 * time.Millisecond,
		MaxWait:      time.Second,
	}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	client.wait = func(context.Context, time.Duration) error { return nil }

	_, err = client.Generate(context.Background(), Request{
		Model:           "wan2.6-i2v-flash",
		Prompt:          "镜头失败案例",
		SegmentIndex:    1,
		ShotIndex:       0,
		SourceImageURL:  "https://example.com/source.jpg",
		DurationSeconds: 6,
	})
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("Generate() error = %v, want provider failed", err)
	}
}
