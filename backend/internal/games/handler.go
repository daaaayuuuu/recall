package games

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"
	"gamegen/backend/internal/gameconfig"
	"gamegen/backend/internal/gametemplates"
	"gamegen/backend/internal/imagemoderation"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/httpapi"
	"gamegen/backend/internal/platform/security"
	"gamegen/backend/internal/platform/storage"
	"gamegen/backend/internal/textai"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	sourceAssetsBucket  = "gamegen-source-assets"
	maxGameConfigBytes  = 1 << 20
	previewAssetURLLife = 15 * time.Minute
)

type Handler struct {
	repository          *Repository
	storage             *storage.Client
	cipher              *contentcrypto.Cipher
	auth                *auth.Handler
	analytics           analytics.Recorder
	letterPolisher      textai.LoveLetterPolisher
	imageReviewer       imagemoderation.Reviewer
	aiConfigProvider    AIConfigProvider
	creatorUser         func(*http.Request) auth.User
	getGameRecord       func(context.Context, string, string) (Game, error)
	getVersionRecord    func(context.Context, string, string, string) (Version, error)
	createGameRecord    func(context.Context, string, string, string, string, sql.NullString, EncryptedInput, int, string, string, time.Time) (Game, Version, error)
	createVersionRecord func(context.Context, string, string, string, EncryptedInput, int, time.Time) (Version, error)
	addAssetRecord      func(context.Context, string, string, string, Asset, bool, int) ([]ObjectRef, error)
	putAssetFile        func(context.Context, string, string, string, string) (int64, error)
	removeAssetObject   func(context.Context, string, string) error
	presignAsset        func(context.Context, string, string, time.Duration) (*url.URL, error)
	logger              *slog.Logger
	bufferSize          int
	maxSourceImageBytes int64
	uploads             *uploadGate
	now                 func() time.Time
}

type AIConfigProvider interface {
	Current(context.Context) (config.AIConfig, error)
}

type uploadGate struct {
	mu      sync.Mutex
	active  map[string]int
	maximum int
}

func newUploadGate(maximum int) *uploadGate {
	if maximum < 1 {
		maximum = 1
	}
	return &uploadGate{active: make(map[string]int), maximum: maximum}
}

func (gate *uploadGate) acquire(userID string) bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.active[userID] >= gate.maximum {
		return false
	}
	gate.active[userID]++
	return true
}

func (gate *uploadGate) release(userID string) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.active[userID]--
	if gate.active[userID] <= 0 {
		delete(gate.active, userID)
	}
}

func NewHandler(repository *Repository, objectStorage *storage.Client, cipher *contentcrypto.Cipher, authHandler *auth.Handler, recorder analytics.Recorder, cfg config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		repository:          repository,
		storage:             objectStorage,
		cipher:              cipher,
		auth:                authHandler,
		analytics:           recorder,
		letterPolisher:      textai.NewDeepSeekPolisher(cfg.AI.Text),
		imageReviewer:       imagemoderation.New(cfg.AI.ImageModeration),
		creatorUser:         auth.CreatorUser,
		getGameRecord:       repository.GetGame,
		getVersionRecord:    repository.GetVersion,
		createGameRecord:    repository.CreateGame,
		createVersionRecord: repository.CreateVersion,
		addAssetRecord:      repository.AddAsset,
		putAssetFile:        objectStorage.PutFile,
		removeAssetObject:   objectStorage.Remove,
		presignAsset:        objectStorage.PresignedGet,
		logger:              logger,
		bufferSize:          cfg.Uploads.StreamBufferBytes,
		maxSourceImageBytes: cfg.AI.ImageToImage.MaxInputBytes,
		uploads:             newUploadGate(cfg.Uploads.MaxConcurrentPerUser),
		now:                 time.Now,
	}
}

// UseAIConfigProvider enables request-time AI configuration. Existing callers
// keep the startup configuration unless a provider is explicitly installed.
func (handler *Handler) UseAIConfigProvider(provider AIConfigProvider) {
	handler.aiConfigProvider = provider
}

