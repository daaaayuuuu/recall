package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"

	"github.com/go-chi/chi/v5"
)

const (
	testCreatorID = "01K00000000000000000000001"
	testSessionID = "01K00000000000000000000002"
	testEventID   = "01K00000000000000000000003"
)

type handlerRepository struct {
	inputs     []RecordInput
	record     RecordResult
	recordErr  error
	listFilter ListFilter
	page       EventPage
	listErr    error
}

type statefulHandlerRepository struct {
	event *Event
	count int
}

func (repository *statefulHandlerRepository) RecordEvent(_ context.Context, input RecordInput) (RecordResult, error) {
	if repository.event != nil {
		if !sameSemantics(*repository.event, input) {
			return RecordResult{}, ErrClientEventIDConflict
		}
		return RecordResult{Event: *repository.event, Duplicate: true}, nil
	}
	repository.count++
	event := eventFromInput(testEventID, time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC), input)
	repository.event = &event
	return RecordResult{Event: event}, nil
}

func (*statefulHandlerRepository) ListAdminEvents(context.Context, ListFilter) (EventPage, error) {
	return EventPage{}, nil
}

func (repository *handlerRepository) RecordEvent(_ context.Context, input RecordInput) (RecordResult, error) {
	repository.inputs = append(repository.inputs, input)
	return repository.record, repository.recordErr
}

func (repository *handlerRepository) ListAdminEvents(_ context.Context, filter ListFilter) (EventPage, error) {
	repository.listFilter = filter
	return repository.page, repository.listErr
}

func TestCreatorEventUsesAuthenticatedIdentityAndRejectsSpoofedFields(t *testing.T) {
	repository := &handlerRepository{record: RecordResult{Event: Event{ID: testEventID}}}
	router := creatorRouter(repository, allowMiddleware, csrfMiddleware, "https://app.example")

	response := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", `{
		"eventName":"creator.page_viewed",
		"clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2",
		"occurredAt":"2026-08-16T02:35:01.123+08:00",
		"properties":{"page":"game-edit"}
	}`, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(repository.inputs) != 1 {
		t.Fatalf("record count = %d", len(repository.inputs))
	}
	input := repository.inputs[0]
	if input.CreatorID != testCreatorID || input.UserSessionID != testSessionID || input.Source != SourceFrontend || input.ActorType != ActorCreator {
		t.Fatalf("untrusted identity reached recorder: %#v", input)
	}
	if input.OccurredAt == nil || input.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurredAt was not normalized: %#v", input.OccurredAt)
	}

	spoofed := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", `{
		"eventName":"creator.page_viewed",
		"clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2",
		"properties":{"page":"games"},
		"creatorId":"01K00000000000000000000009"
	}`, true)
	if spoofed.Code != http.StatusBadRequest || len(repository.inputs) != 1 {
		t.Fatalf("spoof response = %d %s, records = %d", spoofed.Code, spoofed.Body.String(), len(repository.inputs))
	}
}

func TestCreatorEventEnforcesAuthenticationCSRFAndAppOnlyOrigin(t *testing.T) {
	repository := &handlerRepository{record: RecordResult{Event: Event{ID: testEventID}}}
	body := `{"eventName":"creator.page_viewed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"page":"games"}}`

	unauthorized := creatorRouter(repository, denyMiddleware(http.StatusUnauthorized), csrfMiddleware, "https://app.example")
	if response := performJSON(unauthorized, http.MethodPost, "/analytics/events", "https://app.example", body, true); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	router := creatorRouter(repository, allowMiddleware, csrfMiddleware, "https://app.example")
	if response := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", body, false); response.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d, body = %s", response.Code, response.Body.String())
	}
	if response := performJSON(router, http.MethodPost, "/analytics/events", "https://play.example", body, true); response.Code != http.StatusForbidden {
		t.Fatalf("play origin status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(repository.inputs) != 0 {
		t.Fatalf("rejected requests recorded %d events", len(repository.inputs))
	}
}

