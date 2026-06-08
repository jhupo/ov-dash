package http

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform"
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	platformsettings "ov-dash/backend/internal/platform/settings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func NewRouter(runtime *platform.Runtime) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(runtime.Logger))
	r.Use(middleware.Recoverer)
	r.Use(cors(runtime.Config.HTTP.AllowedOrigins))
	r.Use(timeoutExceptWebSocket(30 * time.Second))

	health := NewHealthHandler(runtime.DB, runtime.Queue)

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)

	r.Route("/api/v1", func(r chi.Router) {
		authService := auth.NewService(auth.NewRepository(runtime.DB))
		protectedRouter := r.With(authMiddleware(authService), auditMiddleware(runtime.Audit))
		policy := capability.DefaultRolePolicy()
		requireCapability := func(value capability.Capability) func(http.Handler) http.Handler {
			return capability.RequireCapability(policy, currentCapabilityUser, value)
		}

		modules := defaultRegistry()
		settingsRegistry, err := modules.SettingsRegistry()
		if err != nil {
			runtime.Logger.Error("register platform settings schema", zap.Error(err))
			settingsRegistry = platformsettings.NewRegistry()
		}
		settingsHandler := platformsettings.NewHandler(
			settingsRegistry,
			platformsettings.NewStore(runtime.DB, runtime.Secrets),
		)

		jobRegistry := platformmodule.NewJobRegistry()
		if err := modules.RegisterJobs(platformmodule.Context{
			Config:  runtime.Config,
			DB:      runtime.DB,
			Queue:   runtime.Queue,
			Cache:   runtime.Cache,
			Events:  runtime.Events,
			Logger:  runtime.Logger,
			Secrets: runtime.Secrets,
			Audit:   runtime.Audit,
		}, jobRegistry); err != nil {
			runtime.Logger.Error("register platform job metadata", zap.Error(err))
		}

		r.Get("/health", health.Readiness)
		protectedRouter.With(requireCapability(capability.PlatformRead)).Get("/platform/modules", platformModulesHandler(modules, jobRegistry))
		protectedRouter.With(requireCapability(capability.PlatformRead)).Get("/platform/health", platformHealthHandler(runtime, modules))
		protectedRouter.With(requireCapability(capability.SettingsRead)).Get("/platform/settings/schema", settingsHandler.Schema)
		protectedRouter.With(requireCapability(capability.SettingsRead)).Get("/platform/settings/{key}", settingsHandler.Get)
		protectedRouter.With(requireCapability(capability.SettingsWrite)).Put("/platform/settings/{key}", settingsHandler.Put)

		modules.RegisterHTTP(platformmodule.Context{
			Config:            runtime.Config,
			DB:                runtime.DB,
			Queue:             runtime.Queue,
			Cache:             runtime.Cache,
			Events:            runtime.Events,
			Logger:            runtime.Logger,
			Secrets:           runtime.Secrets,
			Audit:             runtime.Audit,
			Jobs:              jobRegistry,
			PublicRouter:      r,
			ProtectedRouter:   protectedRouter,
			RequireCapability: requireCapability,
		})
	})

	return r
}

func currentCapabilityUser(ctx context.Context) (capability.User, bool) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return capability.User{}, false
	}
	return capability.User{Role: user.Role}, true
}

func timeoutExceptWebSocket(timeout time.Duration) func(http.Handler) http.Handler {
	timeoutMiddleware := middleware.Timeout(timeout)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				next.ServeHTTP(w, r)
				return
			}
			timeoutMiddleware(next).ServeHTTP(w, r)
		})
	}
}

func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (slices.Contains(allowedOrigins, "*") || slices.Contains(allowedOrigins, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-OV-Dash-Token")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
