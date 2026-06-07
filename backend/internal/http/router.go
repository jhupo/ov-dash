package http

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/modules/notifications"
	"ov-dash/backend/internal/platform"
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
		protectedRouter := r.With(authMiddleware(authService))
		telegramNotifications := NewTelegramNotificationsHandler(
			notifications.NewService(
				notifications.NewRepository(runtime.DB),
				notifications.NewProxiedTelegramClient(runtime.Proxy),
			),
		)
		policy := capability.DefaultRolePolicy()
		requireCapability := func(value capability.Capability) func(http.Handler) http.Handler {
			return capability.RequireCapability(policy, currentCapabilityUser, value)
		}

		r.Get("/health", health.Readiness)
		r.Post("/incoming-messages", telegramNotifications.IncomingMessage)
		protectedRouter.Get("/telegram-notifications/settings", telegramNotifications.GetSettings)
		protectedRouter.Put("/telegram-notifications/settings", telegramNotifications.UpdateSettings)
		protectedRouter.Get("/telegram-notifications/users", telegramNotifications.ListUserSettings)
		protectedRouter.Put("/telegram-notifications/users/{userID}", telegramNotifications.UpdateUserSettings)

		defaultRegistry().RegisterRoutes(platformmodule.Context{
			Config:            runtime.Config,
			DB:                runtime.DB,
			Queue:             runtime.Queue,
			Cache:             runtime.Cache,
			Events:            runtime.Events,
			Logger:            runtime.Logger,
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
