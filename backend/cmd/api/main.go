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
	"ov-dash/backend/internal/db"
	apphttp "ov-dash/backend/internal/http"
	"ov-dash/backend/internal/queue"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := logging.New(cfg.App.Env)
	defer func() {
		_ = logger.Sync()
	}()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Fatal("connect postgres", zap.Error(err))
	}
	defer pg.Close()

	redisClient, err := queue.Open(ctx, cfg.Redis)
	if err != nil {
		logger.Fatal("connect redis", zap.Error(err))
	}
	defer redisClient.Close()

	router := apphttp.NewRouter(apphttp.RouterDeps{
		Config: cfg,
		DB:     pg,
		Queue:  redisClient,
		Logger: logger,
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("api listening", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("api server stopped unexpectedly", zap.Error(err))
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	logger.Info("shutting down api")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("api shutdown failed", zap.Error(err))
	}
}
