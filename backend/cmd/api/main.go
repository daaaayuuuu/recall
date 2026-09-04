package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gamegen/backend/internal/aisettings"
	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"
	"gamegen/backend/internal/games"
	"gamegen/backend/internal/generation"
	"gamegen/backend/internal/invitations"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/database"
	"gamegen/backend/internal/platform/health"
	"gamegen/backend/internal/platform/logging"
	"gamegen/backend/internal/platform/storage"
	"gamegen/backend/internal/server"
	"gamegen/backend/internal/sharing"

	"github.com/go-chi/chi/v5"
)

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	if err := cfg.ValidateAPI(); err != nil {
		bootstrapLogger.Error("validate configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.App.Environment, cfg.Log.Level)
	slog.SetDefault(logger)

	db, err := database.Open(cfg.Database)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	objectStorage, err := storage.New(cfg.Storage)
	if err != nil {
		logger.Error("create object storage client", "error", err)
		os.Exit(1)
	}

	dependencies := []health.Dependency{
		{Name: "mysql", Check: db.Check},
		{Name: "minio", Check: objectStorage.Check},
	}

	analyticsRepository := analytics.NewRepository(db)
	analyticsHandler := analytics.NewHandler(analyticsRepository, cfg, logger)
	privateRecorder, publicRecorder, err := analyticsRecordersForSurface(cfg.App.Surface, analyticsRepository)
	if err != nil {
		logger.Error("assemble analytics recorders", "error", err)
		os.Exit(1)
	}
	sharingRepository := sharing.NewRepository(db)
	mounts, err := assembleSurfaceMounts(cfg.App.Surface, func(includePublic bool) ([]server.Mount, error) {
		aiSettingsManager, managerErr := aisettings.NewManager(
			aisettings.NewRepository(db), cfg.AI, cfg.App.Environment, cfg.AISettings,
		)
		if managerErr != nil {
			return nil, fmt.Errorf("create dynamic AI settings manager: %w", managerErr)
		}
		authRepository := auth.NewRepository(db)
		authHandler := auth.NewHandler(authRepository, objectStorage, privateRecorder, cfg, logger)
		contentCipher, cipherErr := contentcrypto.New(cfg.Encryption.ContentKeyV1, cfg.Encryption.ActiveKeyVersion)
		if cipherErr != nil {
			return nil, fmt.Errorf("create content cipher: %w", cipherErr)
		}
		shareCipher, cipherErr := contentcrypto.New(cfg.Encryption.ShareKeyV1, cfg.Encryption.ActiveKeyVersion)
		if cipherErr != nil {
			return nil, fmt.Errorf("create share cipher: %w", cipherErr)
		}
		gameRepository := games.NewRepository(db)
		gameHandler := games.NewHandler(gameRepository, objectStorage, contentCipher, authHandler, privateRecorder, cfg, logger)
		gameHandler.UseAIConfigProvider(aiSettingsManager)
		generationRepository := generation.NewRepository(db)
		generationHandler := generation.NewHandler(generationRepository, authHandler, privateRecorder, logger)
		invitationRepository := invitations.NewRepository(db)
		invitationHandler := invitations.NewHandler(invitationRepository, auth.AdminIdentity, logger)
		mountInvitations := func(router chi.Router) {
			invitationHandler.Mount(router, authHandler.RequireAdminSession, authHandler.RequireAdminMutation)
		}
		aiSettingsHandler := aisettings.NewHandler(aiSettingsManager, auth.AdminIdentity, logger)
		mountAISettings := func(router chi.Router) {
			aiSettingsHandler.Mount(router, authHandler.RequireAdminSession, authHandler.RequireAdminMutation)
		}
		sharingHandler := sharing.NewHandler(sharingRepository, objectStorage, shareCipher, authHandler, privateRecorder, cfg, logger)
		mountAnalytics := func(router chi.Router) {
			analyticsHandler.MountApp(
				router, authHandler.RequireCreatorSession, authHandler.RequireCreatorMutation,
				authHandler.RequireAdminSession,
				func(request *http.Request) (string, string) {
					return auth.CreatorUser(request).ID, auth.CreatorSessionID(request)
				},
			)
		}
		appMounts := []server.Mount{
			authHandler.Mount, gameHandler.Mount, generationHandler.Mount, sharingHandler.MountPrivate,
			mountInvitations, mountAISettings, mountAnalytics,
		}
		if includePublic {
			appMounts = append(appMounts, sharingHandler.MountPublic)
		}
		return appMounts, nil
	}, func() (server.Mount, error) {
		sharingHandler := sharing.NewHandler(sharingRepository, objectStorage, nil, nil, publicRecorder, cfg, logger)
		return sharingHandler.MountPublic, nil
	})
	if err != nil {
		logger.Error("assemble API surface", "error", err)
		os.Exit(1)
	}

	serviceName := "api"
	if cfg.App.Surface != "all" {
		serviceName += "-" + cfg.App.Surface
	}
	appServer, err := server.NewForSurface(serviceName, cfg.HTTP.Address, cfg.HTTP.StaticDir, cfg.App.Surface, logger, dependencies, mounts...)
	if err != nil {
		logger.Error("create API server", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("api starting", "address", cfg.HTTP.Address, "environment", cfg.App.Environment, "surface", cfg.App.Surface)
	if err := appServer.Run(ctx); err != nil {
		logger.Error("api stopped unexpectedly", "error", err)
		os.Exit(1)
	}
	logger.Info("api stopped")
}

func analyticsRecordersForSurface(surface string, recorder analytics.Recorder) (private, public analytics.Recorder, err error) {
	switch surface {
	case "app":
		return recorder, nil, nil
	case "play":
		return nil, recorder, nil
	case "all":
		return recorder, recorder, nil
	default:
		return nil, nil, fmt.Errorf("unsupported surface %q", surface)
	}
}

func assembleSurfaceMounts(
	surface string,
	appFactory func(includePublic bool) ([]server.Mount, error),
	playFactory func() (server.Mount, error),
) ([]server.Mount, error) {
	switch surface {
	case "app":
		return appFactory(false)
	case "play":
		mount, err := playFactory()
		if err != nil {
			return nil, err
		}
		return []server.Mount{mount}, nil
	case "all":
		return appFactory(true)
	default:
		return nil, fmt.Errorf("unsupported surface %q", surface)
	}
}
