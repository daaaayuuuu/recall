package worker

import (
	"context"
	"log/slog"
	"time"

	"gamegen/backend/internal/platform/health"
)

type JobProcessor func(context.Context) (bool, error)

func Run(ctx context.Context, logger *slog.Logger, dependencies []health.Dependency, pollInterval time.Duration, processors ...JobProcessor) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	logger.Info("worker loop ready", "poll_interval", pollInterval.String())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, dependency := range dependencies {
				checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				err := dependency.Check(checkCtx)
				cancel()
				if err != nil {
					logger.Warn("worker dependency unavailable", "dependency", dependency.Name, "error", err)
				}
			}
			for _, process := range processors {
				processed, err := process(ctx)
				if err != nil {
					logger.Error("worker job failed", "error", err)
					continue
				}
				if processed {
					logger.Debug("worker job completed")
				}
			}
		}
	}
}
