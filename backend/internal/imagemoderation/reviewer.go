package imagemoderation

import (
	"context"
	"errors"
	"io"
)

var (
	ErrNotConfigured   = errors.New("image moderation is not configured")
	ErrTimeout         = errors.New("image moderation request timed out")
	ErrUnavailable     = errors.New("image moderation provider is unavailable")
	ErrInvalidResponse = errors.New("image moderation returned an invalid response")
)

// Purpose gives a reviewer controlled product context without coupling it to a
// specific upload handler. Callers must use a stable internal value rather than
// user-provided text.
type Purpose string

const (
	PurposeGameAsset Purpose = "game_asset"
	PurposeAvatar    Purpose = "avatar"
)

type Input struct {
	Image    io.ReadSeeker
	MIMEType string
	Purpose  Purpose
}

type Decision struct {
	Approved          bool
	Categories        []string
	Reason            string
	ProviderRequestID string
}

// Reviewer is the reusable boundary for all image moderation providers.
// Implementations must not close Input.Image.
type Reviewer interface {
	Configured() bool
	Review(context.Context, Input) (Decision, error)
}

type UnconfiguredReviewer struct{}

func (UnconfiguredReviewer) Configured() bool { return false }

func (UnconfiguredReviewer) Review(context.Context, Input) (Decision, error) {
	return Decision{}, ErrNotConfigured
}

// DevelopmentReviewer is a deterministic test double. The runtime factory does
// not select it from environment configuration.
type DevelopmentReviewer struct{}

func (DevelopmentReviewer) Configured() bool { return true }

func (DevelopmentReviewer) Review(context.Context, Input) (Decision, error) {
	return Decision{Approved: true}, nil
}
