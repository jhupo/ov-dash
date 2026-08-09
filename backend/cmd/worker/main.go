package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ov-dash/backend/internal/config"
	platformapp "ov-dash/backend/internal/platform/app"
	"ov-dash/backend/internal/worker"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	application, err := platformapp.OpenWorker(ctx, cfg)
	if err != nil {
		logger := logging.New(cfg.App.Env)
		logger.Fatal("open platform runtime", zap.Error(err))
	}
	defer application.Close()
	runtime := application.Runtime

	runner, err := worker.NewRunner(worker.RunnerDeps{
		Runtime: runtime,
		Jobs:    application.Catalog.Jobs(),
	})
	if err != nil {
		runtime.Logger.Fatal("create worker runner", zap.Error(err))
	}

	if err := runner.Run(ctx); err != nil {
		runtime.Logger.Fatal("worker stopped", zap.Error(err))
	}
}
