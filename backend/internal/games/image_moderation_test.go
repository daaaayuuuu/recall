package games

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/imagemoderation"
)

type stubImageReviewer struct {
	configured bool
	decision   imagemoderation.Decision
	err        error
	calls      int
}

func (reviewer *stubImageReviewer) Configured() bool { return reviewer.configured }

func (reviewer *stubImageReviewer) Review(_ context.Context, input imagemoderation.Input) (imagemoderation.Decision, error) {
	reviewer.calls++
	if input.Image == nil || input.Purpose != imagemoderation.PurposeGameAsset {
		return imagemoderation.Decision{}, errors.New("unexpected moderation input")
	}
	return reviewer.decision, reviewer.err
}

func TestUploadAssetModerationStopsBeforeStorage(t *testing.T) {
	tests := []struct {
		name        string
		reviewer    *stubImageReviewer
		wantStatus  int
		wantCode    string
		wantReviews int
	}{
		{
			name: "rejected image",
			reviewer: &stubImageReviewer{configured: true, decision: imagemoderation.Decision{
				Approved: false, Categories: []string{"privacy_document"}, ProviderRequestID: "review-1",
			}},
			wantStatus: http.StatusUnprocessableEntity, wantCode: "IMAGE_MODERATION_REJECTED", wantReviews: 1,
		},
		{
			name:       "provider unavailable",
			reviewer:   &stubImageReviewer{configured: true, err: imagemoderation.ErrUnavailable},
			wantStatus: http.StatusServiceUnavailable, wantCode: "IMAGE_MODERATION_UNAVAILABLE", wantReviews: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, recorder := newGameAnalyticsEntryHandler(t, nil)
			handler.imageReviewer = test.reviewer
			storageCalls, databaseCalls := 0, 0
			handler.putAssetFile = func(context.Context, string, string, string, string) (int64, error) {
				storageCalls++
				return 0, nil
			}
			handler.addAssetRecord = func(context.Context, string, string, string, Asset, bool, int) ([]ObjectRef, error) {
				databaseCalls++
				return nil, nil
			}
			handler.getVersionRecord = func(context.Context, string, string, string) (Version, error) { return Version{}, nil }

			request := requestWithGameVersionIDs(newPNGUploadRequest(t), "01K00000000000000000000002", "01K00000000000000000000003")
			response := httptest.NewRecorder()
			handler.uploadAsset(response, request)

			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.reviewer.calls != test.wantReviews || storageCalls != 0 || databaseCalls != 0 || len(recorder.RecordedInputs()) != 0 {
				t.Fatalf("reviews=%d storage=%d database=%d events=%d", test.reviewer.calls, storageCalls, databaseCalls, len(recorder.RecordedInputs()))
			}
		})
	}
}

func TestUploadAssetRejectsProcessedImageAboveGenerationLimit(t *testing.T) {
	handler, recorder := newGameAnalyticsEntryHandler(t, nil)
	reviewer := &stubImageReviewer{configured: true, decision: imagemoderation.Decision{Approved: true}}
	handler.imageReviewer = reviewer
	handler.maxSourceImageBytes = 1
	storageCalls, databaseCalls := 0, 0
	handler.putAssetFile = func(context.Context, string, string, string, string) (int64, error) {
		storageCalls++
		return 0, nil
	}
	handler.addAssetRecord = func(context.Context, string, string, string, Asset, bool, int) ([]ObjectRef, error) {
		databaseCalls++
		return nil, nil
	}
	handler.getVersionRecord = func(context.Context, string, string, string) (Version, error) { return Version{}, nil }

	request := requestWithGameVersionIDs(newPNGUploadRequest(t), "01K00000000000000000000002", "01K00000000000000000000003")
	response := httptest.NewRecorder()
	handler.uploadAsset(response, request)

	if response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "IMAGE_TOO_LARGE") ||
		!strings.Contains(response.Body.String(), "超过 0.00 MiB 上限") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if reviewer.calls != 0 || storageCalls != 0 || databaseCalls != 0 || len(recorder.RecordedInputs()) != 0 {
		t.Fatalf("reviews=%d storage=%d database=%d events=%d", reviewer.calls, storageCalls, databaseCalls, len(recorder.RecordedInputs()))
	}
}

func TestUploadAssetSkipsModerationWhenNoKeyConfigured(t *testing.T) {
	handler, recorder := newGameAnalyticsEntryHandler(t, nil)
	reviewer := &stubImageReviewer{configured: false}
	handler.imageReviewer = reviewer
	storageCalls, databaseCalls := 0, 0
	handler.putAssetFile = func(context.Context, string, string, string, string) (int64, error) {
		storageCalls++
		return 68, nil
	}
	handler.addAssetRecord = func(context.Context, string, string, string, Asset, bool, int) ([]ObjectRef, error) {
		databaseCalls++
		return nil, nil
	}
	handler.getVersionRecord = func(context.Context, string, string, string) (Version, error) { return Version{}, nil }
	handler.presignAsset = func(context.Context, string, string, time.Duration) (*url.URL, error) {
		return url.Parse("https://assets.example/preview.png")
	}

	request := requestWithGameVersionIDs(newPNGUploadRequest(t), "01K00000000000000000000002", "01K00000000000000000000003")
	response := httptest.NewRecorder()
	handler.uploadAsset(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if reviewer.calls != 0 || storageCalls != 1 || databaseCalls != 1 || len(recorder.RecordedInputs()) != 1 {
		t.Fatalf("reviews=%d storage=%d database=%d events=%d", reviewer.calls, storageCalls, databaseCalls, len(recorder.RecordedInputs()))
	}
}
