package config

import (
	"os"
	"testing"
	"time"
)

func TestEnvironmentOverridesDefaults(t *testing.T) {
	t.Setenv("GAMEGEN_CONFIG", "")
	t.Setenv("APP_ENVIRONMENT", "test")
	t.Setenv("MYSQL_DSN", "user:password@tcp(localhost:3306)/gamegen")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("MINIO_ACCESS_KEY", "access")
	t.Setenv("MINIO_SECRET_KEY", "secret")
	t.Setenv("MINIO_USE_SSL", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatal(err)
	}
	if cfg.App.Environment != "test" || !cfg.Storage.UseSSL {
		t.Fatalf("environment overrides were not applied: %#v", cfg)
	}
	if cfg.Storage.PublicEndpoint != "localhost:9000" || !cfg.Storage.PublicUseSSL || cfg.Storage.Region != "us-east-1" {
		t.Fatalf("unexpected public storage defaults: %#v", cfg.Storage)
	}
	if cfg.Sharing.MaxLinkLifetimeDays != 90 || cfg.Sharing.PlaySessionMinutes != 30 {
		t.Fatalf("unexpected sharing defaults: %#v", cfg.Sharing)
	}
	if cfg.Generation.LeaseDuration != 60*time.Second || cfg.Generation.MaxExecutions != 3 {
		t.Fatalf("unexpected generation defaults: %#v", cfg.Generation)
	}
}

func TestTextAIUsesDeepSeekDefaultsAndEnvironmentOverrides(t *testing.T) {
	t.Setenv("GAMEGEN_CONFIG", "")
	t.Setenv("DEEPSEEK_API_KEY", "test-secret")
	t.Setenv("AI_TEXT_MODEL", "deepseek-v4-flash")
	t.Setenv("AI_TEXT_TIMEOUT", "12s")
	t.Setenv("AI_TEXT_MAX_OUTPUT_TOKENS", "1500")
	t.Setenv("AI_IMAGE_TO_IMAGE_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Text.Provider != "deepseek" || cfg.AI.Text.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("unexpected DeepSeek defaults: %#v", cfg.AI.Text)
	}
	if cfg.AI.Text.APIKey != "test-secret" || cfg.AI.Text.Model != "deepseek-v4-flash" {
		t.Fatalf("text model environment was not applied: %#v", cfg.AI.Text)
	}
	if cfg.AI.Text.Timeout != 12*time.Second || cfg.AI.Text.MaxOutputTokens != 1500 {
		t.Fatalf("unexpected text model limits: %#v", cfg.AI.Text)
	}
	if cfg.AI.ImageToImage.Provider != "" || cfg.AI.ImageToImage.Model != "" {
		t.Fatal("unconfigured image generation must remain optional")
	}
}

func TestImageToImageConfigurationIsValidatedForWorker(t *testing.T) {
	cfg := defaults()
	if err := cfg.validateImageToImage(); err != nil {
		t.Fatalf("expected local development fallback to pass: %v", err)
	}
	cfg.App.Environment = "production"
	if err := cfg.validateImageToImage(); err != nil {
		t.Fatalf("expected production to allow image passthrough without a provider key: %v", err)
	}
	cfg.AI.ImageToImage.Provider = "openai-compatible"
	if err := cfg.validateImageToImage(); err != nil {
		t.Fatalf("expected a selected provider without a key to remain disabled: %v", err)
	}
	cfg.AI.ImageToImage.APIKey = "secret"
	if err := cfg.validateImageToImage(); err == nil {
		t.Fatal("expected a provider key to require the remaining image-to-image configuration")
	}
	cfg.AI.ImageToImage = AIImageToImageConfig{
		Provider: "openai-compatible", BaseURL: "https://image.example/v1", APIKey: "secret", Model: "image-model",
		Quality: "high", Timeout: 2 * time.Minute, MaxInputBytes: 25 << 20, MaxOutputBytes: 25 << 20,
	}
	if err := cfg.validateImageToImage(); err != nil {
		t.Fatalf("expected complete image-to-image configuration to pass: %v", err)
	}
	cfg.AI.ImageToImage.Provider = "development-copy"
	if err := cfg.validateImageToImage(); err == nil {
		t.Fatal("expected a configured key to reject the legacy passthrough provider")
	}
	cfg.AI.ImageToImage.Provider = "openai-compatible"
	cfg.AI.ImageToImage.BaseURL = "http://image.example/v1"
	if err := cfg.validateImageToImage(); err == nil {
		t.Fatal("expected production image-to-image endpoint to require HTTPS")
	}
}

