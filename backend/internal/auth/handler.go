package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/invitations"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/httpapi"
	"gamegen/backend/internal/platform/imageprocessing"
	"gamegen/backend/internal/platform/ratelimit"
	"gamegen/backend/internal/platform/security"
	"gamegen/backend/internal/platform/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	creatorSessionCookieProduction = "__Host-creator_session"
	creatorSessionCookieLocal      = "creator_session"
	adminSessionCookieProduction   = "__Host-admin_session"
	adminSessionCookieLocal        = "admin_session"
	userAssetsBucket               = "gamegen-user-assets"
	avatarMaximumDimension         = 512
)

type contextKey string

const (
	creatorSessionKey contextKey = "creator-session"
	adminSessionKey   contextKey = "admin-session"
)

type Handler struct {
	repository             *Repository
	storage                *storage.Client
	analytics              analytics.Recorder
	createUserWithInvite   func(context.Context, User, []byte) error
	findUserByLoginID      func(context.Context, string) (User, error)
	createUserSession      func(context.Context, string, string, []byte, []byte, time.Time, time.Time) error
	passwords              security.PasswordHasher
	logger                 *slog.Logger
	adminUsername          string
	adminPasswordHash      string
	adminCredentialHash    [sha256.Size]byte
	dummyPasswordHash      string
	creatorSessionCookie   string
	adminSessionCookie     string
	secureCookies          bool
	trustProxyHeaders      bool
	allowedMutationOrigins map[string]struct{}
	publicLimiter          *ratelimit.Limiter
	loginIdentityLimiter   *ratelimit.Limiter
	adminLimiter           *ratelimit.Limiter
	uploadBufferSize       int
	now                    func() time.Time
}

func NewHandler(repository *Repository, objectStorage *storage.Client, recorder analytics.Recorder, cfg config.Config, logger *slog.Logger) *Handler {
	secure := cfg.App.Environment == "production"
	creatorCookie := creatorSessionCookieLocal
	adminCookie := adminSessionCookieLocal
	if secure {
		creatorCookie = creatorSessionCookieProduction
		adminCookie = adminSessionCookieProduction
	}

	allowedOrigins := map[string]struct{}{}
	mutationURLs := []string{cfg.App.AppBaseURL}
	if cfg.App.Surface == "all" {
		mutationURLs = append(mutationURLs, cfg.App.PlayBaseURL)
	}
	for _, rawURL := range mutationURLs {
		if origin, err := normalizeOrigin(rawURL); err == nil {
			allowedOrigins[origin] = struct{}{}
		}
	}
	if cfg.App.Environment == "development" {
		allowedOrigins["http://localhost:5173"] = struct{}{}
		allowedOrigins["http://127.0.0.1:5173"] = struct{}{}
	}

	passwords := security.NewPasswordHasher()
	dummyPasswordHash, err := passwords.Hash("gamegen-dummy-password")
	if err != nil {
		panic(fmt.Sprintf("create dummy password hash: %v", err))
	}

	return &Handler{
		repository:             repository,
		storage:                objectStorage,
		analytics:              recorder,
		createUserWithInvite:   repository.CreateUserWithInvitation,
		findUserByLoginID:      repository.FindUserByLoginID,
		createUserSession:      repository.CreateUserSession,
		passwords:              passwords,
		logger:                 logger,
		adminUsername:          cfg.Admin.Username,
		adminPasswordHash:      cfg.Admin.PasswordHash,
		adminCredentialHash:    sha256.Sum256([]byte(cfg.Admin.PasswordHash)),
		dummyPasswordHash:      dummyPasswordHash,
		creatorSessionCookie:   creatorCookie,
		adminSessionCookie:     adminCookie,
		secureCookies:          secure,
		trustProxyHeaders:      cfg.HTTP.TrustProxyHeaders,
		allowedMutationOrigins: allowedOrigins,
		publicLimiter:          ratelimit.New(30, time.Minute),
		loginIdentityLimiter:   ratelimit.New(10, 5*time.Minute),
		adminLimiter:           ratelimit.New(5, 5*time.Minute),
		uploadBufferSize:       cfg.Uploads.StreamBufferBytes,
		now:                    time.Now,
	}
}

