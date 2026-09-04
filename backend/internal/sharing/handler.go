package sharing

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"
	"gamegen/backend/internal/gameconfig"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/httpapi"
	"gamegen/backend/internal/platform/ratelimit"
	"gamegen/backend/internal/platform/security"
	"gamegen/backend/internal/platform/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	playSessionCookieProduction = "__Host-play_session"
	playSessionCookieLocal      = "play_session"
	maxGameConfigBytes          = 1 << 20
)

type Handler struct {
	repository              *Repository
	storage                 *storage.Client
	shareCipher             *contentcrypto.Cipher
	auth                    *auth.Handler
	logger                  *slog.Logger
	playBaseURL             string
	playSessionTTL          time.Duration
	maxShareLife            time.Duration
	playCookie              string
	secureCookies           bool
	trustProxyHeaders       bool
	playOrigins             map[string]struct{}
	publicLimiter           *ratelimit.Limiter
	analytics               analytics.Recorder
	creatorUser             func(*http.Request) auth.User
	currentVersion          func(context.Context, string, string) (string, error)
	createShareRecord       func(context.Context, Share, string, time.Time) (Share, error)
	findPublicShare         func(context.Context, string) (Share, error)
	createPlaySessionRecord func(context.Context, string, string, []byte, []byte, time.Time, time.Time) (PlaySession, error)
	findPlaySession         func(context.Context, []byte, time.Time) (PlaySession, error)
	now                     func() time.Time
}

func NewHandler(repository *Repository, objectStorage *storage.Client, shareCipher *contentcrypto.Cipher, authHandler *auth.Handler, recorder analytics.Recorder, cfg config.Config, logger *slog.Logger) *Handler {
	secure := cfg.App.Environment == "production"
	cookieName := playSessionCookieLocal
	if secure {
		cookieName = playSessionCookieProduction
	}
	origins := map[string]struct{}{}
	if origin, err := normalizedOrigin(cfg.App.PlayBaseURL); err == nil {
		origins[origin] = struct{}{}
	}
	if cfg.App.Environment == "development" {
		origins["http://127.0.0.1:5173"] = struct{}{}
		origins["http://localhost:5173"] = struct{}{}
	}
	return &Handler{
		repository: repository, storage: objectStorage, shareCipher: shareCipher, auth: authHandler, logger: logger,
		playBaseURL: strings.TrimRight(cfg.App.PlayBaseURL, "/"), playSessionTTL: time.Duration(cfg.Sharing.PlaySessionMinutes) * time.Minute,
		maxShareLife: time.Duration(cfg.Sharing.MaxLinkLifetimeDays) * 24 * time.Hour,
		playCookie:   cookieName, secureCookies: secure, trustProxyHeaders: cfg.HTTP.TrustProxyHeaders, playOrigins: origins,
		publicLimiter: ratelimit.New(120, time.Minute), analytics: recorder,
		creatorUser: auth.CreatorUser, currentVersion: repository.CurrentVersion,
		createShareRecord: repository.CreateShare, findPublicShare: repository.FindPublicShare,
		createPlaySessionRecord: repository.CreatePlaySession, findPlaySession: repository.FindPlaySession, now: time.Now,
	}
}

func (handler *Handler) Mount(router chi.Router) {
	handler.MountPrivate(router)
	handler.MountPublic(router)
}

func (handler *Handler) MountPrivate(router chi.Router) {
	router.Group(func(router chi.Router) {
		router.Use(handler.auth.RequireCreatorSession)
		router.Get("/games/{gameId}/share-links", handler.listShares)
		router.Get("/games/{gameId}/share-links/{shareId}", handler.getShare)
		router.Group(func(router chi.Router) {
			router.Use(handler.auth.RequireCreatorMutation)
			router.Post("/games/{gameId}/share-links", handler.createShare)
			router.Delete("/games/{gameId}/share-links/{shareId}", handler.revokeShare)
		})
	})
}

