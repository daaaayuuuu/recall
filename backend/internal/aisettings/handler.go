package aisettings

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gamegen/backend/internal/imagegeneration"
	"gamegen/backend/internal/imagemoderation"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/httpapi"
	"gamegen/backend/internal/textai"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type AdminIdentityFunc func(*http.Request) (sessionID, username string)

type Handler struct {
	manager       *Manager
	adminIdentity AdminIdentityFunc
	logger        *slog.Logger
	now           func() time.Time
}

func NewHandler(manager *Manager, adminIdentity AdminIdentityFunc, logger *slog.Logger) *Handler {
	return &Handler{manager: manager, adminIdentity: adminIdentity, logger: logger, now: time.Now}
}

func (handler *Handler) Mount(
	router chi.Router,
	requireAdminSession func(http.Handler) http.Handler,
	requireAdminMutation func(http.Handler) http.Handler,
) {
	router.Group(func(router chi.Router) {
		router.Use(requireAdminSession)
		router.Get("/admin/ai-settings", handler.get)
		router.Group(func(router chi.Router) {
			router.Use(requireAdminMutation)
			router.Put("/admin/ai-settings", handler.update)
			router.Post("/admin/ai-settings/test", handler.testConnection)
		})
	})
}

type keyMutation struct {
	Value string `json:"value"`
	Clear bool   `json:"clear"`
}

type keyMutations struct {
	Text            keyMutation `json:"text"`
	ImageModeration keyMutation `json:"imageModeration"`
	ImageToImage    keyMutation `json:"imageToImage"`
}

type updateRequest struct {
	ExpectedVersion int64        `json:"expectedVersion"`
	Settings        Snapshot     `json:"settings"`
	APIKeys         keyMutations `json:"apiKeys"`
}

type testRequest struct {
	Capability string       `json:"capability"`
	Settings   Snapshot     `json:"settings"`
	APIKeys    keyMutations `json:"apiKeys"`
}

func (handler *Handler) get(w http.ResponseWriter, request *http.Request) {
	view, err := handler.manager.View(request.Context())
	if err != nil {
		handler.internalError(w, request, "load AI settings", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, view)
}

func (handler *Handler) update(w http.ResponseWriter, request *http.Request) {
	var body updateRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_AI_SETTINGS", "AI 配置请求不是有效的 JSON", nil)
		return
	}
	sessionID, username := handler.adminIdentity(request)
	view, fields, err := handler.manager.Publish(
		request.Context(), body.ExpectedVersion, body.Settings, changesFromRequest(body.APIKeys),
		sessionID, username, middleware.GetReqID(request.Context()),
	)
	if errors.Is(err, ErrDynamicDisabled) {
		httpapi.WriteError(w, request, http.StatusConflict, "DYNAMIC_AI_CONFIG_DISABLED", "动态 AI 配置尚未在部署环境中启用", nil)
		return
	}
	if errors.Is(err, ErrVersionConflict) {
		httpapi.WriteError(w, request, http.StatusConflict, "AI_SETTINGS_VERSION_CONFLICT", "AI 配置已被其他管理员更新，请刷新后重试", nil)
		return
	}
	if err != nil {
		handler.internalError(w, request, "publish AI settings", err)
		return
	}
	if len(fields) > 0 {
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "AI_SETTINGS_INVALID", "请检查 AI 配置字段", fields)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, view)
}

func (handler *Handler) testConnection(w http.ResponseWriter, request *http.Request) {
	if !handler.manager.DynamicEnabled() {
		httpapi.WriteError(w, request, http.StatusConflict, "DYNAMIC_AI_CONFIG_DISABLED", "动态 AI 配置尚未在部署环境中启用", nil)
		return
	}
	var body testRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_AI_SETTINGS", "AI 配置请求不是有效的 JSON", nil)
		return
	}
	body.Capability = strings.TrimSpace(body.Capability)
	switch body.Capability {
	case CapabilityText:
		body.Settings.Text.Enabled = true
	case CapabilityImageModeration:
		body.Settings.ImageModeration.Enabled = true
	case CapabilityImageToImage:
		body.Settings.ImageToImage.Enabled = true
	default:
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_AI_CAPABILITY", "请选择有效的 AI 能力", nil)
		return
	}
	configuration, fields, err := handler.manager.Preview(request.Context(), body.Settings, changesFromRequest(body.APIKeys))
	if err != nil {
		handler.internalError(w, request, "prepare AI connection test", err)
		return
	}
	if len(fields) > 0 {
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "AI_SETTINGS_INVALID", "请检查 AI 配置字段", fields)
		return
	}
	started := handler.now()
	if err := runConnectionTest(request.Context(), body.Capability, configuration); err != nil {
		handler.logger.Warn(
			"AI connection test failed", "capability", body.Capability,
			"request_id", middleware.GetReqID(request.Context()), "error", err,
		)
		httpapi.WriteError(w, request, http.StatusBadGateway, "AI_CONNECTION_TEST_FAILED", "AI 服务连接测试失败，请检查地址、模型和密钥", nil)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{
		"capability": body.Capability,
		"latencyMs":  handler.now().Sub(started).Milliseconds(),
	})
}

func runConnectionTest(ctx context.Context, capability string, configuration config.AIConfig) error {
	switch capability {
	case CapabilityText:
		polisher := textai.NewDeepSeekPolisher(configuration.Text)
		if !polisher.Configured() {
			return textai.ErrNotConfigured
		}
		_, err := polisher.PolishLoveLetter(ctx, "连接测试。", "admin-connection-test")
		return err
	case CapabilityImageModeration:
		reviewer := imagemoderation.New(configuration.ImageModeration)
		if !reviewer.Configured() {
			return imagemoderation.ErrNotConfigured
		}
		_, err := reviewer.Review(ctx, imagemoderation.Input{
			Image: bytes.NewReader(testPNG()), MIMEType: "image/png", Purpose: imagemoderation.PurposeGameAsset,
		})
		return err
	case CapabilityImageToImage:
		transformer, err := imagegeneration.New(configuration.ImageToImage)
		if err != nil {
			return err
		}
		_, err = transformer.Transform(ctx, imagegeneration.Input{
			Image: testPNG(), MIMEType: "image/png", Prompt: "Return this small neutral test image as a valid PNG.",
		})
		return err
	default:
		return errors.New("unsupported AI capability")
	}
}

func changesFromRequest(values keyMutations) SecretChanges {
	return SecretChanges{
		Text:            SecretChange{Value: strings.TrimSpace(values.Text.Value), Clear: values.Text.Clear},
		ImageModeration: SecretChange{Value: strings.TrimSpace(values.ImageModeration.Value), Clear: values.ImageModeration.Clear},
		ImageToImage:    SecretChange{Value: strings.TrimSpace(values.ImageToImage.Value), Clear: values.ImageToImage.Clear},
	}
}

func testPNG() []byte {
	value := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			value.Set(x, y, color.RGBA{R: 244, G: 222, B: 190, A: 255})
		}
	}
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, value)
	return encoded.Bytes()
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	handler.logger.Error(operation, "request_id", middleware.GetReqID(request.Context()), "error", err)
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}
