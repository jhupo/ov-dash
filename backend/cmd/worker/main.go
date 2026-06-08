package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/platform"
	"ov-dash/backend/internal/worker"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	runtime, err := platform.Open(ctx, cfg)
	if err != nil {
		logger := logging.New(cfg.App.Env)
		logger.Fatal("open platform runtime", zap.Error(err))
	}
	defer runtime.Close()

	migrationSummary, err := runtime.Migrations.ApplyDir(ctx, cfg.Migrations.Dir)
	if err != nil {
		runtime.Logger.Fatal("apply migrations", zap.Error(err))
	}
	platform.LogMigrationSummary(runtime.Logger, migrationSummary)

	secretSummary, err := runtime.BackfillLegacySecrets(ctx)
	if err != nil {
		runtime.Logger.Fatal("backfill legacy secrets", zap.Error(err))
	}
	platform.LogSecretBackfillSummary(runtime.Logger, secretSummary)

	runner, err := worker.NewRunner(worker.RunnerDeps{
		Runtime: runtime,
	})
	if err != nil {
		runtime.Logger.Fatal("create worker runner", zap.Error(err))
	}

	if err := runner.Run(ctx); err != nil {
		runtime.Logger.Fatal("worker stopped", zap.Error(err))
	}
}
