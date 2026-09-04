package textai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"
)

func TestDeepSeekPolisherSendsExpectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != chatCompletionsPath || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing DeepSeek bearer token")
		}
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "deepseek-v4-flash" || body.Thinking.Type != "disabled" || body.Stream {
			t.Fatalf("unexpected model options: %#v", body)
		}
		if body.UserID != "creator-01" || body.MaxTokens != 2000 {
			t.Fatalf("unexpected request metadata: %#v", body)
		}
		if len(body.Messages) != 2 || !strings.Contains(body.Messages[0].Content, "不得虚构") || body.Messages[1].Content != "我爱你。" {
			t.Fatalf("unexpected messages: %#v", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"与你相伴的每一天，我都格外珍惜。"}}]}`))
	}))
	defer server.Close()

	polisher := NewDeepSeekPolisher(config.AITextConfig{
		BaseURL: server.URL, APIKey: "test-key", Model: "deepseek-v4-flash",
		Timeout: time.Second, MaxOutputTokens: 2000,
	})
	result, err := polisher.PolishLoveLetter(context.Background(), " 我爱你。 ", "creator-01")
	if err != nil {
		t.Fatal(err)
	}
	if result != "与你相伴的每一天，我都格外珍惜。" {
		t.Fatalf("unexpected polished letter: %q", result)
	}
}

func TestDeepSeekPolisherWithoutKeyIsNotConfigured(t *testing.T) {
	polisher := NewDeepSeekPolisher(config.AITextConfig{BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash"})
	if polisher.Configured() {
		t.Fatal("expected a missing API key to leave the polisher disabled")
	}
	if _, err := polisher.PolishLoveLetter(context.Background(), "hello", ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestDeepSeekPolisherMapsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"do not expose this body"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	polisher := NewDeepSeekPolisher(config.AITextConfig{
		BaseURL: server.URL, APIKey: "test-key", Model: "deepseek-v4-flash",
		Timeout: time.Second, MaxOutputTokens: 2000,
	})
	_, err := polisher.PolishLoveLetter(context.Background(), "hello", "")
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "do not expose") {
		t.Fatalf("expected a sanitized provider error, got %v", err)
	}
}

func TestDeepSeekPolisherRejectsEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	polisher := NewDeepSeekPolisher(config.AITextConfig{
		BaseURL: server.URL, APIKey: "test-key", Model: "deepseek-v4-flash",
		Timeout: time.Second, MaxOutputTokens: 2000,
	})
	if _, err := polisher.PolishLoveLetter(context.Background(), "hello", ""); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}
