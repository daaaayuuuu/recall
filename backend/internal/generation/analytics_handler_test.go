package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestGenerationSubmittedEventDistinguishesIdempotentRequests(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
	handler := &Handler{analytics: recorder, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(
		context.Background(), middleware.RequestIDKey, "01K00000000000000000000009",
	))
	for _, deduplicated := range []bool{false, true} {
		handler.recordEvent(request, analytics.RecordInput{
			EventName: analytics.EventGenerationSubmitted, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
			CreatorID: "01K00000000000000000000001", GameID: "01K00000000000000000000002",
			GameVersionID: "01K00000000000000000000003", GenerationRunID: "01K00000000000000000000004",
			Properties: processorProperties(map[string]any{"attemptNumber": 1, "deduplicated": deduplicated}),
		})
	}
	inputs := recorder.RecordedInputs()
	if len(inputs) != 2 {
		t.Fatalf("recorded %d submission events, want 2", len(inputs))
	}
	for _, input := range inputs {
		if _, err := analytics.ValidateRecordInput(input); err != nil {
			t.Errorf("invalid submission event: %v", err)
		}
	}
}

func TestSubmitRunEntryRecordsEachIdempotentSuccessAgainstTheTrustedRun(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
	handler := newGenerationAnalyticsEntryHandler(recorder)
	run := Run{
		ID: "01K00000000000000000000004", GameID: "01K00000000000000000000002",
		GameVersionID: "01K00000000000000000000003", AttemptNumber: 1,
		Status: "queued", Stage: "queued", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	calls := 0
	handler.submitGeneration = func(context.Context, string, string, string, string, string, []byte, time.Time) (Run, bool, error) {
		calls++
		return run, calls > 1, nil
	}
	for index, wantStatus := range []int{http.StatusCreated, http.StatusOK} {
		request := generationSubmitRequest(`{"versionId":"01K00000000000000000000003"}`, "same-idempotency-key")
		response := httptest.NewRecorder()
		handler.submitRun(response, request)
		if response.Code != wantStatus {
			t.Fatalf("request %d status=%d body=%s", index+1, response.Code, response.Body.String())
		}
	}
	inputs := recorder.RecordedInputs()
	if calls != 2 || len(inputs) != 2 {
		t.Fatalf("submit calls=%d events=%d", calls, len(inputs))
	}
	for index, input := range inputs {
		if input.EventName != analytics.EventGenerationSubmitted || input.GenerationRunID != run.ID || input.GameID != run.GameID || input.GameVersionID != run.GameVersionID {
			t.Fatalf("event %d = %#v", index, input)
		}
		var properties struct {
			AttemptNumber int  `json:"attemptNumber"`
			Deduplicated  bool `json:"deduplicated"`
		}
		if err := json.Unmarshal(input.Properties, &properties); err != nil {
			t.Fatal(err)
		}
		if properties.AttemptNumber != 1 || properties.Deduplicated != (index == 1) {
			t.Fatalf("event %d properties=%+v", index, properties)
		}
	}
}

func TestSubmitRunEntryFailureDoesNotRecordAndAnalyticsFailureIsNonBlocking(t *testing.T) {
	t.Run("business failure", func(t *testing.T) {
		recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
		handler := newGenerationAnalyticsEntryHandler(recorder)
		handler.submitGeneration = func(context.Context, string, string, string, string, string, []byte, time.Time) (Run, bool, error) {
			return Run{}, false, ErrAssetsRequired
		}
		response := httptest.NewRecorder()
		handler.submitRun(response, generationSubmitRequest(`{"versionId":"01K00000000000000000000003"}`, "key"))
		if response.Code != http.StatusUnprocessableEntity || len(recorder.RecordedInputs()) != 0 {
			t.Fatalf("status=%d events=%d body=%s", response.Code, len(recorder.RecordedInputs()), response.Body.String())
		}
	})
	t.Run("analytics failure", func(t *testing.T) {
		recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, errors.New("analytics unavailable"))
		handler := newGenerationAnalyticsEntryHandler(recorder)
		handler.submitGeneration = func(context.Context, string, string, string, string, string, []byte, time.Time) (Run, bool, error) {
			return Run{ID: "01K00000000000000000000004", GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003", AttemptNumber: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}, false, nil
		}
		response := httptest.NewRecorder()
		handler.submitRun(response, generationSubmitRequest(`{"versionId":"01K00000000000000000000003"}`, "key"))
		if response.Code != http.StatusCreated || len(recorder.RecordedInputs()) != 1 {
			t.Fatalf("status=%d events=%d body=%s", response.Code, len(recorder.RecordedInputs()), response.Body.String())
		}
	})
}

func newGenerationAnalyticsEntryHandler(recorder analytics.Recorder) *Handler {
	handler := NewHandler(nil, nil, recorder, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.creatorUser = func(*http.Request) auth.User { return auth.User{ID: "01K00000000000000000000001"} }
	return handler
}

func generationSubmitRequest(body, key string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/games/game/generation-runs", bytes.NewBufferString(body))
	request.Header.Set("Idempotency-Key", key)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("gameId", "01K00000000000000000000002")
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
