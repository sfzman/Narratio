package tts

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type Client interface {
	Synthesize(ctx context.Context, request Request) ([]byte, error)
}

type Request struct {
	Text       string
	VoiceID    string
	Format     string
	SampleRate int
}

type HTTPClient struct {
	baseURL        *url.URL
	privateKey     *rsa.PrivateKey
	tokenTTL       time.Duration
	httpClient     *http.Client
	defaultVoiceID string
	emotionPrompt  string
	voicePresets   []model.VoicePreset
	maxRetries     int
	backoff        time.Duration
	sleep          func(context.Context, time.Duration) error
}

type HTTPClientOptions struct {
	MaxRetries int
	Backoff    time.Duration
	Sleep      func(context.Context, time.Duration) error
}

func NewHTTPClient(
	baseURL string,
	jwtPrivateKey string,
	jwtExpireSeconds int,
	defaultVoiceID string,
	emotionPrompt string,
	httpClient *http.Client,
	options ...HTTPClientOptions,
) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse tts base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("tts base url must include scheme and host")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("tts http client is nil")
	}
	if httpClient.Timeout <= 0 {
		return nil, fmt.Errorf("tts http client timeout is not configured")
	}
	privateKey, err := parseRSAPrivateKey(jwtPrivateKey)
	if err != nil {
		return nil, err
	}
	if jwtExpireSeconds <= 0 {
		return nil, fmt.Errorf("tts jwt expire seconds must be positive")
	}

	opts := HTTPClientOptions{
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

	return &HTTPClient{
		baseURL:        parsed,
		privateKey:     privateKey,
		tokenTTL:       time.Duration(jwtExpireSeconds) * time.Second,
		httpClient:     httpClient,
		defaultVoiceID: strings.TrimSpace(defaultVoiceID),
		emotionPrompt:  strings.TrimSpace(emotionPrompt),
		voicePresets:   defaultVoicePresets(),
		maxRetries:     maxInt(opts.MaxRetries, 0),
		backoff:        opts.Backoff,
		sleep:          opts.Sleep,
	}, nil
}

func (c *HTTPClient) Synthesize(ctx context.Context, request Request) ([]byte, error) {
	payload, err := c.buildPayload(request)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal tts request: %w", err)
	}

	endpoint := c.endpointURL()
	for attempt := 0; ; attempt++ {
		httpRequest, err := c.newRequest(ctx, endpoint, body)
		if err != nil {
			return nil, err
		}

		httpResponse, err := c.httpClient.Do(httpRequest)
		if err != nil {
			sendErr := wrapTTSSendError(ctx, c.httpClient.Timeout, err)
			if retryErr := c.retryIfNeeded(ctx, request, endpoint, attempt, sendErr); retryErr != nil {
				return nil, retryErr
			}
			continue
		}

		audioBytes, readErr := io.ReadAll(httpResponse.Body)
		httpResponse.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read tts response: %w", readErr)
		}
		if httpResponse.StatusCode >= http.StatusBadRequest {
			statusErr := &StatusError{
				StatusCode: httpResponse.StatusCode,
				Body:       strings.TrimSpace(string(audioBytes)),
			}
			if retryErr := c.retryIfNeeded(ctx, request, endpoint, attempt, statusErr); retryErr != nil {
				return nil, retryErr
			}
			continue
		}
		if len(audioBytes) == 0 {
			return nil, fmt.Errorf("tts response is empty")
		}

		return audioBytes, nil
	}
}

func (c *HTTPClient) buildPayload(request Request) (map[string]any, error) {
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return nil, fmt.Errorf("tts request text is empty")
	}

	preset := c.resolveVoicePreset(request.VoiceID)
	if strings.TrimSpace(preset.ReferenceAudio) == "" {
		return nil, fmt.Errorf("tts request reference audio is empty")
	}

	return map[string]any{
		"text":            text,
		"reference_audio": preset.ReferenceAudio,
		"emotion_prompt":  c.emotionPrompt,
	}, nil
}

