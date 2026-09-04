package generation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"
	"gamegen/backend/internal/platform/httpapi"
	"gamegen/backend/internal/platform/security"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	repository       *Repository
	auth             *auth.Handler
	analytics        analytics.Recorder
	creatorUser      func(*http.Request) auth.User
	submitGeneration func(context.Context, string, string, string, string, string, []byte, time.Time) (Run, bool, error)
	logger           *slog.Logger
	now              func() time.Time
}

func NewHandler(repository *Repository, authHandler *auth.Handler, recorder analytics.Recorder, logger *slog.Logger) *Handler {
	return &Handler{
		repository: repository, auth: authHandler, analytics: recorder,
		creatorUser: auth.CreatorUser, submitGeneration: repository.Submit,
		logger: logger, now: time.Now,
	}
}

func (handler *Handler) Mount(router chi.Router) {
	router.Group(func(router chi.Router) {
		router.Use(handler.auth.RequireCreatorSession)
		router.Get("/games/{gameId}/generation-runs", handler.listRuns)
		router.Get("/games/{gameId}/generation-runs/{runId}", handler.getRun)
		router.Group(func(router chi.Router) {
			router.Use(handler.auth.RequireCreatorMutation)
			router.Post("/games/{gameId}/generation-runs", handler.submitRun)
			router.Post("/games/{gameId}/generation-runs/{runId}/cancel", handler.cancelRun)
		})
	})

	router.Group(func(router chi.Router) {
		router.Use(handler.auth.RequireAdminSession)
		router.Get("/admin/generation-runs", handler.listAdminRuns)
		router.Get("/admin/generation-runs/{runId}", handler.getAdminRun)
	})
}

type submitRequest struct {
	VersionID string `json:"versionId"`
}

func (handler *Handler) submitRun(w http.ResponseWriter, request *http.Request) {
	user := handler.creatorUser(request)
	var body submitRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil || strings.TrimSpace(body.VersionID) == "" {
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_REQUEST", "请选择要创建的游戏版本", nil)
		return
	}
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		httpapi.WriteError(w, request, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "提交创建时必须提供有效的 Idempotency-Key", nil)
		return
	}
	keyHash := sha256.Sum256([]byte(key))
	runID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate run id", err)
		return
	}
	traceID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate run trace id", err)
		return
	}
	run, reused, err := handler.submitGeneration(
		request.Context(), runID, traceID, user.ID, chi.URLParam(request, "gameId"), body.VersionID, keyHash[:], handler.now().UTC(),
	)
	if handler.writeError(w, request, err) {
		return
	}
	status := http.StatusCreated
	if reused {
		status = http.StatusOK
	}
	properties, _ := json.Marshal(map[string]any{"attemptNumber": run.AttemptNumber, "deduplicated": reused})
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventGenerationSubmitted, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: user.ID, GameID: run.GameID, GameVersionID: run.GameVersionID, GenerationRunID: run.ID,
		Properties: properties,
	})
	httpapi.WriteData(w, request, status, runDTO(run, false))
}

func (handler *Handler) recordEvent(request *http.Request, input analytics.RecordInput) {
	input.RequestID = middleware.GetReqID(request.Context())
	recorder := handler.analytics
	if recorder == nil {
		recorder = analytics.NoopRecorder{}
	}
	if _, err := recorder.RecordEvent(request.Context(), input); err != nil {
		handler.logger.Warn("analytics event recording failed",
			"event_name", input.EventName, "source", input.Source,
			"error_code", "ANALYTICS_WRITE_FAILED", "request_id", input.RequestID,
		)
	}
}

