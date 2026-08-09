package platform

import (
	"context"

	"ov-dash/backend/internal/cache"
	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/secret"
	"ov-dash/backend/internal/queue"
	"ov-dash/backend/pkg/logging"

	"go.uber.org/zap"
)

type Runtime struct {
	Config     config.Config
	DB         *db.Pool
	Queue      *queue.Client
	Cache      *cache.Cache
	Events     *events.Bus
	Logger     *zap.Logger
	Secrets    *secret.Store
	Audit      *audit.Recorder
	Authorizer capability.Authorizer
	Outbox     *events.Outbox
}

func OpenAPI(ctx context.Context, cfg config.Config) (*Runtime, error) {
	return open(ctx, cfg, true)
}

func OpenWorker(ctx context.Context, cfg config.Config) (*Runtime, error) {
	return open(ctx, cfg, false)
}

func open(ctx context.Context, cfg config.Config, withCache bool) (*Runtime, error) {
	logger := logging.New(cfg.App.Env)
	if err := cfg.Security.Validate(); err != nil {
		_ = logger.Sync()
		return nil, err
	}
	if err := cfg.HTTP.Validate(); err != nil {
		_ = logger.Sync()
		return nil, err
	}
	if defaultKeyIDs := cfg.Security.DefaultSecretKeyIDs(); len(defaultKeyIDs) > 0 && cfg.App.Env != "local" {
		logger.Warn(
			"APP_SECRET_KEYS_JSON contains the local development default; replace these keys after rotating stored secrets",
			zap.Strings("key_ids", defaultKeyIDs),
		)
	}

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		_ = logger.Sync()
		return nil, err
	}

	var cacheClient *cache.Cache
	if withCache {
		cacheClient, err = cache.Open(ctx, cfg.Redis)
		if err != nil {
			pg.Close()
			_ = logger.Sync()
			return nil, err
		}
	}

	queueClient, err := queue.Open(pg)
	if err != nil {
		if cacheClient != nil {
			_ = cacheClient.Close()
		}
		pg.Close()
		_ = logger.Sync()
		return nil, err
	}

	eventBus := events.NewBus(logger)
	eventBus.SubscribeAll(events.LogHandler(logger))

	secrets, err := secret.NewStore(pg, cfg.Security.ActiveSecretKeyID, cfg.Security.SecretKeys)
	if err != nil {
		if cacheClient != nil {
			_ = cacheClient.Close()
		}
		pg.Close()
		_ = logger.Sync()
		return nil, err
	}

	return &Runtime{
		Config:     cfg,
		DB:         pg,
		Queue:      queueClient,
		Cache:      cacheClient,
		Events:     eventBus,
		Logger:     logger,
		Secrets:    secrets,
		Audit:      audit.NewRecorder(pg),
		Authorizer: capability.NewPostgresAuthorizer(pg),
		Outbox:     events.NewOutbox(pg),
	}, nil
}

func (r *Runtime) Close() {
	if r.Cache != nil {
		if err := r.Cache.Close(); err != nil && r.Logger != nil {
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
