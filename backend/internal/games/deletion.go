package games

import (
	"context"
	"log/slog"
	"time"

	"gamegen/backend/internal/platform/storage"
)

type DeletionProcessor struct {
	repository *Repository
	storage    *storage.Client
	logger     *slog.Logger
	now        func() time.Time
}

func NewDeletionProcessor(repository *Repository, objectStorage *storage.Client, logger *slog.Logger) *DeletionProcessor {
	return &DeletionProcessor{repository: repository, storage: objectStorage, logger: logger, now: time.Now}
}

func (processor *DeletionProcessor) ProcessOne(ctx context.Context) (bool, error) {
	now := processor.now().UTC()
	job, claimed, err := processor.repository.ClaimDeletionJob(ctx, now)
	if err != nil || !claimed {
		return claimed, err
	}

	for _, object := range job.Payload.ObjectKeys {
		if err := processor.storage.Remove(ctx, object.Bucket, object.Key); err != nil {
			processor.logger.Error("remove game object during deletion", "job_id", job.ID, "error", err)
			if failErr := processor.repository.FailDeletionJob(ctx, job.ID, "OBJECT_STORAGE_DELETE_FAILED", now.Add(time.Minute)); failErr != nil {
				return true, failErr
			}
			return true, err
		}
	}
	if err := processor.repository.CompleteDeletionJob(ctx, job, now); err != nil {
		_ = processor.repository.FailDeletionJob(ctx, job.ID, "DATABASE_DELETE_FAILED", now.Add(time.Minute))
		return true, err
	}
	processor.logger.Info("game deletion completed", "job_id", job.ID, "game_id", job.GameID)
	return true, nil
}
