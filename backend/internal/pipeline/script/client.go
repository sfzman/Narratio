package script

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNoTextContent           = errors.New("qwen text response has no text content")
	ErrHTTPClientNil           = errors.New("http client is nil")
	ErrHTTPClientTimeoutNotSet = errors.New("http client timeout is not configured")
	ErrNoResponseChoices       = errors.New("qwen text response has no choices")
)

type TextClient interface {
	Generate(ctx context.Context, request TextRequest) (TextResponse, error)
}

type TextRequest struct {
	Model          string              `json:"model"`
	Messages       []ChatMessage       `json:"messages"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Temperature    *float64            `json:"temperature,omitempty"`
	ResponseFormat *ResponseFormatSpec `json:"response_format,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponseFormatSpec struct {
	Type string `json:"type"`
}

func (r TextRequest) Validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("text request model is empty")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("text request messages are empty")
	}
	for i, message := range r.Messages {
		if strings.TrimSpace(message.Role) == "" {
			return fmt.Errorf("text request message %d role is empty", i)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("text request message %d content is empty", i)
		}
	}
	if r.ResponseFormat != nil && strings.TrimSpace(r.ResponseFormat.Type) == "" {
		return fmt.Errorf("text request response_format.type is empty")
	}

	return nil
}

type TextResponse struct {
	ID        string         `json:"id,omitempty"`
	Object    string         `json:"object,omitempty"`
	Created   int64          `json:"created,omitempty"`
	Model     string         `json:"model,omitempty"`
	Choices   []Choice       `json:"choices"`
	Error     *ResponseError `json:"error,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

type Choice struct {
	Index        int         `json:"index,omitempty"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type ResponseError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

func (r TextResponse) FirstText() (string, error) {
	for _, choice := range r.Choices {
		text := strings.TrimSpace(choice.Message.Content)
		if text != "" {
			return text, nil
		}
	}

	if len(r.Choices) == 0 {
		return "", ErrNoResponseChoices
	}

	return "", ErrNoTextContent
}

func (r TextResponse) FirstFinishReason() string {
	for _, choice := range r.Choices {
		reason := strings.TrimSpace(choice.FinishReason)
		if reason != "" {
			return reason
		}
	}

	return ""
}

type HTTPTextClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
	backoff    time.Duration
	sleep      func(context.Context, time.Duration) error
}

type HTTPTextClientOptions struct {
	MaxRetries int
	Backoff    time.Duration
	Sleep      func(context.Context, time.Duration) error
}

func NewHTTPTextClient(
	baseURL string,
	apiKey string,
	httpClient *http.Client,
	options ...HTTPTextClientOptions,
) (*HTTPTextClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("qwen text base url is empty")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("qwen text api key is empty")
	}
	if httpClient == nil {
		return nil, ErrHTTPClientNil
	}
	if httpClient.Timeout <= 0 {
		return nil, ErrHTTPClientTimeoutNotSet
	}

	opts := HTTPTextClientOptions{
		MaxRetries: 0,
		Backoff:    0,
		Sleep:      sleepWithContext,
	}
	if len(options) > 0 {
		opts = options[0]
		if opts.Sleep == nil {
			opts.Sleep = sleepWithContext
		}
	}

	return &HTTPTextClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
		maxRetries: max(opts.MaxRetries, 0),
		backoff:    opts.Backoff,
		sleep:      opts.Sleep,
	}, nil
}