func TestFrontendEventValidationIdempotencyAndConflictStatuses(t *testing.T) {
	repository := &handlerRepository{record: RecordResult{Event: Event{ID: testEventID}, Duplicate: true}}
	router := creatorRouter(repository, allowMiddleware, csrfMiddleware, "https://app.example")
	valid := `{"eventName":"creator.page_viewed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"page":"games"}}`
	response := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", valid, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate response = %d %s", response.Code, response.Body.String())
	}

	repository.recordErr = ErrClientEventIDConflict
	response = performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", valid, true)
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict response = %d %s", response.Code, response.Body.String())
	}

	repository.recordErr = &ValidationError{Field: "properties.page", Message: "has an invalid value"}
	response = performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", valid, true)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}

	badTime := strings.Replace(valid, `"properties"`, `"occurredAt":"yesterday","properties"`, 1)
	response = performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", badTime, true)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("time response = %d %s", response.Code, response.Body.String())
	}
}

func TestCreatorHTTPIdempotencyPersistsOnceAndConflictsOnDifferentSemantics(t *testing.T) {
	repository := &statefulHandlerRepository{}
	router := creatorRouter(repository, allowMiddleware, csrfMiddleware, "https://app.example")
	body := `{"eventName":"creator.page_viewed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"page":"games"}}`
	first := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", body, true)
	second := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", body, true)
	if first.Code != http.StatusCreated || second.Code != http.StatusOK {
		t.Fatalf("idempotent statuses = %d then %d; bodies = %s / %s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	if repository.count != 1 || repository.event == nil {
		t.Fatalf("stored count = %d, event = %#v", repository.count, repository.event)
	}
	for _, response := range []*httptest.ResponseRecorder{first, second} {
		if !strings.Contains(response.Body.String(), `"eventId":"`+testEventID+`"`) {
			t.Errorf("response does not contain stable event ID: %s", response.Body.String())
		}
	}

	changed := strings.Replace(body, `"page":"games"`, `"page":"settings"`, 1)
	conflict := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", changed, true)
	if conflict.Code != http.StatusConflict || repository.count != 1 {
		t.Fatalf("semantic conflict = %d %s, stored count = %d", conflict.Code, conflict.Body.String(), repository.count)
	}
}

func TestCreatorEventRejectsEveryClientDeclaredIdentityField(t *testing.T) {
	fields := map[string]string{
		"userId": `"creator_login"`, "creatorId": `"01K00000000000000000000009"`, "loginId": `"creator_login"`,
		"userSessionId": `"01K00000000000000000000009"`, "gameId": `"01K00000000000000000000009"`,
		"gameVersionId": `"01K00000000000000000000009"`, "generationRunId": `"01K00000000000000000000009"`,
		"shareId": `"01K00000000000000000000009"`, "playSessionId": `"01K00000000000000000000009"`,
		"requestId": `"01K00000000000000000000009"`, "source": `"frontend"`, "actorType": `"creator"`,
	}
	for field, value := range fields {
		t.Run(field, func(t *testing.T) {
			repository := &handlerRepository{record: RecordResult{Event: Event{ID: testEventID}}}
			router := creatorRouter(repository, allowMiddleware, csrfMiddleware, "https://app.example")
			body := fmt.Sprintf(`{"eventName":"creator.page_viewed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"page":"games"},%q:%s}`, field, value)
			response := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", body, true)
			if response.Code != http.StatusBadRequest || len(repository.inputs) != 0 {
				t.Fatalf("response = %d %s, recorded=%d", response.Code, response.Body.String(), len(repository.inputs))
			}
		})
	}
}

func TestPublicEventRejectsEveryClientDeclaredIdentityField(t *testing.T) {
	fields := map[string]string{
		"userId": `"creator_login"`, "creatorId": `"01K00000000000000000000009"`, "loginId": `"creator_login"`,
		"userSessionId": `"01K00000000000000000000009"`, "gameId": `"01K00000000000000000000009"`,
		"gameVersionId": `"01K00000000000000000000009"`, "generationRunId": `"01K00000000000000000000009"`,
		"shareId": `"01K00000000000000000000009"`, "playSessionId": `"01K00000000000000000000009"`,
		"requestId": `"01K00000000000000000000009"`, "publicId": `"01K00000000000000000000009"`,
		"source": `"frontend"`, "actorType": `"receiver"`,
	}
	identity := PublicIdentity{
		GameID: "01K00000000000000000000004", GameVersionID: "01K00000000000000000000005",
		ShareID: "01K00000000000000000000006", PlaySessionID: "01K00000000000000000000007",
	}
	for field, value := range fields {
		t.Run(field, func(t *testing.T) {
			recorder := NewFakeRecorder(RecordResult{Event: Event{ID: testEventID}}, nil)
			body := fmt.Sprintf(`{"eventName":"play.completed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public"},%q:%s}`, field, value)
			request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(body))
			response := httptest.NewRecorder()
			RecordPublicEvent(recorder, testLogger(), response, request, identity)
			if response.Code != http.StatusBadRequest || len(recorder.RecordedInputs()) != 0 {
				t.Fatalf("response = %d %s, recorded=%d", response.Code, response.Body.String(), len(recorder.RecordedInputs()))
			}
		})
	}
}

