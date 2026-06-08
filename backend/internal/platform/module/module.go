package module

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"ov-dash/backend/internal/cache"
	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/secret"
	"ov-dash/backend/internal/platform/settings"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Module interface {
	ID() string
}

type HTTPModule interface {
	Module
	RegisterHTTP(ctx Context)
}

type LegacyRouteModule interface {
	Module
	RegisterRoutes(ctx Context)
}

type JobModule interface {
	Module
	RegisterJobs(ctx Context, reg *JobRegistry) error
}

type EventModule interface {
	Module
	RegisterEvents(bus *events.Bus)
}

type CapabilityModule interface {
	Module
	Capabilities() []capability.Capability
}

type HealthModule interface {
	Module
	HealthChecks() []HealthCheck
}

type SettingsModule interface {
	Module
	SettingsSchemas() []settings.Schema
}

type Context struct {
	Config            config.Config
	DB                *db.Pool
	Queue             *queue.Client
	Cache             *cache.Cache
	Events            *events.Bus
	Logger            *zap.Logger
	Secrets           *secret.Store
	Audit             *audit.Recorder
	Jobs              *JobRegistry
	PublicRouter      chi.Router
	ProtectedRouter   chi.Router
	RequireCapability func(capability.Capability) func(http.Handler) http.Handler
}

type JobHandler interface {
	Handle(ctx context.Context, job queue.Job) error
}

type JobDefinition struct {
	Type        string
	Description string
	Timeout     time.Duration
	MaxAttempts int
	Handler     JobHandler
}

type JobRegistry struct {
	jobs map[string]JobDefinition
}

func NewJobRegistry() *JobRegistry {
	return &JobRegistry{jobs: map[string]JobDefinition{}}
}

func (r *JobRegistry) Register(def JobDefinition) error {
	if r == nil {
		return errors.New("job registry is nil")
	}
	def.Type = strings.TrimSpace(def.Type)
	if def.Type == "" {
		return errors.New("job type is required")
	}
	if def.Handler == nil {
		return fmt.Errorf("job handler is required for %s", def.Type)
	}
	if _, exists := r.jobs[def.Type]; exists {
		return fmt.Errorf("job type already registered: %s", def.Type)
	}
	r.jobs[def.Type] = def
	return nil
}

func (r *JobRegistry) Definition(jobType string) (JobDefinition, bool) {
	if r == nil {
		return JobDefinition{}, false
	}
	def, ok := r.jobs[jobType]
	return def, ok
}

func (r *JobRegistry) Definitions() []JobDefinition {
	if r == nil {
		return nil
	}
	definitions := make([]JobDefinition, 0, len(r.jobs))
	for _, def := range r.jobs {
		definitions = append(definitions, def)
	}
	slices.SortFunc(definitions, func(a, b JobDefinition) int {
		return strings.Compare(a.Type, b.Type)
	})
	return definitions
}

func (r *JobRegistry) Handlers() map[string]JobHandler {
	if r == nil {
		return nil
	}
	handlers := make(map[string]JobHandler, len(r.jobs))
	for jobType, def := range r.jobs {
		handlers[jobType] = def.Handler
	}
	return handlers
}

type HealthCheck struct {
	ID    string
	Name  string
	Check func(ctx context.Context) error
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

func (r *Registry) RegisterHTTP(ctx Context) {
	for _, module := range r.modules {
		if httpModule, ok := module.(HTTPModule); ok {
			httpModule.RegisterHTTP(ctx)
			continue
		}
		if legacyModule, ok := module.(LegacyRouteModule); ok {
			legacyModule.RegisterRoutes(ctx)
		}
	}
}

func (r *Registry) RegisterJobs(ctx Context, jobs *JobRegistry) error {
	for _, module := range r.modules {
		jobModule, ok := module.(JobModule)
		if !ok {
			continue
		}
		if err := jobModule.RegisterJobs(ctx, jobs); err != nil {
			return fmt.Errorf("register module %s jobs: %w", module.ID(), err)
		}
	}
	return nil
}

func (r *Registry) RegisterEvents(bus *events.Bus) {
	for _, module := range r.modules {
		eventModule, ok := module.(EventModule)
		if !ok {
			continue
		}
		eventModule.RegisterEvents(bus)
	}
}

func (r *Registry) Capabilities() []capability.Capability {
	seen := map[capability.Capability]struct{}{}
	for _, module := range r.modules {
		capabilityModule, ok := module.(CapabilityModule)
		if !ok {
			continue
		}
		for _, value := range capabilityModule.Capabilities() {
			seen[value] = struct{}{}
		}
	}

	values := make([]capability.Capability, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	slices.SortFunc(values, func(a, b capability.Capability) int {
		return strings.Compare(string(a), string(b))
	})
	return values
}

func (r *Registry) HealthChecks() []HealthCheck {
	checks := []HealthCheck{}
	for _, module := range r.modules {
		healthModule, ok := module.(HealthModule)
		if !ok {
			continue
		}
		checks = append(checks, healthModule.HealthChecks()...)
	}
	return checks
}

func (r *Registry) SettingsSchemas() []settings.Schema {
	registry, err := r.SettingsRegistry()
	if err != nil {
		return nil
	}
	return registry.Schemas()
}

func (r *Registry) SettingsRegistry() (*settings.Registry, error) {
	registry := settings.NewRegistry()
	for _, module := range r.modules {
		settingsModule, ok := module.(SettingsModule)
		if !ok {
			continue
		}
		if err := registry.Register(module.ID(), settingsModule.SettingsSchemas()); err != nil {
			return nil, fmt.Errorf("register module %s settings: %w", module.ID(), err)
		}
	}
	return registry, nil
}

func (r *Registry) Modules() []Module {
	modules := make([]Module, len(r.modules))
	copy(modules, r.modules)
	return modules
}
