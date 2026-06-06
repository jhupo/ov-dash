package platform

import (
	"context"

	"ov-dash/backend/internal/cache"
	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/database"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/modules/proxy"
	"ov-dash/backend/internal/queue"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

type Runtime struct {
	Config     config.Config
	DB         *db.Pool
	Queue      *queue.Client
	Cache      *cache.Cache
	Migrations *database.MigrationRunner
	Events     *events.Bus
	Logger     *zap.Logger
	Proxy      *proxy.Service
}

func Open(ctx context.Context, cfg config.Config) (*Runtime, error) {
	logger := logging.New(cfg.App.Env)

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		_ = logger.Sync()
		return nil, err
	}

	queueClient, err := queue.Open(ctx, cfg.Redis)
	if err != nil {
		pg.Close()
		_ = logger.Sync()
		return nil, err
	}

	eventBus := events.NewBus(logger)
	eventBus.SubscribeAll(events.LogHandler(logger))

	return &Runtime{
		Config:     cfg,
		DB:         pg,
		Queue:      queueClient,
		Cache:      cache.New(queueClient),
		Migrations: database.NewMigrationRunner(pg),
		Events:     eventBus,
		Logger:     logger,
		Proxy:      proxy.NewService(proxy.NewRepository(pg)),
	}, nil
}

func (r *Runtime) Close() {
	if r.Queue != nil {
		if err := r.Queue.Close(); err != nil && r.Logger != nil {
			r.Logger.Error("close redis", zap.Error(err))
		}
	}
	if r.DB != nil {
		r.DB.Close()
	}
	if r.Logger != nil {
		_ = r.Logger.Sync()
	}
}
