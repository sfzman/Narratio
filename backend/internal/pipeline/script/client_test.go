package script

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPTextClientGenerate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatible-mode/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q", got)
		}

		var request TextRequest
		if err := decodeRequest(r, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "qwen-max" {
			t.Fatalf("model = %q", request.Model)
		}
		if request.MaxTokens != 4096 {
			t.Fatalf("max_tokens = %d", request.MaxTokens)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("messages = %d", len(request.Messages))
		}
		if request.Messages[0].Role != "system" {
			t.Fatalf("messages[0].role = %q", request.Messages[0].Role)
		}
		if request.Messages[1].Content != "outline this article" {
			t.Fatalf("messages[1].content = %q", request.Messages[1].Content)
		}
		if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
			t.Fatalf("response_format = %#v", request.ResponseFormat)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_123",
			"request_id":"req_123",
			"choices":[
				{"index":0,"message":{"role":"assistant","content":"{\"segments\":[]}"}}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPTextClient(server.URL+"/compatible-mode/v1", "test-key", &http.Client{
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPTextClient() error = %v", err)
	}

	response, err := client.Generate(context.Background(), TextRequest{
		Model:     "qwen-max",
		MaxTokens: 4096,
		Messages: []ChatMessage{
			{Role: "system", Content: "respond in JSON"},
			{Role: "user", Content: "outline this article"},
		},
		ResponseFormat: &ResponseFormatSpec{Type: "json_object"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if response.ID != "chatcmpl_123" {
		t.Fatalf("id = %q", response.ID)
	}
	if response.RequestID != "req_123" {
		t.Fatalf("request_id = %q", response.RequestID)
	}
	text, err := response.FirstText()
	if err != nil {
		t.Fatalf("FirstText() error = %v", err)
	}
	if text != "{\"segments\":[]}" {
		t.Fatalf("text = %q", text)
	}
}

func TestHTTPTextClientGenerateStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := NewHTTPTextClient(server.URL, "test-key", &http.Client{
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPTextClient() error = %v", err)
	}

	_, err = client.Generate(context.Background(), TextRequest{
		Model: "qwen-max",
		Messages: []ChatMessage{
			{Role: "user", Content: "retry me"},
		},
	})
	if err == nil {
		t.Fatal("Generate() error = nil, want status error")
	}

	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d", statusErr.StatusCode)
	}
	if !statusErr.Retryable() {
		t.Fatal("Retryable() = false, want true")
	}
}

func TestHTTPTextClientGenerateInvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[`))
	}))
	defer server.Close()

	client, err := NewHTTPTextClient(server.URL, "test-key", &http.Client{
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPTextClient() error = %v", err)
	}

	_, err = client.Generate(context.Background(), TextRequest{
		Model: "qwen-max",
		Messages: []ChatMessage{
			{Role: "user", Content: "bad json"},
		},
	})
	if err == nil {
		t.Fatal("Generate() error = nil, want decode error")
	}
	if !strings.Contains(err.Error(), "decode text response") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPTextClientGenerateRetriesStatusError(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_retry",
			"choices":[
				{"index":0,"message":{"role":"assistant","content":"{\"ok\":true}"}}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPTextClient(
		server.URL,
		"test-key",
		&http.Client{Timeout: time.Second},
		HTTPTextClientOptions{
			MaxRetries: 2,
			Backoff:    time.Second,
			Sleep: func(context.Context, time.Duration) error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewHTTPTextClient() error = %v", err)
	}

	response, err := client.Generate(context.Background(), TextRequest{
		Model: "qwen-max",
		Messages: []ChatMessage{
			{Role: "user", Content: "retry me"},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	text, err := response.FirstText()
	if err != nil {
		t.Fatalf("FirstText() error = %v", err)
	}
	if text != "{\"ok\":true}" {
		t.Fatalf("text = %q", text)
	}
}

func TestHTTPTextClientGenerateStopsAfterRetryLimit(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := NewHTTPTextClient(
		server.URL,
		"test-key",
		&http.Client{Timeout: time.Second},
		HTTPTextClientOptions{
			MaxRetries: 1,
			Backoff:    time.Second,
			Sleep: func(context.Context, time.Duration) error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewHTTPTextClient() error = %v", err)
	}

	_, err = client.Generate(context.Background(), TextRequest{
		Model: "qwen-max",
		Messages: []ChatMessage{
			{Role: "user", Content: "retry me"},
		},
	})
	if err == nil {
		t.Fatal("Generate() error = nil, want status error")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T, want *StatusError", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestTextResponseFirstTextNoContent(t *testing.T) {
	t.Parallel()

	_, err := TextResponse{
		Choices: []Choice{
			{Message: ChatMessage{Role: "assistant", Content: "   "}},
		},
	}.FirstText()
	if !errors.Is(err, ErrNoTextContent) {
		t.Fatalf("FirstText() error = %v, want %v", err, ErrNoTextContent)
	}
}

func TestTextResponseFirstTextNoChoices(t *testing.T) {
	t.Parallel()

	_, err := TextResponse{}.FirstText()
	if !errors.Is(err, ErrNoResponseChoices) {
		t.Fatalf("FirstText() error = %v, want %v", err, ErrNoResponseChoices)
	}
}

func TestNewHTTPTextClientRequiresTimeout(t *testing.T) {
	t.Parallel()

	_, err := NewHTTPTextClient("https://example.com", "test-key", &http.Client{})
	if !errors.Is(err, ErrHTTPClientTimeoutNotSet) {
		t.Fatalf("NewHTTPTextClient() error = %v, want %v", err, ErrHTTPClientTimeoutNotSet)
	}
}

func decodeRequest(r *http.Request, dst *TextRequest) error {
	return decodeJSON(r.Body, dst)
}

func decodeJSON(body io.Reader, dst *TextRequest) error {
	return json.NewDecoder(body).Decode(dst)
}
