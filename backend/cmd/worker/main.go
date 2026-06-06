package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/queue"
	"ov-dash/backend/internal/worker"
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

	runner := worker.NewRunner(worker.RunnerDeps{
		Config: cfg,
		DB:     pg,
		Queue:  redisClient,
		Logger: logger,
	})

	if err := runner.Run(ctx); err != nil {
		logger.Fatal("worker stopped", zap.Error(err))
	}
}
