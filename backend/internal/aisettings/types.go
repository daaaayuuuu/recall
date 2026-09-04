package aisettings

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gamegen/backend/internal/platform/config"
)

const (
	CapabilityText            = "text"
	CapabilityImageModeration = "image_moderation"
	CapabilityImageToImage    = "image_to_image"
)

var (
	ErrVersionConflict = errors.New("AI settings version conflict")
	ErrDynamicDisabled = errors.New("dynamic AI settings are disabled")
)

type TextSettings struct {
	Enabled         bool   `json:"enabled"`
	Provider        string `json:"provider"`
	BaseURL         string `json:"baseUrl"`
	Model           string `json:"model"`
	Timeout         string `json:"timeout"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
}

type ImageModerationSettings struct {
	Enabled         bool   `json:"enabled"`
	Provider        string `json:"provider"`
	BaseURL         string `json:"baseUrl"`
	Model           string `json:"model"`
	Timeout         string `json:"timeout"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
}

type ImageToImageSettings struct {
	Enabled        bool   `json:"enabled"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"baseUrl"`
	Model          string `json:"model"`
	Quality        string `json:"quality"`
	Timeout        string `json:"timeout"`
	MaxInputBytes  int64  `json:"maxInputBytes"`
	MaxOutputBytes int64  `json:"maxOutputBytes"`
}

type Snapshot struct {
	Text            TextSettings            `json:"text"`
	ImageModeration ImageModerationSettings `json:"imageModeration"`
	ImageToImage    ImageToImageSettings    `json:"imageToImage"`
}

type SecretEnvelope struct {
	Ciphertext []byte
	Nonce      []byte
}

type Record struct {
	ID                    string
	Version               int64
	Settings              Snapshot
	TextSecret            SecretEnvelope
	ImageModerationSecret SecretEnvelope
	ImageToImageSecret    SecretEnvelope
	EncryptionKeyVersion  int
	CreatedByAdmin        string
	CreatedAt             time.Time
}

type SaveInput struct {
	ExpectedVersion int64
	Record          Record
	AdminSessionID  string
	AdminUsername   string
	RequestID       string
	AuditID         string
}

type Store interface {
	Current(ctx context.Context) (Record, error)
	Save(ctx context.Context, input SaveInput) error
}

type SecretChange struct {
	Value string
	Clear bool
}

type SecretChanges struct {
	Text            SecretChange
	ImageModeration SecretChange
	ImageToImage    SecretChange
}

type APIKeyStatus struct {
	Configured bool   `json:"configured"`
	Hint       string `json:"hint,omitempty"`
	Source     string `json:"source"`
}

type View struct {
	DynamicEnabled bool        `json:"dynamicEnabled"`
	Version        int64       `json:"version"`
	Source         string      `json:"source"`
	Settings       Snapshot    `json:"settings"`
	APIKeys        APIKeyViews `json:"apiKeys"`
	UpdatedBy      string      `json:"updatedBy,omitempty"`
	UpdatedAt      *time.Time  `json:"updatedAt,omitempty"`
}

type APIKeyViews struct {
	Text            APIKeyStatus `json:"text"`
	ImageModeration APIKeyStatus `json:"imageModeration"`
	ImageToImage    APIKeyStatus `json:"imageToImage"`
}

func snapshotFromConfig(cfg config.AIConfig) Snapshot {
	return Snapshot{
		Text: TextSettings{
			Enabled: strings.TrimSpace(cfg.Text.APIKey) != "", Provider: cfg.Text.Provider,
			BaseURL: cfg.Text.BaseURL, Model: cfg.Text.Model, Timeout: durationString(cfg.Text.TimeoutEvery, cfg.Text.Timeout),
			MaxOutputTokens: cfg.Text.MaxOutputTokens,
		},
		ImageModeration: ImageModerationSettings{
			Enabled: strings.TrimSpace(cfg.ImageModeration.APIKey) != "", Provider: cfg.ImageModeration.Provider,
			BaseURL: cfg.ImageModeration.BaseURL, Model: cfg.ImageModeration.Model,
			Timeout:         durationString(cfg.ImageModeration.TimeoutEvery, cfg.ImageModeration.Timeout),
			MaxOutputTokens: cfg.ImageModeration.MaxOutputTokens,
		},
		ImageToImage: ImageToImageSettings{
			Enabled: strings.TrimSpace(cfg.ImageToImage.APIKey) != "", Provider: cfg.ImageToImage.Provider,
			BaseURL: cfg.ImageToImage.BaseURL, Model: cfg.ImageToImage.Model, Quality: cfg.ImageToImage.Quality,
			Timeout:       durationString(cfg.ImageToImage.TimeoutEvery, cfg.ImageToImage.Timeout),
			MaxInputBytes: cfg.ImageToImage.MaxInputBytes, MaxOutputBytes: cfg.ImageToImage.MaxOutputBytes,
		},
	}
}

