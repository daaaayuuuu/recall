package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gamegen/backend/internal/aisettings"
	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/games"
	"gamegen/backend/internal/generation"
	"gamegen/backend/internal/imagegeneration"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/database"
	"gamegen/backend/internal/platform/health"
	"gamegen/backend/internal/platform/logging"
	"gamegen/backend/internal/platform/security"
	"gamegen/backend/internal/platform/storage"
	"gamegen/backend/internal/server"
	workerprocess "gamegen/backend/internal/worker"
)

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	if err := cfg.ValidateWorker(); err != nil {
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
	aiSettingsManager, err := aisettings.NewManager(
		aisettings.NewRepository(db), cfg.AI, cfg.App.Environment, cfg.AISettings,
	)
	if err != nil {
		logger.Error("create dynamic AI settings manager", "error", err)
		os.Exit(1)
	}

	objectStorage, err := storage.New(cfg.Storage)
	if err != nil {
		logger.Error("create object storage client", "error", err)
		os.Exit(1)
	}
	contentCipher, err := contentcrypto.New(cfg.Encryption.ContentKeyV1, cfg.Encryption.ActiveKeyVersion)
	if err != nil {
		logger.Error("create content cipher", "error", err)
		os.Exit(1)
	}
	imageTransformer, err := imagegeneration.New(cfg.AI.ImageToImage)
	if err != nil {
		logger.Error("create image-to-image transformer", "error", err)
		os.Exit(1)
	}

	dependencies := []health.Dependency{
		{Name: "mysql", Check: db.Check},
		{Name: "minio", Check: objectStorage.Check},
	}
	healthServer, err := server.New("worker", cfg.Worker.HealthAddress, "", logger, dependencies)
	if err != nil {
		logger.Error("create worker health server", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	gameRepository := games.NewRepository(db)
	deletionProcessor := games.NewDeletionProcessor(gameRepository, objectStorage, logger)
	workerID, err := security.NewID()
	if err != nil {
		logger.Error("create worker id", "error", err)
		os.Exit(1)
	}
	generationRepository := generation.NewRepository(db)
	analyticsRecorder := newWorkerAnalyticsRecorder(db)
	generationProcessor := generation.NewProcessor(
		generationRepository, objectStorage, imageTransformer, cfg.AI.ImageToImage.MaxInputBytes,
		analyticsRecorder, workerID, cfg.Generation, contentCipher, logger,
	)
	generationProcessor.UseAIConfigProvider(aiSettingsManager)
	go workerprocess.Run(ctx, logger, dependencies, cfg.Worker.PollInterval, generationProcessor.ProcessOne, deletionProcessor.ProcessOne)

	logger.Info("worker starting", "health_address", cfg.Worker.HealthAddress, "environment", cfg.App.Environment)
	if err := healthServer.Run(ctx); err != nil {
		logger.Error("worker health server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
	logger.Info("worker stopped")
}

func newWorkerAnalyticsRecorder(db *database.DB) analytics.Recorder {
	return analytics.NewRepository(db)
}