func (handler *Handler) listRuns(w http.ResponseWriter, request *http.Request) {
	runs, err := handler.repository.List(request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"))
	if handler.writeError(w, request, err) {
		return
	}
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		items = append(items, runDTO(run, false))
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": items})
}

func (handler *Handler) getRun(w http.ResponseWriter, request *http.Request) {
	run, err := handler.repository.Get(
		request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"), chi.URLParam(request, "runId"),
	)
	if handler.writeError(w, request, err) {
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, runDTO(run, false))
}

func (handler *Handler) cancelRun(w http.ResponseWriter, request *http.Request) {
	run, err := handler.repository.RequestCancel(
		request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"), chi.URLParam(request, "runId"), handler.now().UTC(),
	)
	if handler.writeError(w, request, err) {
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, runDTO(run, false))
}

func (handler *Handler) listAdminRuns(w http.ResponseWriter, request *http.Request) {
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	if status != "" && !validRunStatus(status) {
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_STATUS_FILTER", "任务状态筛选值无效", nil)
		return
	}
	runs, err := handler.repository.ListAdmin(request.Context(), status)
	if handler.writeError(w, request, err) {
		return
	}
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		items = append(items, runDTO(run, true))
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": items})
}

func (handler *Handler) getAdminRun(w http.ResponseWriter, request *http.Request) {
	run, err := handler.repository.GetAdmin(request.Context(), chi.URLParam(request, "runId"))
	if handler.writeError(w, request, err) {
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, runDTO(run, true))
}

func runDTO(run Run, admin bool) map[string]any {
	data := map[string]any{
		"id": run.ID, "gameId": run.GameID, "gameVersionId": run.GameVersionID,
		"attemptNumber": run.AttemptNumber, "triggerType": run.TriggerType,
		"status": run.Status, "stage": run.Stage, "progress": run.Progress,
		"errorCode": nullableString(run.ErrorCode), "errorMessage": userErrorMessage(run),
		"retryable": run.Retryable, "cancelRequested": run.CancelRequestedAt.Valid,
		"createdAt": run.CreatedAt.UTC().Format(time.RFC3339Nano), "updatedAt": run.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"startedAt": nullableTime(run.StartedAt), "completedAt": nullableTime(run.CompletedAt),
	}
	if admin {
		var details any
		if len(run.SanitizedDetails) > 0 {
			_ = json.Unmarshal(run.SanitizedDetails, &details)
		}
		data["executionCount"] = run.ExecutionCount
		data["traceId"] = run.TraceID
		data["adminMessage"] = nullableString(run.AdminMessage)
		data["sanitizedDetails"] = details
	}
	return data
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

func userErrorMessage(run Run) any {
	code := run.ErrorCode.String
	if code == "INPUT_VALIDATION_FAILED" && len(run.SanitizedDetails) > 0 {
		var details struct {
			ErrorType string `json:"errorType"`
		}
		if json.Unmarshal(run.SanitizedDetails, &details) == nil && details.ErrorType == "image_input_limit" {
			return "照片处理后的文件超过生成上限，请压缩图片或缩小尺寸后重新上传"
		}
	}
	messages := map[string]string{
		"INPUT_VALIDATION_FAILED": "游戏输入不完整，请检查后重试",
		"PROVIDER_UNAVAILABLE":    "创建服务暂时不可用，可以稍后重试",
		"STORAGE_WRITE_FAILED":    "创建结果暂时无法保存，可以稍后重试",
		"TASK_LEASE_EXHAUSTED":    "任务多次中断，请重新发起创建",
		"INTERNAL_ERROR":          "创建失败，可以稍后重试",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return nil
}

func validRunStatus(status string) bool {
	return status == "queued" || status == "running" || status == "succeeded" || status == "failed" || status == "cancelled"
}

func (handler *Handler) writeError(w http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		httpapi.WriteError(w, request, http.StatusNotFound, "GENERATION_RUN_NOT_FOUND", "创建记录不存在", nil)
	case errors.Is(err, ErrAssetsRequired):
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "ASSETS_REQUIRED", "请至少上传一张图片后再提交创建", nil)
	case errors.Is(err, ErrMaterialsIncomplete):
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "MATERIALS_INCOMPLETE", "必填照片尚未上传完整", nil)
	case errors.Is(err, ErrGenerationDisabled):
		httpapi.WriteError(w, request, http.StatusConflict, "GENERATION_NOT_AVAILABLE", "这个模板的生成管线正在开发中", nil)
	case errors.Is(err, ErrVersionNotReady):
		httpapi.WriteError(w, request, http.StatusConflict, "VERSION_NOT_SUBMITTABLE", "当前版本不能提交创建", nil)
	case errors.Is(err, ErrActiveRun):
		httpapi.WriteError(w, request, http.StatusConflict, "GENERATION_ALREADY_ACTIVE", "当前版本已有正在进行的创建任务", nil)
	case errors.Is(err, ErrIdempotencyConflict):
		httpapi.WriteError(w, request, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "该幂等键已用于其他创建请求", nil)
	default:
		handler.internalError(w, request, "generation repository operation", err)
	}
	return true
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	handler.logger.Error(operation, "error", err, "request_id", middleware.GetReqID(request.Context()))
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}
