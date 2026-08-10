package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/modules/registry"
	"ov-dash/backend/internal/platform"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type Application struct {
	Runtime *platform.Runtime
	Catalog *platformmodule.Catalog

	closeOnce sync.Once
}

func OpenAPI(ctx context.Context, cfg config.Config) (*Application, error) {
	return open(ctx, cfg, platform.OpenAPI)
}

func OpenWorker(ctx context.Context, cfg config.Config) (*Application, error) {
	return open(ctx, cfg, platform.OpenWorker)
}

func open(
	ctx context.Context,
	cfg config.Config,
	openRuntime func(context.Context, config.Config) (*platform.Runtime, error),
) (*Application, error) {
	runtime, err := openRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	application, err := build(runtime, registry.Default()...)
	if err != nil {
		runtime.Close()
		return nil, err
	}
	return application, nil
}

func build(runtime *platform.Runtime, modules ...platformmodule.Module) (*Application, error) {
	if runtime == nil {
		return nil, errors.New("platform runtime is required")
	}
	catalog, err := platformmodule.NewCatalog(ModuleContext(runtime), modules...)
	if err != nil {
		return nil, fmt.Errorf("build module catalog: %w", err)
	}
	catalog.RegisterEvents(runtime.Events)
	return &Application{Runtime: runtime, Catalog: catalog}, nil
}

func ModuleContext(runtime *platform.Runtime) platformmodule.Context {
	if runtime == nil {
		return platformmodule.Context{}
	}
	return platformmodule.Context{
		Config: runtime.Config, DB: runtime.DB, Queue: runtime.Queue, Cache: runtime.Cache,
		Events: runtime.Events, Outbox: runtime.Outbox, Logger: runtime.Logger, Secrets: runtime.Secrets, Audit: runtime.Audit,
		RequestShutdown: runtime.Lifecycle.RequestShutdown,
	}
}

func (a *Application) Close() {
	if a == nil {
		return
	}
	a.closeOnce.Do(func() {
		if a.Runtime != nil {
			a.Runtime.Close()
		}
	})
}
