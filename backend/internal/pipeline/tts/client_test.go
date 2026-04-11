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
	if claims["iat"] == 0 {
		t.Fatalf("jwt iat = %#v, want non-zero", claims["iat"])
	}
	if claims["exp"] <= claims["iat"] {
		t.Fatalf("jwt exp = %d, iat = %d", claims["exp"], claims["iat"])
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
