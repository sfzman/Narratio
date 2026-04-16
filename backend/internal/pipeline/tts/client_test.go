package tts

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPClientSynthesizeBuildsNarrationRequest(t *testing.T) {
	t.Parallel()

	var received map[string]any
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tts" {
			t.Fatalf("request path = %q", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Fatalf("Authorization = %q, want Bearer token", authHeader)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFfakeWAVE"))
	}))
	defer server.Close()

	privateKeyPEM := mustGenerateRSAPrivateKeyPEM(t)
	client, err := NewHTTPClient(
		server.URL,
		privateKeyPEM,
		300,
		"male_calm",
		"https://example.com/emotion.wav",
		&http.Client{Timeout: 3 * time.Second},
	)
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}

	audioBytes, err := client.Synthesize(context.Background(), Request{
		Text:       "第一句。",
		VoiceID:    "default",
		Format:     "wav",
		SampleRate: 24000,
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	claims := decodeBearerTokenClaims(t, authHeader)
	if claims["exp"] == 0 {
		t.Fatalf("jwt exp = %#v, want non-zero", claims["exp"])
	}
	if string(audioBytes) != "RIFFfakeWAVE" {
		t.Fatalf("audioBytes = %q", string(audioBytes))
	}
	if received["text"] != "第一句。" {
		t.Fatalf("payload.text = %#v", received["text"])
	}
	if received["emotion_prompt"] != "https://example.com/emotion.wav" {
		t.Fatalf("payload.emotion_prompt = %#v", received["emotion_prompt"])
	}
	if received["reference_audio"] == "" {
		t.Fatalf("payload.reference_audio = %#v", received["reference_audio"])
	}
}

func TestHTTPClientSynthesizeFallsBackToDefaultVoicePreset(t *testing.T) {
	t.Parallel()

	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		_, _ = w.Write([]byte("RIFFfakeWAVE"))
	}))
	defer server.Close()

	privateKeyPEM := mustGenerateRSAPrivateKeyPEM(t)
	client, err := NewHTTPClient(
		server.URL,
		privateKeyPEM,
		300,
		"boy",
		"https://example.com/emotion.wav",
		&http.Client{Timeout: 3 * time.Second},
	)
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}

	_, err = client.Synthesize(context.Background(), Request{
		Text:    "旁白",
		VoiceID: "unknown_voice",
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if received["reference_audio"] != "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%AD%A3%E5%A4%AA.wav" {
		t.Fatalf("payload.reference_audio = %#v", received["reference_audio"])
	}
}

func TestHTTPClientSynthesizeRetriesRetryableStatus(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "upstream busy", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFfakeWAVE"))
	}))
	defer server.Close()

	sleepCalls := make([]time.Duration, 0, 2)
	privateKeyPEM := mustGenerateRSAPrivateKeyPEM(t)
	client, err := NewHTTPClient(
		server.URL,
		privateKeyPEM,
		300,
		"male_calm",
		"https://example.com/emotion.wav",
		&http.Client{Timeout: 3 * time.Second},
		HTTPClientOptions{
			MaxRetries: 2,
			Backoff:    time.Second,
			Sleep: func(_ context.Context, duration time.Duration) error {
				sleepCalls = append(sleepCalls, duration)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}

	audioBytes, err := client.Synthesize(context.Background(), Request{
		Text:    "第一句。",
		VoiceID: "male_calm",
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if string(audioBytes) != "RIFFfakeWAVE" {
		t.Fatalf("audioBytes = %q", string(audioBytes))
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(sleepCalls) != 2 || sleepCalls[0] != time.Second || sleepCalls[1] != 2*time.Second {
		t.Fatalf("sleepCalls = %#v, want [1s 2s]", sleepCalls)
	}
}

func TestHTTPClientSynthesizeStopsAfterRetryLimit(t *testing.T) {
	t.Parallel()

	attempts := 0
	privateKeyPEM := mustGenerateRSAPrivateKeyPEM(t)
	client, err := NewHTTPClient(
		"https://example.com",
		privateKeyPEM,
		300,
		"male_calm",
		"https://example.com/emotion.wav",
		&http.Client{
			Timeout: 3 * time.Second,
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				return nil, context.DeadlineExceeded
			}),
		},
		HTTPClientOptions{
			MaxRetries: 2,
			Backoff:    time.Second,
			Sleep: func(_ context.Context, duration time.Duration) error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}

	_, err = client.Synthesize(context.Background(), Request{
		Text:    "第一句。",
		VoiceID: "male_calm",
	})
	if err == nil {
		t.Fatal("Synthesize() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "send tts request") {
		t.Fatalf("err = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestNewHTTPClientRequiresTimeout(t *testing.T) {
	t.Parallel()

	privateKeyPEM := mustGenerateRSAPrivateKeyPEM(t)
	_, err := NewHTTPClient(
		"https://example.com",
		privateKeyPEM,
		300,
		"male_calm",
		"https://example.com/emotion.wav",
		&http.Client{},
	)
	if err == nil {
		t.Fatal("NewHTTPClient() error = nil, want timeout error")
	}
}

func TestNewHTTPClientRequiresJWTPrivateKey(t *testing.T) {
	t.Parallel()

	_, err := NewHTTPClient(
		"https://example.com",
		"",
		300,
		"male_calm",
		"https://example.com/emotion.wav",
		&http.Client{Timeout: 3 * time.Second},
	)
	if err == nil {
		t.Fatal("NewHTTPClient() error = nil, want private key error")
	}
}

func mustGenerateRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}))
}

func decodeBearerTokenClaims(t *testing.T, header string) map[string]int64 {
	t.Helper()

	token := strings.TrimPrefix(header, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts = %d, want 3", len(parts))
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}

	var claims map[string]int64
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	return claims
}