func (handler *Handler) Mount(router chi.Router) {
	router.Group(func(router chi.Router) {
		router.Use(handler.auth.RequireCreatorSession)
		router.Get("/games", handler.listGames)
		router.Get("/games/{gameId}", handler.getGame)
		router.Get("/games/{gameId}/versions", handler.listVersions)
		router.Get("/games/{gameId}/versions/{versionId}", handler.getVersion)
		router.Get("/games/{gameId}/versions/{versionId}/assets", handler.listAssets)
		router.Get("/games/{gameId}/versions/{versionId}/preview", handler.getPreview)
		router.Get("/templates", handler.listTemplates)

		router.Group(func(router chi.Router) {
			router.Use(handler.auth.RequireCreatorMutation)
			router.Post("/ai/love-letter/polish", handler.polishLoveLetter)
			router.Post("/games", handler.createGame)
			router.Patch("/games/{gameId}", handler.updateGame)
			router.Delete("/games/{gameId}", handler.deleteGame)
			router.Post("/games/{gameId}/versions", handler.createVersion)
			router.Post("/games/{gameId}/versions/{versionId}/assets", handler.uploadAsset)
			router.Patch("/games/{gameId}/versions/{versionId}/assets/order", handler.reorderAssets)
			router.Delete("/games/{gameId}/versions/{versionId}/assets/{assetId}", handler.deleteAsset)
		})
	})
}

type inputPayload struct {
	MemoryText  string            `json:"memoryText,omitempty"`
	SceneInputs map[string]string `json:"sceneInputs,omitempty"`
}

type gameMutationRequest struct {
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	MemoryText      string            `json:"memoryText,omitempty"`
	TemplateID      string            `json:"templateId"`
	TemplateVersion string            `json:"templateVersion"`
	SceneInputs     map[string]string `json:"sceneInputs"`
}

type polishLoveLetterRequest struct {
	Text string `json:"text"`
}

func (handler *Handler) polishLoveLetter(w http.ResponseWriter, request *http.Request) {
	var body polishLoveLetterRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		validationError(w, request, map[string]string{"text": "请先填写情书内容"})
		return
	}
	if utf8.RuneCountInString(text) > 1000 {
		validationError(w, request, map[string]string{"text": "情书不能超过 1000 个字符"})
		return
	}
	polisher := handler.letterPolisher
	if handler.aiConfigProvider != nil {
		aiConfig, err := handler.aiConfigProvider.Current(request.Context())
		if err != nil {
			handler.logger.Error("load dynamic AI settings", "request_id", middleware.GetReqID(request.Context()), "error", err)
			httpapi.WriteError(w, request, http.StatusServiceUnavailable, "AI_CONFIG_UNAVAILABLE", "AI 配置暂时不可用，请稍后再试", nil)
			return
		}
		polisher = textai.NewDeepSeekPolisher(aiConfig.Text)
	}
	if polisher == nil || !polisher.Configured() {
		httpapi.WriteData(w, request, http.StatusOK, map[string]any{
			"polishedText": text,
			"skipped":      true,
		})
		return
	}
	creator := handler.creatorUser(request)
	polished, err := polisher.PolishLoveLetter(request.Context(), text, creator.ID)
	if err != nil {
		status := http.StatusBadGateway
		code := "AI_POLISH_UNAVAILABLE"
		message := "AI 润色服务暂时不可用，请稍后再试"
		if errors.Is(err, textai.ErrTimeout) {
			status = http.StatusGatewayTimeout
			code = "AI_POLISH_TIMEOUT"
			message = "AI 润色等待超时，请稍后再试"
		}
		handler.logger.Error(
			"polish love letter",
			"error", err,
			"request_id", middleware.GetReqID(request.Context()),
		)
		httpapi.WriteError(w, request, status, code, message, nil)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"polishedText": polished, "skipped": false})
}

func (handler *Handler) listTemplates(w http.ResponseWriter, request *http.Request) {
	maxSourceImageBytes := handler.maxSourceImageBytes
	if handler.aiConfigProvider != nil {
		aiConfig, err := handler.aiConfigProvider.Current(request.Context())
		if err != nil {
			handler.internalError(w, request, "load dynamic AI settings", err)
			return
		}
		maxSourceImageBytes = aiConfig.ImageToImage.MaxInputBytes
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{
		"items": gametemplates.List(), "maxSourceImageBytes": maxSourceImageBytes,
	})
}

