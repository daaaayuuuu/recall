package imagegeneration

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"
)

func TestOpenAICompatibleTransformsImageUsingEditEndpoint(t *testing.T) {
	source := []byte("source-image")
	output := []byte("generated-image")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/images/edits" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request %s authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		file, header, err := request.FormFile("image")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		actual, _ := io.ReadAll(file)
		if string(actual) != string(source) || header.Filename != "source.jpg" {
			t.Fatalf("unexpected source file %q %q", header.Filename, actual)
		}
		if request.FormValue("model") != "image-model" || request.FormValue("prompt") != "storybook" ||
			request.FormValue("output_format") != "png" || request.FormValue("quality") != "medium" {
			t.Fatalf("unexpected edit fields: %#v", request.MultipartForm.Value)
		}
		response.Header().Set("x-request-id", "request-123")
		_, _ = response.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(output) + `"}]}`))
	}))
	defer server.Close()

	transformer := NewOpenAICompatible(config.AIImageToImageConfig{
		BaseURL: server.URL + "/v1", APIKey: "secret", Model: "image-model", Quality: "medium",
		Timeout: time.Second, MaxInputBytes: 1024, MaxOutputBytes: 1024,
	})
	result, err := transformer.Transform(context.Background(), Input{Image: source, MIMEType: "image/jpeg", Prompt: "storybook"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Image) != string(output) || result.ProviderRequestID != "request-123" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOpenAICompatibleRejectsOversizedInputAndInvalidOutput(t *testing.T) {
	transformer := NewOpenAICompatible(config.AIImageToImageConfig{MaxInputBytes: 3, MaxOutputBytes: 10, Timeout: time.Second})
	if _, err := transformer.Transform(context.Background(), Input{Image: []byte("large")}); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("expected input limit error, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	transformer = NewOpenAICompatible(config.AIImageToImageConfig{
		BaseURL: server.URL, Timeout: time.Second, MaxInputBytes: 1024, MaxOutputBytes: 1024,
	})
	if _, err := transformer.Transform(context.Background(), Input{Image: []byte("ok")}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected invalid output error, got %v", err)
	}
}

func TestPassthroughHonorsConfiguredLimits(t *testing.T) {
	transformer, err := New(config.AIImageToImageConfig{
		Provider: "openai-compatible", MaxInputBytes: 4, MaxOutputBytes: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transformer.Transform(context.Background(), Input{Image: []byte("1234")}); !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("expected output limit error, got %v", err)
	}
	if _, err := transformer.Transform(context.Background(), Input{Image: []byte("12345")}); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("expected input limit error, got %v", err)
	}
}

func TestNoKeyUsesPassthroughEvenWhenProviderIsSelected(t *testing.T) {
	transformer, err := New(config.AIImageToImageConfig{
		Provider: "openai-compatible", MaxInputBytes: 10, MaxOutputBytes: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := transformer.Transform(context.Background(), Input{Image: []byte("source")})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Image) != "source" || result.ProviderRequestID != "" {
		t.Fatalf("unexpected passthrough result: %#v", result)
	}
}