func (handler *Handler) MountPublic(router chi.Router) {
	router.Route("/public", func(router chi.Router) {
		router.Use(publicSecurityHeaders)
		router.Use(handler.limitPublic)
		router.Group(func(router chi.Router) {
			router.Use(handler.verifyPlayOrigin)
			router.Post("/shares/{publicId}/resolve", handler.resolveShare)
			router.Post("/shares/{publicId}/play-sessions", handler.createPlaySession)
			router.Post("/play-sessions/current/refresh-assets", handler.refreshAssets)
			router.Post("/play-sessions/current/events", handler.recordPlayEvent)
		})
		router.Get("/play-sessions/current/game-config", handler.getGameConfig)
	})
}

type createShareRequest struct {
	ExpiresAt string `json:"expiresAt"`
}

func (handler *Handler) createShare(w http.ResponseWriter, request *http.Request) {
	user := handler.creatorUser(request)
	var body createShareRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_REQUEST", "请求内容不是有效的 JSON", nil)
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, body.ExpiresAt)
	now := handler.now().UTC()
	if err != nil || !expiresAt.After(now) || expiresAt.After(now.Add(handler.maxShareLife)) {
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "INVALID_SHARE_EXPIRY", "分享截止时间必须晚于当前时间且不超过 90 天", map[string]string{"expiresAt": "请选择有效的分享截止时间"})
		return
	}
	gameID := chi.URLParam(request, "gameId")
	versionID, err := handler.currentVersion(request.Context(), user.ID, gameID)
	if handler.writeError(w, request, err) {
		return
	}
	shareID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate share id", err)
		return
	}
	publicID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate public share id", err)
		return
	}
	secret, secretHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate share secret", err)
		return
	}
	ciphertext, nonce, keyVersion, err := handler.shareCipher.Encrypt([]byte(secret), []byte(shareID))
	if err != nil {
		handler.internalError(w, request, "encrypt share secret", err)
		return
	}
	share, err := handler.createShareRecord(request.Context(), Share{
		ID: shareID, GameID: gameID, GameVersionID: versionID, PublicID: publicID,
		SecretHash: secretHash[:], SecretCiphertext: ciphertext, SecretNonce: nonce,
		EncryptionKeyVersion: keyVersion, ExpiresAt: expiresAt.UTC(), CreatedAt: now,
	}, user.ID, now)
	if handler.writeError(w, request, err) {
		return
	}
	lifetimeDays := int((share.ExpiresAt.Sub(share.CreatedAt) + 24*time.Hour - 1) / (24 * time.Hour))
	handler.recordBusinessEvent(request, analytics.RecordInput{
		EventName: analytics.EventShareCreated, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: user.ID, GameID: share.GameID, GameVersionID: share.GameVersionID, ShareID: share.ID,
		Properties: sharingEventProperties(map[string]any{"lifetimeDays": lifetimeDays}),
	})
	httpapi.WriteData(w, request, http.StatusCreated, handler.shareDTO(share, secret, now))
}

