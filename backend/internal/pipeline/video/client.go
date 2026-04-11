package video

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultVideoResolution                  = "720P"
	defaultVideoPollInterval                = 10 * time.Second
	defaultVideoMaxWait                     = 15 * time.Minute
	defaultVideoMaxRequestBytes             = 6 * 1024 * 1024
	defaultVideoImageJPEGQuality            = 80
	defaultVideoImageMinJPEGQuality         = 45
	defaultVideoNegativePromptTextArtifacts = "字幕，台词字幕，屏幕文字，文字叠加，底部字幕条，中文字幕，英文字幕，text，caption，subtitle，subtitles，logo，水印，贴纸字卡"
)

var defaultVideoImageMaxEdgeCandidates = []int{2688, 2048, 1536, 1280, 1024, 896, 768}

type Client interface {
	Generate(ctx context.Context, request Request) (Response, error)
}

type Request struct {
	Model           string
	Prompt          string
	AspectRatio     model.AspectRatio
	SegmentIndex    int
	ShotIndex       int
	SourceImagePath string
	SourceImageURL  string
	DurationSeconds float64
}

type Response struct {
	ProviderTaskID  string
	ProviderStatus  string
	RequestID       string
	Model           string
	VideoURL        string
	VideoData       []byte
	DurationSeconds float64
	ActualPrompt    string
	OriginalPrompt  string
}

type GenerationConfig struct {
	Model                  string
	Resolution             string
	NegativePrompt         string
	PollInterval           time.Duration
	MaxWait                time.Duration
	MaxRequestBytes        int
	ImageJPEGQuality       int
	ImageMinJPEGQuality    int
	ImageMaxEdgeCandidates []int
	PromptExtend           bool
	ShotType               string
	Watermark              bool
}

type HTTPClient struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	config     GenerationConfig
	wait       func(context.Context, time.Duration) error
}

func NewHTTPClient(
	baseURL string,
	apiKey string,
	config GenerationConfig,
	httpClient *http.Client,
) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse video base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("video base url must include scheme and host")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("video api key is empty")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("video http client is nil")
	}
	if httpClient.Timeout <= 0 {
		return nil, fmt.Errorf("video http client timeout is not configured")
	}

	return &HTTPClient{
		baseURL:    parsed,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
		config:     normalizeGenerationConfig(config),
		wait:       waitForDuration,
	}, nil
}

func normalizeGenerationConfig(cfg GenerationConfig) GenerationConfig {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "wan2.6-i2v-flash"
	}
	if strings.TrimSpace(cfg.Resolution) == "" {
		cfg.Resolution = defaultVideoResolution
	}
	cfg.NegativePrompt = mergeVideoNegativePrompt(cfg.NegativePrompt)
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultVideoPollInterval
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = defaultVideoMaxWait
	}
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = defaultVideoMaxRequestBytes
	}
	if cfg.ImageJPEGQuality <= 0 {
		cfg.ImageJPEGQuality = defaultVideoImageJPEGQuality
	}
	if cfg.ImageMinJPEGQuality <= 0 {
		cfg.ImageMinJPEGQuality = defaultVideoImageMinJPEGQuality
	}
	if len(cfg.ImageMaxEdgeCandidates) == 0 {
		cfg.ImageMaxEdgeCandidates = append([]int(nil), defaultVideoImageMaxEdgeCandidates...)
	}
	if !cfg.PromptExtend {
		cfg.PromptExtend = true
	}
	if strings.TrimSpace(cfg.ShotType) == "" {
		cfg.ShotType = "multi"
	}

	return cfg
}

func (c *HTTPClient) Generate(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(request.Model) == "" {
		request.Model = c.config.Model
	}
	if request.DurationSeconds <= 0 {
		return Response{}, fmt.Errorf("video request duration_seconds must be positive")
	}

	submitResponse, err := c.submitVideoTask(ctx, request)
	if err != nil {
		return Response{}, err
	}
	submitOutput := extractVideoOutput(submitResponse)
	if submitOutput.ProviderTaskID == "" {
		return Response{}, fmt.Errorf("video submit response missing task_id")
	}

	finalOutput, err := c.pollVideoTask(ctx, submitOutput.ProviderTaskID)
	if err != nil {
		return Response{}, err
	}
	if finalOutput.VideoURL == "" {
		return Response{}, fmt.Errorf("video task succeeded without video_url")
	}

	videoData, err := c.downloadVideo(ctx, finalOutput.VideoURL)
	if err != nil {
		return Response{}, err
	}

	return Response{
		ProviderTaskID:  finalOutput.ProviderTaskID,
		ProviderStatus:  finalOutput.ProviderStatus,
		RequestID:       finalOutput.RequestID,
		Model:           request.Model,
		VideoURL:        finalOutput.VideoURL,
		VideoData:       videoData,
		DurationSeconds: request.DurationSeconds,
		ActualPrompt:    finalOutput.ActualPrompt,
		OriginalPrompt:  finalOutput.OriginalPrompt,
	}, nil
}

