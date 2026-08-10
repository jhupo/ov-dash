package module

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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

var moduleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)

type Module interface {
	Manifest() Manifest
	Register(*Registrar) error
}

type Manifest struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type ModuleDescriptor struct {
	Manifest
	Capabilities []capability.Descriptor `json:"capabilities"`
	Jobs         []JobDescriptor         `json:"jobs,omitempty"`
	HealthChecks []HealthDescriptor      `json:"health_checks,omitempty"`
	Settings     []settings.Schema       `json:"settings,omitempty"`
}

type JobDescriptor struct {
	Type           string `json:"type"`
	Description    string `json:"description,omitempty"`
	TimeoutSeconds int64  `json:"timeout_seconds,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty"`
	ModuleID       string `json:"module_id"`
}

type HealthDescriptor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Context struct {
	Config            config.Config
	DB                *db.Pool
	Queue             *queue.Client
	Cache             *cache.Cache
	Events            *events.Bus
	Outbox            *events.Outbox
	Logger            *zap.Logger
	Secrets           *secret.Store
	Audit             *audit.Recorder
	Jobs              *JobRegistry
	PublicRouter      chi.Router
	ProtectedRouter   chi.Router
	RequireCapability func(capability.Capability) func(http.Handler) http.Handler
	RequestShutdown   func()
}

type JobHandler interface {
	Handle(context.Context, queue.Job) error
}

type JobDefinition struct {
	Type        string
	Description string
	Timeout     time.Duration
	MaxAttempts int
	ModuleID    string
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
	def.Description = strings.TrimSpace(def.Description)
	def.ModuleID = strings.TrimSpace(def.ModuleID)
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
	definition, ok := r.jobs[strings.TrimSpace(jobType)]
	return definition, ok
}

func (r *JobRegistry) Definitions() []JobDefinition {
	if r == nil {
		return nil
	}
	definitions := make([]JobDefinition, 0, len(r.jobs))
	for _, definition := range r.jobs {
		definitions = append(definitions, definition)
	}
	slices.SortFunc(definitions, func(a, b JobDefinition) int { return strings.Compare(a.Type, b.Type) })
	return definitions
}

