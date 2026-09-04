package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	HTTP       HTTPConfig       `yaml:"http"`
	Worker     WorkerConfig     `yaml:"worker"`
	Database   DatabaseConfig   `yaml:"database"`
	Storage    StorageConfig    `yaml:"storage"`
	Admin      AdminConfig      `yaml:"admin"`
	Encryption EncryptionConfig `yaml:"encryption"`
	Uploads    UploadConfig     `yaml:"uploads"`
	Generation GenerationConfig `yaml:"generation"`
	AI         AIConfig         `yaml:"ai"`
	AISettings DynamicAIConfig  `yaml:"ai_settings"`
	Sharing    SharingConfig    `yaml:"sharing"`
	Log        LogConfig        `yaml:"log"`
}

type AppConfig struct {
	Environment string `yaml:"environment"`
	AppBaseURL  string `yaml:"app_base_url"`
	PlayBaseURL string `yaml:"play_base_url"`
	Surface     string `yaml:"surface"`
}

type HTTPConfig struct {
	Address           string `yaml:"address"`
	StaticDir         string `yaml:"static_dir"`
	TrustProxyHeaders bool   `yaml:"trust_proxy_headers"`
}

type WorkerConfig struct {
	HealthAddress string        `yaml:"health_address"`
	PollInterval  time.Duration `yaml:"-"`
	PollEvery     string        `yaml:"poll_interval"`
}

