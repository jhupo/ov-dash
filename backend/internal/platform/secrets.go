package platform

import (
	"context"

	"ov-dash/backend/internal/platform/secret"

	"go.uber.org/zap"
)

func (r *Runtime) BackfillLegacySecrets(ctx context.Context) (secret.BackfillSummary, error) {
	return secret.BackfillLegacyCredentials(ctx, r.DB, r.Secrets)
}

func LogSecretBackfillSummary(logger *zap.Logger, summary secret.BackfillSummary) {
	if logger == nil {
		return
	}
	logger.Info(
		"legacy secrets backfill checked",
		zap.Int("proxy_passwords", summary.ProxyPasswords),
		zap.Int("server_passwords", summary.ServerPasswords),
		zap.Int("server_private_keys", summary.ServerPrivateKeys),
	)
}
