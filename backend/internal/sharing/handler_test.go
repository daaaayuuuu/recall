package sharing

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/ratelimit"

	"github.com/go-chi/chi/v5"
)

func TestShareStatus(t *testing.T) {
	now := time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	share := Share{ExpiresAt: now.Add(time.Hour)}
	if status := shareStatus(share, now); status != "active" {
		t.Fatalf("expected active, got %q", status)
	}
	share.RevokedAt = sql.NullTime{Time: now, Valid: true}
	if status := shareStatus(share, now); status != "revoked" {
		t.Fatalf("expected revoked, got %q", status)
	}
	share.RevokedAt = sql.NullTime{}
	share.ExpiresAt = now
	if status := shareStatus(share, now); status != "expired" {
		t.Fatalf("expected expired, got %q", status)
	}
}

func TestPrivateAndPublicMountsAreDisjoint(t *testing.T) {
	handler := &Handler{}
	privateRoutes := mountedRoutes(t, handler.MountPrivate)
	publicRoutes := mountedRoutes(t, handler.MountPublic)

	if len(privateRoutes) == 0 || len(publicRoutes) == 0 {
		t.Fatalf("expected both surfaces to register routes: private=%v public=%v", privateRoutes, publicRoutes)
	}
	for _, route := range privateRoutes {
		if strings.HasPrefix(route, "/public") {
			t.Fatalf("private surface unexpectedly registered public route %q", route)
		}
	}
	for _, route := range publicRoutes {
		if !strings.HasPrefix(route, "/public/") {
			t.Fatalf("public surface unexpectedly registered private route %q", route)
		}
	}
}

