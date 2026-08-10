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
	"ov-dash/backend/internal/worker"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

var version = "local"

func main() {
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	cfg := config.Load()
	if cfg.App.Version == "local" && version != "local" {
		cfg.App.Version = version
	}
	application, err := platformapp.OpenAPI(signalCtx, cfg)
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

	runCtx, cancelRun := context.WithCancel(signalCtx)
	defer cancelRun()
	server := &http.Server{
		Addr:              cfg.HTTP.Addr(),
		Handler:           apphttp.NewRouter(application),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	workerErrors := make(chan error, 1)
	go func() {
		runtime.Logger.Info("application listening", zap.String("addr", server.Addr))
		serverErrors <- server.ListenAndServe()
	}()
	go func() {
		workerErrors <- runner.Run(runCtx)
	}()

	var workerStopped bool
	select {
	case <-signalCtx.Done():
	case <-runtime.Lifecycle.Done():
		runtime.Logger.Info("application shutdown requested")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runtime.Logger.Error("http server stopped unexpectedly", zap.Error(err))
		}
	case err := <-workerErrors:
		workerStopped = true
		if err != nil && !errors.Is(err, context.Canceled) {
			runtime.Logger.Error("worker stopped unexpectedly", zap.Error(err))
		}
	}

	cancelRun()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		runtime.Logger.Error("http shutdown failed", zap.Error(err))
	}
	if !workerStopped {
		select {
		case err := <-workerErrors:
			if err != nil && !errors.Is(err, context.Canceled) {
				runtime.Logger.Error("worker shutdown failed", zap.Error(err))
			}
		case <-shutdownCtx.Done():
			runtime.Logger.Error("worker shutdown timed out", zap.Error(shutdownCtx.Err()))
		}
	}
}
