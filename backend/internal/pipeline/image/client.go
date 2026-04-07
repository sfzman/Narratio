package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const (
	defaultImageSize           = "1280*720"
	defaultImageNegativePrompt = "人物面部特写, 模糊, 低质量"
)

type Client interface {
	Generate(ctx context.Context, request Request) (Response, error)
}

type Request struct {
	Model          string
	Prompt         string
	Size           string
	NegativePrompt string
}

type Response struct {
	RequestID string
	Model     string
	ImageURL  string
	ImageData []byte
}

type GenerationConfig struct {
	Model          string
	Size           string
	NegativePrompt string
}

type HTTPClient struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, apiKey string, httpClient *http.Client) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse image base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("image base url must include scheme and host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &HTTPClient{
		baseURL:    parsed,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
	}, nil
}

func normalizeGenerationConfig(cfg GenerationConfig) GenerationConfig {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "qwen-image-2.0"
	}
	if strings.TrimSpace(cfg.Size) == "" {
		cfg.Size = defaultImageSize
	}
	if strings.TrimSpace(cfg.NegativePrompt) == "" {
		cfg.NegativePrompt = defaultImageNegativePrompt
	}

	return cfg
}

func (c *HTTPClient) Generate(ctx context.Context, request Request) (Response, error) {
	payload, err := json.Marshal(buildRequestPayload(request))
	if err != nil {
		return Response{}, fmt.Errorf("marshal image request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpointURL(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return Response{}, fmt.Errorf("build image request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("send image request: %w", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read image response: %w", err)
	}
	if httpResponse.StatusCode >= 400 {
		return Response{}, fmt.Errorf("image request failed: status=%d body=%s", httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed httpResponsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Response{}, fmt.Errorf("decode image response: %w", err)
	}

	imageURL, err := parsed.firstImageURL()
	if err != nil {
		return Response{}, err
	}
	imageData, err := c.downloadImage(ctx, imageURL)
	if err != nil {
		return Response{}, err
	}

	return Response{
		RequestID: parsed.RequestID,
		Model:     request.Model,
		ImageURL:  imageURL,
		ImageData: imageData,
	}, nil
}

func buildRequestPayload(request Request) map[string]any {
	return map[string]any{
		"model": request.Model,
		"input": map[string]any{
			"messages": []map[string]any{
				{
					"role": "user",
					"content": []map[string]string{
						{"text": request.Prompt},
					},
				},
			},
		},
		"parameters": map[string]any{
			"negative_prompt": request.NegativePrompt,
			"size":            request.Size,
		},
	}
}

func (c *HTTPClient) endpointURL() string {
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, "/services/aigc/multimodal-generation/generation")
	return endpoint.String()
}

func (c *HTTPClient) downloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build image download request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read downloaded image: %w", err)
	}
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("download image failed: status=%d", response.StatusCode)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("download image failed: empty body")
	}

	return data, nil
}

type httpResponsePayload struct {
	RequestID string `json:"request_id"`
	Output    struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
					Text  string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (p httpResponsePayload) firstImageURL() (string, error) {
	for _, choice := range p.Output.Choices {
		for _, item := range choice.Message.Content {
			if strings.TrimSpace(item.Image) != "" {
				return strings.TrimSpace(item.Image), nil
			}
		}
	}

	if p.Code != "" || p.Message != "" {
		return "", fmt.Errorf("image response failed: code=%s message=%s", p.Code, p.Message)
	}

	return "", fmt.Errorf("image response missing image url")
}