func (handler *Handler) listShares(w http.ResponseWriter, request *http.Request) {
	now := handler.now().UTC()
	shares, err := handler.repository.ListShares(request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"))
	if handler.writeError(w, request, err) {
		return
	}
	items := make([]map[string]any, 0, len(shares))
	for _, share := range shares {
		secret, err := handler.decryptShareSecret(share)
		if err != nil {
			handler.internalError(w, request, "decrypt share secret", err)
			return
		}
		items = append(items, handler.shareDTO(share, secret, now))
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": items})
}

func (handler *Handler) getShare(w http.ResponseWriter, request *http.Request) {
	share, err := handler.repository.GetShare(request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"), chi.URLParam(request, "shareId"))
	if handler.writeError(w, request, err) {
		return
	}
	secret, err := handler.decryptShareSecret(share)
	if err != nil {
		handler.internalError(w, request, "decrypt share secret", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, handler.shareDTO(share, secret, handler.now().UTC()))
}

func (handler *Handler) revokeShare(w http.ResponseWriter, request *http.Request) {
	share, err := handler.repository.RevokeShare(request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"), chi.URLParam(request, "shareId"), handler.now().UTC())
	if handler.writeError(w, request, err) {
		return
	}
	secret, err := handler.decryptShareSecret(share)
	if err != nil {
		handler.internalError(w, request, "decrypt revoked share secret", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, handler.shareDTO(share, secret, handler.now().UTC()))
}

type secretRequest struct {
	Secret string `json:"secret"`
}

func (handler *Handler) resolveShare(w http.ResponseWriter, request *http.Request) {
	secret, share, ok := handler.validatePublicShare(w, request)
	_ = secret
	if !ok {
		return
	}
	handler.recordBusinessEvent(request, analytics.RecordInput{
		EventName: analytics.EventShareOpened, Source: analytics.SourceAPI, ActorType: analytics.ActorReceiver,
		GameID: share.GameID, GameVersionID: share.GameVersionID, ShareID: share.ID, Properties: json.RawMessage(`{}`),
	})
	httpapi.WriteData(w, request, http.StatusOK, publicShareDTO(share))
}

func (handler *Handler) createPlaySession(w http.ResponseWriter, request *http.Request) {
	secret, _, ok := handler.validatePublicShare(w, request)
	if !ok {
		return
	}
	sessionID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate play session id", err)
		return
	}
	token, tokenHash, err := security.NewToken()
	if err != nil {
		handler.internalError(w, request, "generate play session token", err)
		return
	}
	secretHash := security.HashToken(secret)
	now := handler.now().UTC()
	expiresAt := now.Add(handler.playSessionTTL)
	session, err := handler.createPlaySessionRecord(request.Context(), sessionID, chi.URLParam(request, "publicId"), secretHash[:], tokenHash[:], expiresAt, now)
	if handler.writePublicError(w, request, err) {
		return
	}
	handler.setPlayCookie(w, token, expiresAt)
	handler.recordBusinessEvent(request, analytics.RecordInput{
		EventName: analytics.EventPlayStarted, Source: analytics.SourceAPI, ActorType: analytics.ActorReceiver,
		GameID: session.GameID, GameVersionID: session.GameVersionID, ShareID: session.ShareLinkID, PlaySessionID: session.ID,
		Properties: json.RawMessage(`{}`),
	})
	httpapi.WriteData(w, request, http.StatusCreated, map[string]any{
		"expiresAt": session.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"game":      map[string]any{"title": session.GameTitle, "templateId": session.TemplateID, "templateVersion": session.TemplateVersion},
	})
}

func (handler *Handler) getGameConfig(w http.ResponseWriter, request *http.Request) {
	session, ok := handler.requirePlaySession(w, request)
	if !ok {
		return
	}
	data, err := handler.storage.ReadAll(request.Context(), session.ConfigBucket, session.ConfigObjectKey, maxGameConfigBytes)
	if err != nil {
		handler.internalError(w, request, "read public game config", err)
		return
	}
	document, err := gameconfig.Decode(data)
	if err != nil {
		handler.internalError(w, request, "decode public game config", err)
		return
	}
	if document.TemplateID != session.TemplateID || document.TemplateVersion != session.TemplateVersion {
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "GAME_CONFIG_INVALID", "游戏配置暂时无法加载", nil)
		return
	}
	assets, err := handler.publicAssets(request, session)
	if err != nil {
		handler.internalError(w, request, "create public asset URLs", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{
		"templateId": document.TemplateID, "templateVersion": document.TemplateVersion,
		"configVersion": document.ConfigVersion, "config": document.Config, "assets": assets,
		"playSessionExpiresAt": session.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

func (handler *Handler) refreshAssets(w http.ResponseWriter, request *http.Request) {
	session, ok := handler.requirePlaySession(w, request)
	if !ok {
		return
	}
	assets, err := handler.publicAssets(request, session)
	if err != nil {
		handler.internalError(w, request, "refresh public asset URLs", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"assets": assets})
}

func (handler *Handler) recordPlayEvent(w http.ResponseWriter, request *http.Request) {
	session, ok := handler.requirePlaySession(w, request)
	if !ok {
		return
	}
	analytics.RecordPublicEvent(handler.analytics, handler.logger, w, request, analytics.PublicIdentity{
		GameID: session.GameID, GameVersionID: session.GameVersionID,
		ShareID: session.ShareLinkID, PlaySessionID: session.ID,
	})
}

func (handler *Handler) recordBusinessEvent(request *http.Request, input analytics.RecordInput) {
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

func sharingEventProperties(properties map[string]any) json.RawMessage {
	encoded, _ := json.Marshal(properties)
	return encoded
}

func (handler *Handler) validatePublicShare(w http.ResponseWriter, request *http.Request) (string, Share, bool) {
	var body secretRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil || strings.TrimSpace(body.Secret) == "" {
		httpapi.WriteError(w, request, http.StatusBadRequest, "SHARE_SECRET_REQUIRED", "分享凭据缺失", nil)
		return "", Share{}, false
	}
	share, err := handler.findPublicShare(request.Context(), chi.URLParam(request, "publicId"))
	if handler.writePublicError(w, request, err) {
		return "", Share{}, false
	}
	providedHash := security.HashToken(body.Secret)
	if len(share.SecretHash) != len(providedHash) || subtle.ConstantTimeCompare(share.SecretHash, providedHash[:]) != 1 {
		handler.publicEnded(w, request)
		return "", Share{}, false
	}
	if share.RevokedAt.Valid || !handler.now().UTC().Before(share.ExpiresAt) || share.GameStatus == "deleting" || share.VersionStatus != "ready" {
		handler.publicEnded(w, request)
		return "", Share{}, false
	}
	return body.Secret, share, true
}

func (handler *Handler) requirePlaySession(w http.ResponseWriter, request *http.Request) (PlaySession, bool) {
	cookie, err := request.Cookie(handler.playCookie)
	if err != nil || cookie.Value == "" {
		handler.playExpired(w, request)
		return PlaySession{}, false
	}
	hash := security.HashToken(cookie.Value)
	session, err := handler.findPlaySession(request.Context(), hash[:], handler.now().UTC())
	if handler.writePublicError(w, request, err) {
		if errors.Is(err, ErrPlayExpired) {
			handler.clearPlayCookie(w)
		}
		return PlaySession{}, false
	}
	return session, true
}

func (handler *Handler) publicAssets(request *http.Request, session PlaySession) ([]map[string]any, error) {
	assets, err := handler.repository.ListRenderAssets(request.Context(), session)
	if err != nil {
		return nil, err
	}
	now := handler.now().UTC()
	ttl := 15 * time.Minute
	if remaining := session.ExpiresAt.Sub(now); remaining < ttl {
		ttl = remaining
	}
	if ttl <= 0 {
		return nil, ErrPlayExpired
	}
	expiresAt := now.Add(ttl).UTC()
	items := make([]map[string]any, 0, len(assets))
	for index, asset := range assets {
		presigned, err := handler.storage.PresignedGet(request.Context(), asset.Bucket, asset.ObjectKey, ttl)
		if err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"key": fmt.Sprintf("render-%d", index+1), "type": "image", "url": presigned.String(),
			"mimeType": asset.MIMEType, "expiresAt": expiresAt.Format(time.RFC3339Nano),
		})
	}
	return items, nil
}

func (handler *Handler) decryptShareSecret(share Share) (string, error) {
	plaintext, err := handler.shareCipher.Decrypt(share.SecretCiphertext, share.SecretNonce, []byte(share.ID), share.EncryptionKeyVersion)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (handler *Handler) shareDTO(share Share, secret string, now time.Time) map[string]any {
	return map[string]any{
		"id": share.ID, "gameId": share.GameID, "gameVersionId": share.GameVersionID, "publicId": share.PublicID,
		"url":    fmt.Sprintf("%s/play/%s#t=%s", handler.playBaseURL, share.PublicID, url.QueryEscape(secret)),
		"status": shareStatus(share, now), "expiresAt": share.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"revokedAt": nullableTime(share.RevokedAt), "createdAt": share.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func publicShareDTO(share Share) map[string]any {
	displayName := "一位朋友"
	if share.CreatorNickname.Valid && strings.TrimSpace(share.CreatorNickname.String) != "" {
		displayName = share.CreatorNickname.String
	}
	return map[string]any{
		"creator": map[string]string{"displayName": displayName},
		"share":   map[string]string{"expiresAt": share.ExpiresAt.UTC().Format(time.RFC3339Nano)},
		"game":    map[string]any{"title": share.GameTitle, "ready": true},
	}
}

func shareStatus(share Share, now time.Time) string {
	if share.RevokedAt.Valid {
		return "revoked"
	}
	if !now.Before(share.ExpiresAt) {
		return "expired"
	}
	return "active"
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func (handler *Handler) setPlayCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: handler.playCookie, Value: token, Path: "/", HttpOnly: true, Secure: handler.secureCookies,
		SameSite: http.SameSiteStrictMode, Expires: expiresAt.UTC(), MaxAge: int(time.Until(expiresAt).Seconds()),
	})
}

func (handler *Handler) clearPlayCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: handler.playCookie, Value: "", Path: "/", HttpOnly: true, Secure: handler.secureCookies,
		SameSite: http.SameSiteStrictMode, Expires: time.Unix(0, 0), MaxAge: -1,
	})
}

func (handler *Handler) verifyPlayOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		origin, err := normalizedOrigin(request.Header.Get("Origin"))
		if err != nil {
			httpapi.WriteError(w, request, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "请求来源不受信任", nil)
			return
		}
		if _, ok := handler.playOrigins[origin]; !ok {
			httpapi.WriteError(w, request, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "请求来源不受信任", nil)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func normalizedOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid origin")
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), nil
}

func publicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, request)
	})
}

