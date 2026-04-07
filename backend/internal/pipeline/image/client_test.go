package image

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientGenerate(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/multimodal-generation/generation":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("authorization = %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"request_id":"req_img_1","output":{"choices":[{"message":{"content":[{"image":"` + serverURL + `/generated/test.jpg"}]}}]}}`))
		case "/generated/test.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fake-jpeg-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewHTTPClient(server.URL+"/api/v1", "test-key", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}

	response, err := client.Generate(context.Background(), Request{
		Model:          "qwen-image-2.0",
		Prompt:         "night rain on the bridge",
		Size:           "1280*720",
		NegativePrompt: "人物面部特写, 模糊, 低质量",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if response.RequestID != "req_img_1" {
		t.Fatalf("RequestID = %q", response.RequestID)
	}
	if response.ImageURL == "" {
		t.Fatal("ImageURL = empty, want non-empty")
	}
	if string(response.ImageData) != "fake-jpeg-bytes" {
		t.Fatalf("ImageData = %q", string(response.ImageData))
	}
	if requestBody["model"] != "qwen-image-2.0" {
		t.Fatalf("model = %#v", requestBody["model"])
	}
	input, ok := requestBody["input"].(map[string]any)
	if !ok {
		t.Fatalf("input = %#v, want object", requestBody["input"])
	}
	if _, ok := input["messages"].([]any); !ok {
		t.Fatalf("input.messages = %#v, want array", input["messages"])
	}
	parameters, ok := requestBody["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v, want object", requestBody["parameters"])
	}
	if parameters["size"] != "1280*720" {
		t.Fatalf("parameters.size = %#v", parameters["size"])
	}
	if parameters["negative_prompt"] != "人物面部特写, 模糊, 低质量" {
		t.Fatalf("parameters.negative_prompt = %#v", parameters["negative_prompt"])
	}
}
