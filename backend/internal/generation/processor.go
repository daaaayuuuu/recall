package generation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/gameconfig"
	"gamegen/backend/internal/imagegeneration"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/imageprocessing"
	"gamegen/backend/internal/platform/security"
	"gamegen/backend/internal/platform/storage"
)

const (
	artifactsBucket    = "gamegen-artifacts"
	renderAssetsBucket = "gamegen-render-assets"
)

type Processor struct {
	repository       processorRepository
	analytics        analytics.Recorder
	workerID         string
	config           config.GenerationConfig
	contentCipher    *contentcrypto.Cipher
	transformer      imagegeneration.Transformer
	aiConfigProvider AIConfigProvider
	maxSourceBytes   int64
	logger           *slog.Logger
	now              func() time.Time
	createArtifact   func(context.Context, Run) (Artifact, error)
	removeArtifact   func(context.Context, string, string) error
	readObject       func(context.Context, string, string, int64) ([]byte, error)
	putFile          func(context.Context, string, string, string, string) (int64, error)
}

type AIConfigProvider interface {
	Current(context.Context) (config.AIConfig, error)
}

type processorRepository interface {
	Claim(context.Context, string, int, time.Duration, time.Time) (Run, bool, error)
	UpdateProgress(context.Context, string, string, string, int, time.Duration, time.Time) (bool, error)
	CompleteCancelled(context.Context, Run, string, time.Time) error
	Fail(context.Context, Run, string, Failure, time.Time) error
	CompleteSuccess(context.Context, Run, string, Artifact, time.Time) error
	LoadVersionInput(context.Context, string, string) (VersionInput, error)
	LoadSourceAssets(context.Context, string, string, string) ([]SourceAsset, error)
}

func NewProcessor(
	repository processorRepository,
	objectStorage *storage.Client,
	transformer imagegeneration.Transformer,
	maxSourceBytes int64,
	recorder analytics.Recorder,
	workerID string,
	cfg config.GenerationConfig,
	contentCipher *contentcrypto.Cipher,
	logger *slog.Logger,
) *Processor {
	processor := &Processor{
		repository: repository, transformer: transformer, maxSourceBytes: maxSourceBytes,
		analytics: recorder, workerID: workerID, config: cfg, contentCipher: contentCipher, logger: logger, now: time.Now,
	}
	processor.createArtifact = processor.createAndStoreArtifact
	processor.removeArtifact = objectStorage.Remove
	processor.readObject = objectStorage.ReadAll
	processor.putFile = objectStorage.PutFile
	return processor
}

// UseAIConfigProvider resolves one immutable AI configuration snapshot for
// each generation execution. In-flight executions therefore do not change
// model or limits halfway through processing.
func (processor *Processor) UseAIConfigProvider(provider AIConfigProvider) {
	processor.aiConfigProvider = provider
}

func (processor *Processor) ProcessOne(ctx context.Context) (bool, error) {
	now := processor.now().UTC()
	run, claimed, err := processor.repository.Claim(
		ctx, processor.workerID, processor.config.MaxExecutions, processor.config.LeaseDuration, now,
	)
	if err != nil || !claimed {
		return claimed, err
	}
	if run.Status == "failed" {
		processor.recordFailure(ctx, run)
		return true, nil
	}
	if run.Status == "cancelled" {
		return true, nil
	}

	artifact, cancelRequested, err := processor.createWithHeartbeat(ctx, run)
	if cancelRequested {
		processor.removeGeneratedArtifact(ctx, artifact)
		return true, processor.repository.CompleteCancelled(ctx, run, processor.workerID, processor.now().UTC())
	}
	if err != nil {
		processor.removeGeneratedArtifact(ctx, artifact)
		if errors.Is(err, errLeaseMaintenance) || errors.Is(err, context.Canceled) {
			return true, err
		}
		failure := generationFailure(err)
		if failErr := processor.repository.Fail(ctx, run, processor.workerID, failure, processor.now().UTC()); failErr != nil {
			return true, errors.Join(err, failErr)
		}
		run.ErrorCode = sql.NullString{String: failure.Code, Valid: true}
		run.Retryable = failure.Retryable
		processor.recordFailure(ctx, run)
		return true, nil
	}
	cancelRequested, err = processor.repository.UpdateProgress(
		ctx, run.ID, processor.workerID, "saving_results", 0, processor.config.LeaseDuration, processor.now().UTC(),
	)
	if err != nil {
		processor.removeGeneratedArtifact(ctx, artifact)
		return true, err
	}
	if cancelRequested {
		processor.removeGeneratedArtifact(ctx, artifact)
		return true, processor.repository.CompleteCancelled(ctx, run, processor.workerID, processor.now().UTC())
	}
	if err := processor.repository.CompleteSuccess(ctx, run, processor.workerID, artifact, processor.now().UTC()); err != nil {
		processor.removeGeneratedArtifact(ctx, artifact)
		if errors.Is(err, ErrCancellationWon) {
			return true, nil
		}
		return true, err
	}
	processor.recordSuccess(ctx, run)
	processor.logger.Info("generation completed", "run_id", run.ID, "trace_id", run.TraceID, "game_id", run.GameID)
	return true, nil
}