func (handler *Handler) limitPublic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		host := httpapi.ClientAddress(request, handler.trustProxyHeaders)
		if !handler.publicLimiter.Allow(host) {
			httpapi.WriteError(w, request, http.StatusTooManyRequests, "RATE_LIMITED", "请求过于频繁，请稍后重试", nil)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (handler *Handler) writeError(w http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		httpapi.WriteError(w, request, http.StatusNotFound, "SHARE_NOT_FOUND", "分享记录不存在", nil)
	case errors.Is(err, ErrGameNotReady):
		httpapi.WriteError(w, request, http.StatusConflict, "GAME_NOT_READY", "只有创建完成的当前版本可以分享", nil)
	default:
		handler.internalError(w, request, "sharing repository operation", err)
	}
	return true
}

func (handler *Handler) writePublicError(w http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrShareEnded) {
		handler.publicEnded(w, request)
		return true
	}
	if errors.Is(err, ErrPlayExpired) {
		handler.playExpired(w, request)
		return true
	}
	handler.internalError(w, request, "public sharing operation", err)
	return true
}

func (handler *Handler) publicEnded(w http.ResponseWriter, request *http.Request) {
	httpapi.WriteError(w, request, http.StatusGone, "SHARE_ENDED", "这份游戏分享已经结束，请联系分享者获取新链接", nil)
}

func (handler *Handler) playExpired(w http.ResponseWriter, request *http.Request) {
	httpapi.WriteError(w, request, http.StatusUnauthorized, "PLAY_SESSION_EXPIRED", "本局游戏已经结束，请重新打开有效的分享链接", nil)
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	handler.logger.Error(operation, "error", err, "request_id", middleware.GetReqID(request.Context()))
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}