func (handler *Handler) Mount(router chi.Router) {
	router.Group(func(router chi.Router) {
		router.Use(handler.verifyMutationOrigin)
		router.Post("/auth/register", handler.register)
		router.Post("/auth/login", handler.login)
		router.Post("/admin/auth/login", handler.adminLogin)
	})

	router.Group(func(router chi.Router) {
		router.Use(handler.requireCreatorSession)
		router.Get("/auth/session", handler.creatorSession)

		router.Group(func(router chi.Router) {
			router.Use(handler.verifyMutationOrigin)
			router.Post("/auth/csrf", handler.creatorCSRF)
		})

		router.Group(func(router chi.Router) {
			router.Use(handler.verifyMutationOrigin)
			router.Use(handler.requireCreatorCSRF)
			router.Post("/auth/logout", handler.logout)
			router.Patch("/me", handler.updateMe)
			router.Post("/me/avatar", handler.uploadAvatar)
			router.Delete("/me/avatar", handler.deleteAvatar)
			router.Put("/me/password", handler.changePassword)
		})

		router.Get("/me", handler.me)
		router.Get("/me/avatar", handler.getAvatar)
	})

	router.Group(func(router chi.Router) {
		router.Use(handler.requireAdminSession)
		router.Get("/admin/auth/session", handler.adminSession)
		router.Group(func(router chi.Router) {
			router.Use(handler.verifyMutationOrigin)
			router.Post("/admin/auth/csrf", handler.adminCSRF)
		})
		router.Group(func(router chi.Router) {
			router.Use(handler.verifyMutationOrigin)
			router.Use(handler.requireAdminCSRF)
			router.Post("/admin/auth/logout", handler.adminLogout)
		})
	})
}

// RequireCreatorSession authenticates a creator and stores the session in the request context.
func (handler *Handler) RequireCreatorSession(next http.Handler) http.Handler {
	return handler.requireCreatorSession(next)
}

// RequireCreatorMutation verifies both the request origin and the session CSRF token.
// It must run inside RequireCreatorSession.
func (handler *Handler) RequireCreatorMutation(next http.Handler) http.Handler {
	return handler.verifyMutationOrigin(handler.requireCreatorCSRF(next))
}

// RequireAdminSession authenticates the configured administrator.
func (handler *Handler) RequireAdminSession(next http.Handler) http.Handler {
	return handler.requireAdminSession(next)
}

// RequireAdminMutation verifies both the request origin and the administrator
// CSRF token. It must run inside RequireAdminSession.
func (handler *Handler) RequireAdminMutation(next http.Handler) http.Handler {
	return handler.verifyMutationOrigin(handler.requireAdminCSRF(next))
}

// CreatorUser returns the authenticated creator stored by RequireCreatorSession.
func CreatorUser(request *http.Request) User {
	return creatorSessionFrom(request.Context()).User
}

// CreatorSessionID returns the opaque database identifier for the creator
// session authenticated by RequireCreatorSession. It never exposes the cookie
// token or its hash.
func CreatorSessionID(request *http.Request) string {
	return creatorSessionFrom(request.Context()).ID
}

// AdminIdentity returns the trusted administrator session identity populated by
// RequireAdminSession. It never exposes cookie or CSRF values.
func AdminIdentity(request *http.Request) (sessionID, username string) {
	session := adminSessionFrom(request.Context())
	return session.ID, session.Username
}

type registerRequest struct {
	InvitationCode string `json:"invitationCode"`
	UserID         string `json:"userId"`
	Password       string `json:"password"`
	Nickname       string `json:"nickname"`
}

