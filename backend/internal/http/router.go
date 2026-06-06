package http

import (
	"net/http"
	"slices"
	"time"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/modules/apps"
	"ov-dash/backend/internal/modules/chats"
	"ov-dash/backend/internal/modules/dashboard"
	"ov-dash/backend/internal/modules/proxy"
	"ov-dash/backend/internal/modules/tasks"
	"ov-dash/backend/internal/modules/users"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type RouterDeps struct {
	Config config.Config
	DB     *db.Pool
	Queue  *queue.Client
	Logger *zap.Logger
}

func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(cors(deps.Config.HTTP.AllowedOrigins))
	r.Use(middleware.Timeout(30 * time.Second))

	health := NewHealthHandler(deps.DB, deps.Queue)

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)

	r.Route("/api/v1", func(r chi.Router) {
		proxySettings := NewProxySettingsHandler(proxy.NewService(proxy.NewRepository(deps.DB)))

		r.Get("/health", health.Readiness)
		r.Post("/jobs", NewJobsHandler(deps.Queue, deps.Config.Worker).Create)
		r.Get("/dashboard", NewDashboardHandler(dashboard.NewService()).Snapshot)
		r.Get("/tasks", NewTasksHandler(tasks.NewService(tasks.NewRepository(deps.DB))).List)
		r.Get("/users", NewUsersHandler(users.NewService(users.NewRepository(deps.DB))).List)
		r.Get("/apps", NewAppsHandler(apps.NewService(apps.NewRepository(deps.DB))).List)
		r.Get("/chats", NewChatsHandler(chats.NewService(chats.NewRepository(deps.DB))).ListConversations)
		r.Get("/proxy-settings", proxySettings.Get)
		r.Put("/proxy-settings", proxySettings.Update)
	})

	return r
}

func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (slices.Contains(allowedOrigins, "*") || slices.Contains(allowedOrigins, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
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