func (c *HTTPTextClient) Generate(
	ctx context.Context,
	request TextRequest,
) (TextResponse, error) {
	if err := request.Validate(); err != nil {
		return TextResponse{}, fmt.Errorf("validate text request: %w", err)
	}

	body, err := marshalTextRequest(request)
	if err != nil {
		return TextResponse{}, err
	}

	httpRequest, err := c.newRequest(ctx, body)
	if err != nil {
		return TextResponse{}, err
	}

	startedAt := time.Now()
	slog.Info("dashscope text request sending",
		"url", httpRequest.URL.String(),
		"model", request.Model,
		"message_count", len(request.Messages),
		"max_tokens", request.MaxTokens,
		"http_timeout_seconds", int(c.httpClient.Timeout.Seconds()),
	)
	for attempt := 0; ; attempt++ {
		httpResponse, err := c.httpClient.Do(httpRequest)
		if err == nil {
			slog.Info("dashscope text response received",
				"url", httpRequest.URL.String(),
				"model", request.Model,
				"status_code", httpResponse.StatusCode,
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"attempt", attempt+1,
			)
			if httpResponse.StatusCode < http.StatusOK ||
				httpResponse.StatusCode >= http.StatusMultipleChoices {
				statusErr := newStatusError(httpResponse)
				httpResponse.Body.Close()
				if retryErr := c.retryIfNeeded(ctx, request, httpRequest.URL.String(), attempt, statusErr); retryErr != nil {
					return TextResponse{}, retryErr
				}
				httpRequest, err = c.newRequest(ctx, body)
				if err != nil {
					return TextResponse{}, err
				}
				continue
			}

			response, decodeErr := decodeTextResponse(httpResponse.Body)
			httpResponse.Body.Close()
			if decodeErr != nil {
				return TextResponse{}, decodeErr
			}
			return response, nil
		}

		sendErr := fmt.Errorf("send qwen text request: %w", err)
		if retryErr := c.retryIfNeeded(ctx, request, httpRequest.URL.String(), attempt, sendErr); retryErr != nil {
			return TextResponse{}, retryErr
		}
		httpRequest, err = c.newRequest(ctx, body)
		if err != nil {
			return TextResponse{}, err
		}
	}
}

func marshalTextRequest(request TextRequest) ([]byte, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal text request: %w", err)
	}

	return body, nil
}

func (c *HTTPTextClient) newRequest(
	ctx context.Context,
	body []byte,
) (*http.Request, error) {
	endpoint, err := buildChatCompletionsURL(c.baseURL)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build qwen text request: %w", err)
	}

	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	return request, nil
}

func buildChatCompletionsURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse qwen text base url: %w", err)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/chat/completions"
	return parsed.String(), nil
}

func decodeTextResponse(body io.Reader) (TextResponse, error) {
	var response TextResponse
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return TextResponse{}, fmt.Errorf("decode text response: %w", err)
	}

	return response, nil
}

func (c *HTTPTextClient) retryIfNeeded(
	ctx context.Context,
	request TextRequest,
	endpoint string,
	attempt int,
	err error,
) error {
	if !isRetryableTextError(err) || attempt >= c.maxRetries {
		return err
	}

	backoff := c.retryBackoff(attempt)
	slog.Warn("dashscope text request retry scheduled",
		"url", endpoint,
		"model", request.Model,
		"attempt", attempt+1,
		"next_attempt", attempt+2,
		"backoff_ms", backoff.Milliseconds(),
		"error", err,
	)
	if sleepErr := c.sleep(ctx, backoff); sleepErr != nil {
		return err
	}

	return nil
}

func (c *HTTPTextClient) retryBackoff(attempt int) time.Duration {
	if c.backoff <= 0 {
		return 0
	}

	multiplier := math.Pow(2, float64(attempt))
	return time.Duration(multiplier) * c.backoff
}

func isRetryableTextError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Retryable()
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("qwen text request failed with status %d", e.StatusCode)
	}

	return fmt.Sprintf(
		"qwen text request failed with status %d: %s",
		e.StatusCode,
		e.Body,
	)
}

func (e *StatusError) Retryable() bool {
	return IsRetryableStatus(e.StatusCode)
}

func IsRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func newStatusError(response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil {
		return fmt.Errorf(
			"read qwen text error response body (status %d): %w",
			response.StatusCode,
			err,
		)
	}

	return &StatusError{
		StatusCode: response.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}