type DatabaseConfig struct {
	DSN             string `yaml:"dsn"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
}

type StorageConfig struct {
	Endpoint       string `yaml:"endpoint"`
	PublicEndpoint string `yaml:"public_endpoint"`
	AccessKey      string `yaml:"access_key"`
	SecretKey      string `yaml:"secret_key"`
	Region         string `yaml:"region"`
	UseSSL         bool   `yaml:"use_ssl"`
	PublicUseSSL   bool   `yaml:"public_use_ssl"`
}

type AdminConfig struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type EncryptionConfig struct {
	ActiveKeyVersion int    `yaml:"active_key_version"`
	ContentKeyV1     string `yaml:"content_key_v1"`
	ShareKeyV1       string `yaml:"share_key_v1"`
}

type UploadConfig struct {
	MaxConcurrentPerUser  int `yaml:"max_concurrent_per_user"`
	StreamBufferBytes     int `yaml:"stream_buffer_bytes"`
	StagingObjectTTLHours int `yaml:"staging_object_ttl_hours"`
}

type GenerationConfig struct {
	LeaseDuration time.Duration `yaml:"-"`
	LeaseEvery    string        `yaml:"lease_duration"`
	MaxExecutions int           `yaml:"max_executions"`
}

type AIConfig struct {
	Text            AITextConfig            `yaml:"text"`
	ImageModeration AIImageModerationConfig `yaml:"image_moderation"`
	ImageToImage    AIImageToImageConfig    `yaml:"image_to_image"`
}

type AITextConfig struct {
	Provider        string        `yaml:"provider"`
	BaseURL         string        `yaml:"base_url"`
	APIKey          string        `yaml:"api_key"`
	Model           string        `yaml:"model"`
	Timeout         time.Duration `yaml:"-"`
	TimeoutEvery    string        `yaml:"timeout"`
	MaxOutputTokens int           `yaml:"max_output_tokens"`
}

type AIImageModerationConfig struct {
	Provider        string        `yaml:"provider"`
	BaseURL         string        `yaml:"base_url"`
	APIKey          string        `yaml:"api_key"`
	Model           string        `yaml:"model"`
	Timeout         time.Duration `yaml:"-"`
	TimeoutEvery    string        `yaml:"timeout"`
	MaxOutputTokens int           `yaml:"max_output_tokens"`
}

type AIImageToImageConfig struct {
	Provider       string        `yaml:"provider"`
	BaseURL        string        `yaml:"base_url"`
	APIKey         string        `yaml:"api_key"`
	Model          string        `yaml:"model"`
	Quality        string        `yaml:"quality"`
	Timeout        time.Duration `yaml:"-"`
	TimeoutEvery   string        `yaml:"timeout"`
	MaxInputBytes  int64         `yaml:"max_input_bytes"`
	MaxOutputBytes int64         `yaml:"max_output_bytes"`
}

type DynamicAIConfig struct {
	Enabled              bool          `yaml:"enabled"`
	EncryptionKeyV1      string        `yaml:"encryption_key_v1"`
	EncryptionKeyVersion int           `yaml:"encryption_key_version"`
	RefreshInterval      time.Duration `yaml:"-"`
	RefreshEvery         string        `yaml:"refresh_interval"`
}

type SharingConfig struct {
	MaxLinkLifetimeDays int `yaml:"max_link_lifetime_days"`
	PlaySessionMinutes  int `yaml:"play_session_minutes"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

func Load() (Config, error) {
	cfg := defaults()
	path := os.Getenv("GAMEGEN_CONFIG")
	if path == "" {
		path = "config/config.local.yaml"
	}

	data, err := os.ReadFile(path)
	if err == nil {
		expanded := os.ExpandEnv(string(data))
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
			return Config{}, fmt.Errorf("decode %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) || os.Getenv("GAMEGEN_CONFIG") != "" {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	if err := applyEnvironment(&cfg); err != nil {
		return Config{}, err
	}
	if cfg.Worker.PollEvery != "" {
		cfg.Worker.PollInterval, err = time.ParseDuration(cfg.Worker.PollEvery)
		if err != nil {
			return Config{}, fmt.Errorf("worker.poll_interval: %w", err)
		}
	}
	if cfg.Generation.LeaseEvery != "" {
		cfg.Generation.LeaseDuration, err = time.ParseDuration(cfg.Generation.LeaseEvery)
		if err != nil {
			return Config{}, fmt.Errorf("generation.lease_duration: %w", err)
		}
	}
	if cfg.AI.Text.TimeoutEvery != "" {
		cfg.AI.Text.Timeout, err = time.ParseDuration(cfg.AI.Text.TimeoutEvery)
		if err != nil {
			return Config{}, fmt.Errorf("ai.text.timeout: %w", err)
		}
	}
	if cfg.AI.ImageModeration.TimeoutEvery != "" {
		cfg.AI.ImageModeration.Timeout, err = time.ParseDuration(cfg.AI.ImageModeration.TimeoutEvery)
		if err != nil {
			return Config{}, fmt.Errorf("ai.image_moderation.timeout: %w", err)
		}
	}
	if cfg.AI.ImageToImage.TimeoutEvery != "" {
		cfg.AI.ImageToImage.Timeout, err = time.ParseDuration(cfg.AI.ImageToImage.TimeoutEvery)
		if err != nil {
			return Config{}, fmt.Errorf("ai.image_to_image.timeout: %w", err)
		}
	}
	if cfg.AISettings.RefreshEvery != "" {
		cfg.AISettings.RefreshInterval, err = time.ParseDuration(cfg.AISettings.RefreshEvery)
		if err != nil {
			return Config{}, fmt.Errorf("ai_settings.refresh_interval: %w", err)
		}
	}
	return cfg, nil
}

func (c Config) ValidateDatabase() error {
	if strings.TrimSpace(c.Database.DSN) == "" {
		return errors.New("MYSQL_DSN is required")
	}
	return nil
}

func (c Config) ValidateRuntime() error {
	return c.ValidateAPI()
}

func (c Config) ValidateAPI() error {
	if err := c.validateRuntimeBase(); err != nil {
		return err
	}
	if c.App.Surface == "play" {
		return nil
	}
	if err := c.validateDynamicAI(); err != nil {
		return err
	}
	if err := c.validateTextAI(); err != nil {
		return err
	}
	if err := c.validateImageModeration(); err != nil {
		return err
	}
	if c.App.Environment == "production" {
		if c.Admin.PasswordHash == "" || c.Encryption.ContentKeyV1 == "" || c.Encryption.ShareKeyV1 == "" {
			return errors.New("production app/admin and encryption secrets are required")
		}
	}
	return nil
}

func (c Config) validateImageModeration() error {
	moderation := c.AI.ImageModeration
	if strings.TrimSpace(moderation.APIKey) == "" {
		return nil
	}
	provider := strings.TrimSpace(c.AI.ImageModeration.Provider)
	if provider != "openai-compatible" {
		return errors.New("AI_IMAGE_MODERATION_PROVIDER must be openai-compatible")
	}
	if strings.TrimSpace(moderation.BaseURL) == "" || strings.TrimSpace(moderation.APIKey) == "" || strings.TrimSpace(moderation.Model) == "" {
		return errors.New("AI_IMAGE_MODERATION_BASE_URL, AI_IMAGE_MODERATION_API_KEY, and AI_IMAGE_MODERATION_MODEL are required")
	}
	if c.App.Environment == "production" && !strings.HasPrefix(strings.TrimSpace(moderation.BaseURL), "https://") {
		return errors.New("AI_IMAGE_MODERATION_BASE_URL must use HTTPS in production")
	}
	if moderation.Timeout <= 0 || moderation.MaxOutputTokens < 1 {
		return errors.New("AI_IMAGE_MODERATION_TIMEOUT and AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS are invalid")
	}
	return nil
}

func (c Config) validateTextAI() error {
	if strings.TrimSpace(c.AI.Text.APIKey) == "" {
		return nil
	}
	if strings.TrimSpace(c.AI.Text.Provider) != "deepseek" {
		return errors.New("AI_TEXT_PROVIDER must be deepseek when DEEPSEEK_API_KEY is configured")
	}
	if strings.TrimSpace(c.AI.Text.BaseURL) == "" || strings.TrimSpace(c.AI.Text.Model) == "" {
		return errors.New("AI_TEXT_BASE_URL and AI_TEXT_MODEL are required when DEEPSEEK_API_KEY is configured")
	}
	if c.AI.Text.Timeout <= 0 || c.AI.Text.MaxOutputTokens < 1 {
		return errors.New("AI_TEXT_TIMEOUT and AI_TEXT_MAX_OUTPUT_TOKENS are invalid")
	}
	return nil
}

func (c Config) ValidateWorker() error {
	if err := c.validateRuntimeBase(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Encryption.ContentKeyV1) == "" {
		return errors.New("CONTENT_ENCRYPTION_KEY_V1 is required by the worker")
	}
	if err := c.validateDynamicAI(); err != nil {
		return err
	}
	if err := c.validateImageToImage(); err != nil {
		return err
	}
	return nil
}

func (c Config) validateDynamicAI() error {
	if !c.AISettings.Enabled {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c.AISettings.EncryptionKeyV1))
	if err != nil || len(key) != 32 {
		return errors.New("AI_CONFIG_ENCRYPTION_KEY_V1 must be Base64-encoded 32 bytes when dynamic AI config is enabled")
	}
	if c.AISettings.EncryptionKeyVersion < 1 {
		return errors.New("AI config encryption key version must be positive")
	}
	if c.AISettings.RefreshInterval <= 0 || c.AISettings.RefreshInterval > time.Minute {
		return errors.New("AI_CONFIG_REFRESH_INTERVAL must be greater than zero and at most 1m")
	}
	return nil
}

func (c Config) validateImageToImage() error {
	image := c.AI.ImageToImage
	if image.MaxInputBytes < 1 || image.MaxInputBytes > 100<<20 || image.MaxOutputBytes < 1 || image.MaxOutputBytes > 100<<20 {
		return errors.New("AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES and AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES must be between 1 byte and 100 MiB")
	}
	switch image.Quality {
	case "auto", "low", "medium", "high":
	default:
		return errors.New("AI_IMAGE_TO_IMAGE_QUALITY must be auto, low, medium, or high")
	}
	if strings.TrimSpace(image.APIKey) == "" {
		return nil
	}
	provider := strings.TrimSpace(c.AI.ImageToImage.Provider)
	if provider != "openai-compatible" {
		return errors.New("AI_IMAGE_TO_IMAGE_PROVIDER must be openai-compatible")
	}
	if strings.TrimSpace(image.BaseURL) == "" || strings.TrimSpace(image.APIKey) == "" || strings.TrimSpace(image.Model) == "" {
		return errors.New("AI_IMAGE_TO_IMAGE_BASE_URL, AI_IMAGE_TO_IMAGE_API_KEY, and AI_IMAGE_TO_IMAGE_MODEL are required")
	}
	if c.App.Environment == "production" && !strings.HasPrefix(strings.TrimSpace(image.BaseURL), "https://") {
		return errors.New("AI_IMAGE_TO_IMAGE_BASE_URL must use HTTPS in production")
	}
	if image.Timeout <= 0 {
		return errors.New("AI_IMAGE_TO_IMAGE_TIMEOUT is invalid")
	}
	return nil
}

func (c Config) validateRuntimeBase() error {
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Storage.Endpoint) == "" {
		return errors.New("MINIO_ENDPOINT is required")
	}
	if strings.TrimSpace(c.Storage.PublicEndpoint) == "" {
		return errors.New("MINIO_PUBLIC_ENDPOINT is required")
	}
	if strings.TrimSpace(c.Storage.AccessKey) == "" || strings.TrimSpace(c.Storage.SecretKey) == "" {
		return errors.New("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required")
	}
	if strings.TrimSpace(c.Storage.Region) == "" {
		return errors.New("MINIO_REGION is required")
	}
	if c.App.Environment != "development" && c.App.Environment != "test" && c.App.Environment != "production" {
		return fmt.Errorf("unsupported app environment %q", c.App.Environment)
	}
	if c.App.Surface != "all" && c.App.Surface != "app" && c.App.Surface != "play" {
		return fmt.Errorf("unsupported SERVICE_SURFACE %q", c.App.Surface)
	}
	if c.App.Environment == "production" {
		if !strings.HasPrefix(c.App.AppBaseURL, "https://") || !strings.HasPrefix(c.App.PlayBaseURL, "https://") {
			return errors.New("production app and play URLs must use HTTPS")
		}
		if !c.Storage.PublicUseSSL {
			return errors.New("production public object storage endpoint must use HTTPS")
		}
	}
	if c.Generation.LeaseDuration <= 0 || c.Generation.MaxExecutions < 1 {
		return errors.New("generation lease duration and max executions are invalid")
	}
	if c.Sharing.MaxLinkLifetimeDays < 1 || c.Sharing.MaxLinkLifetimeDays > 90 || c.Sharing.PlaySessionMinutes != 30 {
		return errors.New("sharing max lifetime must be 1-90 days and play sessions must be 30 minutes")
	}
	return nil
}