func TestPublicEventAcceptsOnlyReceiverEventsAndTrustedLinks(t *testing.T) {
	recorder := NewFakeRecorder(RecordResult{Event: Event{ID: testEventID}}, nil)
	identity := PublicIdentity{
		GameID: "01K00000000000000000000004", GameVersionID: "01K00000000000000000000005",
		ShareID: "01K00000000000000000000006", PlaySessionID: "01K00000000000000000000007",
	}
	request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(`{
		"eventName":"play.completed",
		"clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2",
		"properties":{"mode":"public"}
	}`))
	response := httptest.NewRecorder()
	RecordPublicEvent(recorder, testLogger(), response, request, identity)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	input := recorder.RecordedInputs()[0]
	if input.GameID != identity.GameID || input.GameVersionID != identity.GameVersionID || input.ShareID != identity.ShareID || input.PlaySessionID != identity.PlaySessionID {
		t.Fatalf("public trusted links = %#v", input)
	}
	if input.Source != SourceFrontend || input.ActorType != ActorReceiver {
		t.Fatalf("public source/actor = %#v", input)
	}

	request = httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(`{
		"eventName":"creator.page_viewed",
		"clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2",
		"properties":{"page":"games"}
	}`))
	response = httptest.NewRecorder()
	RecordPublicEvent(recorder, testLogger(), response, request, identity)
	if response.Code != http.StatusUnprocessableEntity || len(recorder.RecordedInputs()) != 1 {
		t.Fatalf("wrong event response = %d %s", response.Code, response.Body.String())
	}
}