func durationString(raw string, parsed time.Duration) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	return parsed.String()
}

func (settings Snapshot) toConfig(secrets plainSecrets) (config.AIConfig, error) {
	textTimeout, err := time.ParseDuration(settings.Text.Timeout)
	if err != nil {
		return config.AIConfig{}, err
	}
	moderationTimeout, err := time.ParseDuration(settings.ImageModeration.Timeout)
	if err != nil {
		return config.AIConfig{}, err
	}
	imageTimeout, err := time.ParseDuration(settings.ImageToImage.Timeout)
	if err != nil {
		return config.AIConfig{}, err
	}
	result := config.AIConfig{
		Text: config.AITextConfig{
			Provider: settings.Text.Provider, BaseURL: settings.Text.BaseURL, Model: settings.Text.Model,
			Timeout: textTimeout, TimeoutEvery: settings.Text.Timeout, MaxOutputTokens: settings.Text.MaxOutputTokens,
		},
		ImageModeration: config.AIImageModerationConfig{
			Provider: settings.ImageModeration.Provider, BaseURL: settings.ImageModeration.BaseURL,
			Model: settings.ImageModeration.Model, Timeout: moderationTimeout,
			TimeoutEvery: settings.ImageModeration.Timeout, MaxOutputTokens: settings.ImageModeration.MaxOutputTokens,
		},
		ImageToImage: config.AIImageToImageConfig{
			Provider: settings.ImageToImage.Provider, BaseURL: settings.ImageToImage.BaseURL,
			Model: settings.ImageToImage.Model, Quality: settings.ImageToImage.Quality,
			Timeout: imageTimeout, TimeoutEvery: settings.ImageToImage.Timeout,
			MaxInputBytes: settings.ImageToImage.MaxInputBytes, MaxOutputBytes: settings.ImageToImage.MaxOutputBytes,
		},
	}
	if settings.Text.Enabled {
		result.Text.APIKey = secrets.Text
	}
	if settings.ImageModeration.Enabled {
		result.ImageModeration.APIKey = secrets.ImageModeration
	}
	if settings.ImageToImage.Enabled {
		result.ImageToImage.APIKey = secrets.ImageToImage
	}
	return result, nil
}

func validateSnapshot(settings Snapshot, secrets plainSecrets, environment string) map[string]string {
	fields := map[string]string{}
	validateDuration(fields, "text.timeout", settings.Text.Timeout)
	validateTokenLimit(fields, "text.maxOutputTokens", settings.Text.MaxOutputTokens)
	validateDuration(fields, "imageModeration.timeout", settings.ImageModeration.Timeout)
	validateTokenLimit(fields, "imageModeration.maxOutputTokens", settings.ImageModeration.MaxOutputTokens)
	validateDuration(fields, "imageToImage.timeout", settings.ImageToImage.Timeout)

	if settings.Text.Enabled {
		validateProvider(fields, "text", settings.Text.Provider, "deepseek")
		validateEndpoint(fields, "text.baseUrl", settings.Text.BaseURL, environment)
		validateModel(fields, "text.model", settings.Text.Model)
		validateSecret(fields, "text.apiKey", secrets.Text)
	}
	if settings.ImageModeration.Enabled {
		validateProvider(fields, "imageModeration", settings.ImageModeration.Provider, "openai-compatible")
		validateEndpoint(fields, "imageModeration.baseUrl", settings.ImageModeration.BaseURL, environment)
		validateModel(fields, "imageModeration.model", settings.ImageModeration.Model)
		validateSecret(fields, "imageModeration.apiKey", secrets.ImageModeration)
	}
	if settings.ImageToImage.Enabled {
		validateProvider(fields, "imageToImage", settings.ImageToImage.Provider, "openai-compatible")
		validateEndpoint(fields, "imageToImage.baseUrl", settings.ImageToImage.BaseURL, environment)
		validateModel(fields, "imageToImage.model", settings.ImageToImage.Model)
		validateSecret(fields, "imageToImage.apiKey", secrets.ImageToImage)
	}
	if settings.ImageToImage.MaxInputBytes < 1 || settings.ImageToImage.MaxInputBytes > 100<<20 {
		fields["imageToImage.maxInputBytes"] = "必须在 1 字节到 100 MiB 之间"
	}
	if settings.ImageToImage.MaxOutputBytes < 1 || settings.ImageToImage.MaxOutputBytes > 100<<20 {
		fields["imageToImage.maxOutputBytes"] = "必须在 1 字节到 100 MiB 之间"
	}
	switch settings.ImageToImage.Quality {
	case "auto", "low", "medium", "high":
	default:
		fields["imageToImage.quality"] = "必须是 auto、low、medium 或 high"
	}
	return fields
}

