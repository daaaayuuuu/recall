package imagegeneration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"gamegen/backend/internal/platform/config"
)

type OpenAICompatible struct {
	endpoint       string
	apiKey         string
	model          string
	quality        string
	maxInputBytes  int64
	maxOutputBytes int64
	client         *http.Client
}

func NewOpenAICompatible(cfg config.AIImageToImageConfig) *OpenAICompatible {
	return &OpenAICompatible{
		endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/images/edits",
		apiKey:   cfg.APIKey, model: cfg.Model, quality: cfg.Quality,
		maxInputBytes: cfg.MaxInputBytes, maxOutputBytes: cfg.MaxOutputBytes,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (transformer *OpenAICompatible) Transform(ctx context.Context, input Input) (Result, error) {
	if int64(len(input.Image)) > transformer.maxInputBytes {
		return Result{}, ErrInputTooLarge
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "source"+imageExtension(input.MIMEType))
	if err != nil {
		return Result{}, fmt.Errorf("create image edit multipart file: %w", err)
	}
	if _, err := part.Write(input.Image); err != nil {
		return Result{}, fmt.Errorf("write image edit multipart file: %w", err)
	}
	fields := map[string]string{
		"model": transformer.model, "prompt": input.Prompt, "n": "1",
		"output_format": "png", "size": "auto", "quality": transformer.quality,
	}
	for key, value := range fields {
		if value != "" {
			if err := writer.WriteField(key, value); err != nil {
				return Result{}, fmt.Errorf("write image edit field: %w", err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		return Result{}, fmt.Errorf("close image edit multipart request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, transformer.endpoint, &body)
	if err != nil {
		return Result{}, fmt.Errorf("create image edit request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+transformer.apiKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := transformer.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			return Result{}, ErrTimeout
		}
		return Result{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return Result{}, fmt.Errorf("%w: status %d", ErrUnavailable, response.StatusCode)
	}

	responseLimit := transformer.maxOutputBytes*2 + (1 << 20)
	encoded, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return Result{}, fmt.Errorf("%w: read response", ErrUnavailable)
	}
	if int64(len(encoded)) > responseLimit {
		return Result{}, ErrOutputTooLarge
	}
	var payload struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil || len(payload.Data) != 1 || payload.Data[0].B64JSON == "" {
		return Result{}, ErrInvalidOutput
	}
	if int64(base64.StdEncoding.DecodedLen(len(payload.Data[0].B64JSON))) > transformer.maxOutputBytes {
		return Result{}, ErrOutputTooLarge
	}
	image, err := base64.StdEncoding.DecodeString(payload.Data[0].B64JSON)
	if err != nil || len(image) == 0 {
		return Result{}, ErrInvalidOutput
	}
	if int64(len(image)) > transformer.maxOutputBytes {
		return Result{}, ErrOutputTooLarge
	}
	return Result{Image: image, ProviderRequestID: response.Header.Get("x-request-id")}, nil
}

func imageExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/png":
		return ".png"
	default:
		return ".png"
	}
}