func defaults() Config {
	return Config{
		App: AppConfig{
			Environment: "development",
			AppBaseURL:  "http://127.0.0.1:5173",
			PlayBaseURL: "http://127.0.0.1:5173",
			Surface:     "all",
		},
		HTTP:   HTTPConfig{Address: ":8080"},
		Worker: WorkerConfig{HealthAddress: ":8081", PollInterval: 5 * time.Second, PollEvery: "5s"},
		Database: DatabaseConfig{
			MaxOpenConns:    20,
			MaxIdleConns:    10,
			ConnMaxLifetime: "5m",
		},
		Storage:    StorageConfig{Region: "us-east-1"},
		Encryption: EncryptionConfig{ActiveKeyVersion: 1},
		Uploads:    UploadConfig{MaxConcurrentPerUser: 3, StreamBufferBytes: 1048576, StagingObjectTTLHours: 24},
		Generation: GenerationConfig{LeaseDuration: 60 * time.Second, LeaseEvery: "60s", MaxExecutions: 3},
		AI: AIConfig{
			Text: AITextConfig{
				Provider: "deepseek", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash",
				Timeout: 30 * time.Second, TimeoutEvery: "30s", MaxOutputTokens: 2000,
			},
			ImageModeration: AIImageModerationConfig{
				Timeout:      20 * time.Second,
				TimeoutEvery: "20s", MaxOutputTokens: 300,
			},
			ImageToImage: AIImageToImageConfig{
				Quality: "medium", Timeout: 3 * time.Minute,
				TimeoutEvery: "3m", MaxInputBytes: 25 << 20, MaxOutputBytes: 25 << 20,
			},
		},
		AISettings: DynamicAIConfig{
			EncryptionKeyVersion: 1,
			RefreshInterval:      2 * time.Second,
			RefreshEvery:         "2s",
		},
		Sharing: SharingConfig{MaxLinkLifetimeDays: 90, PlaySessionMinutes: 30},
		Log:     LogConfig{Level: "info"},
	}
}

