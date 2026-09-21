package ocpp16hal

import (
	"context"
	"log/slog"
	"time"
)

const v1ChargerOperationRecoveryInterval = 2 * time.Second

// RunV1ChargerOperationRecovery keeps administrative charger-operation
// convergence independent from the time-sensitive charging deadline worker.
// It performs one sequential, bounded pass at a time; a slow OCPP operation
// delays only this worker and cannot cause overlapping recovery passes.
func (h *HAL) RunV1ChargerOperationRecovery(ctx context.Context) {
	if h == nil || h.v1Store == nil {
		return
	}
	runV1ChargerOperationRecoveryWorker(ctx, v1ChargerOperationRecoveryInterval, h.DispatchPendingV1ChargerOperations, h.logger)
}

func runV1ChargerOperationRecoveryWorker(ctx context.Context, interval time.Duration, recoverPass func(context.Context) error, logger *slog.Logger) {
	if interval <= 0 {
		interval = v1ChargerOperationRecoveryInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := recoverPass(ctx); err != nil && ctx.Err() == nil && logger != nil {
			logger.Warn("persisted charger-operation recovery pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