func (c *HTTPClient) submitVideoTask(
	ctx context.Context,
	request Request,
) (map[string]any, error) {
	imageInput, inputKind, err := c.buildVideoImageInput(request)
	if err != nil {
		return nil, err
	}

	response, err := c.apiRequest(
		ctx,
		http.MethodPost,
		"/api/v1/services/aigc/video-generation/video-synthesis",
		c.buildVideoSubmitPayload(request, imageInput),
		true,
	)
	if err == nil || inputKind != "url" {
		return response, err
	}

	fallbackInput, fallbackKind, fallbackErr := c.buildLocalVideoImageInput(request)
	if fallbackErr != nil {
		return nil, err
	}
	if fallbackKind == "url" {
		return nil, err
	}

	return c.apiRequest(
		ctx,
		http.MethodPost,
		"/api/v1/services/aigc/video-generation/video-synthesis",
		c.buildVideoSubmitPayload(request, fallbackInput),
		true,
	)
}

func (c *HTTPClient) pollVideoTask(
	ctx context.Context,
	providerTaskID string,
) (videoTaskOutput, error) {
	startedAt := time.Now()
	for {
		response, err := c.apiRequest(
			ctx,
			http.MethodGet,
			"/api/v1/tasks/"+url.PathEscape(providerTaskID),
			nil,
			false,
		)
		if err != nil {
			return videoTaskOutput{}, err
		}

		output := extractVideoOutput(response)
		if output.ProviderTaskID == "" {
			output.ProviderTaskID = providerTaskID
		}
		switch output.ProviderStatus {
		case "SUCCEEDED":
			return output, nil
		case "FAILED", "CANCELED":
			if output.Message == "" {
				output.Message = fmt.Sprintf("video task finished with status %s", output.ProviderStatus)
			}
			return videoTaskOutput{}, fmt.Errorf(output.Message)
		}

		if time.Since(startedAt) > c.config.MaxWait {
			return videoTaskOutput{}, fmt.Errorf("video task polling timed out after %s", c.config.MaxWait)
		}
		if err := c.wait(ctx, c.config.PollInterval); err != nil {
			return videoTaskOutput{}, err
		}
	}
}

func (c *HTTPClient) downloadVideo(ctx context.Context, videoURL string) ([]byte, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(videoURL), nil)
	if err != nil {
		return nil, fmt.Errorf("build video download request: %w", err)
	}

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("download video: %w", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("read downloaded video: %w", err)
	}
	if httpResponse.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("download video failed: status=%d", httpResponse.StatusCode)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("download video failed: empty body")
	}

	return body, nil
}

func (c *HTTPClient) apiRequest(
	ctx context.Context,
	method string,
	requestPath string,
	payload map[string]any,
	async bool,
) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal video request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		method,
		c.endpointURL(requestPath),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("build video request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if async {
		httpRequest.Header.Set("X-DashScope-Async", "enable")
	}

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("send video request: %w", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("read video response: %w", err)
	}

	var data map[string]any
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &data); err != nil {
			return nil, fmt.Errorf(
				"decode video response: status=%d body=%s: %w",
				httpResponse.StatusCode,
				strings.TrimSpace(string(responseBody)),
				err,
			)
		}
	} else {
		data = map[string]any{}
	}

	if httpResponse.StatusCode >= http.StatusBadRequest {
		output, _ := data["output"].(map[string]any)
		errorCode := outputStringAny(output, "code")
		if errorCode == "" {
			errorCode = outputStringAny(data, "code")
		}
		message := outputStringAny(output, "message")
		if message == "" {
			message = outputStringAny(data, "message")
		}
		if message == "" {
			message = strings.TrimSpace(string(responseBody))
		}
		return nil, fmt.Errorf(
			"video request failed: status=%d code=%s message=%s",
			httpResponse.StatusCode,
			errorCode,
			message,
		)
	}

	return data, nil
}

func (c *HTTPClient) buildVideoSubmitPayload(
	request Request,
	imageInput string,
) map[string]any {
	return map[string]any{
		"model": request.Model,
		"input": map[string]any{
			"prompt":          strings.TrimSpace(request.Prompt),
			"negative_prompt": c.config.NegativePrompt,
			"img_url":         imageInput,
		},
		"parameters": map[string]any{
			"resolution":    c.config.Resolution,
			"duration":      int(request.DurationSeconds),
			"prompt_extend": c.config.PromptExtend,
			"shot_type":     c.config.ShotType,
			"watermark":     c.config.Watermark,
		},
	}
}

func (c *HTTPClient) buildVideoImageInput(
	request Request,
) (string, string, error) {
	remoteURL := strings.TrimSpace(request.SourceImageURL)
	if remoteURL != "" {
		return remoteURL, "url", nil
	}

	return c.buildLocalVideoImageInput(request)
}