func (r *JobRegistry) DefinitionsForModule(moduleID string) []JobDefinition {
	definitions := []JobDefinition{}
	for _, definition := range r.Definitions() {
		if definition.ModuleID == moduleID {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

type HealthCheck struct {
	ID    string
	Name  string
	Check func(context.Context) error
}

type HTTPRegistrar func(Context)
type EventRegistrar func(*events.Bus)

type catalogEntry struct {
	manifest     Manifest
	http         []HTTPRegistrar
	events       []EventRegistrar
	capabilities []capability.Capability
	health       []HealthCheck
}

type Catalog struct {
	ctx          Context
	entries      []catalogEntry
	jobs         *JobRegistry
	settings     *settings.Registry
	capabilities []capability.Capability
	health       []HealthCheck
}

type Registrar struct {
	catalog *Catalog
	entry   *catalogEntry
}

func NewCatalog(ctx Context, modules ...Module) (*Catalog, error) {
	ordered, err := orderModules(modules)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{
		ctx:      ctx,
		jobs:     NewJobRegistry(),
		settings: settings.NewRegistry(),
	}
	capabilityOwners := map[capability.Capability]string{}
	healthOwners := map[string]string{}
	for _, module := range ordered {
		entry := catalogEntry{manifest: normalizeManifest(module.Manifest())}
		catalog.entries = append(catalog.entries, entry)
		registrar := &Registrar{catalog: catalog, entry: &catalog.entries[len(catalog.entries)-1]}
		if err := module.Register(registrar); err != nil {
			return nil, fmt.Errorf("register module %s: %w", entry.manifest.ID, err)
		}
		for _, value := range registrar.entry.capabilities {
			if owner, exists := capabilityOwners[value]; exists {
				return nil, fmt.Errorf("capability %s is registered by both %s and %s", value, owner, entry.manifest.ID)
			}
			capabilityOwners[value] = entry.manifest.ID
			catalog.capabilities = append(catalog.capabilities, value)
		}
		for _, check := range registrar.entry.health {
			if owner, exists := healthOwners[check.ID]; exists {
				return nil, fmt.Errorf("health check %s is registered by both %s and %s", check.ID, owner, entry.manifest.ID)
			}
			healthOwners[check.ID] = entry.manifest.ID
			catalog.health = append(catalog.health, check)
		}
	}
	slices.SortFunc(catalog.capabilities, func(a, b capability.Capability) int {
		return strings.Compare(string(a), string(b))
	})
	slices.SortFunc(catalog.health, func(a, b HealthCheck) int { return strings.Compare(a.ID, b.ID) })
	return catalog, nil
}

func (r *Registrar) Context() Context {
	if r == nil || r.catalog == nil {
		return Context{}
	}
	return r.catalog.ctx
}

func (r *Registrar) HTTP(register HTTPRegistrar) error {
	if r == nil || r.entry == nil || register == nil {
		return errors.New("HTTP registrar is required")
	}
	r.entry.http = append(r.entry.http, register)
	return nil
}

func (r *Registrar) Job(definition JobDefinition) error {
	if r == nil || r.entry == nil {
		return errors.New("module registrar is nil")
	}
	definition.ModuleID = r.entry.manifest.ID
	return r.catalog.jobs.Register(definition)
}

func (r *Registrar) Capabilities(values ...capability.Capability) error {
	if r == nil || r.entry == nil {
		return errors.New("module registrar is nil")
	}
	seen := map[capability.Capability]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(string(value)) == "" {
			return errors.New("capability is required")
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("capability registered twice in module %s: %s", r.entry.manifest.ID, value)
		}
		seen[value] = struct{}{}
		r.entry.capabilities = append(r.entry.capabilities, value)
	}
	return nil
}

func (r *Registrar) Health(checks ...HealthCheck) error {
	if r == nil || r.entry == nil {
		return errors.New("module registrar is nil")
	}
	seen := map[string]struct{}{}
	for _, check := range checks {
		check.ID = strings.TrimSpace(check.ID)
		check.Name = strings.TrimSpace(check.Name)
		if check.ID == "" || check.Check == nil {
			return errors.New("health check id and function are required")
		}
		if _, exists := seen[check.ID]; exists {
			return fmt.Errorf("health check registered twice in module %s: %s", r.entry.manifest.ID, check.ID)
		}
		seen[check.ID] = struct{}{}
		r.entry.health = append(r.entry.health, check)
	}
	return nil
}

func (r *Registrar) Settings(schemas ...settings.Schema) error {
	if r == nil || r.entry == nil {
		return errors.New("module registrar is nil")
	}
	return r.catalog.settings.Register(r.entry.manifest.ID, schemas)
}

func (r *Registrar) Events(register EventRegistrar) error {
	if r == nil || r.entry == nil || register == nil {
		return errors.New("event registrar is required")
	}
	r.entry.events = append(r.entry.events, register)
	return nil
}

func (c *Catalog) RegisterHTTP(ctx Context) {
	if c == nil {
		return
	}
	ctx.Jobs = c.jobs
	for _, entry := range c.entries {
		for _, register := range entry.http {
			register(ctx)
		}
	}
}

func (c *Catalog) RegisterEvents(bus *events.Bus) {
	if c == nil || bus == nil {
		return
	}
	for _, entry := range c.entries {
		for _, register := range entry.events {
			register(bus)
		}
	}
}

func (c *Catalog) Jobs() *JobRegistry           { return c.jobs }
func (c *Catalog) Settings() *settings.Registry { return c.settings }

func (c *Catalog) Capabilities() []capability.Capability {
	return slices.Clone(c.capabilities)
}

func (c *Catalog) HealthChecks() []HealthCheck {
	return slices.Clone(c.health)
}

func (c *Catalog) Descriptors() []ModuleDescriptor {
	if c == nil {
		return nil
	}
	descriptors := make([]ModuleDescriptor, 0, len(c.entries))
	for _, entry := range c.entries {
		descriptor := ModuleDescriptor{
			Manifest:     entry.manifest,
			Capabilities: capability.Descriptors(entry.capabilities),
			Settings:     moduleSettings(c.settings.Schemas(), entry.manifest.ID),
		}
		for _, definition := range c.jobs.DefinitionsForModule(entry.manifest.ID) {
			descriptor.Jobs = append(descriptor.Jobs, DescribeJob(definition))
		}
		for _, check := range entry.health {
			descriptor.HealthChecks = append(descriptor.HealthChecks, HealthDescriptor{ID: check.ID, Name: check.Name})
		}
		descriptors = append(descriptors, descriptor)
	}
	return descriptors
}

func DescribeJob(definition JobDefinition) JobDescriptor {
	return JobDescriptor{
		Type: definition.Type, Description: definition.Description,
		TimeoutSeconds: int64(definition.Timeout.Seconds()), MaxAttempts: definition.MaxAttempts,
		ModuleID: definition.ModuleID,
	}
}

func orderModules(modules []Module) ([]Module, error) {
	byID := make(map[string]Module, len(modules))
	manifests := make(map[string]Manifest, len(modules))
	inputOrder := make([]string, 0, len(modules))
	for _, module := range modules {
		if module == nil {
			return nil, errors.New("module is nil")
		}
		manifest := normalizeManifest(module.Manifest())
		if !moduleIDPattern.MatchString(manifest.ID) {
			return nil, fmt.Errorf("invalid module id: %s", manifest.ID)
		}
		if _, exists := byID[manifest.ID]; exists {
			return nil, fmt.Errorf("module id already registered: %s", manifest.ID)
		}
		byID[manifest.ID] = module
		manifests[manifest.ID] = manifest
		inputOrder = append(inputOrder, manifest.ID)
	}
	for id, manifest := range manifests {
		for _, dependency := range manifest.DependsOn {
			if _, exists := byID[dependency]; !exists {
				return nil, fmt.Errorf("module %s depends on missing module %s", id, dependency)
			}
		}
	}
	state := map[string]uint8{}
	ordered := make([]Module, 0, len(modules))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("module dependency cycle includes %s", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range manifests[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, byID[id])
		return nil
	}
	for _, id := range inputOrder {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func normalizeManifest(manifest Manifest) Manifest {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Title = strings.TrimSpace(manifest.Title)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Kind = strings.TrimSpace(manifest.Kind)
	if manifest.Title == "" {
		manifest.Title = manifest.ID
	}
	if manifest.Kind == "" {
		manifest.Kind = "module"
	}
	manifest.Tags = uniqueStrings(manifest.Tags)
	manifest.DependsOn = uniqueStrings(manifest.DependsOn)
	return manifest
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func moduleSettings(schemas []settings.Schema, moduleID string) []settings.Schema {
	result := []settings.Schema{}
	for _, schema := range schemas {
		if schema.ModuleID == moduleID {
			result = append(result, schema)
		}
	}
	return result
}
