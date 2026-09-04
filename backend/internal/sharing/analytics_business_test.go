package sharing

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"

	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/security"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"
)

func TestSharingBusinessEventsMatchFrozenContract(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
	handler := &Handler{analytics: recorder, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(
		context.Background(), middleware.RequestIDKey, "01K00000000000000000000009",
	))
	common := analytics.RecordInput{
		Source: analytics.SourceAPI, GameID: "01K00000000000000000000002",
		GameVersionID: "01K00000000000000000000003", ShareID: "01K00000000000000000000004",
	}
	created := common
	created.EventName, created.ActorType, created.CreatorID = analytics.EventShareCreated, analytics.ActorCreator, "01K00000000000000000000001"
	created.Properties = sharingEventProperties(map[string]any{"lifetimeDays": 7})
	handler.recordBusinessEvent(request, created)
	opened := common
	opened.EventName, opened.ActorType, opened.Properties = analytics.EventShareOpened, analytics.ActorReceiver, sharingEventProperties(map[string]any{})
	handler.recordBusinessEvent(request, opened)
	started := opened
	started.EventName, started.PlaySessionID = analytics.EventPlayStarted, "01K00000000000000000000005"
	handler.recordBusinessEvent(request, started)

	inputs := recorder.RecordedInputs()
	if len(inputs) != 3 {
		t.Fatalf("recorded %d sharing events, want 3", len(inputs))
	}
	for _, input := range inputs {
		if _, err := analytics.ValidateRecordInput(input); err != nil {
			t.Errorf("invalid %s event: %v", input.EventName, err)
		}
	}
}

func TestCreateShareEntryRecordsOnlySuccessfulCreation(t *testing.T) {
	tests := []struct {
		name        string
		currentErr  error
		createErr   error
		recorderErr error
		wantStatus  int
		wantRecords int
	}{
		{"success", nil, nil, nil, http.StatusCreated, 1},
		{"current version failure", errors.New("database unavailable"), nil, nil, http.StatusInternalServerError, 0},
		{"create database failure", nil, errors.New("database unavailable"), nil, http.StatusInternalServerError, 0},
		{"analytics failure", nil, nil, errors.New("analytics unavailable"), http.StatusCreated, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, recorder, _ := newSharingAnalyticsEntryHandler(t, test.recorderErr)
			handler.currentVersion = func(context.Context, string, string) (string, error) {
				if test.currentErr != nil {
					return "", test.currentErr
				}
				return "01K00000000000000000000003", nil
			}
			handler.createShareRecord = func(_ context.Context, share Share, userID string, _ time.Time) (Share, error) {
				if test.createErr != nil {
					return Share{}, test.createErr
				}
				share.CreatedByUserID = userID
				return share, nil
			}
			expiresAt := handler.now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
			request := sharingRequestWithPublicOrGameID(httptest.NewRequest(http.MethodPost, "/games/game/share-links", strings.NewReader(`{"expiresAt":"`+expiresAt+`"}`)), "gameId", "01K00000000000000000000002")
			response := httptest.NewRecorder()
			handler.createShare(response, request)
			if response.Code != test.wantStatus || len(recorder.RecordedInputs()) != test.wantRecords {
				t.Fatalf("status=%d records=%d body=%s", response.Code, len(recorder.RecordedInputs()), response.Body.String())
			}
			if test.wantRecords == 1 {
				input := recorder.RecordedInputs()[0]
				if input.EventName != analytics.EventShareCreated || !bytes.Contains(input.Properties, []byte(`"lifetimeDays":7`)) {
					t.Fatalf("event=%#v", input)
				}
			}
		})
	}
}

func TestResolveAndCreateSessionEntriesHaveExclusiveEventsAndValidationIsPure(t *testing.T) {
	handler, recorder, share := newSharingAnalyticsEntryHandler(t, nil)
	lookups := 0
	handler.findPublicShare = func(context.Context, string) (Share, error) { lookups++; return share, nil }
	handler.createPlaySessionRecord = func(_ context.Context, sessionID, _ string, _ []byte, _ []byte, expiresAt, _ time.Time) (PlaySession, error) {
		return PlaySession{ID: sessionID, ShareLinkID: share.ID, GameID: share.GameID, GameVersionID: share.GameVersionID, ExpiresAt: expiresAt, GameTitle: "Memory", TemplateID: "memory-game", TemplateVersion: "1.0.0"}, nil
	}

	validationRequest := sharingRequestWithPublicOrGameID(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"secret":"valid-secret"}`)), "publicId", share.PublicID)
	if _, _, ok := handler.validatePublicShare(httptest.NewRecorder(), validationRequest); !ok {
		t.Fatal("valid share rejected")
	}
	if len(recorder.RecordedInputs()) != 0 {
		t.Fatal("validation helper produced analytics side effects")
	}

	perform := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"secret":"valid-secret"}`))
		request.Header.Set("Origin", "https://play.example")
		response := httptest.NewRecorder()
		router := chi.NewRouter()
		handler.MountPublic(router)
		router.ServeHTTP(response, request)
		return response
	}
	if response := perform("/public/shares/" + share.PublicID + "/resolve"); response.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", response.Code, response.Body.String())
	}
	inputs := recorder.RecordedInputs()
	if len(inputs) != 1 || inputs[0].EventName != analytics.EventShareOpened {
		t.Fatalf("resolve events=%#v", inputs)
	}
	if response := perform("/public/shares/" + share.PublicID + "/play-sessions"); response.Code != http.StatusCreated {
		t.Fatalf("session status=%d body=%s", response.Code, response.Body.String())
	}
	inputs = recorder.RecordedInputs()
	if len(inputs) != 2 || inputs[1].EventName != analytics.EventPlayStarted {
		t.Fatalf("session events=%#v", inputs)
	}
	if lookups != 3 {
		t.Fatalf("share lookups=%d, want validation + resolve + session", lookups)
	}
}

