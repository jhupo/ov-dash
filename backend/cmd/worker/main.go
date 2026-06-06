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

	runner := worker.NewRunner(worker.RunnerDeps{
		Runtime: runtime,
	})

	if err := runner.Run(ctx); err != nil {
		runtime.Logger.Fatal("worker stopped", zap.Error(err))
	}
}