func (handler *Handler) register(w http.ResponseWriter, request *http.Request) {
	if !handler.allowRequest(w, request, handler.publicLimiter, "register:"+handler.clientAddress(request)) {
		return
	}
	var body registerRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}

	loginID, err := normalizeUserID(body.UserID)
	if err != nil {
		validationError(w, request, map[string]string{"userId": "用户ID须为 3–32 位，以字母开头，仅使用小写字母、数字、下划线或连字符"})
		return
	}
	if err := security.ValidatePassword(body.Password); err != nil {
		validationError(w, request, map[string]string{"password": "密码长度应为 8–128 个字符"})
		return
	}
	normalizedInvitation, err := invitations.NormalizeCode(body.InvitationCode)
	if err != nil {
		invitationError(w, request)
		return
	}
	nickname, err := normalizeNickname(body.Nickname)
	if err != nil {
		validationError(w, request, map[string]string{"nickname": "昵称不能超过 64 个字符"})
		return
	}

	passwordHash, err := handler.passwords.Hash(body.Password)
	if err != nil {
		handler.internalError(w, request, "hash registration password", err)
		return
	}
	userID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate user id", err)
		return
	}
	now := handler.now().UTC()
	user := User{
		ID:           userID,
		LoginID:      loginID,
		PasswordHash: passwordHash,
		Nickname:     nickname,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	invitationHash := invitations.HashCode(normalizedInvitation)
	if err := handler.createUserWithInvite(request.Context(), user, invitationHash[:]); err != nil {
		if errors.Is(err, ErrUserIDExists) {
			httpapi.WriteError(w, request, http.StatusConflict, "USER_ID_ALREADY_REGISTERED", "该用户ID已被使用", nil)
			return
		}
		if errors.Is(err, invitations.ErrInvalidOrUsed) {
			invitationError(w, request)
			return
		}
		handler.internalError(w, request, "create user", err)
		return
	}
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventCreatorRegistered, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: user.ID, Properties: json.RawMessage(`{}`),
	})

	httpapi.WriteData(w, request, http.StatusCreated, map[string]any{
		"user":    userDTO(user),
		"message": "注册成功，请使用用户ID登录",
	})
}

func invitationError(w http.ResponseWriter, request *http.Request) {
	httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "INVITATION_CODE_INVALID_OR_USED", "邀请码无效或已被使用", map[string]string{
		"invitationCode": "请输入有效且未使用的邀请码",
	})
}

type loginRequest struct {
	UserID   string `json:"userId"`
	Password string `json:"password"`
}

func (handler *Handler) login(w http.ResponseWriter, request *http.Request) {
	if !handler.allowRequest(w, request, handler.publicLimiter, "login-ip:"+handler.clientAddress(request)) {
		return
	}
	var body loginRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	loginID, err := normalizeUserID(body.UserID)
	if err != nil || body.Password == "" {
		invalidCredentials(w, request)
		return
	}
	if !handler.allowRequest(w, request, handler.loginIdentityLimiter, "login-user-id:"+loginID) {
		return
	}

	user, err := handler.findUserByLoginID(request.Context(), loginID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		handler.internalError(w, request, "find login user", err)
		return
	}
	if errors.Is(err, ErrNotFound) {
		_, _ = handler.passwords.Verify(body.Password, handler.dummyPasswordHash)
		invalidCredentials(w, request)
		return
	}
	valid, err := handler.passwords.Verify(body.Password, user.PasswordHash)
	if err != nil {
		handler.internalError(w, request, "verify user password", err)
		return
	}
	if !valid || user.Status != "active" {
		invalidCredentials(w, request)
		return
	}

	sessionID, _ := security.NewID()
	token, tokenHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate user session token", err)
		return
	}
	_, csrfHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate initial csrf token", err)
		return
	}
	now := handler.now().UTC()
	expiresAt := now.Add(30 * 24 * time.Hour)
	if err := handler.createUserSession(request.Context(), sessionID, user.ID, tokenHash[:], csrfHash[:], expiresAt, now); err != nil {
		handler.internalError(w, request, "create user session", err)
		return
	}
	handler.setSessionCookie(w, handler.creatorSessionCookie, token, expiresAt, http.SameSiteLaxMode)
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventCreatorLoggedIn, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: user.ID, UserSessionID: sessionID, Properties: json.RawMessage(`{}`),
	})
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"user": userDTO(user)})
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

func (handler *Handler) creatorSession(w http.ResponseWriter, request *http.Request) {
	session := creatorSessionFrom(request.Context())
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"user": userDTO(session.User)})
}

