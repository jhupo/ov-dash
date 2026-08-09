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
	platformapp "ov-dash/backend/internal/platform/app"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	application, err := platformapp.OpenAPI(ctx, cfg)
	if err != nil {
		logger := logging.New(cfg.App.Env)
		logger.Fatal("open platform runtime", zap.Error(err))
	}
	defer application.Close()
	runtime := application.Runtime

	router := apphttp.NewRouter(application)

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