func TestPublicBusinessEntriesRejectSecretExpiryAndDatabaseFailuresWithoutEvents(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		secret     string
		findErr    error
		expired    bool
		sessionErr error
		wantStatus int
	}{
		{"wrong secret", "/resolve", "wrong", nil, false, nil, http.StatusGone},
		{"expired", "/resolve", "valid-secret", nil, true, nil, http.StatusGone},
		{"lookup database failure", "/resolve", "valid-secret", errors.New("database unavailable"), false, nil, http.StatusInternalServerError},
		{"session database failure", "/play-sessions", "valid-secret", nil, false, errors.New("database unavailable"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, recorder, share := newSharingAnalyticsEntryHandler(t, nil)
			if test.expired {
				share.ExpiresAt = handler.now()
			}
			handler.findPublicShare = func(context.Context, string) (Share, error) { return share, test.findErr }
			handler.createPlaySessionRecord = func(context.Context, string, string, []byte, []byte, time.Time, time.Time) (PlaySession, error) {
				return PlaySession{}, test.sessionErr
			}
			request := httptest.NewRequest(http.MethodPost, "/public/shares/"+share.PublicID+test.path, strings.NewReader(`{"secret":"`+test.secret+`"}`))
			request.Header.Set("Origin", "https://play.example")
			response := httptest.NewRecorder()
			router := chi.NewRouter()
			handler.MountPublic(router)
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus || len(recorder.RecordedInputs()) != 0 {
				t.Fatalf("status=%d records=%d body=%s", response.Code, len(recorder.RecordedInputs()), response.Body.String())
			}
		})
	}
}

func TestResolveAnalyticsFailureDoesNotChangeSuccessResponse(t *testing.T) {
	handler, recorder, share := newSharingAnalyticsEntryHandler(t, errors.New("analytics unavailable"))
	handler.findPublicShare = func(context.Context, string) (Share, error) { return share, nil }
	request := httptest.NewRequest(http.MethodPost, "/public/shares/"+share.PublicID+"/resolve", strings.NewReader(`{"secret":"valid-secret"}`))
	request.Header.Set("Origin", "https://play.example")
	response := httptest.NewRecorder()
	router := chi.NewRouter()
	handler.MountPublic(router)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(recorder.RecordedInputs()) != 1 {
		t.Fatalf("status=%d records=%d body=%s", response.Code, len(recorder.RecordedInputs()), response.Body.String())
	}
}

func newSharingAnalyticsEntryHandler(t *testing.T, recorderErr error) (*Handler, *analytics.FakeRecorder, Share) {
	t.Helper()
	cipher, err := contentcrypto.New(base64.StdEncoding.EncodeToString(make([]byte, 32)), 1)
	if err != nil {
		t.Fatal(err)
	}
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, recorderErr)
	handler := NewHandler(nil, nil, cipher, nil, recorder, config.Config{
		App:     config.AppConfig{Environment: "test", PlayBaseURL: "https://play.example"},
		Sharing: config.SharingConfig{MaxLinkLifetimeDays: 90, PlaySessionMinutes: 30},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	handler.now = func() time.Time { return now }
	handler.creatorUser = func(*http.Request) auth.User { return auth.User{ID: "01K00000000000000000000001"} }
	hash := security.HashToken("valid-secret")
	share := Share{
		ID: "01K00000000000000000000004", GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		PublicID: "01K00000000000000000000005", SecretHash: hash[:], ExpiresAt: now.Add(24 * time.Hour),
		GameStatus: "ready", VersionStatus: "ready", GameTitle: "Memory",
	}
	return handler, recorder, share
}

func sharingRequestWithPublicOrGameID(request *http.Request, name, value string) *http.Request {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add(name, value)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