func (handler *Handler) creatorCSRF(w http.ResponseWriter, request *http.Request) {
	session := creatorSessionFrom(request.Context())
	token, tokenHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate csrf token", err)
		return
	}
	if err := handler.repository.RotateUserCSRF(request.Context(), session.ID, tokenHash[:], handler.now().UTC()); err != nil {
		handler.internalError(w, request, "rotate csrf token", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]string{"csrfToken": token})
}

func (handler *Handler) logout(w http.ResponseWriter, request *http.Request) {
	session := creatorSessionFrom(request.Context())
	if err := handler.repository.RevokeUserSession(request.Context(), session.ID, handler.now().UTC()); err != nil {
		handler.internalError(w, request, "revoke user session", err)
		return
	}
	handler.clearSessionCookie(w, handler.creatorSessionCookie, http.SameSiteLaxMode)
	httpapi.WriteData(w, request, http.StatusOK, map[string]string{"message": "已退出登录"})
}

func (handler *Handler) me(w http.ResponseWriter, request *http.Request) {
	session := creatorSessionFrom(request.Context())
	httpapi.WriteData(w, request, http.StatusOK, userDTO(session.User))
}

type updateMeRequest struct {
	Nickname string `json:"nickname"`
}

func (handler *Handler) updateMe(w http.ResponseWriter, request *http.Request) {
	var body updateMeRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	nickname, err := normalizeNickname(body.Nickname)
	if err != nil {
		validationError(w, request, map[string]string{"nickname": "昵称不能超过 64 个字符"})
		return
	}
	user, err := handler.repository.UpdateNickname(request.Context(), creatorSessionFrom(request.Context()).User.ID, nickname)
	if err != nil {
		handler.internalError(w, request, "update profile", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, userDTO(user))
}

func (handler *Handler) getAvatar(w http.ResponseWriter, request *http.Request) {
	asset, err := handler.repository.GetAvatar(request.Context(), creatorSessionFrom(request.Context()).User.ID)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, request, http.StatusNotFound, "AVATAR_NOT_FOUND", "当前用户尚未设置头像", nil)
		return
	}
	if err != nil {
		handler.internalError(w, request, "get avatar", err)
		return
	}
	location, err := handler.storage.PresignedGet(request.Context(), asset.Bucket, asset.ObjectKey, 15*time.Minute)
	if err != nil {
		handler.internalError(w, request, "create avatar URL", err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	http.Redirect(w, request, location.String(), http.StatusTemporaryRedirect)
}

func (handler *Handler) uploadAvatar(w http.ResponseWriter, request *http.Request) {
	multipartReader, err := request.MultipartReader()
	if err != nil {
		badRequest(w, request, "请使用 multipart/form-data 上传头像")
		return
	}
	var staged *os.File
	defer func() {
		if staged != nil {
			_ = staged.Close()
			_ = os.Remove(staged.Name())
		}
	}()

	for {
		part, err := multipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			badRequest(w, request, "无法读取上传内容")
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		if staged != nil {
			_ = part.Close()
			validationError(w, request, map[string]string{"file": "每次只能上传一张头像"})
			return
		}
		file, _, err := storage.CopyToTemporaryFile(part, handler.uploadBufferSize)
		_ = part.Close()
		if err != nil {
			handler.internalError(w, request, "stage avatar upload", err)
			return
		}
		staged = file
	}
	if staged == nil {
		validationError(w, request, map[string]string{"file": "请选择要上传的头像"})
		return
	}

	processed, err := imageprocessing.ProcessAvatar(staged, avatarMaximumDimension)
	if errors.Is(err, imageprocessing.ErrInvalidImage) {
		httpapi.WriteError(w, request, http.StatusUnsupportedMediaType, "IMAGE_INVALID", "文件不是受支持且可正常解码的 JPEG、PNG 或 WebP 图片", nil)
		return
	}
	if err != nil {
		handler.logger.Warn("reject unsafe avatar", "request_id", middleware.GetReqID(request.Context()))
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "IMAGE_UNSAFE", "图片尺寸超过安全处理限制", nil)
		return
	}
	defer processed.File.Close()
	defer os.Remove(processed.File.Name())

	userID := creatorSessionFrom(request.Context()).User.ID
	assetID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate avatar id", err)
		return
	}
	objectKey := fmt.Sprintf("users/%s/avatar/%s.png", userID, assetID)
	uploadedSize, err := handler.storage.PutFile(request.Context(), userAssetsBucket, objectKey, processed.File.Name(), processed.MIMEType)
	if err != nil {
		handler.internalError(w, request, "store avatar", err)
		return
	}
	asset := AvatarAsset{
		ID: assetID, OwnerUserID: userID, Bucket: userAssetsBucket, ObjectKey: objectKey,
		MIMEType: processed.MIMEType, SizeBytes: uploadedSize, ChecksumSHA256: processed.ChecksumSHA256[:],
		Width: processed.Width, Height: processed.Height, CreatedAt: handler.now().UTC(),
	}
	user, previous, err := handler.repository.ReplaceAvatar(request.Context(), userID, asset)
	if err != nil {
		_ = handler.storage.Remove(request.Context(), userAssetsBucket, objectKey)
		handler.internalError(w, request, "replace avatar", err)
		return
	}
	if previous != nil {
		if err := handler.storage.Remove(request.Context(), previous.Bucket, previous.ObjectKey); err != nil {
			handler.logger.Error("remove replaced avatar object", "user_id", userID, "error_type", "object_storage_removal_failed")
		}
	}
	httpapi.WriteData(w, request, http.StatusCreated, userDTO(user))
}

