package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/database"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform"
	"ov-dash/backend/internal/platform/secret"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := logging.New(cfg.App.Env)
	defer logger.Sync()

	if err := cfg.Security.Validate(); err != nil {
		logger.Fatal("validate secret keyring", zap.Error(err))
	}
	if defaultKeyIDs := cfg.Security.DefaultSecretKeyIDs(); len(defaultKeyIDs) > 0 && cfg.App.Env != "local" {
		logger.Warn(
			"APP_SECRET_KEYS_JSON contains the local development default; replace these keys after rotating stored secrets",
			zap.Strings("key_ids", defaultKeyIDs),
		)
	}

	pool, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Fatal("open postgres", zap.Error(err))
	}
	defer pool.Close()

	summary, err := database.NewMigrationRunner(pool).ApplyDir(ctx, cfg.Migrations.Dir)
	platform.LogMigrationSummary(logger, summary)
	if err != nil {
		logger.Fatal("apply migrations", zap.Error(err))
	}

	secretStore, err := secret.NewStore(pool, cfg.Security.ActiveSecretKeyID, cfg.Security.SecretKeys)
	if err != nil {
		logger.Fatal("open secret store", zap.Error(err))
	}
	secretSummary, err := secret.BackfillLegacyCredentials(
		ctx,
		pool,
		secretStore,
	)
	if err != nil {
		logger.Fatal("backfill legacy secrets", zap.Error(err))
	}
	platform.LogSecretBackfillSummary(logger, secretSummary)
	rotated, err := secretStore.Rotate(ctx)
	if err != nil {
		logger.Fatal("rotate secrets", zap.Error(err))
	}
	logger.Info("secret rotation complete", zap.Int("rotated", rotated), zap.String("active_key_id", cfg.Security.ActiveSecretKeyID))
}