func TestAdminQueryPassesValidatedFiltersAndOmitsClientEventID(t *testing.T) {
	now := time.Date(2026, 8, 16, 2, 35, 1, 123456000, time.UTC)
	loginID := "creator_01"
	clientEventID := "2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2"
	repository := &handlerRepository{page: EventPage{Items: []Event{{
		ID: testEventID, EventName: EventCreatorPageViewed, Source: SourceFrontend, ActorType: ActorCreator,
		LoginID: &loginID, ClientEventID: &clientEventID, Properties: json.RawMessage(`{"page":"games"}`), CreatedAt: now,
	}}, NextCursor: &Cursor{Version: 1, CreatedAt: now, ID: testEventID}}}
	router := adminRouter(repository, allowMiddleware)
	cursor, err := EncodeCursor(Cursor{Version: 1, CreatedAt: now.Add(time.Minute), ID: "01K00000000000000000000008"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet,
		"/admin/behavior-events?eventName=creator.page_viewed&creatorId="+testCreatorID+
			"&loginId=Creator_01&gameId=01K00000000000000000000004&source=frontend"+
			"&from=2026-08-15T00:00:00Z&to=2026-08-17T00:00:00Z&limit=25&cursor="+url.QueryEscape(cursor), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	filter := repository.listFilter
	if filter.EventName != EventCreatorPageViewed || filter.CreatorID != testCreatorID || filter.LoginID != "creator_01" || filter.Limit != 25 || filter.From == nil || filter.To == nil || filter.Cursor == nil || filter.Cursor.ID != "01K00000000000000000000008" {
		t.Fatalf("filter = %#v", filter)
	}
	if strings.Contains(response.Body.String(), "clientEventId") || !strings.Contains(response.Body.String(), `"loginId":"creator_01"`) {
		t.Fatalf("admin DTO boundary violated: %s", response.Body.String())
	}
}

func TestAdminQueryRequiresAdminAndRejectsInvalidFilters(t *testing.T) {
	repository := &handlerRepository{}
	unauthorized := adminRouter(repository, denyMiddleware(http.StatusUnauthorized))
	response := httptest.NewRecorder()
	unauthorized.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/behavior-events", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("creator/admin boundary status = %d", response.Code)
	}

	router := adminRouter(repository, allowMiddleware)
	for _, query := range []string{
		"?limit=101", "?source=browser", "?from=not-time", "?unknown=value", "?limit=1&limit=2",
		"?creatorId=bad", "?gameId=bad", "?loginId=ab", "?eventName=unknown.event", "?cursor=not-base64",
		"?from=2026-08-17T00:00:00Z&to=2026-08-16T00:00:00Z",
	} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/behavior-events"+query, nil))
		if response.Code != http.StatusUnprocessableEntity {
			t.Errorf("query %q status = %d, body = %s", query, response.Code, response.Body.String())
		}
	}
}

func TestAdminQueryAppliesDefaultAndMaximumLimit(t *testing.T) {
	repository := &handlerRepository{}
	router := adminRouter(repository, allowMiddleware)
	for _, test := range []struct {
		query string
		want  int
	}{{"", 50}, {"?limit=100", 100}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/behavior-events"+test.query, nil))
		if response.Code != http.StatusOK || repository.listFilter.Limit != test.want {
			t.Errorf("query %q = status %d limit %d, want %d", test.query, response.Code, repository.listFilter.Limit, test.want)
		}
	}
}

func creatorRouter(repository interface {
	Recorder
	AdminEventLister
}, session, csrf func(http.Handler) http.Handler, appOrigin string) http.Handler {
	handler := NewHandler(repository, config.Config{App: config.AppConfig{Environment: "production", AppBaseURL: appOrigin}}, testLogger())
	router := chi.NewRouter()
	handler.MountApp(router, session, csrf, allowMiddleware, func(*http.Request) (string, string) {
		return testCreatorID, testSessionID
	})
	return router
}

func adminRouter(repository interface {
	Recorder
	AdminEventLister
}, admin func(http.Handler) http.Handler) http.Handler {
	handler := NewHandler(repository, config.Config{App: config.AppConfig{Environment: "test", AppBaseURL: "https://app.example"}}, testLogger())
	router := chi.NewRouter()
	handler.MountApp(router, allowMiddleware, allowMiddleware, admin, func(*http.Request) (string, string) { return "", "" })
	return router
}

func allowMiddleware(next http.Handler) http.Handler { return next }

func denyMiddleware(status int) func(http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	}
}

