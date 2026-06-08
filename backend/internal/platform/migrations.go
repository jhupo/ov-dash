package platform

import (
	"ov-dash/backend/internal/database"

	"go.uber.org/zap"
)

func LogMigrationSummary(logger *zap.Logger, summary database.MigrationSummary) {
	if logger == nil {
		return
	}
	logger.Info(
		"migrations checked",
		zap.Int("applied", len(summary.Applied)),
		zap.Int("skipped", len(summary.Skipped)),
		zap.Int("failed", len(summary.Failed)),
		zap.Duration("duration", summary.Duration),
	)
}