func TestImageToImageEnvironmentOverridesAreLoaded(t *testing.T) {
	t.Setenv("GAMEGEN_CONFIG", "")
	t.Setenv("AI_IMAGE_TO_IMAGE_PROVIDER", "openai-compatible")
	t.Setenv("AI_IMAGE_TO_IMAGE_BASE_URL", "https://image.example/v1")
	t.Setenv("AI_IMAGE_TO_IMAGE_API_KEY", "test-secret")
	t.Setenv("AI_IMAGE_TO_IMAGE_MODEL", "image-model")
	t.Setenv("AI_IMAGE_TO_IMAGE_QUALITY", "high")
	t.Setenv("AI_IMAGE_TO_IMAGE_TIMEOUT", "2m")
	t.Setenv("AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES", "1024")
	t.Setenv("AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES", "2048")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	image := cfg.AI.ImageToImage
	if image.Provider != "openai-compatible" || image.BaseURL != "https://image.example/v1" || image.APIKey != "test-secret" ||
		image.Model != "image-model" || image.Quality != "high" || image.Timeout != 2*time.Minute ||
		image.MaxInputBytes != 1024 || image.MaxOutputBytes != 2048 {
		t.Fatalf("image-to-image environment was not applied: %#v", image)
	}
}

func TestTextAIIsOptionalButValidatedWhenConfigured(t *testing.T) {
	cfg := defaults()
	if err := cfg.validateTextAI(); err != nil {
		t.Fatalf("expected an empty API key to keep text AI optional: %v", err)
	}

	cfg.AI.Text.APIKey = "test-secret"
	cfg.AI.Text.Provider = "unsupported"
	if err := cfg.validateTextAI(); err == nil {
		t.Fatal("expected a configured unsupported provider to be rejected")
	}
}

func TestImageModerationIsOptionalButValidatedWhenConfigured(t *testing.T) {
	cfg := defaults()
	if err := cfg.validateImageModeration(); err != nil {
		t.Fatalf("expected unconfigured moderation to be optional: %v", err)
	}

	cfg.App.Environment = "production"
	if err := cfg.validateImageModeration(); err != nil {
		t.Fatalf("expected production to allow skipped moderation without a provider key: %v", err)
	}
	cfg.AI.ImageModeration.Provider = "openai-compatible"
	if err := cfg.validateImageModeration(); err != nil {
		t.Fatalf("expected a selected moderation provider without a key to remain disabled: %v", err)
	}
	cfg.AI.ImageModeration.APIKey = "secret"
	if err := cfg.validateImageModeration(); err == nil {
		t.Fatal("expected a moderation key to require the remaining provider configuration")
	}
	cfg.AI.ImageModeration = AIImageModerationConfig{
		Provider: "openai-compatible", BaseURL: "https://vision.example/v1",
		APIKey: "secret", Model: "vision-model", Timeout: 20 * time.Second, MaxOutputTokens: 300,
	}
	if err := cfg.validateImageModeration(); err != nil {
		t.Fatalf("expected complete production moderation configuration: %v", err)
	}
	cfg.AI.ImageModeration.Provider = "development-allow"
	if err := cfg.validateImageModeration(); err == nil {
		t.Fatal("expected a configured key to reject the legacy allow provider")
	}
	cfg.AI.ImageModeration.Provider = "openai-compatible"
	cfg.AI.ImageModeration.BaseURL = "http://vision.example/v1"
	if err := cfg.validateImageModeration(); err == nil {
		t.Fatal("expected production image moderation endpoint to require HTTPS")
	}
}

func TestImageModerationEnvironmentOverridesAreLoaded(t *testing.T) {
	t.Setenv("GAMEGEN_CONFIG", "")
	t.Setenv("AI_IMAGE_MODERATION_PROVIDER", "openai-compatible")
	t.Setenv("AI_IMAGE_MODERATION_BASE_URL", "https://vision.example/v1")
	t.Setenv("AI_IMAGE_MODERATION_API_KEY", "test-secret")
	t.Setenv("AI_IMAGE_MODERATION_MODEL", "vision-model")
	t.Setenv("AI_IMAGE_MODERATION_TIMEOUT", "9s")
	t.Setenv("AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS", "180")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	moderation := cfg.AI.ImageModeration
	if moderation.Provider != "openai-compatible" || moderation.BaseURL != "https://vision.example/v1" ||
		moderation.APIKey != "test-secret" || moderation.Model != "vision-model" ||
		moderation.Timeout != 9*time.Second || moderation.MaxOutputTokens != 180 {
		t.Fatalf("image moderation environment was not applied: %#v", moderation)
	}
}

func TestPlatformPortConfiguresAPIAndWorkerUnlessExplicit(t *testing.T) {
	withoutEnvironment(t, "HTTP_ADDRESS")
	withoutEnvironment(t, "WORKER_HEALTH_ADDRESS")
	t.Setenv("GAMEGEN_CONFIG", "")
	t.Setenv("PORT", "4567")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Address != ":4567" || cfg.Worker.HealthAddress != ":4567" {
		t.Fatalf("expected platform port on both processes, got HTTP=%q worker=%q", cfg.HTTP.Address, cfg.Worker.HealthAddress)
	}

	t.Setenv("HTTP_ADDRESS", ":9000")
	t.Setenv("WORKER_HEALTH_ADDRESS", ":9001")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Address != ":9000" || cfg.Worker.HealthAddress != ":9001" {
		t.Fatalf("expected explicit addresses to win, got HTTP=%q worker=%q", cfg.HTTP.Address, cfg.Worker.HealthAddress)
	}
}