func (c *HTTPClient) resolveVoicePreset(voiceID string) model.VoicePreset {
	normalized := model.NormalizeVoicePresetID(voiceID)
	if normalized == model.DefaultVoicePresetID && strings.TrimSpace(c.defaultVoiceID) != "" {
		normalized = strings.TrimSpace(c.defaultVoiceID)
	}

	for _, preset := range c.voicePresets {
		if preset.ID == normalized {
			return preset
		}
	}
	for _, preset := range c.voicePresets {
		if preset.ID == c.defaultVoiceID {
			return preset
		}
	}
	if len(c.voicePresets) > 0 {
		return c.voicePresets[0]
	}

	return model.VoicePreset{}
}

func (c *HTTPClient) buildBearerToken() (string, error) {
	if c == nil || c.privateKey == nil {
		return "", fmt.Errorf("tts jwt private key is not configured")
	}

	now := time.Now().UTC()
	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	payloadJSON, err := json.Marshal(map[string]int64{
		// "iat": now.Unix(), // 如果传了反而会因为服务器和客户端时间不一致导致部分 JWT 库校验失败，暂时先不传
		"exp": now.Add(c.tokenTTL).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal jwt payload: %w", err)
	}

	signingInput := base64RawURL(headerJSON) + "." + base64RawURL(payloadJSON)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt token: %w", err)
	}

	return signingInput + "." + base64RawURL(signature), nil
}

func (c *HTTPClient) endpointURL() string {
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, "/api/v1/tts")
	return endpoint.String()
}

func (c *HTTPClient) newRequest(
	ctx context.Context,
	endpoint string,
	body []byte,
) (*http.Request, error) {
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build tts request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "audio/wav")
	token, err := c.buildBearerToken()
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)

	return httpRequest, nil
}

func (c *HTTPClient) retryIfNeeded(
	ctx context.Context,
	request Request,
	endpoint string,
	attempt int,
	err error,
) error {
	if !isRetryableTTSError(err) || attempt >= c.maxRetries {
		return err
	}

	backoff := c.retryBackoff(attempt)
	slog.Warn("tts request retry scheduled",
		"url", endpoint,
		"voice_id", request.VoiceID,
		"text_length", len([]rune(request.Text)),
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

func (c *HTTPClient) retryBackoff(attempt int) time.Duration {
	if c.backoff <= 0 {
		return 0
	}

	return time.Duration(1<<attempt) * c.backoff
}

func wrapTTSSendError(
	ctx context.Context,
	httpTimeout time.Duration,
	err error,
) error {
	timeoutDetail := ""
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		timeoutDetail = "task context deadline exceeded"
	case strings.Contains(err.Error(), "Client.Timeout exceeded"):
		timeoutDetail = fmt.Sprintf("http client timeout exceeded (%s)", httpTimeout)
	case errors.Is(err, context.DeadlineExceeded):
		timeoutDetail = "request deadline exceeded"
	}
	if timeoutDetail != "" {
		return fmt.Errorf("send tts request (%s): %w", timeoutDetail, err)
	}

	return fmt.Errorf("send tts request: %w", err)
}

func parseRSAPrivateKey(value string) (*rsa.PrivateKey, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\\n", "\n"))
	if normalized == "" {
		return nil, fmt.Errorf("tts jwt private key is empty")
	}

	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, fmt.Errorf("decode tts jwt private key pem: no block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("tts jwt private key is not rsa")
		}
		return rsaKey, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse tts jwt private key: %w", err)
	}

	return key, nil
}

func base64RawURL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func defaultVoicePresets() []model.VoicePreset {
	return model.DefaultVoicePresets()
}

type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("tts request failed with status %d", e.StatusCode)
	}

	return fmt.Sprintf("tts request failed with status %d: %s", e.StatusCode, e.Body)
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

func isRetryableTTSError(err error) bool {
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

func maxInt(value int, floor int) int {
	if value < floor {
		return floor
	}

	return value
}