func (handler *Handler) deleteAvatar(w http.ResponseWriter, request *http.Request) {
	userID := creatorSessionFrom(request.Context()).User.ID
	user, object, err := handler.repository.RemoveAvatar(request.Context(), userID)
	if err != nil {
		handler.internalError(w, request, "delete avatar", err)
		return
	}
	if object != nil {
		if err := handler.storage.Remove(request.Context(), object.Bucket, object.ObjectKey); err != nil {
			handler.logger.Error("remove deleted avatar object", "user_id", userID, "error_type", "object_storage_removal_failed")
		}
	}
	httpapi.WriteData(w, request, http.StatusOK, userDTO(user))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (handler *Handler) changePassword(w http.ResponseWriter, request *http.Request) {
	var body changePasswordRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	session := creatorSessionFrom(request.Context())
	valid, err := handler.passwords.Verify(body.CurrentPassword, session.User.PasswordHash)
	if err != nil {
		handler.internalError(w, request, "verify current password", err)
		return
	}
	if !valid {
		httpapi.WriteError(w, request, http.StatusUnauthorized, "CURRENT_PASSWORD_INVALID", "当前密码不正确", nil)
		return
	}
	passwordHash, err := handler.passwords.Hash(body.NewPassword)
	if err != nil {
		validationError(w, request, map[string]string{"newPassword": "密码长度应为 8–128 个字符"})
		return
	}
	if err := handler.repository.ChangePassword(request.Context(), session.User.ID, passwordHash, handler.now().UTC()); err != nil {
		handler.internalError(w, request, "change password", err)
		return
	}
	handler.clearSessionCookie(w, handler.creatorSessionCookie, http.SameSiteLaxMode)
	httpapi.WriteData(w, request, http.StatusOK, map[string]string{"message": "密码已修改，请重新登录"})
}

type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (handler *Handler) adminLogin(w http.ResponseWriter, request *http.Request) {
	if !handler.allowRequest(w, request, handler.adminLimiter, "admin-login:"+handler.clientAddress(request)) {
		return
	}
	var body adminLoginRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	if handler.adminUsername == "" || handler.adminPasswordHash == "" {
		httpapi.WriteError(w, request, http.StatusServiceUnavailable, "ADMIN_AUTH_NOT_CONFIGURED", "管理员认证尚未配置", nil)
		return
	}
	valid, err := handler.passwords.Verify(body.Password, handler.adminPasswordHash)
	if err != nil {
		handler.logger.Warn("administrator password hash is invalid")
		httpapi.WriteError(w, request, http.StatusServiceUnavailable, "ADMIN_AUTH_NOT_CONFIGURED", "管理员认证尚未正确配置", nil)
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Username), []byte(handler.adminUsername)) != 1 || !valid {
		invalidCredentials(w, request)
		return
	}

	sessionID, _ := security.NewID()
	token, tokenHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate admin session token", err)
		return
	}
	_, csrfHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate admin csrf token", err)
		return
	}
	now := handler.now().UTC()
	session := AdminSession{
		ID:                    sessionID,
		Username:              handler.adminUsername,
		CSRFTokenHash:         csrfHash[:],
		CredentialFingerprint: handler.adminCredentialHash[:],
		ExpiresAt:             now.Add(8 * time.Hour),
	}
	if err := handler.repository.CreateAdminSession(request.Context(), session, tokenHash[:], now); err != nil {
		handler.internalError(w, request, "create admin session", err)
		return
	}
	auditID, _ := security.NewID()
	if err := handler.repository.CreateAdminAuditLog(request.Context(), auditID, session.ID, session.Username, "admin.login", middleware.GetReqID(request.Context()), now); err != nil {
		handler.logger.Error("write admin login audit log", "error", err)
	}
	handler.setSessionCookie(w, handler.adminSessionCookie, token, session.ExpiresAt, http.SameSiteStrictMode)
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"admin": map[string]string{"username": session.Username}})
}