type heartbeatResult struct {
	cancelRequested bool
	err             error
}

var errLeaseMaintenance = errors.New("generation lease maintenance failed")

func (processor *Processor) createWithHeartbeat(ctx context.Context, run Run) (Artifact, bool, error) {
	cancelRequested, err := processor.repository.UpdateProgress(
		ctx, run.ID, processor.workerID, "transforming_images", 0, processor.config.LeaseDuration, processor.now().UTC(),
	)
	if err != nil || cancelRequested {
		if err != nil {
			return Artifact{}, false, fmt.Errorf("%w: %v", errLeaseMaintenance, err)
		}
		return Artifact{}, true, nil
	}
	processor.logger.Info("generation image transformation started", "run_id", run.ID, "trace_id", run.TraceID)

	operationContext, cancel := context.WithCancel(ctx)
	defer cancel()
	var heartbeat <-chan heartbeatResult
	if interval := heartbeatInterval(processor.config.LeaseDuration); interval > 0 {
		result := make(chan heartbeatResult, 1)
		heartbeat = result
		go processor.maintainLease(operationContext, cancel, run, interval, result)
	}
	artifact, createErr := processor.createArtifact(operationContext, run)
	cancel()
	if heartbeat != nil {
		result := <-heartbeat
		if result.cancelRequested {
			return artifact, true, nil
		}
		if result.err != nil {
			return artifact, false, fmt.Errorf("%w: %v", errLeaseMaintenance, result.err)
		}
	}
	return artifact, false, createErr
}

func (processor *Processor) maintainLease(
	ctx context.Context,
	cancel context.CancelFunc,
	run Run,
	interval time.Duration,
	result chan<- heartbeatResult,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			result <- heartbeatResult{}
			return
		case <-ticker.C:
			cancelRequested, err := processor.repository.UpdateProgress(
				ctx, run.ID, processor.workerID, "transforming_images", 0,
				processor.config.LeaseDuration, processor.now().UTC(),
			)
			if err != nil || cancelRequested {
				result <- heartbeatResult{cancelRequested: cancelRequested, err: err}
				cancel()
				return
			}
		}
	}
}

func heartbeatInterval(leaseDuration time.Duration) time.Duration {
	if leaseDuration <= 0 {
		return 0
	}
	interval := leaseDuration / 3
	if interval > 20*time.Second {
		return 20 * time.Second
	}
	if interval < 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	return interval
}

func (processor *Processor) recordSuccess(ctx context.Context, run Run) {
	processor.recordEvent(ctx, analytics.RecordInput{
		EventName: analytics.EventGenerationSucceeded, Source: analytics.SourceWorker, ActorType: analytics.ActorSystem,
		CreatorID: run.UserID, GameID: run.GameID, GameVersionID: run.GameVersionID, GenerationRunID: run.ID,
		Properties: processorProperties(map[string]any{"executionCount": run.ExecutionCount}),
	})
}

func (processor *Processor) recordFailure(ctx context.Context, run Run) {
	processor.recordEvent(ctx, analytics.RecordInput{
		EventName: analytics.EventGenerationFailed, Source: analytics.SourceWorker, ActorType: analytics.ActorSystem,
		CreatorID: run.UserID, GameID: run.GameID, GameVersionID: run.GameVersionID, GenerationRunID: run.ID,
		Properties: processorProperties(map[string]any{
			"errorCode": run.ErrorCode.String, "retryable": run.Retryable, "executionCount": run.ExecutionCount,
		}),
	})
}

func (processor *Processor) recordEvent(ctx context.Context, input analytics.RecordInput) {
	recorder := processor.analytics
	if recorder == nil {
		recorder = analytics.NoopRecorder{}
	}
	if _, err := recorder.RecordEvent(ctx, input); err != nil {
		processor.logger.Warn("analytics event recording failed",
			"event_name", input.EventName, "source", input.Source,
			"error_code", "ANALYTICS_WRITE_FAILED", "generation_run_id", input.GenerationRunID,
		)
	}
}

func processorProperties(properties map[string]any) json.RawMessage {
	encoded, _ := json.Marshal(properties)
	return encoded
}

var errArtifactStorage = errors.New("generated artifact storage operation failed")