func (c *HTTPClient) buildLocalVideoImageInput(
	request Request,
) (string, string, error) {
	sourcePath := strings.TrimSpace(request.SourceImagePath)
	if sourcePath == "" {
		return "", "", fmt.Errorf("video source_image_path is empty")
	}

	imageBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("read video source image: %w", err)
	}

	mimeType := detectImageMimeType(sourcePath)
	imageInput := encodeBytesAsDataURL(imageBytes, mimeType)
	if c.estimateVideoPayloadSize(request, imageInput) <= c.config.MaxRequestBytes {
		return imageInput, "local", nil
	}

	compressedInput, err := c.compressImageForVideo(request, imageBytes)
	if err != nil {
		return "", "", err
	}

	return compressedInput, "local", nil
}

func (c *HTTPClient) compressImageForVideo(
	request Request,
	imageBytes []byte,
) (string, error) {
	sourceImage, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return "", fmt.Errorf("decode video source image: %w", err)
	}

	qualityCandidates := buildQualityCandidates(
		c.config.ImageJPEGQuality,
		c.config.ImageMinJPEGQuality,
	)
	for _, maxEdge := range c.config.ImageMaxEdgeCandidates {
		candidateImage := resizeImageToMaxEdge(sourceImage, maxEdge)
		for _, quality := range qualityCandidates {
			candidateBytes, err := encodeJPEG(candidateImage, quality)
			if err != nil {
				return "", fmt.Errorf("encode video source image as jpeg: %w", err)
			}
			imageInput := encodeBytesAsDataURL(candidateBytes, "image/jpeg")
			if c.estimateVideoPayloadSize(request, imageInput) <= c.config.MaxRequestBytes {
				return imageInput, nil
			}
		}
	}

	return "", fmt.Errorf("compressed video image still exceeds max request bytes")
}

func (c *HTTPClient) estimateVideoPayloadSize(
	request Request,
	imageInput string,
) int {
	payload := c.buildVideoSubmitPayload(request, imageInput)
	encoded, _ := json.Marshal(payload)
	return len(encoded)
}

func (c *HTTPClient) endpointURL(requestPath string) string {
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, requestPath)
	return endpoint.String()
}

type videoTaskOutput struct {
	ProviderTaskID string
	ProviderStatus string
	VideoURL       string
	RequestID      string
	Message        string
	ActualPrompt   string
	OriginalPrompt string
}

func extractVideoOutput(response map[string]any) videoTaskOutput {
	output, _ := response["output"].(map[string]any)
	return videoTaskOutput{
		ProviderTaskID: strings.TrimSpace(outputStringAny(output, "task_id")),
		ProviderStatus: strings.ToUpper(strings.TrimSpace(outputStringAny(output, "task_status"))),
		VideoURL:       strings.TrimSpace(outputStringAny(output, "video_url")),
		RequestID:      strings.TrimSpace(outputStringAny(response, "request_id")),
		Message:        strings.TrimSpace(outputStringAny(output, "message")),
		ActualPrompt:   strings.TrimSpace(outputStringAny(output, "actual_prompt")),
		OriginalPrompt: strings.TrimSpace(outputStringAny(output, "orig_prompt")),
	}
}

func mergeVideoNegativePrompt(value string) string {
	parts := make([]string, 0, 2)
	if clean := strings.TrimSpace(value); clean != "" {
		parts = append(parts, clean)
	}
	parts = append(parts, defaultVideoNegativePromptTextArtifacts)
	return strings.Join(parts, "，")
}

func encodeBytesAsDataURL(imageBytes []byte, mimeType string) string {
	encoded := base64.StdEncoding.EncodeToString(imageBytes)
	return "data:" + mimeType + ";base64," + encoded
}

func detectImageMimeType(sourcePath string) string {
	lower := strings.ToLower(sourcePath)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

func buildQualityCandidates(initial int, minimum int) []int {
	candidates := make([]int, 0, 5)
	for _, value := range []int{initial, 70, 60, 50, minimum} {
		if value <= 0 {
			continue
		}
		alreadyIncluded := false
		for _, existing := range candidates {
			if existing == value {
				alreadyIncluded = true
				break
			}
		}
		if !alreadyIncluded {
			candidates = append(candidates, value)
		}
	}

	return candidates
}

func resizeImageToMaxEdge(src image.Image, maxEdge int) image.Image {
	if maxEdge <= 0 {
		return src
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxEdge && height <= maxEdge {
		return src
	}

	newWidth := width
	newHeight := height
	if width >= height {
		newWidth = maxEdge
		newHeight = height * maxEdge / width
	} else {
		newHeight = maxEdge
		newWidth = width * maxEdge / height
	}
	if newWidth <= 0 {
		newWidth = 1
	}
	if newHeight <= 0 {
		newHeight = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		sourceY := bounds.Min.Y + y*height/newHeight
		for x := 0; x < newWidth; x++ {
			sourceX := bounds.Min.X + x*width/newWidth
			dst.Set(x, y, src.At(sourceX, sourceY))
		}
	}

	return dst
}

func encodeJPEG(src image.Image, quality int) ([]byte, error) {
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, src, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func outputStringAny(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func waitForDuration(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
