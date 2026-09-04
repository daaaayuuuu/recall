package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/httpapi"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type AdminEventLister interface {
	ListAdminEvents(context.Context, ListFilter) (EventPage, error)
}

type Handler struct {
	recorder   Recorder
	lister     AdminEventLister
	logger     *slog.Logger
	appOrigins map[string]struct{}
}

type CreatorIdentityFunc func(*http.Request) (creatorID, userSessionID string)

type PublicIdentity struct {
	GameID        string
	GameVersionID string
	ShareID       string
	PlaySessionID string
}

func NewHandler(repository interface {
	Recorder
	AdminEventLister
}, cfg config.Config, logger *slog.Logger) *Handler {
	origins := make(map[string]struct{})
	if origin, err := normalizedOrigin(cfg.App.AppBaseURL); err == nil {
		origins[origin] = struct{}{}
	}
	if cfg.App.Environment == "development" {
		origins["http://127.0.0.1:5173"] = struct{}{}
		origins["http://localhost:5173"] = struct{}{}
	}
	return &Handler{recorder: repository, lister: repository, logger: logger, appOrigins: origins}
}

func (handler *Handler) MountApp(
	router chi.Router,
	requireCreatorSession func(http.Handler) http.Handler,
	requireCreatorMutation func(http.Handler) http.Handler,
	requireAdminSession func(http.Handler) http.Handler,
	creatorIdentity CreatorIdentityFunc,
) {
	router.Group(func(router chi.Router) {
		router.Use(requireCreatorSession)
		router.Use(handler.verifyAppOrigin)
		router.Use(requireCreatorMutation)
		router.Post("/analytics/events", func(w http.ResponseWriter, request *http.Request) {
			creatorID, sessionID := creatorIdentity(request)
			handler.recordCreatorEvent(w, request, creatorID, sessionID)
		})
	})
	router.Group(func(router chi.Router) {
		router.Use(requireAdminSession)
		router.Get("/admin/behavior-events", handler.listAdminEvents)
	})
}

type frontendEventRequest struct {
	EventName     EventName       `json:"eventName"`
	ClientEventID string          `json:"clientEventId"`
	OccurredAt    json.RawMessage `json:"occurredAt"`
	Properties    json.RawMessage `json:"properties"`
}

func (handler *Handler) recordCreatorEvent(w http.ResponseWriter, request *http.Request, creatorID, sessionID string) {
	body, occurredAt, ok := decodeFrontendEvent(w, request)
	if !ok {
		return
	}
	if body.EventName != EventCreatorPageViewed {
		writeValidationError(w, request, &ValidationError{Field: "eventName", Message: "is not accepted by this endpoint"})
		return
	}
	handler.recordFrontend(w, request, RecordInput{
		EventName: body.EventName, Source: SourceFrontend, ActorType: ActorCreator,
		CreatorID: creatorID, UserSessionID: sessionID, RequestID: middleware.GetReqID(request.Context()),
		ClientEventID: body.ClientEventID, Properties: body.Properties, OccurredAt: occurredAt,
	})
}

// RecordPublicEvent handles a receiver event after Sharing has authenticated the
// current play-session cookie and derived every business association from it.
func RecordPublicEvent(recorder Recorder, logger *slog.Logger, w http.ResponseWriter, request *http.Request, identity PublicIdentity) {
	body, occurredAt, ok := decodeFrontendEvent(w, request)
	if !ok {
		return
	}
	if body.EventName != EventPlayCompleted && body.EventName != EventPlayReplayed {
		writeValidationError(w, request, &ValidationError{Field: "eventName", Message: "is not accepted by this endpoint"})
		return
	}
	recordFrontend(recorder, logger, w, request, RecordInput{
		EventName: body.EventName, Source: SourceFrontend, ActorType: ActorReceiver,
		GameID: identity.GameID, GameVersionID: identity.GameVersionID,
		ShareID: identity.ShareID, PlaySessionID: identity.PlaySessionID,
		RequestID: middleware.GetReqID(request.Context()), ClientEventID: body.ClientEventID,
		Properties: body.Properties, OccurredAt: occurredAt,
	})
}