func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-CSRF-Token") != "valid" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func performJSON(handler http.Handler, method, path, origin, body string, csrf bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	if csrf {
		request.Header.Set("X-CSRF-Token", "valid")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAdminDTOCannotRegressToImplicitEventSerialization(t *testing.T) {
	clientID := "secret-client-id"
	encoded, err := json.Marshal(newAdminEventDTO(Event{ClientEventID: &clientID, Properties: json.RawMessage(`{}`)}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("clientEvent")) {
		t.Fatalf("admin DTO leaked client event id: %s", encoded)
	}
}

func TestRecorderFailureReturnsServerErrorWithoutResponseLeak(t *testing.T) {
	repository := &handlerRepository{recordErr: errors.New("database object_key secret")}
	router := creatorRouter(repository, allowMiddleware, csrfMiddleware, "https://app.example")
	response := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example",
		`{"eventName":"creator.page_viewed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"page":"games"}}`, true)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "object_key") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRecorderFailureLogUsesStableCodeWithoutDatabaseDetails(t *testing.T) {
	repository := &handlerRepository{recordErr: errors.New("database secret object_key")}
	var logs bytes.Buffer
	request := httptest.NewRequest(http.MethodPost, "/analytics/events", nil)
	response := httptest.NewRecorder()
	recordFrontend(repository, slog.New(slog.NewJSONHandler(&logs, nil)), response, request, RecordInput{
		EventName: EventCreatorPageViewed, Source: SourceFrontend, ActorType: ActorCreator,
		CreatorID: testCreatorID, UserSessionID: testSessionID,
		ClientEventID: "2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2", Properties: json.RawMessage(`{"page":"games"}`),
	})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(logs.String(), "object_key") || !strings.Contains(logs.String(), "ANALYTICS_RECORD_FAILED") {
		t.Fatalf("unsafe analytics log: %s", logs.String())
	}
}

func TestDuplicatePropertyKeyCanaryIsNotReflectedByCreatorOrPublicHandlers(t *testing.T) {
	const canary = "SENSITIVE_SECRET_URL_LOGIN_ID_USER_TEXT_CANARY"
	creatorBody := `{"eventName":"creator.page_viewed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"page":"games","` + canary + `":true,"` + canary + `":false}}`
	publicBody := `{"eventName":"play.completed","clientEventId":"2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2","properties":{"mode":"public","` + canary + `":true,"` + canary + `":false}}`

	t.Run("creator", func(t *testing.T) {
		repository := &handlerRepository{record: RecordResult{Event: Event{ID: testEventID}}}
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		handler := NewHandler(repository, config.Config{App: config.AppConfig{Environment: "production", AppBaseURL: "https://app.example"}}, logger)
		router := chi.NewRouter()
		handler.MountApp(router, allowMiddleware, csrfMiddleware, allowMiddleware, func(*http.Request) (string, string) {
			return testCreatorID, testSessionID
		})
		response := performJSON(router, http.MethodPost, "/analytics/events", "https://app.example", creatorBody, true)
		assertSensitiveCanaryRejectedWithoutReflection(t, response, logs.String(), canary)
		if len(repository.inputs) != 0 {
			t.Fatalf("creator recorder received %d invalid events", len(repository.inputs))
		}
	})

	t.Run("public", func(t *testing.T) {
		recorder := NewFakeRecorder(RecordResult{Event: Event{ID: testEventID}}, nil)
		var logs bytes.Buffer
		request := httptest.NewRequest(http.MethodPost, "/public/play-sessions/current/events", strings.NewReader(publicBody))
		response := httptest.NewRecorder()
		RecordPublicEvent(recorder, slog.New(slog.NewJSONHandler(&logs, nil)), response, request, PublicIdentity{
			GameID: "01K00000000000000000000004", GameVersionID: "01K00000000000000000000005",
			ShareID: "01K00000000000000000000006", PlaySessionID: "01K00000000000000000000007",
		})
		assertSensitiveCanaryRejectedWithoutReflection(t, response, logs.String(), canary)
		if len(recorder.RecordedInputs()) != 0 {
			t.Fatalf("public recorder received %d invalid events", len(recorder.RecordedInputs()))
		}
	})
}

func assertSensitiveCanaryRejectedWithoutReflection(t *testing.T, response *httptest.ResponseRecorder, logs, canary string) {
	t.Helper()
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), canary) {
		t.Fatalf("response reflected sensitive canary: %s", response.Body.String())
	}
	if strings.Contains(logs, canary) {
		t.Fatalf("logs reflected sensitive canary: %s", logs)
	}
	if !strings.Contains(response.Body.String(), `"properties":"contains a duplicate key"`) {
		t.Fatalf("response does not use fixed duplicate-key error: %s", response.Body.String())
	}
}
