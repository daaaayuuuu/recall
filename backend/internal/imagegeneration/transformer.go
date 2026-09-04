package imagegeneration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gamegen/backend/internal/platform/config"
)

var (
	ErrNotConfigured  = errors.New("image-to-image provider is not configured")
	ErrInputTooLarge  = errors.New("image-to-image input exceeds configured limit")
	ErrOutputTooLarge = errors.New("image-to-image output exceeds configured limit")
	ErrTimeout        = errors.New("image-to-image request timed out")
	ErrUnavailable    = errors.New("image-to-image provider is unavailable")
	ErrInvalidOutput  = errors.New("image-to-image provider returned an invalid image response")
)

type Input struct {
	Image    []byte
	MIMEType string
	Prompt   string
}

type Result struct {
	Image             []byte
	ProviderRequestID string
}

type Transformer interface {
	Transform(context.Context, Input) (Result, error)
}

func New(cfg config.AIImageToImageConfig) (Transformer, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Passthrough{maxInputBytes: cfg.MaxInputBytes, maxOutputBytes: cfg.MaxOutputBytes}, nil
	}
	switch strings.TrimSpace(cfg.Provider) {
	case "openai-compatible":
		return NewOpenAICompatible(cfg), nil
	case "":
		return nil, ErrNotConfigured
	default:
		return nil, fmt.Errorf("unsupported image-to-image provider %q", cfg.Provider)
	}
}

// Passthrough keeps the full creation flow available when no image model key is
// configured. The Worker still normalizes and stores the image as a game_render.
type Passthrough struct {
	maxInputBytes  int64
	maxOutputBytes int64
}

func (transformer Passthrough) Transform(_ context.Context, input Input) (Result, error) {
	if int64(len(input.Image)) > transformer.maxInputBytes {
		return Result{}, ErrInputTooLarge
	}
	if int64(len(input.Image)) > transformer.maxOutputBytes {
		return Result{}, ErrOutputTooLarge
	}
	return Result{Image: append([]byte(nil), input.Image...)}, nil
}