func validateDuration(fields map[string]string, field, value string) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration <= 0 || duration > 10*time.Minute {
		fields[field] = "必须是大于 0 且不超过 10 分钟的时长，例如 30s 或 3m"
	}
}

func validateTokenLimit(fields map[string]string, field string, value int) {
	if value < 1 || value > 100_000 {
		fields[field] = "必须在 1–100000 之间"
	}
}

func validateProvider(fields map[string]string, prefix, value, expected string) {
	if strings.TrimSpace(value) != expected {
		fields[prefix+".provider"] = fmt.Sprintf("当前仅支持 %s", expected)
	}
}

func validateEndpoint(fields map[string]string, field, value, environment string) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		fields[field] = "请输入有效的 HTTP(S) 服务地址，且地址中不能包含凭据"
		return
	}
	if environment == "production" && parsed.Scheme != "https" {
		fields[field] = "生产环境必须使用 HTTPS"
	}
	if len(value) > 2048 {
		fields[field] = "服务地址不能超过 2048 个字符"
	}
}

func validateModel(fields map[string]string, field, value string) {
	length := len(strings.TrimSpace(value))
	if length == 0 || length > 256 {
		fields[field] = "模型名称不能为空且不能超过 256 个字符"
	}
}

func validateSecret(fields map[string]string, field, value string) {
	length := len(strings.TrimSpace(value))
	if length == 0 || length > 4096 {
		fields[field] = "启用后必须配置不超过 4096 个字符的 API Key"
	}
}

func normalizeSnapshot(settings Snapshot) Snapshot {
	settings.Text.Provider = strings.TrimSpace(settings.Text.Provider)
	settings.Text.BaseURL = strings.TrimRight(strings.TrimSpace(settings.Text.BaseURL), "/")
	settings.Text.Model = strings.TrimSpace(settings.Text.Model)
	settings.Text.Timeout = strings.TrimSpace(settings.Text.Timeout)
	settings.ImageModeration.Provider = strings.TrimSpace(settings.ImageModeration.Provider)
	settings.ImageModeration.BaseURL = strings.TrimRight(strings.TrimSpace(settings.ImageModeration.BaseURL), "/")
	settings.ImageModeration.Model = strings.TrimSpace(settings.ImageModeration.Model)
	settings.ImageModeration.Timeout = strings.TrimSpace(settings.ImageModeration.Timeout)
	settings.ImageToImage.Provider = strings.TrimSpace(settings.ImageToImage.Provider)
	settings.ImageToImage.BaseURL = strings.TrimRight(strings.TrimSpace(settings.ImageToImage.BaseURL), "/")
	settings.ImageToImage.Model = strings.TrimSpace(settings.ImageToImage.Model)
	settings.ImageToImage.Quality = strings.TrimSpace(settings.ImageToImage.Quality)
	settings.ImageToImage.Timeout = strings.TrimSpace(settings.ImageToImage.Timeout)
	return settings
}

type plainSecrets struct {
	Text            string
	ImageModeration string
	ImageToImage    string
}

func secretStatus(value, source string) APIKeyStatus {
	value = strings.TrimSpace(value)
	status := APIKeyStatus{Configured: value != "", Source: source}
	if len(value) <= 4 {
		if value != "" {
			status.Hint = "••••"
		}
		return status
	}
	status.Hint = "••••" + value[len(value)-4:]
	return status
}
