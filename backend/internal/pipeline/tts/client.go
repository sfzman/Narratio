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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
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

type VoicePreset struct {
	ID             string
	ReferenceAudio string
}

type HTTPClient struct {
	baseURL        *url.URL
	privateKey     *rsa.PrivateKey
	tokenTTL       time.Duration
	httpClient     *http.Client
	defaultVoiceID string
	emotionPrompt  string
	voicePresets   []VoicePreset
}

func NewHTTPClient(
	baseURL string,
	jwtPrivateKey string,
	jwtExpireSeconds int,
	defaultVoiceID string,
	emotionPrompt string,
	httpClient *http.Client,
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

	return &HTTPClient{
		baseURL:        parsed,
		privateKey:     privateKey,
		tokenTTL:       time.Duration(jwtExpireSeconds) * time.Second,
		httpClient:     httpClient,
		defaultVoiceID: strings.TrimSpace(defaultVoiceID),
		emotionPrompt:  strings.TrimSpace(emotionPrompt),
		voicePresets:   defaultVoicePresets(),
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

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpointURL(),
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

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("send tts request: %w", err)
	}
	defer httpResponse.Body.Close()

	audioBytes, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("read tts response: %w", err)
	}
	if httpResponse.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf(
			"tts request failed: status=%d body=%s",
			httpResponse.StatusCode,
			strings.TrimSpace(string(audioBytes)),
		)
	}
	if len(audioBytes) == 0 {
		return nil, fmt.Errorf("tts response is empty")
	}

	return audioBytes, nil
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

func (c *HTTPClient) resolveVoicePreset(voiceID string) VoicePreset {
	normalized := strings.TrimSpace(voiceID)
	if normalized == "" || normalized == "default" {
		normalized = c.defaultVoiceID
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

	return VoicePreset{}
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
		"iat": now.Unix(),
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

func defaultVoicePresets() []VoicePreset {
	return []VoicePreset{
		{
			ID:             "male_calm",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%B2%89%E7%A8%B3%E9%9D%92%E5%B9%B4%E9%9F%B3.MP3",
		},
		{
			ID:             "male_strong",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E7%8E%8B%E6%98%8E%E5%86%9B.MP3",
		},
		{
			ID:             "female_explainer",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E5%A5%B3_%E8%A7%A3%E8%AF%B4%E5%B0%8F%E7%BE%8E.MP3",
		},
		{
			ID:             "female_documentary",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E5%A5%B3_%E4%B8%93%E9%A2%98%E7%89%87%E9%85%8D%E9%9F%B3.MP3",
		},
		{
			ID:             "boy",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%AD%A3%E5%A4%AA.wav",
		},
	}
}