func TestPlaySurfaceDoesNotRequirePrivateApplicationSecrets(t *testing.T) {
	cfg := defaults()
	cfg.App.Environment = "production"
	cfg.App.Surface = "play"
	cfg.App.AppBaseURL = "https://app.example.com"
	cfg.App.PlayBaseURL = "https://play.example.com"
	cfg.Database.DSN = "user:password@tcp(mysql:3306)/gamegen"
	cfg.Storage.Endpoint = "minio:9000"
	cfg.Storage.PublicEndpoint = "assets.example.com"
	cfg.Storage.AccessKey = "access"
	cfg.Storage.SecretKey = "secret"
	cfg.Storage.PublicUseSSL = true

	if err := cfg.ValidateAPI(); err != nil {
		t.Fatalf("expected public play surface to run without app or encryption secrets: %v", err)
	}
}

func TestWorkerDoesNotRequireAPIOnlySecrets(t *testing.T) {
	cfg := defaults()
	cfg.App.Environment = "production"
	cfg.App.AppBaseURL = "https://app.example.com"
	cfg.App.PlayBaseURL = "https://play.example.com"
	cfg.Database.DSN = "user:password@tcp(mysql:3306)/gamegen"
	cfg.Storage.Endpoint = "minio:9000"
	cfg.Storage.PublicEndpoint = "assets.example.com"
	cfg.Storage.AccessKey = "access"
	cfg.Storage.SecretKey = "secret"
	cfg.Storage.PublicUseSSL = true
	cfg.Encryption.ContentKeyV1 = "content-key-required-by-worker"

	if err := cfg.ValidateWorker(); err != nil {
		t.Fatalf("expected worker to run without admin, share, or AI secrets: %v", err)
	}
}

func TestWorkerRequiresContentEncryptionKey(t *testing.T) {
	cfg := defaults()
	cfg.Database.DSN = "user:password@tcp(localhost:3306)/gamegen"
	cfg.Storage.Endpoint = "localhost:9000"
	cfg.Storage.PublicEndpoint = "localhost:9000"
	cfg.Storage.AccessKey = "access"
	cfg.Storage.SecretKey = "secret"

	if err := cfg.ValidateWorker(); err == nil {
		t.Fatal("expected worker to require the content encryption key")
	}
}

func TestInvalidServiceSurfaceIsRejected(t *testing.T) {
	cfg := defaults()
	cfg.App.Surface = "everything"
	cfg.Database.DSN = "user:password@tcp(localhost:3306)/gamegen"
	cfg.Storage.Endpoint = "localhost:9000"
	cfg.Storage.PublicEndpoint = "localhost:9000"
	cfg.Storage.AccessKey = "access"
	cfg.Storage.SecretKey = "secret"

	if err := cfg.ValidateAPI(); err == nil {
		t.Fatal("expected an unknown service surface to be rejected")
	}
}

func TestDynamicAISettingsRequireIndependentValidKey(t *testing.T) {
	cfg := defaults()
	cfg.AISettings.Enabled = true
	cfg.AISettings.EncryptionKeyV1 = "not-base64"
	if err := cfg.validateDynamicAI(); err == nil {
		t.Fatal("expected invalid dynamic AI settings key to be rejected")
	}
	cfg.AISettings.EncryptionKeyV1 = "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="
	if err := cfg.validateDynamicAI(); err != nil {
		t.Fatalf("expected a valid dynamic AI settings key: %v", err)
	}
}

func TestProductionRequiresSecureExternalServices(t *testing.T) {
	cfg := defaults()
	cfg.App.Environment = "production"
	cfg.App.AppBaseURL = "https://app.example.com"
	cfg.App.PlayBaseURL = "https://play.example.com"
	cfg.Database.DSN = "user:password@tcp(mysql:3306)/gamegen"
	cfg.Storage.Endpoint = "minio:9000"
	cfg.Storage.PublicEndpoint = "assets.example.com"
	cfg.Storage.AccessKey = "access"
	cfg.Storage.SecretKey = "secret"
	cfg.Admin.PasswordHash = "argon2id-hash"
	cfg.Encryption.ContentKeyV1 = "content-key"
	cfg.Encryption.ShareKeyV1 = "share-key"

	if err := cfg.ValidateRuntime(); err == nil {
		t.Fatal("expected insecure production storage to be rejected")
	}
	cfg.Storage.PublicUseSSL = true
	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatalf("expected secure production configuration to pass: %v", err)
	}
}

func withoutEnvironment(t *testing.T, key string) {
	t.Helper()
	value, exists := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}