func (handler *Handler) recordFrontend(w http.ResponseWriter, request *http.Request, input RecordInput) {
	recordFrontend(handler.recorder, handler.logger, w, request, input)
}

func recordFrontend(recorder Recorder, logger *slog.Logger, w http.ResponseWriter, request *http.Request, input RecordInput) {
	properties, err := ValidateRecordInput(input)
	if err != nil {
		writeRecordError(logger, w, request, err)
		return
	}
	input.Properties = properties
	result, err := recorder.RecordEvent(request.Context(), input)
	if err != nil {
		writeRecordError(logger, w, request, err)
		return
	}
	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	httpapi.WriteData(w, request, status, map[string]any{"eventId": result.Event.ID, "duplicate": result.Duplicate})
}

func decodeFrontendEvent(w http.ResponseWriter, request *http.Request) (frontendEventRequest, *time.Time, bool) {
	var body frontendEventRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_REQUEST", "请求内容不是有效的 JSON", nil)
		return frontendEventRequest{}, nil, false
	}
	var occurredAt *time.Time
	if len(body.OccurredAt) > 0 {
		var occurredAtText string
		if string(body.OccurredAt) == "null" || json.Unmarshal(body.OccurredAt, &occurredAtText) != nil {
			writeValidationError(w, request, &ValidationError{Field: "occurredAt", Message: "must be an RFC 3339 timestamp"})
			return frontendEventRequest{}, nil, false
		}
		parsed, err := time.Parse(time.RFC3339Nano, occurredAtText)
		if err != nil {
			writeValidationError(w, request, &ValidationError{Field: "occurredAt", Message: "must be an RFC 3339 timestamp"})
			return frontendEventRequest{}, nil, false
		}
		parsed = parsed.UTC()
		occurredAt = &parsed
	}
	return body, occurredAt, true
}