func (handler *Handler) adminSession(w http.ResponseWriter, request *http.Request) {
	session := adminSessionFrom(request.Context())
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"admin": map[string]string{"username": session.Username}})
}

func (handler *Handler) adminCSRF(w http.ResponseWriter, request *http.Request) {
	session := adminSessionFrom(request.Context())
	token, tokenHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate admin csrf token", err)
		return
	}
	if err := handler.repository.RotateAdminCSRF(request.Context(), session.ID, tokenHash[:], handler.now().UTC()); err != nil {
		handler.internalError(w, request, "rotate admin csrf token", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]string{"csrfToken": token})
}

func (handler *Handler) adminLogout(w http.ResponseWriter, request *http.Request) {
	session := adminSessionFrom(request.Context())
	now := handler.now().UTC()
	auditID, _ := security.NewID()
	if err := handler.repository.CreateAdminAuditLog(request.Context(), auditID, session.ID, session.Username, "admin.logout", middleware.GetReqID(request.Context()), now); err != nil {
		handler.logger.Error("write admin logout audit log", "error", err)
	}
	if err := handler.repository.RevokeAdminSession(request.Context(), session.ID, now); err != nil {
		handler.internalError(w, request, "revoke admin session", err)
		return
	}
	handler.clearSessionCookie(w, handler.adminSessionCookie, http.SameSiteStrictMode)
	httpapi.WriteData(w, request, http.StatusOK, map[string]string{"message": "已退出管理后台"})
}

func (handler *Handler) requireCreatorSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(handler.creatorSessionCookie)
		if err != nil || cookie.Value == "" {
			unauthorized(w, request)
			return
		}
		hash := security.HashToken(cookie.Value)
		session, err := handler.repository.GetUserSession(request.Context(), hash[:], handler.now().UTC())
		if errors.Is(err, ErrNotFound) {
			handler.clearSessionCookie(w, handler.creatorSessionCookie, http.SameSiteLaxMode)
			unauthorized(w, request)
			return
		}
		if err != nil {
			handler.internalError(w, request, "load user session", err)
			return
		}
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), creatorSessionKey, session)))
	})
}

func (handler *Handler) requireAdminSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(handler.adminSessionCookie)
		if err != nil || cookie.Value == "" {
			unauthorized(w, request)
			return
		}
		hash := security.HashToken(cookie.Value)
		session, err := handler.repository.GetAdminSession(request.Context(), hash[:], handler.adminCredentialHash[:], handler.now().UTC())
		if errors.Is(err, ErrNotFound) {
			handler.clearSessionCookie(w, handler.adminSessionCookie, http.SameSiteStrictMode)
			unauthorized(w, request)
			return
		}
		if err != nil {
			handler.internalError(w, request, "load admin session", err)
			return
		}
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), adminSessionKey, session)))
	})
}

func (handler *Handler) requireCreatorCSRF(next http.Handler) http.Handler {
	return handler.requireCSRF(next, func(request *http.Request) []byte {
		return creatorSessionFrom(request.Context()).CSRFTokenHash
	})
}