func (handler *Handler) createGame(w http.ResponseWriter, request *http.Request) {
	var body gameMutationRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	title, description, validation := validateGameMetadata(body.Title, body.Description)
	if validation != nil {
		validationError(w, request, validation)
		return
	}
	definition, ok := gametemplates.Find(strings.TrimSpace(body.TemplateID), strings.TrimSpace(body.TemplateVersion))
	if !ok {
		validationError(w, request, map[string]string{"templateId": "请选择可用的游戏模板"})
		return
	}
	sceneInputs, fields := definition.ValidateSceneInputs(body.SceneInputs)
	if len(fields) > 0 {
		validationError(w, request, fields)
		return
	}

	gameID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate game id", err)
		return
	}
	versionID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate game version id", err)
		return
	}
	payload := inputPayload{SceneInputs: sceneInputs}
	encrypted, err := handler.encryptInput(versionID, payload)
	if err != nil {
		handler.internalError(w, request, "encrypt initial game input", err)
		return
	}
	now := handler.now().UTC()
	creator := handler.creatorUser(request)
	game, version, err := handler.createGameRecord(
		request.Context(), gameID, versionID, creator.ID,
		title, description, encrypted, definition.InputSchemaVersion, definition.ID, definition.Version, now,
	)
	if err != nil {
		handler.internalError(w, request, "create game", err)
		return
	}
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventGameCreated, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: creator.ID, GameID: game.ID, GameVersionID: version.ID,
		Properties: eventProperties(map[string]any{"templateId": version.TemplateID}),
	})
	gameData, err := handler.gameDTO(request, game)
	if err != nil {
		handler.internalError(w, request, "create game cover URL", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusCreated, map[string]any{
		"game": gameData, "version": handler.versionDTO(version, payload),
	})
}

func (handler *Handler) listGames(w http.ResponseWriter, request *http.Request) {
	items, err := handler.repository.ListGames(request.Context(), auth.CreatorUser(request).ID)
	if err != nil {
		handler.internalError(w, request, "list games", err)
		return
	}
	data := make([]map[string]any, 0, len(items))
	for _, game := range items {
		gameData, err := handler.gameDTO(request, game)
		if err != nil {
			handler.internalError(w, request, "create game cover URL", err)
			return
		}
		data = append(data, gameData)
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": data})
}

func (handler *Handler) getGame(w http.ResponseWriter, request *http.Request) {
	creator := handler.creatorUser(request)
	game, err := handler.getGameRecord(request.Context(), creator.ID, chi.URLParam(request, "gameId"))
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	gameData, err := handler.gameDTO(request, game)
	if err != nil {
		handler.internalError(w, request, "create game cover URL", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, gameData)
}

type metadataRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (handler *Handler) updateGame(w http.ResponseWriter, request *http.Request) {
	var body metadataRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	title, description, validation := validateGameMetadata(body.Title, body.Description)
	if validation != nil {
		validationError(w, request, validation)
		return
	}
	game, err := handler.repository.UpdateGame(
		request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"),
		title, description, handler.now().UTC(),
	)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	gameData, err := handler.gameDTO(request, game)
	if err != nil {
		handler.internalError(w, request, "create game cover URL", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, gameData)
}

type versionRequest struct {
	MemoryText  string            `json:"memoryText,omitempty"`
	SceneInputs map[string]string `json:"sceneInputs"`
}

func (handler *Handler) createVersion(w http.ResponseWriter, request *http.Request) {
	var body versionRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	creator := handler.creatorUser(request)
	game, err := handler.getGameRecord(request.Context(), creator.ID, chi.URLParam(request, "gameId"))
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	if !game.CurrentVersionID.Valid {
		httpapi.WriteError(w, request, http.StatusConflict, "GAME_NOT_EDITABLE", "游戏没有可继承的当前版本", nil)
		return
	}
	current, err := handler.getVersionRecord(request.Context(), creator.ID, game.ID, game.CurrentVersionID.String)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	payload := inputPayload{}
	inputSchemaVersion := current.InputSchemaVersion
	if definition, ok := gametemplates.Find(current.TemplateID, current.TemplateVersion); ok {
		sceneInputs, fields := definition.ValidateSceneInputs(body.SceneInputs)
		if len(fields) > 0 {
			validationError(w, request, fields)
			return
		}
		payload.SceneInputs = sceneInputs
		inputSchemaVersion = definition.InputSchemaVersion
	} else {
		if utf8.RuneCountInString(body.MemoryText) > 10_000 {
			validationError(w, request, map[string]string{"memoryText": "回忆文本不能超过 10000 个字符"})
			return
		}
		payload.MemoryText = body.MemoryText
	}
	if inputSchemaVersion < 1 {
		inputSchemaVersion = 1
	}
	if current.TemplateID == "" {
		httpapi.WriteError(w, request, http.StatusConflict, "GAME_NOT_EDITABLE", "当前游戏模板信息不完整", nil)
		return
	}
	versionID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate version id", err)
		return
	}
	encrypted, err := handler.encryptInput(versionID, payload)
	if err != nil {
		handler.internalError(w, request, "encrypt game version input", err)
		return
	}
	version, err := handler.createVersionRecord(
		request.Context(), versionID, creator.ID, chi.URLParam(request, "gameId"),
		encrypted, inputSchemaVersion, handler.now().UTC(),
	)
	if errors.Is(err, ErrGameNotEditable) {
		httpapi.WriteError(w, request, http.StatusConflict, "GAME_NOT_EDITABLE", "游戏当前正在处理中，不能创建新版本", nil)
		return
	}
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventGameVersionCreated, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: creator.ID, GameID: version.GameID, GameVersionID: version.ID,
		Properties: eventProperties(map[string]any{"versionNumber": version.VersionNumber, "templateId": version.TemplateID}),
	})
	httpapi.WriteData(w, request, http.StatusCreated, handler.versionDTO(version, payload))
}