func mountedRoutes(t *testing.T, mount func(chi.Router)) []string {
	t.Helper()
	router := chi.NewRouter()
	mount(router)
	routes := make([]string, 0)
	if err := chi.Walk(router, func(_ string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, route)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return routes
}

func TestPublicShareDTOUsesNicknameFallbackAndNoPrivateFields(t *testing.T) {
	share := Share{GameTitle: "夏日回忆", ExpiresAt: time.Now().Add(time.Hour)}
	data := publicShareDTO(share)
	creator := data["creator"].(map[string]string)
	if creator["displayName"] != "一位朋友" {
		t.Fatalf("unexpected fallback: %#v", creator)
	}
	for _, forbidden := range []string{"createdByUserId", "secretHash", "secretCiphertext", "userId", "email"} {
		if _, exists := data[forbidden]; exists {
			t.Fatalf("public DTO leaked %s", forbidden)
		}
	}
}

func TestPublicPlayEventRequiresValidSessionAndUsesTrustedAssociations(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{Event: analytics.Event{ID: "01K00000000000000000000009"}}, nil)
	handler := newPublicAnalyticsHandler(recorder)
	session := PlaySession{
		ID: "01K00000000000000000000001", ShareLinkID: "01K00000000000000000000002",
		GameID: "01K00000000000000000000003", GameVersionID: "01K00000000000000000000004",
	}
	var lookups int
	handler.findPlaySession = func(_ context.Context, _ []byte, _ time.Time) (PlaySession, error) {
		lookups++
		return session, nil
	}
	router := chi.NewRouter()
	handler.MountPublic(router)

	request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(`{
		"eventName":"play.completed",
		"clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2",
		"properties":{"mode":"public"}
	}`))
	request.Header.Set("Origin", "https://play.example")
	request.AddCookie(&http.Cookie{Name: "play_session", Value: "opaque-cookie-value"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if lookups != 1 {
		t.Fatalf("session lookups = %d", lookups)
	}
	inputs := recorder.RecordedInputs()
	if len(inputs) != 1 {
		t.Fatalf("record count = %d", len(inputs))
	}
	input := inputs[0]
	if input.GameID != session.GameID || input.GameVersionID != session.GameVersionID || input.ShareID != session.ShareLinkID || input.PlaySessionID != session.ID {
		t.Fatalf("record associations = %#v", input)
	}
	for _, forbidden := range []string{"opaque-cookie-value", "play_session", "gameId", "shareId", "loginId", "bucket", "objectKey"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("response leaked %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestPublicPlayEventRejectsMissingExpiredOrRevokedSession(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
	handler := newPublicAnalyticsHandler(recorder)
	handler.findPlaySession = func(_ context.Context, _ []byte, _ time.Time) (PlaySession, error) {
		return PlaySession{}, ErrPlayExpired
	}
	router := chi.NewRouter()
	handler.MountPublic(router)
	body := `{"eventName":"play.replayed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public"}}`

	request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(body))
	request.Header.Set("Origin", "https://play.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(body))
	request.Header.Set("Origin", "https://play.example")
	request.AddCookie(&http.Cookie{Name: "play_session", Value: "expired"})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("expired session response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if len(recorder.RecordedInputs()) != 0 {
		t.Fatalf("invalid session recorded events: %#v", recorder.RecordedInputs())
	}
}

func TestPublicPlayEventRejectsRepositorySessionWhenShareHasExpired(t *testing.T) {
	now := time.Date(2026, 8, 16, 4, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		shareExpires time.Time
	}{
		{"share expires exactly now", now},
		{"share expired before now", now.Add(-time.Microsecond)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, backend := openPlaySessionRepository(t, now, test.shareExpires)
			recorder := analytics.NewFakeRecorder(analytics.RecordResult{Event: analytics.Event{ID: "01K00000000000000000000009"}}, nil)
			handler := NewHandler(repository, nil, nil, nil, recorder, config.Config{
				App:     config.AppConfig{Environment: "test", PlayBaseURL: "https://play.example"},
				Sharing: config.SharingConfig{MaxLinkLifetimeDays: 90, PlaySessionMinutes: 30},
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			handler.now = func() time.Time { return now }
			router := chi.NewRouter()
			handler.MountPublic(router)

			request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(
				`{"eventName":"play.completed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public"}}`))
			request.Header.Set("Origin", "https://play.example")
			request.AddCookie(&http.Cookie{Name: "play_session", Value: "opaque-session-token"})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if len(recorder.RecordedInputs()) != 0 {
				t.Fatalf("expired share recorded analytics: %#v", recorder.RecordedInputs())
			}
			if backend.lastSeenUpdates != 0 {
				t.Fatalf("expired share updated last_seen %d times", backend.lastSeenUpdates)
			}
		})
	}
}

func TestPublicPlayEventEnforcesPlayOriginAndStrictBody(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
	handler := newPublicAnalyticsHandler(recorder)
	handler.findPlaySession = func(_ context.Context, _ []byte, _ time.Time) (PlaySession, error) {
		return PlaySession{
			ID: "01K00000000000000000000001", ShareLinkID: "01K00000000000000000000002",
			GameID: "01K00000000000000000000003", GameVersionID: "01K00000000000000000000004",
		}, nil
	}
	router := chi.NewRouter()
	handler.MountPublic(router)
	valid := `{"eventName":"play.completed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public"}}`

	request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(valid))
	request.Header.Set("Origin", "https://app.example")
	request.AddCookie(&http.Cookie{Name: "play_session", Value: "valid"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("app origin status = %d, body = %s", response.Code, response.Body.String())
	}

	spoofed := strings.Replace(valid, `"properties"`, `"gameId":"01K00000000000000000000009","properties"`, 1)
	request = httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(spoofed))
	request.Header.Set("Origin", "https://play.example")
	request.AddCookie(&http.Cookie{Name: "play_session", Value: "valid"})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("spoofed body status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(recorder.RecordedInputs()) != 0 {
		t.Fatalf("rejected requests recorded events: %#v", recorder.RecordedInputs())
	}
}

func newPublicAnalyticsHandler(recorder analytics.Recorder) *Handler {
	handler := NewHandler(nil, nil, nil, nil, recorder, config.Config{
		App:     config.AppConfig{Environment: "test", PlayBaseURL: "https://play.example"},
		Sharing: config.SharingConfig{MaxLinkLifetimeDays: 90, PlaySessionMinutes: 30},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return handler
}

func TestPublicPlayEventRepositoryFailureDoesNotLeakDetails(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, errors.New("secret object key"))
	handler := newPublicAnalyticsHandler(recorder)
	handler.findPlaySession = func(_ context.Context, _ []byte, _ time.Time) (PlaySession, error) {
		return PlaySession{
			ID: "01K00000000000000000000001", ShareLinkID: "01K00000000000000000000002",
			GameID: "01K00000000000000000000003", GameVersionID: "01K00000000000000000000004",
		}, nil
	}
	router := chi.NewRouter()
	handler.MountPublic(router)
	request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(
		`{"eventName":"play.completed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public"}}`))
	request.Header.Set("Origin", "https://play.example")
	request.AddCookie(&http.Cookie{Name: "play_session", Value: "valid"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "object key") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestPublicPlayEventRateLimitReturns429BeforeSessionOrRecorder(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{Event: analytics.Event{ID: "01K00000000000000000000009"}}, nil)
	handler := newPublicAnalyticsHandler(recorder)
	handler.publicLimiter = ratelimit.New(1, time.Hour)
	lookups := 0
	handler.findPlaySession = func(_ context.Context, _ []byte, _ time.Time) (PlaySession, error) {
		lookups++
		return PlaySession{
			ID: "01K00000000000000000000001", ShareLinkID: "01K00000000000000000000002",
			GameID: "01K00000000000000000000003", GameVersionID: "01K00000000000000000000004",
		}, nil
	}
	router := chi.NewRouter()
	handler.MountPublic(router)
	body := `{"eventName":"play.completed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public"}}`
	perform := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(body))
		request.Header.Set("Origin", "https://play.example")
		request.AddCookie(&http.Cookie{Name: "play_session", Value: "valid"})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if first := perform(); first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	if second := perform(); second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	if len(recorder.RecordedInputs()) != 1 || lookups != 1 {
		t.Fatalf("rate-limited side effects: records=%d lookups=%d", len(recorder.RecordedInputs()), lookups)
	}
}
