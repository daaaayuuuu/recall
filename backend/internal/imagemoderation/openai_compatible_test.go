package imagemoderation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"
)

func TestOpenAICompatibleReviewerApprovesSafeImage(t *testing.T) {
	var requestBody moderationChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != chatCompletionsPath || request.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatalf("unexpected provider request: %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"review-1","choices":[{"message":{"content":"{\"approved\":true,\"categories\":[],\"reason\":\"Benign portrait.\"}"}}]}`))
	}))
	defer server.Close()

	reviewer := NewOpenAICompatibleReviewer(config.AIImageModerationConfig{
		BaseURL: server.URL, APIKey: "test-secret", Model: "vision-test",
		Timeout: time.Second, MaxOutputTokens: 200,
	})
	decision, err := reviewer.Review(context.Background(), Input{
		Image: pngImage(t, 4, 3), MIMEType: "image/png", Purpose: PurposeGameAsset,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Approved || decision.ProviderRequestID != "review-1" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if requestBody.Model != "vision-test" || requestBody.ResponseFormat.Type != "json_object" || len(requestBody.Messages) != 2 {
		t.Fatalf("unexpected request body: %#v", requestBody)
	}
	encoded, _ := json.Marshal(requestBody.Messages[1].Content)
	if !strings.Contains(string(encoded), "data:image/jpeg;base64,") || strings.Contains(string(encoded), "private-name") {
		t.Fatalf("expected an inline sanitized preview, got %s", encoded)
	}
}

func TestOpenAICompatibleReviewerReturnsRejectedDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"review-2","choices":[{"message":{"content":"{\"approved\":false,\"categories\":[\"privacy_document\"],\"reason\":\"Readable identity document.\"}"}}]}`))
	}))
	defer server.Close()
	reviewer := NewOpenAICompatibleReviewer(config.AIImageModerationConfig{
		BaseURL: server.URL, APIKey: "secret", Model: "vision-test",
		Timeout: time.Second, MaxOutputTokens: 200,
	})
	decision, err := reviewer.Review(context.Background(), Input{Image: pngImage(t, 2, 2), Purpose: PurposeGameAsset})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Approved || len(decision.Categories) != 1 || decision.Categories[0] != "privacy_document" {
		t.Fatalf("unexpected rejection: %#v", decision)
	}
}

func TestNoKeyProducesUnconfiguredReviewerEvenWhenProviderIsSelected(t *testing.T) {
	reviewer := New(config.AIImageModerationConfig{Provider: "openai-compatible"})
	if reviewer.Configured() {
		t.Fatal("expected moderation to be disabled without an API key")
	}
}

func TestParseProviderDecisionRejectsInconsistentOrUnknownOutput(t *testing.T) {
	invalid := []string{
		`{"approved":true,"categories":["other_unsafe"],"reason":"unsafe"}`,
		`{"approved":false,"categories":[],"reason":"unsafe"}`,
		`{"approved":false,"categories":["made_up"],"reason":"unsafe"}`,
		`{"approved":true,"categories":[],"reason":""}`,
		`{"approved":true,"categories":[],"reason":"safe"} trailing`,
	}
	for _, content := range invalid {
		if _, err := parseProviderDecision(content); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("expected invalid response for %q, got %v", content, err)
		}
	}
}

func TestEncodeModerationPreviewDownsizesAndRemovesOriginalEncoding(t *testing.T) {
	preview, err := encodeModerationPreview(pngImage(t, 1400, 700))
	if err != nil {
		t.Fatal(err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || decoded.Bounds().Dx() != 768 || decoded.Bounds().Dy() != 384 {
		t.Fatalf("unexpected moderation preview: format=%s bounds=%v", format, decoded.Bounds())
	}
}

func pngImage(t *testing.T, width, height int) *bytes.Reader {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	value.Set(0, 0, color.RGBA{R: 255, G: 100, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(encoded.Bytes())
}