func (handler *Handler) listVersions(w http.ResponseWriter, request *http.Request) {
	versions, err := handler.repository.ListVersions(request.Context(), auth.CreatorUser(request).ID, chi.URLParam(request, "gameId"))
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	items := make([]map[string]any, 0, len(versions))
	for _, version := range versions {
		payload, err := handler.decryptInput(version)
		if err != nil {
			handler.internalError(w, request, "decrypt game version input", err)
			return
		}
		items = append(items, handler.versionDTO(version, payload))
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": items})
}

func (handler *Handler) getVersion(w http.ResponseWriter, request *http.Request) {
	version, err := handler.repository.GetVersion(
		request.Context(), auth.CreatorUser(request).ID,
		chi.URLParam(request, "gameId"), chi.URLParam(request, "versionId"),
	)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	payload, err := handler.decryptInput(version)
	if err != nil {
		handler.internalError(w, request, "decrypt game version input", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, handler.versionDTO(version, payload))
}

func (handler *Handler) getPreview(w http.ResponseWriter, request *http.Request) {
	userID := auth.CreatorUser(request).ID
	gameID := chi.URLParam(request, "gameId")
	versionID := chi.URLParam(request, "versionId")
	preview, err := handler.repository.GetPreview(request.Context(), userID, gameID, versionID)
	if errors.Is(err, ErrVersionNotReady) {
		httpapi.WriteError(w, request, http.StatusConflict, "GAME_VERSION_NOT_READY", "只有创建完成的版本可以试玩", nil)
		return
	}
	if handler.writeRepositoryError(w, request, err) {
		return
	}

	data, err := handler.storage.ReadAll(
		request.Context(), preview.ConfigBucket.String, preview.ConfigObjectKey.String, maxGameConfigBytes,
	)
	if err != nil {
		handler.internalError(w, request, "read creator preview config", err)
		return
	}
	document, err := decodePreviewDocument(data, preview)
	if err != nil {
		handler.logger.Warn("reject invalid creator preview config", "game_id", gameID, "version_id", versionID, "error", err)
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "GAME_CONFIG_INVALID", "游戏配置暂时无法加载", nil)
		return
	}
	assets, err := handler.previewAssets(request, userID, gameID, versionID)
	if err != nil {
		handler.internalError(w, request, "create creator preview asset URLs", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{
		"game":       map[string]any{"id": preview.GameID, "title": preview.GameTitle},
		"version":    map[string]any{"id": preview.VersionID, "versionNumber": preview.VersionNumber},
		"templateId": document.TemplateID, "templateVersion": document.TemplateVersion,
		"configVersion": document.ConfigVersion, "config": document.Config, "assets": assets,
	})
}

func decodePreviewDocument(data []byte, preview Preview) (gameconfig.Document, error) {
	document, err := gameconfig.Decode(data)
	if err != nil {
		return gameconfig.Document{}, err
	}
	if document.TemplateID != preview.TemplateID || document.TemplateVersion != preview.TemplateVersion {
		return gameconfig.Document{}, errors.New("game config template does not match version")
	}
	return document, nil
}

func (handler *Handler) previewAssets(request *http.Request, userID, gameID, versionID string) ([]map[string]any, error) {
	assets, err := handler.repository.ListPreviewAssets(request.Context(), userID, gameID, versionID)
	if err != nil {
		return nil, err
	}
	expiresAt := handler.now().UTC().Add(previewAssetURLLife)
	items := make([]map[string]any, 0, len(assets))
	for index, asset := range assets {
		presigned, err := handler.storage.PresignedGet(request.Context(), asset.Bucket, asset.ObjectKey, previewAssetURLLife)
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

func (handler *Handler) uploadAsset(w http.ResponseWriter, request *http.Request) {
	user := handler.creatorUser(request)
	if !handler.uploads.acquire(user.ID) {
		httpapi.WriteError(w, request, http.StatusTooManyRequests, "UPLOAD_CONCURRENCY_LIMIT", "当前上传任务较多，请稍后重试", nil)
		return
	}
	defer handler.uploads.release(user.ID)

	multipartReader, err := request.MultipartReader()
	if err != nil {
		badRequest(w, request, "请使用 multipart/form-data 上传图片")
		return
	}
	role := "source"
	slotKey := ""
	sortOrder := 0
	var staged io.ReadSeekCloser
	var stagedPath string
	defer func() {
		if staged != nil {
			_ = staged.Close()
			_ = removeTemporary(stagedPath)
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
		switch part.FormName() {
		case "file":
			if staged != nil {
				part.Close()
				validationError(w, request, map[string]string{"file": "每次只能上传一张图片"})
				return
			}
			file, _, err := storage.CopyToTemporaryFile(part, handler.bufferSize)
			part.Close()
			if err != nil {
				handler.internalError(w, request, "stage image upload", err)
				return
			}
			staged, stagedPath = file, file.Name()
		case "role":
			value, err := io.ReadAll(io.LimitReader(part, 64))
			part.Close()
			if err != nil {
				badRequest(w, request, "无法读取素材角色")
				return
			}
			role = string(value)
		case "slotKey":
			value, err := io.ReadAll(io.LimitReader(part, 128))
			part.Close()
			if err != nil {
				badRequest(w, request, "无法读取素材槽位")
				return
			}
			slotKey = strings.TrimSpace(string(value))
		case "sortOrder":
			value, err := io.ReadAll(io.LimitReader(part, 32))
			part.Close()
			if err != nil {
				badRequest(w, request, "无法读取素材顺序")
				return
			}
			sortOrder, err = strconv.Atoi(string(value))
			if err != nil || sortOrder < 0 {
				validationError(w, request, map[string]string{"sortOrder": "素材顺序必须是非负整数"})
				return
			}
		default:
			part.Close()
		}
	}
	if staged == nil {
		validationError(w, request, map[string]string{"file": "请选择要上传的图片"})
		return
	}
	if role != "source" && role != "cover" {
		validationError(w, request, map[string]string{"role": "素材角色必须是 source 或 cover"})
		return
	}

	gameID := chi.URLParam(request, "gameId")
	versionID := chi.URLParam(request, "versionId")
	version, err := handler.getVersionRecord(request.Context(), user.ID, gameID, versionID)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	replaceExisting := false
	maxItems := 0
	if definition, ok := gametemplates.Find(version.TemplateID, version.TemplateVersion); ok {
		slot, found := definition.AssetSlot(slotKey)
		if !found {
			validationError(w, request, map[string]string{"slotKey": "请选择模板中有效的图片位置"})
			return
		}
		role = "source"
		if slotKey == definition.Cover.Key {
			role = "cover"
		}
		maxItems = slot.MaxItems
		replaceExisting = slot.MaxItems == 1
		if replaceExisting {
			sortOrder = 0
		}
	} else if slotKey != "" {
		validationError(w, request, map[string]string{"slotKey": "旧模板不支持素材槽位"})
		return
	}

	processed, err := ProcessImage(staged)
	if errors.Is(err, ErrInvalidImage) {
		httpapi.WriteError(w, request, http.StatusUnsupportedMediaType, "IMAGE_INVALID", "文件不是受支持且可正常解码的 JPEG、PNG 或 WebP 图片", nil)
		return
	}
	if err != nil {
		handler.logger.Warn("reject unsafe image", "error", err, "request_id", middleware.GetReqID(request.Context()))
		httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "IMAGE_UNSAFE", "图片尺寸超过安全处理限制", nil)
		return
	}
	defer processed.File.Close()
	defer removeTemporary(processed.File.Name())
	maxSourceImageBytes := handler.maxSourceImageBytes
	imageReviewer := handler.imageReviewer
	if handler.aiConfigProvider != nil {
		aiConfig, err := handler.aiConfigProvider.Current(request.Context())
		if err != nil {
			handler.logger.Warn("dynamic AI settings unavailable", "request_id", middleware.GetReqID(request.Context()), "error", err)
			httpapi.WriteError(w, request, http.StatusServiceUnavailable, "AI_CONFIG_UNAVAILABLE", "AI 配置暂时不可用，请稍后重试", nil)
			return
		}
		maxSourceImageBytes = aiConfig.ImageToImage.MaxInputBytes
		imageReviewer = imagemoderation.New(aiConfig.ImageModeration)
	}
	if maxSourceImageBytes > 0 && processed.SizeBytes > maxSourceImageBytes {
		actualSize := formatImageSize(processed.SizeBytes)
		maximumSize := formatImageSize(maxSourceImageBytes)
		message := fmt.Sprintf(
			"图片处理后为 %s，超过 %s 上限，请压缩图片或缩小尺寸后重试",
			actualSize, maximumSize,
		)
		httpapi.WriteError(
			w, request, http.StatusUnprocessableEntity, "IMAGE_TOO_LARGE", message,
			map[string]string{"file": message},
		)
		return
	}
	if imageReviewer != nil && imageReviewer.Configured() {
		decision, err := imageReviewer.Review(request.Context(), imagemoderation.Input{
			Image: processed.File, MIMEType: processed.MIMEType, Purpose: imagemoderation.PurposeGameAsset,
		})
		if err != nil {
			handler.logger.Warn(
				"image moderation unavailable",
				"error", err,
				"request_id", middleware.GetReqID(request.Context()),
			)
			httpapi.WriteError(
				w, request, http.StatusServiceUnavailable, "IMAGE_MODERATION_UNAVAILABLE",
				"图片安全审核暂时不可用，请稍后重试", nil,
			)
			return
		}
		if !decision.Approved {
			handler.logger.Info(
				"image upload rejected by moderation",
				"categories", decision.Categories,
				"provider_request_id", decision.ProviderRequestID,
				"request_id", middleware.GetReqID(request.Context()),
			)
			httpapi.WriteError(
				w, request, http.StatusUnprocessableEntity, "IMAGE_MODERATION_REJECTED",
				"图片未通过安全审核，请更换后重新上传", nil,
			)
			return
		}
	} else {
		handler.logger.Info(
			"image moderation skipped because no provider key is configured",
			"request_id", middleware.GetReqID(request.Context()),
		)
	}

	assetID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate asset id", err)
		return
	}
	objectKey := fmt.Sprintf("users/%s/games/%s/versions/%s/source/%s.png", user.ID, gameID, versionID, assetID)
	uploadedSize, err := handler.putAssetFile(request.Context(), sourceAssetsBucket, objectKey, processed.File.Name(), processed.MIMEType)
	if err != nil {
		handler.internalError(w, request, "store image asset", err)
		return
	}
	kind := "game_source"
	if role == "cover" {
		kind = "game_cover"
	}
	asset := Asset{
		ID: assetID, GameVersionID: versionID, Kind: kind, Role: role, SlotKey: slotKey,
		Bucket: sourceAssetsBucket, ObjectKey: objectKey, MIMEType: processed.MIMEType,
		SizeBytes: uploadedSize, ChecksumSHA256: processed.ChecksumSHA256[:],
		Width: processed.Width, Height: processed.Height, SortOrder: sortOrder, CreatedAt: handler.now().UTC(),
	}
	replaced, err := handler.addAssetRecord(request.Context(), user.ID, gameID, versionID, asset, replaceExisting, maxItems)
	if err != nil {
		_ = handler.removeAssetObject(request.Context(), sourceAssetsBucket, objectKey)
		if errors.Is(err, ErrVersionNotDraft) {
			httpapi.WriteError(w, request, http.StatusConflict, "VERSION_NOT_EDITABLE", "只有草稿版本可以修改素材", nil)
			return
		}
		if errors.Is(err, ErrAssetSlotFull) {
			httpapi.WriteError(w, request, http.StatusConflict, "ASSET_SLOT_FULL", "这个位置上传的图片数量已达到上限", nil)
			return
		}
		if handler.writeRepositoryError(w, request, err) {
			return
		}
		return
	}
	for _, object := range replaced {
		if err := handler.removeAssetObject(request.Context(), object.Bucket, object.Key); err != nil {
			handler.logger.Error("remove replaced asset object", "object_key", object.Key, "error", err)
		}
	}
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventAssetUploaded, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: user.ID, GameID: gameID, GameVersionID: versionID,
		Properties: eventProperties(map[string]any{"kind": asset.Kind, "mimeType": asset.MIMEType, "sizeBytes": asset.SizeBytes}),
	})
	assetURL, err := handler.assetURL(request, asset)
	if err != nil {
		handler.internalError(w, request, "create asset preview URL", err)
		return
	}
	httpapi.WriteData(w, request, http.StatusCreated, assetDTO(asset, assetURL))
}

func formatImageSize(size int64) string {
	return strconv.FormatFloat(float64(size)/(1<<20), 'f', 2, 64) + " MiB"
}

type reorderAssetsRequest struct {
	SlotKey  string   `json:"slotKey"`
	AssetIDs []string `json:"assetIds"`
}

func (handler *Handler) reorderAssets(w http.ResponseWriter, request *http.Request) {
	var body reorderAssetsRequest
	if err := httpapi.DecodeJSON(w, request, &body); err != nil {
		badRequest(w, request, "请求内容不是有效的 JSON")
		return
	}
	body.SlotKey = strings.TrimSpace(body.SlotKey)
	version, err := handler.repository.GetVersion(
		request.Context(), auth.CreatorUser(request).ID,
		chi.URLParam(request, "gameId"), chi.URLParam(request, "versionId"),
	)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	definition, ok := gametemplates.Find(version.TemplateID, version.TemplateVersion)
	if !ok {
		validationError(w, request, map[string]string{"slotKey": "当前模板不支持素材排序"})
		return
	}
	slot, ok := definition.AssetSlot(body.SlotKey)
	if !ok || !slot.Sortable {
		validationError(w, request, map[string]string{"slotKey": "这个素材位置不支持排序"})
		return
	}
	if len(body.AssetIDs) > slot.MaxItems {
		validationError(w, request, map[string]string{"assetIds": "素材数量超过模板上限"})
		return
	}
	err = handler.repository.ReorderAssets(
		request.Context(), auth.CreatorUser(request).ID,
		chi.URLParam(request, "gameId"), chi.URLParam(request, "versionId"),
		body.SlotKey, body.AssetIDs, handler.now().UTC(),
	)
	if errors.Is(err, ErrVersionNotDraft) {
		httpapi.WriteError(w, request, http.StatusConflict, "VERSION_NOT_EDITABLE", "只有草稿版本可以调整素材顺序", nil)
		return
	}
	if errors.Is(err, ErrAssetOrder) {
		httpapi.WriteError(w, request, http.StatusConflict, "ASSET_ORDER_MISMATCH", "素材列表已变化，请刷新后重试", nil)
		return
	}
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func eventProperties(properties map[string]any) json.RawMessage {
	encoded, _ := json.Marshal(properties)
	return encoded
}

func (handler *Handler) listAssets(w http.ResponseWriter, request *http.Request) {
	assets, err := handler.repository.ListAssets(
		request.Context(), auth.CreatorUser(request).ID,
		chi.URLParam(request, "gameId"), chi.URLParam(request, "versionId"),
	)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	items := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		assetURL, err := handler.assetURL(request, asset)
		if err != nil {
			handler.internalError(w, request, "create asset preview URL", err)
			return
		}
		items = append(items, assetDTO(asset, assetURL))
	}
	httpapi.WriteData(w, request, http.StatusOK, map[string]any{"items": items})
}

func (handler *Handler) deleteAsset(w http.ResponseWriter, request *http.Request) {
	userID := auth.CreatorUser(request).ID
	gameID := chi.URLParam(request, "gameId")
	versionID := chi.URLParam(request, "versionId")
	assetID := chi.URLParam(request, "assetId")
	asset, err := handler.repository.GetAsset(request.Context(), userID, gameID, versionID, assetID)
	if handler.writeRepositoryError(w, request, err) {
		return
	}
	removeObject, err := handler.repository.DeleteAsset(
		request.Context(), userID, gameID, versionID, assetID, handler.now().UTC(),
	)
	if err != nil {
		if handler.writeRepositoryError(w, request, err) {
			return
		}
	}
	if removeObject {
		if err := handler.storage.Remove(request.Context(), asset.Bucket, asset.ObjectKey); err != nil {
			handler.logger.Error("remove deleted asset object", "asset_id", asset.ID, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) deleteGame(w http.ResponseWriter, request *http.Request) {
	jobID, err := security.NewID()
	if err != nil {
		handler.internalError(w, request, "generate deletion job id", err)
		return
	}
	if err := handler.repository.RequestDeletion(
		request.Context(), jobID, auth.CreatorUser(request).ID,
		chi.URLParam(request, "gameId"), handler.now().UTC(),
	); handler.writeRepositoryError(w, request, err) {
		return
	}
	httpapi.WriteData(w, request, http.StatusAccepted, map[string]string{
		"deletionJobId": jobID, "message": "游戏已从列表移除，后台正在永久清理相关数据",
	})
}

func (handler *Handler) encryptInput(versionID string, input inputPayload) (EncryptedInput, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return EncryptedInput{}, fmt.Errorf("encode input payload: %w", err)
	}
	ciphertext, nonce, keyVersion, err := handler.cipher.Encrypt(payload, []byte(versionID))
	return EncryptedInput{Ciphertext: ciphertext, Nonce: nonce, KeyVersion: keyVersion}, err
}

func (handler *Handler) decryptInput(version Version) (inputPayload, error) {
	plaintext, err := handler.cipher.Decrypt(
		version.InputPayloadCiphertext, version.InputPayloadNonce, []byte(version.ID), version.EncryptionKeyVersion,
	)
	if err != nil {
		return inputPayload{}, err
	}
	var payload inputPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return inputPayload{}, fmt.Errorf("decode input payload: %w", err)
	}
	if payload.SceneInputs == nil {
		payload.SceneInputs = map[string]string{}
	}
	return payload, nil
}

func (handler *Handler) assetURL(request *http.Request, asset Asset) (string, error) {
	presigned, err := handler.presignAsset(request.Context(), asset.Bucket, asset.ObjectKey, 15*time.Minute)
	if err != nil {
		return "", err
	}
	return presigned.String(), nil
}

func (handler *Handler) versionDTO(version Version, input inputPayload) map[string]any {
	return map[string]any{
		"id": version.ID, "gameId": version.GameID, "versionNumber": version.VersionNumber,
		"status": version.Status, "memoryText": input.MemoryText, "sceneInputs": input.SceneInputs,
		"inputSchemaVersion": version.InputSchemaVersion, "templateId": version.TemplateID,
		"templateVersion": version.TemplateVersion, "assetCount": version.AssetCount,
		"createdAt": version.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt": version.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (handler *Handler) gameDTO(request *http.Request, game Game) (map[string]any, error) {
	var coverPreviewURL any
	if game.CoverAssetID.Valid && game.CoverBucket.Valid && game.CoverObjectKey.Valid {
		presigned, err := handler.storage.PresignedGet(request.Context(), game.CoverBucket.String, game.CoverObjectKey.String, 15*time.Minute)
		if err != nil {
			return nil, err
		}
		coverPreviewURL = presigned.String()
	}
	return map[string]any{
		"id": game.ID, "title": game.Title, "description": nullableString(game.Description),
		"coverAssetId": nullableString(game.CoverAssetID), "coverPreviewUrl": coverPreviewURL,
		"status": game.Status, "currentVersionId": nullableString(game.CurrentVersionID),
		"assetCount": game.AssetCount,
		"createdAt":  game.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":  game.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func assetDTO(asset Asset, previewURL string) map[string]any {
	return map[string]any{
		"id": asset.ID, "role": asset.Role, "slotKey": asset.SlotKey, "mimeType": asset.MIMEType,
		"sizeBytes": asset.SizeBytes, "width": asset.Width, "height": asset.Height,
		"sortOrder": asset.SortOrder, "previewUrl": previewURL,
		"createdAt": asset.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func validateGameMetadata(rawTitle, rawDescription string) (string, sql.NullString, map[string]string) {
	fields := map[string]string{}
	title := strings.TrimSpace(rawTitle)
	description := strings.TrimSpace(rawDescription)
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 120 {
		fields["title"] = "游戏名称应为 1–120 个字符"
	}
	if utf8.RuneCountInString(description) > 500 {
		fields["description"] = "游戏描述不能超过 500 个字符"
	}
	if len(fields) > 0 {
		return "", sql.NullString{}, fields
	}
	return title, sql.NullString{String: description, Valid: description != ""}, nil
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func (handler *Handler) writeRepositoryError(w http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, request, http.StatusNotFound, "GAME_RESOURCE_NOT_FOUND", "游戏或素材不存在", nil)
		return true
	}
	handler.internalError(w, request, "game repository operation", err)
	return true
}

func (handler *Handler) internalError(w http.ResponseWriter, request *http.Request, operation string, err error) {
	handler.logger.Error(operation, "error", err, "request_id", middleware.GetReqID(request.Context()))
	httpapi.WriteError(w, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试", nil)
}

func badRequest(w http.ResponseWriter, request *http.Request, message string) {
	httpapi.WriteError(w, request, http.StatusBadRequest, "INVALID_REQUEST", message, nil)
}

func validationError(w http.ResponseWriter, request *http.Request, fields map[string]string) {
	httpapi.WriteError(w, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "请检查输入内容", fields)
}

func removeTemporary(path string) error {
	if path == "" {
		return nil
	}
	return os.Remove(path)
}