func generationFailure(err error) Failure {
	failure := Failure{
		Code: "INTERNAL_ERROR", AdminMessage: "游戏生成发生内部错误",
		SanitizedDetails: map[string]any{"errorType": "generation_internal", "generatorVersion": "image-to-image-1.0.0"},
		Retryable:        false,
	}
	switch {
	case errors.Is(err, errArtifactStorage):
		failure.Code = "STORAGE_WRITE_FAILED"
		failure.AdminMessage = "生成素材无法从对象存储读取或写入"
		failure.SanitizedDetails["errorType"] = "artifact_storage"
		failure.Retryable = true
	case errors.Is(err, imagegeneration.ErrInputTooLarge):
		failure.Code = "INPUT_VALIDATION_FAILED"
		failure.AdminMessage = "源图片超过图生图输入限制"
		failure.SanitizedDetails["errorType"] = "image_input_limit"
	case errors.Is(err, imagegeneration.ErrOutputTooLarge), errors.Is(err, imagegeneration.ErrInvalidOutput):
		failure.Code = "PROVIDER_UNAVAILABLE"
		failure.AdminMessage = "图生图服务返回了不可用的图片"
		failure.SanitizedDetails["errorType"] = "image_provider_invalid_output"
		failure.Retryable = true
	case errors.Is(err, imagegeneration.ErrUnavailable), errors.Is(err, imagegeneration.ErrTimeout), errors.Is(err, imagegeneration.ErrNotConfigured):
		failure.Code = "PROVIDER_UNAVAILABLE"
		failure.AdminMessage = "图生图服务当前不可用"
		failure.SanitizedDetails["errorType"] = "image_provider_unavailable"
		failure.Retryable = true
	}
	return failure
}

func (processor *Processor) createAndStoreArtifact(ctx context.Context, run Run) (Artifact, error) {
	assetID, err := security.NewID()
	if err != nil {
		return Artifact{}, fmt.Errorf("generate artifact id: %w", err)
	}
	configDocument, err := processor.buildConfigDocument(ctx, run)
	if err != nil {
		return Artifact{}, err
	}
	file, err := os.CreateTemp("", "gamegen-config-*.json")
	if err != nil {
		return Artifact{}, fmt.Errorf("create config artifact: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hasher := sha256.New()
	encoded, err := json.Marshal(configDocument)
	if err != nil {
		return Artifact{}, fmt.Errorf("encode config artifact: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		return Artifact{}, fmt.Errorf("write config artifact: %w", err)
	}
	if _, err := hasher.Write(encoded); err != nil {
		return Artifact{}, fmt.Errorf("hash config artifact: %w", err)
	}
	objectKey := fmt.Sprintf("users/%s/games/%s/versions/%s/config/%s.json", run.UserID, run.GameID, run.GameVersionID, assetID)
	size, err := processor.putFile(ctx, artifactsBucket, objectKey, file.Name(), "application/json")
	if err != nil {
		return Artifact{}, fmt.Errorf("%w: put config artifact: %v", errArtifactStorage, err)
	}
	artifact := Artifact{
		ID: assetID, OwnerUserID: run.UserID, Bucket: artifactsBucket, ObjectKey: objectKey,
		MIMEType: "application/json", SizeBytes: size, ChecksumSHA256: hasher.Sum(nil), CreatedAt: processor.now().UTC(),
	}
	sources, err := processor.repository.LoadSourceAssets(ctx, run.UserID, run.GameID, run.GameVersionID)
	if err != nil {
		processor.removeGeneratedArtifact(ctx, artifact)
		return Artifact{}, fmt.Errorf("load source assets: %w", err)
	}
	transformer := processor.transformer
	maxSourceBytes := processor.maxSourceBytes
	if processor.aiConfigProvider != nil {
		aiConfig, err := processor.aiConfigProvider.Current(ctx)
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, fmt.Errorf("load dynamic AI settings: %w", err)
		}
		transformer, err = imagegeneration.New(aiConfig.ImageToImage)
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, err
		}
		maxSourceBytes = aiConfig.ImageToImage.MaxInputBytes
	}
	for index, source := range sources {
		if transformer == nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, imagegeneration.ErrNotConfigured
		}
		if maxSourceBytes > 0 && source.SizeBytes > maxSourceBytes {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, imagegeneration.ErrInputTooLarge
		}
		sourceImage, err := processor.readObject(ctx, source.Bucket, source.ObjectKey, maxSourceBytes)
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			if errors.Is(err, storage.ErrReadLimitExceeded) {
				return Artifact{}, imagegeneration.ErrInputTooLarge
			}
			return Artifact{}, fmt.Errorf("%w: read source image: %v", errArtifactStorage, err)
		}
		transformed, err := transformer.Transform(ctx, imagegeneration.Input{
			Image: sourceImage, MIMEType: source.MIMEType, Prompt: imageTransformPrompt(run, source),
		})
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, err
		}
		processed, err := imageprocessing.Process(bytes.NewReader(transformed.Image))
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, fmt.Errorf("%w: generated bytes are not a supported image", imagegeneration.ErrInvalidOutput)
		}
		processedPath := processed.File.Name()
		defer os.Remove(processedPath)
		defer processed.File.Close()

		renderID, err := security.NewID()
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, fmt.Errorf("generate render asset id: %w", err)
		}
		renderObjectKey := fmt.Sprintf(
			"users/%s/games/%s/versions/%s/render/%s.png",
			run.UserID, run.GameID, run.GameVersionID, renderID,
		)
		generatedSize, err := processor.putFile(ctx, renderAssetsBucket, renderObjectKey, processedPath, processed.MIMEType)
		if err != nil {
			processor.removeGeneratedArtifact(ctx, artifact)
			return Artifact{}, fmt.Errorf("%w: put generated image: %v", errArtifactStorage, err)
		}
		checksum := append([]byte(nil), processed.ChecksumSHA256[:]...)
		artifact.RenderAssets = append(artifact.RenderAssets, RenderAsset{
			ID: renderID, OwnerUserID: run.UserID, Bucket: renderAssetsBucket, ObjectKey: renderObjectKey,
			MIMEType: processed.MIMEType, SizeBytes: generatedSize, ChecksumSHA256: checksum,
			Width: processed.Width, Height: processed.Height, SlotKey: source.SlotKey, SortOrder: index,
			CreatedAt: processor.now().UTC(),
		})
		processor.logger.Info(
			"generation image transformed", "run_id", run.ID, "trace_id", run.TraceID,
			"slot_key", source.SlotKey, "provider_request_id", transformed.ProviderRequestID,
		)
	}
	return artifact, nil
}