func (handler *Handler) listAdminEvents(w http.ResponseWriter, request *http.Request) {
	filter, err := listFilterFromRequest(request)
	if err != nil {
		writeValidationError(w, request, err)
		return
	}
	page, err := handler.lister.ListAdminEvents(request.Context(), filter)
	if err != nil {
		var validation *ValidationError
		if errors.As(err, &validation) {
			writeValidationError(w, request, validation)
			return
		}
		handler.internalError(w, request, "list behavior events", err)
		return
	}
	items := make([]adminEventDTO, 0, len(page.Items))
	for _, event := range page.Items {
		items = append(items, newAdminEventDTO(event))
	}
	var nextCursor *string
	if page.NextCursor != nil {
		encoded, err := EncodeCursor(*page.NextCursor)
		if err != nil {
			handler.internalError(w, request, "encode behavior event cursor", err)
			return
		}
		nextCursor = &encoded
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": items, "nextCursor": nextCursor})
}

func listFilterFromRequest(request *http.Request) (ListFilter, error) {
	allowed := map[string]struct{}{
		"eventName": {}, "creatorId": {}, "loginId": {}, "gameId": {}, "source": {},
		"from": {}, "to": {}, "cursor": {}, "limit": {},
	}
	query := request.URL.Query()
	for name, values := range query {
		if _, ok := allowed[name]; !ok {
			return ListFilter{}, &ValidationError{Field: name, Message: "is not a supported filter"}
		}
		if len(values) != 1 {
			return ListFilter{}, &ValidationError{Field: name, Message: "must be provided once"}
		}
	}
	filter := ListFilter{
		EventName: EventName(query.Get("eventName")), CreatorID: query.Get("creatorId"),
		LoginID: query.Get("loginId"), GameID: query.Get("gameId"), Source: Source(query.Get("source")),
	}
	var err error
	if raw, exists := singleQueryValue(query, "from"); exists {
		filter.From, err = parseQueryTime("from", raw)
		if err != nil {
			return ListFilter{}, err
		}
	}
	if raw, exists := singleQueryValue(query, "to"); exists {
		filter.To, err = parseQueryTime("to", raw)
		if err != nil {
			return ListFilter{}, err
		}
	}
	if raw, exists := singleQueryValue(query, "cursor"); exists {
		cursor, err := DecodeCursor(raw)
		if err != nil {
			return ListFilter{}, err
		}
		filter.Cursor = &cursor
	}
	if raw, exists := singleQueryValue(query, "limit"); exists {
		filter.Limit, err = strconv.Atoi(raw)
		if err != nil {
			return ListFilter{}, &ValidationError{Field: "limit", Message: "must be an integer"}
		}
	}
	return ValidateListFilter(filter)
}

func singleQueryValue(values url.Values, name string) (string, bool) {
	items, exists := values[name]
	return values.Get(name), exists && len(items) == 1
}

func parseQueryTime(field, value string) (*time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, &ValidationError{Field: field, Message: "must be an RFC 3339 timestamp"}
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

type adminEventDTO struct {
	ID              string          `json:"id"`
	EventName       EventName       `json:"eventName"`
	Source          Source          `json:"source"`
	ActorType       ActorType       `json:"actorType"`
	CreatorID       *string         `json:"creatorId"`
	LoginID         *string         `json:"loginId"`
	UserSessionID   *string         `json:"userSessionId"`
	GameID          *string         `json:"gameId"`
	GameVersionID   *string         `json:"gameVersionId"`
	GenerationRunID *string         `json:"generationRunId"`
	ShareID         *string         `json:"shareId"`
	PlaySessionID   *string         `json:"playSessionId"`
	RequestID       *string         `json:"requestId"`
	Properties      json.RawMessage `json:"properties"`
	OccurredAt      *time.Time      `json:"occurredAt"`
	CreatedAt       time.Time       `json:"createdAt"`
}

func newAdminEventDTO(event Event) adminEventDTO {
	return adminEventDTO{
		ID: event.ID, EventName: event.EventName, Source: event.Source, ActorType: event.ActorType,
		CreatorID: event.CreatorID, LoginID: event.LoginID, UserSessionID: event.UserSessionID,
		GameID: event.GameID, GameVersionID: event.GameVersionID, GenerationRunID: event.GenerationRunID,
		ShareID: event.ShareID, PlaySessionID: event.PlaySessionID, RequestID: event.RequestID,
		Properties: event.Properties, OccurredAt: event.OccurredAt, CreatedAt: event.CreatedAt,
	}
}

func (handler *Handler) verifyAppOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		origin, err := normalizedOrigin(request.Header.Get("Origin"))
		if err != nil {
			httpapi.WriteError(w, request, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "请求来源不受信任", nil)
			return
		}
		if _, allowed := handler.appOrigins[origin]; !allowed {
			httpapi.WriteError(w, request, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "请求来源不受信任", nil)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func normalizedOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid origin")
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), nil
}

func writeRecordError(logger *slog.Logger, w http.ResponseWriter, request *http.Request, err error) {
	var validation *ValidationError
	switch {
	case errors.Is(err, ErrClientEventIDConflict):
		httpapi.WriteError(w, request, http.StatusConflict, "CLIENT_EVENT_ID_CONFLICT", "客户端事件标识已用于其他事件", nil)
	case errors.As(err, &validation):
		writeValidationError(w, request, validation)
	default:
		if logger != nil {
			logger.Error("record behavior event", "error_code", "ANALYTICS_RECORD_FAILED", "request_id", middleware.GetReqID(request.Context()))
		}
		httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
	}
}

func writeValidationError(w http.ResponseWriter, request *http.Request, err error) {
	field := "request"
	message := err.Error()
	var validation *ValidationError
	if errors.As(err, &validation) {
		if validation.Field != "" {
			field = validation.Field
		}
		message = validation.Message
	}
	httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "ANALYTICS_VALIDATION_FAILED", "行为事件参数无效", map[string]string{field: message})
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	if handler.logger != nil {
		handler.logger.Error(operation, "error", err, "request_id", middleware.GetReqID(request.Context()))
	}
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}
