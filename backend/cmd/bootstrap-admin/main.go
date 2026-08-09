package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := logging.New(cfg.App.Env)
	defer logger.Sync()

	pool, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Fatal("open postgres", zap.Error(err))
	}
	defer pool.Close()

	user, err := auth.NewService(auth.NewRepository(pool)).BootstrapAdmin(ctx, auth.BootstrapAdminInput{
		Email:     os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		Password:  os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		Username:  os.Getenv("BOOTSTRAP_ADMIN_USERNAME"),
		FirstName: os.Getenv("BOOTSTRAP_ADMIN_FIRST_NAME"),
		LastName:  os.Getenv("BOOTSTRAP_ADMIN_LAST_NAME"),
	})
	if err != nil {
		logger.Fatal("bootstrap administrator", zap.Error(err))
	}

	logger.Info("administrator bootstrapped", zap.String("user_id", user.ID), zap.String("email", user.Email))
}
