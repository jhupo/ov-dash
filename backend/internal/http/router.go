package http

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/auth"
	platformapp "ov-dash/backend/internal/platform/app"
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	platformsettings "ov-dash/backend/internal/platform/settings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(application *platformapp.Application) http.Handler {
	runtime := application.Runtime
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(trustedProxy(runtime.Config.HTTP.TrustedProxyCIDRs))
	r.Use(requestLogger(runtime.Logger))
	r.Use(middleware.Recoverer)
	r.Use(cors(runtime.Config.HTTP.AllowedOrigins))
	r.Use(timeoutExceptWebSocket(30 * time.Second))

	health := NewHealthHandler(
		runtime.DB,
		runtime.Cache,
		runtime.Queue,
		runtime.Config.Migrations.Dir,
		runtime.Config.App.Version,
	)

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)

	r.Route("/api/v1", func(r chi.Router) {
		authService := auth.NewService(auth.NewRepository(runtime.DB))
		protectedRouter := r.With(authMiddleware(authService), auditMiddleware(runtime.Audit))
		requireCapability := func(value capability.Capability) func(http.Handler) http.Handler {
			return capability.RequireCapability(runtime.Authorizer, currentCapabilityUser, value)
		}

		catalog := application.Catalog
		settingsHandler := platformsettings.NewHandler(
			catalog.Settings(),
			platformsettings.NewStore(runtime.DB, runtime.Secrets),
		)

		moduleCtx := platformapp.ModuleContext(runtime)
		moduleCtx.Jobs = catalog.Jobs()

		r.Get("/health", health.Readiness)
		protectedRouter.With(requireCapability(capability.PlatformRead)).Get("/platform/modules", platformModulesHandler(catalog))
		protectedRouter.With(requireCapability(capability.PlatformRead)).Get("/platform/health", platformHealthHandler(runtime, catalog))
		protectedRouter.With(requireCapability(capability.SettingsRead)).Get("/platform/settings/schema", settingsHandler.Schema)
		protectedRouter.With(requireCapability(capability.SettingsRead)).Get("/platform/settings/{key}", settingsHandler.Get)
		protectedRouter.With(requireCapability(capability.SettingsWrite)).Put("/platform/settings/{key}", settingsHandler.Put)

		catalog.RegisterHTTP(platformmodule.Context{
			Config:            runtime.Config,
			DB:                runtime.DB,
			Queue:             runtime.Queue,
			Cache:             runtime.Cache,
			Events:            runtime.Events,
			Outbox:            runtime.Outbox,
			Logger:            runtime.Logger,
			Secrets:           runtime.Secrets,
			Audit:             runtime.Audit,
			Jobs:              catalog.Jobs(),
			PublicRouter:      r,
			ProtectedRouter:   protectedRouter,
			RequireCapability: requireCapability,
		})
	})

	return r
}

func trustedProxy(cidrs []string) func(http.Handler) http.Handler {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, value := range cidrs {
		if prefix, err := netip.ParsePrefix(value); err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remote, ok := requestAddress(r.RemoteAddr)
			if ok && addressTrusted(remote, prefixes) {
				current := remote
				forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
				for index := len(forwarded) - 1; index >= 0 && addressTrusted(current, prefixes); index-- {
					candidate, err := netip.ParseAddr(strings.TrimSpace(forwarded[index]))
					if err != nil {
						break
					}
					current = candidate.Unmap()
				}
				r.RemoteAddr = current.String()
			} else if ok {
				r.RemoteAddr = remote.String()
			}
			r.Header.Del("Forwarded")
			r.Header.Del("X-Forwarded-For")
			r.Header.Del("X-Real-IP")
			next.ServeHTTP(w, r)
		})
	}
}

func requestAddress(remote string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remote))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(remote), "[]")
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func addressTrusted(address netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
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