func (handler *Handler) requireAdminCSRF(next http.Handler) http.Handler {
	return handler.requireCSRF(next, func(request *http.Request) []byte {
		return adminSessionFrom(request.Context()).CSRFTokenHash
	})
}

func (handler *Handler) requireCSRF(next http.Handler, expected func(*http.Request) []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("X-CSRF-Token")
		hash := security.HashToken(token)
		if token == "" || subtle.ConstantTimeCompare(hash[:], expected(request)) != 1 {
			httpapi.WriteError(w, request, http.StatusForbidden, "CSRF_TOKEN_INVALID", "请求安全凭据无效，请刷新后重试", nil)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (handler *Handler) verifyMutationOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		origin, err := normalizeOrigin(request.Header.Get("Origin"))
		if err != nil {
			httpapi.WriteError(w, request, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "请求来源不受信任", nil)
			return
		}
		if _, allowed := handler.allowedMutationOrigins[origin]; !allowed {
			httpapi.WriteError(w, request, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "请求来源不受信任", nil)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func creatorSessionFrom(ctx context.Context) UserSession {
	return ctx.Value(creatorSessionKey).(UserSession)
}

func adminSessionFrom(ctx context.Context) AdminSession {
	return ctx.Value(adminSessionKey).(AdminSession)
}

func userDTO(user User) map[string]any {
	return map[string]any{
		"id":            user.ID,
		"userId":        user.LoginID,
		"nickname":      nullableString(user.Nickname),
		"avatarAssetId": nullableString(user.AvatarAssetID),
		"createdAt":     user.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":     user.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func normalizeUserID(value string) (string, error) {
	loginID := strings.ToLower(strings.TrimSpace(value))
	if len(loginID) < 3 || len(loginID) > 32 || loginID[0] < 'a' || loginID[0] > 'z' {
		return "", errors.New("invalid user id")
	}
	for index := 1; index < len(loginID); index++ {
		character := loginID[index]
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return "", errors.New("invalid user id")
		}
	}
	switch loginID {
	case "admin", "administrator", "root", "support", "system":
		return "", errors.New("reserved user id")
	}
	return loginID, nil
}

func normalizeNickname(value string) (sql.NullString, error) {
	nickname := strings.TrimSpace(value)
	if utf8.RuneCountInString(nickname) > 64 {
		return sql.NullString{}, errors.New("nickname too long")
	}
	return sql.NullString{String: nickname, Valid: nickname != ""}, nil
}

func normalizeOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("invalid origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("invalid origin scheme")
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), nil
}

func (handler *Handler) setSessionCookie(w http.ResponseWriter, name, value string, expiresAt time.Time, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", HttpOnly: true, Secure: handler.secureCookies,
		SameSite: sameSite, Expires: expiresAt.UTC(), MaxAge: int(time.Until(expiresAt).Seconds()),
	})
}

func (handler *Handler) clearSessionCookie(w http.ResponseWriter, name string, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", HttpOnly: true, Secure: handler.secureCookies,
		SameSite: sameSite, Expires: time.Unix(0, 0), MaxAge: -1,
	})
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	handler.logger.Error(operation, "error", err, "request_id", middleware.GetReqID(request.Context()))
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}

func (handler *Handler) allowRequest(w http.ResponseWriter, request *http.Request, limiter *ratelimit.Limiter, key string) bool {
	if limiter.Allow(key) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	httpapi.WriteError(w, request, http.StatusTooManyRequests, "RATE_LIMITED", "请求过于频繁，请稍后重试", nil)
	return false
}

func (handler *Handler) clientAddress(request *http.Request) string {
	return httpapi.ClientAddress(request, handler.trustProxyHeaders)
}

func badRequest(w http.ResponseWriter, request *http.Request, message string) {
	httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_REQUEST", message, nil)
}

func validationError(w http.ResponseWriter, request *http.Request, fields map[string]string) {
	httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "请检查输入内容", fields)
}

func invalidCredentials(w http.ResponseWriter, request *http.Request) {
	httpapi.WriteError(w, request, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码不正确", nil)
}

func unauthorized(w http.ResponseWriter, request *http.Request) {
	httpapi.WriteError(w, request, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "请先登录", nil)
}