func applyEnvironment(cfg *Config) error {
	setString(&cfg.App.Environment, "APP_ENVIRONMENT")
	setString(&cfg.App.AppBaseURL, "APP_BASE_URL")
	setString(&cfg.App.PlayBaseURL, "PLAY_BASE_URL")
	setString(&cfg.App.Surface, "SERVICE_SURFACE")
	setString(&cfg.HTTP.Address, "HTTP_ADDRESS")
	setString(&cfg.HTTP.StaticDir, "WEB_STATIC_DIR")
	setString(&cfg.Worker.HealthAddress, "WORKER_HEALTH_ADDRESS")
	setString(&cfg.Worker.PollEvery, "WORKER_POLL_INTERVAL")
	setString(&cfg.Database.DSN, "MYSQL_DSN")
	setString(&cfg.Storage.Endpoint, "MINIO_ENDPOINT")
	setString(&cfg.Storage.PublicEndpoint, "MINIO_PUBLIC_ENDPOINT")
	setString(&cfg.Storage.AccessKey, "MINIO_ACCESS_KEY")
	setString(&cfg.Storage.SecretKey, "MINIO_SECRET_KEY")
	setString(&cfg.Storage.Region, "MINIO_REGION")
	setString(&cfg.Admin.Username, "ADMIN_USERNAME")
	setString(&cfg.Admin.PasswordHash, "ADMIN_PASSWORD_HASH")
	setString(&cfg.Encryption.ContentKeyV1, "CONTENT_ENCRYPTION_KEY_V1")
	setString(&cfg.Encryption.ShareKeyV1, "SHARE_ENCRYPTION_KEY_V1")
	setString(&cfg.Generation.LeaseEvery, "GENERATION_LEASE_DURATION")
	setString(&cfg.AI.Text.Provider, "AI_TEXT_PROVIDER")
	setString(&cfg.AI.Text.BaseURL, "AI_TEXT_BASE_URL")
	setString(&cfg.AI.Text.APIKey, "DEEPSEEK_API_KEY")
	setString(&cfg.AI.Text.Model, "AI_TEXT_MODEL")
	setString(&cfg.AI.Text.TimeoutEvery, "AI_TEXT_TIMEOUT")
	setString(&cfg.AI.ImageModeration.Provider, "AI_IMAGE_MODERATION_PROVIDER")
	setString(&cfg.AI.ImageModeration.BaseURL, "AI_IMAGE_MODERATION_BASE_URL")
	setString(&cfg.AI.ImageModeration.APIKey, "AI_IMAGE_MODERATION_API_KEY")
	setString(&cfg.AI.ImageModeration.Model, "AI_IMAGE_MODERATION_MODEL")
	setString(&cfg.AI.ImageModeration.TimeoutEvery, "AI_IMAGE_MODERATION_TIMEOUT")
	setString(&cfg.AI.ImageToImage.Provider, "AI_IMAGE_TO_IMAGE_PROVIDER")
	setString(&cfg.AI.ImageToImage.BaseURL, "AI_IMAGE_TO_IMAGE_BASE_URL")
	setString(&cfg.AI.ImageToImage.APIKey, "AI_IMAGE_TO_IMAGE_API_KEY")
	setString(&cfg.AI.ImageToImage.Model, "AI_IMAGE_TO_IMAGE_MODEL")
	setString(&cfg.AI.ImageToImage.Quality, "AI_IMAGE_TO_IMAGE_QUALITY")
	setString(&cfg.AI.ImageToImage.TimeoutEvery, "AI_IMAGE_TO_IMAGE_TIMEOUT")
	setString(&cfg.AISettings.EncryptionKeyV1, "AI_CONFIG_ENCRYPTION_KEY_V1")
	setString(&cfg.AISettings.RefreshEvery, "AI_CONFIG_REFRESH_INTERVAL")
	setString(&cfg.Log.Level, "LOG_LEVEL")

	if err := applyPlatformPort(cfg); err != nil {
		return err
	}
	if err := setBool(&cfg.HTTP.TrustProxyHeaders, "TRUST_PROXY_HEADERS"); err != nil {
		return err
	}
	if err := setBool(&cfg.AISettings.Enabled, "DYNAMIC_AI_CONFIG_ENABLED"); err != nil {
		return err
	}

	if err := setBool(&cfg.Storage.UseSSL, "MINIO_USE_SSL"); err != nil {
		return err
	}
	if cfg.Storage.PublicEndpoint == "" {
		cfg.Storage.PublicEndpoint = cfg.Storage.Endpoint
		cfg.Storage.PublicUseSSL = cfg.Storage.UseSSL
	}
	if err := setBool(&cfg.Storage.PublicUseSSL, "MINIO_PUBLIC_USE_SSL"); err != nil {
		return err
	}
	if err := setInt(&cfg.Generation.MaxExecutions, "GENERATION_MAX_EXECUTIONS"); err != nil {
		return err
	}
	if err := setInt(&cfg.AI.Text.MaxOutputTokens, "AI_TEXT_MAX_OUTPUT_TOKENS"); err != nil {
		return err
	}
	if err := setInt(&cfg.AI.ImageModeration.MaxOutputTokens, "AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS"); err != nil {
		return err
	}
	if err := setInt64(&cfg.AI.ImageToImage.MaxInputBytes, "AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES"); err != nil {
		return err
	}
	if err := setInt64(&cfg.AI.ImageToImage.MaxOutputBytes, "AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES"); err != nil {
		return err
	}
	return nil
}

func applyPlatformPort(cfg *Config) error {
	value, ok := os.LookupEnv("PORT")
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("PORT must be an integer between 1 and 65535")
	}
	address := ":" + strconv.Itoa(port)
	if _, explicit := os.LookupEnv("HTTP_ADDRESS"); !explicit {
		cfg.HTTP.Address = address
	}
	if _, explicit := os.LookupEnv("WORKER_HEALTH_ADDRESS"); !explicit {
		cfg.Worker.HealthAddress = address
	}
	return nil
}

func setString(target *string, key string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func setInt(target *int, key string) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s must be an integer: %w", key, err)
	}
	*target = parsed
	return nil
}

func setInt64(target *int64, key string) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("%s must be an integer: %w", key, err)
	}
	*target = parsed
	return nil
}

func setBool(target *bool, key string) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	*target = parsed
	return nil
}