func imageTransformPrompt(run Run, source SourceAsset) string {
	prompt := "Transform this source photo into a warm hand-drawn romantic storybook illustration for a mobile game. " +
		"Use clean dark navy ink outlines, soft flat pastel colors, a warm cream paper background, and subtle paper texture. " +
		"Preserve the exact number of people, each person's identity, facial features, hairstyle, skin tone, pose, clothing, composition, and important objects. " +
		"Do not add or remove people, text, logos, frames, speech bubbles, or watermarks."
	if run.TemplateID != "love-journey" {
		prompt = "Transform this source image into a polished hand-drawn storybook illustration suitable for a mobile game. " +
			"Preserve the people, identity, pose, composition, colors, and important objects. Do not add text, logos, or watermarks."
	}
	if source.SlotKey == "cover" {
		prompt += " Keep the main subjects centered and leave natural breathing room around them for a game cover."
	}
	return prompt
}

func (processor *Processor) removeGeneratedArtifact(ctx context.Context, artifact Artifact) {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if artifact.Bucket != "" && artifact.ObjectKey != "" {
		_ = processor.removeArtifact(cleanupContext, artifact.Bucket, artifact.ObjectKey)
	}
	for _, render := range artifact.RenderAssets {
		_ = processor.removeArtifact(cleanupContext, render.Bucket, render.ObjectKey)
	}
}

type generationInputPayload struct {
	SceneInputs map[string]string `json:"sceneInputs"`
}

func (processor *Processor) buildConfigDocument(ctx context.Context, run Run) (gameconfig.Document, error) {
	document := gameconfig.Document{
		TemplateID: run.TemplateID, TemplateVersion: run.TemplateVersion, ConfigVersion: 1,
		Config: gameconfig.Config{OpeningTitle: run.GameTitle, Rounds: []map[string]any{}},
	}
	if run.TemplateID == "love-journey" && run.TemplateVersion == "1.1.0" {
		if processor.contentCipher == nil {
			return gameconfig.Document{}, errors.New("content cipher is required for love journey 1.1 generation")
		}
		encrypted, err := processor.repository.LoadVersionInput(ctx, run.GameID, run.GameVersionID)
		if err != nil {
			return gameconfig.Document{}, fmt.Errorf("load generation input: %w", err)
		}
		plaintext, err := processor.contentCipher.Decrypt(
			encrypted.Ciphertext, encrypted.Nonce, []byte(run.GameVersionID), encrypted.KeyVersion,
		)
		if err != nil {
			return gameconfig.Document{}, fmt.Errorf("decrypt generation input: %w", err)
		}
		var input generationInputPayload
		if err := json.Unmarshal(plaintext, &input); err != nil {
			return gameconfig.Document{}, errors.New("decode generation input")
		}
		document.Config.LoveLetter = strings.TrimSpace(input.SceneInputs["loveLetter"])
		document.Config.LetterPassword = strings.TrimSpace(input.SceneInputs["letterPassword"])
		document.Config.PasswordHint = strings.TrimSpace(input.SceneInputs["passwordHint"])
	}
	if err := document.Validate(); err != nil {
		return gameconfig.Document{}, fmt.Errorf("validate config artifact: %w", err)
	}
	return document, nil
}
