package invitations

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gamegen/backend/internal/platform/httpapi"
	"gamegen/backend/internal/platform/security"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	defaultListLimit = 50
	maximumListLimit = 100
	generateAttempts = 5
)

type invitationStore interface {
	Create(context.Context, Invitation, []byte, string, string, string) error
	List(context.Context, int) ([]Invitation, error)
	Revoke(context.Context, string, string, string, string, string, time.Time) (Invitation, error)
}

type AdminIdentityFunc func(*http.Request) (sessionID, username string)

type Handler struct {
	store         invitationStore
	adminIdentity AdminIdentityFunc
	logger        *slog.Logger
	now           func() time.Time
	generateCode  func() (string, error)
}

func NewHandler(store invitationStore, adminIdentity AdminIdentityFunc, logger *slog.Logger) *Handler {
	return &Handler{store: store, adminIdentity: adminIdentity, logger: logger, now: time.Now, generateCode: GenerateCode}
}

func (handler *Handler) Mount(
	router chi.Router,
	requireAdminSession func(http.Handler) http.Handler,
	requireAdminMutation func(http.Handler) http.Handler,
) {
	router.Group(func(router chi.Router) {
		router.Use(requireAdminSession)
		router.Get("/admin/invitation-codes", handler.list)
		router.Group(func(router chi.Router) {
			router.Use(requireAdminMutation)
			router.Post("/admin/invitation-codes", handler.create)
			router.Delete("/admin/invitation-codes/{invitationId}", handler.revoke)
		})
	})
}

func (handler *Handler) create(w http.ResponseWriter, request *http.Request) {
	sessionID, username := handler.adminIdentity(request)
	for attempt := 0; attempt < generateAttempts; attempt++ {
		code, err := handler.generateCode()
		if err != nil {
			handler.internalError(w, request, "generate invitation code", err)
			return
		}
		invitationID, err := security.NewID()
		if err != nil {
			handler.internalError(w, request, "generate invitation id", err)
			return
		}
		auditID, err := security.NewID()
		if err != nil {
			handler.internalError(w, request, "generate invitation audit id", err)
			return
		}
		now := handler.now().UTC()
		invitation := Invitation{
			ID: invitationID, CodeSuffix: CodeSuffix(code), CreatedByAdmin: username, CreatedAt: now,
		}
		hash := HashCode(code)
		err = handler.store.Create(
			request.Context(), invitation, hash[:], sessionID,
			middleware.GetReqID(request.Context()), auditID,
		)
		if errors.Is(err, ErrCodeCollision) {
			continue
		}
		if err != nil {
			handler.internalError(w, request, "create invitation", err)
			return
		}
		data := invitationDTO(invitation)
		data["code"] = code
		httpapi.WriteData(w, request, http.StatusCreated, data)
		return
	}
	handler.internalError(w, request, "create unique invitation", ErrCodeCollision)
}

func (handler *Handler) list(w http.ResponseWriter, request *http.Request) {
	limit := defaultListLimit
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maximumListLimit {
			httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_LIMIT", "列表数量必须在 1–100 之间", nil)
			return
		}
		limit = parsed
	}
	items, err := handler.store.List(request.Context(), limit)
	if err != nil {
		handler.internalError(w, request, "list invitations", err)
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, invitation := range items {
		result = append(result, invitationDTO(invitation))
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": result})
}

func (handler *Handler) revoke(w http.ResponseWriter, request *http.Request) {
	sessionID, username := handler.adminIdentity(request)
	auditID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate invitation audit id", err)
		return
	}
	invitation, err := handler.store.Revoke(
		request.Context(), chi.URLParam(request, "invitationId"), sessionID, username,
		middleware.GetReqID(request.Context()), auditID, handler.now().UTC(),
	)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, request, http.StatusNotFound, "INVITATION_NOT_FOUND", "邀请码不存在", nil)
		return
	}
	if errors.Is(err, ErrNotRevocable) {
		httpapi.WriteError(w, request, http.StatusConflict, "INVITATION_NOT_REVOCABLE", "邀请码已经使用或撤销", nil)
		return
	}
	if err != nil {
		handler.internalError(w, request, "revoke invitation", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, invitationDTO(invitation))
}

func invitationDTO(invitation Invitation) map[string]any {
	return map[string]any{
		"id":              invitation.ID,
		"codeHint":        "••••-" + invitation.CodeSuffix,
		"status":          invitation.Status(),
		"createdByAdmin":  invitation.CreatedByAdmin,
		"usedByCreatorId": nullableString(invitation.UsedByCreatorID),
		"usedByLoginId":   nullableString(invitation.UsedByLoginID),
		"usedAt":          nullableTime(invitation.UsedAt),
		"revokedAt":       nullableTime(invitation.RevokedAt),
		"createdAt":       invitation.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	handler.logger.Error(operation, "request_id", middleware.GetReqID(request.Context()), "error", err)
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}
