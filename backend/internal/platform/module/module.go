package module

import (
	"net/http"

	"ov-dash/backend/internal/cache"
	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Module interface {
	Name() string
	RegisterRoutes(ctx Context)
}

type Context struct {
	Config            config.Config
	DB                *db.Pool
	Queue             *queue.Client
	Cache             *cache.Cache
	Events            *events.Bus
	Logger            *zap.Logger
	PublicRouter      chi.Router
	ProtectedRouter   chi.Router
	RequireCapability func(capability.Capability) func(http.Handler) http.Handler
}

type Registry struct {
	modules []Module
}

func NewRegistry(modules ...Module) *Registry {
	registry := &Registry{}
	registry.Add(modules...)
	return registry
}

func (r *Registry) Add(modules ...Module) {
	r.modules = append(r.modules, modules...)
}

func (r *Registry) RegisterRoutes(ctx Context) {
	for _, module := range r.modules {
		module.RegisterRoutes(ctx)
	}
}

func (r *Registry) Modules() []Module {
	modules := make([]Module, len(r.modules))
	copy(modules, r.modules)
	return modules
}
