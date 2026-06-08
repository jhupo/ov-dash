package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ov-dash/backend/internal/config"
	apphttp "ov-dash/backend/internal/http"
	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform"
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

	if err := auth.NewService(auth.NewRepository(runtime.DB)).EnsureDefaultAdmin(ctx); err != nil {
		runtime.Logger.Fatal("ensure default admin", zap.Error(err))
	}

	router := apphttp.NewRouter(runtime)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		runtime.Logger.Info("api listening", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			runtime.Logger.Fatal("api server stopped unexpectedly", zap.Error(err))
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	runtime.Logger.Info("shutting down api")
	if err := server.Shutdown(shutdownCtx); err != nil {
		runtime.Logger.Error("api shutdown failed", zap.Error(err))
	}
}
