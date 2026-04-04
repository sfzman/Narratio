package script

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type fakeTextClient struct {
	request  TextRequest
	response TextResponse
	err      error
}

func (f *fakeTextClient) Generate(_ context.Context, request TextRequest) (TextResponse, error) {
	f.request = request
	if f.err != nil {
		return TextResponse{}, f.err
	}

	return f.response, nil
}

func TestOutlineExecutorExecuteWithInjectedTextClient(t *testing.T) {
	t.Parallel()

	client := &fakeTextClient{
		response: TextResponse{
			RequestID: "req_outline_1",
			Model:     "qwen-plus",
			Choices: []Choice{
				{Message: ChatMessage{Role: "assistant", Content: `{"segments":[]}`}},
			},
		},
	}
	executor := NewOutlineExecutorWithClient(client, TextGenerationConfig{
		Model: "qwen-plus",
	})
	job := model.Job{ID: 1, PublicID: "job_outline_llm"}
	task := model.Task{
		ID:      10,
		Key:     "outline",
		Type:    model.TaskTypeOutline,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "This is a test article for outline generation.",
			"language": "en",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if client.request.Model != "qwen-plus" {
		t.Fatalf("request model = %q", client.request.Model)
	}
	if len(client.request.Messages) != 2 {
		t.Fatalf("request messages = %d", len(client.request.Messages))
	}
	if got.OutputRef["llm_request_id"] != "req_outline_1" {
		t.Fatalf("llm_request_id = %#v", got.OutputRef["llm_request_id"])
	}
	if got.OutputRef["llm_model"] != "qwen-plus" {
		t.Fatalf("llm_model = %#v", got.OutputRef["llm_model"])
	}
	if got.OutputRef["llm_response_preview"] != `{"segments":[]}` {
		t.Fatalf("llm_response_preview = %#v", got.OutputRef["llm_response_preview"])
	}
}

func TestScriptExecutorExecuteWithInjectedTextClient(t *testing.T) {
	t.Parallel()

	client := &fakeTextClient{
		response: TextResponse{
			RequestID: "req_script_1",
			Choices: []Choice{
				{Message: ChatMessage{Role: "assistant", Content: `{"segments":[{"text":"hello"}]}`}},
			},
		},
	}
	executor := NewScriptExecutorWithClient(client, TextGenerationConfig{
		Model: "qwen-plus",
	})
	job := model.Job{ID: 2, PublicID: "job_script_llm"}
	task := model.Task{
		ID:   20,
		Key:  "script",
		Type: model.TaskTypeScript,
		Payload: map[string]any{
			"article":  "A short article for script generation.",
			"language": "en",
			"voice_id": "default",
		},
		OutputRef: map[string]any{},
	}
	dependencies := map[string]model.Task{
		"outline": {
			Key: "outline",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_llm/outline.json",
			},
		},
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_llm/character_sheet.json",
			},
		},
	}

	got, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	userPrompt := client.request.Messages[1].Content
	if !strings.Contains(userPrompt, "jobs/job_script_llm/outline.json") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, "jobs/job_script_llm/character_sheet.json") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if got.OutputRef["llm_request_id"] != "req_script_1" {
		t.Fatalf("llm_request_id = %#v", got.OutputRef["llm_request_id"])
	}
}

func TestOutlineExecutorExecuteReturnsClientError(t *testing.T) {
	t.Parallel()

	executor := NewOutlineExecutorWithClient(&fakeTextClient{
		err: fmt.Errorf("upstream unavailable"),
	}, TextGenerationConfig{})
	job := model.Job{ID: 1, PublicID: "job_outline_error"}
	task := model.Task{
		ID:      10,
		Key:     "outline",
		Type:    model.TaskTypeOutline,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "This is a test article for outline generation.",
			"language": "en",
		},
		OutputRef: map[string]any{},
	}

	_, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err == nil {
		t.Fatal("Execute() error = nil, want client error")
	}
}
