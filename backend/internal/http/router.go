package http

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/apps"
	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/modules/chats"
	"ov-dash/backend/internal/modules/dashboard"
	"ov-dash/backend/internal/modules/notifications"
	"ov-dash/backend/internal/modules/servers"
	"ov-dash/backend/internal/modules/tasks"
	"ov-dash/backend/internal/modules/users"
	"ov-dash/backend/internal/modules/wiki"
	"ov-dash/backend/internal/platform"

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
		proxySettings := NewProxySettingsHandler(runtime.Proxy)
		authService := auth.NewService(auth.NewRepository(runtime.DB))
		authHandler := NewAuthHandler(authService)
		telegramNotifications := NewTelegramNotificationsHandler(
			notifications.NewService(
				notifications.NewRepository(runtime.DB),
				notifications.NewProxiedTelegramClient(runtime.Proxy),
			),
		)
		serverRepository := servers.NewRepository(runtime.DB)
		serverConnections := NewServerConnectionsHandler(
			servers.NewService(serverRepository),
			servers.NewCollector(serverRepository),
		)
		wikiPages := NewWikiHandler(
			wiki.NewService(wiki.NewRepository(runtime.DB)),
			runtime.Config.Uploads.WikiDir,
		)

		r.Get("/health", health.Readiness)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/incoming-messages", telegramNotifications.IncomingMessage)

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware(authService))

			r.Post("/auth/logout", authHandler.Logout)
			r.Get("/auth/me", authHandler.Me)
			r.Put("/auth/password", authHandler.ChangePassword)
			r.Get("/platform", NewPlatformHandler(runtime).Status)
			r.Post("/jobs", NewJobsHandler(runtime).Create)
			r.Get("/dashboard", NewDashboardHandler(dashboard.NewService()).Snapshot)
			tasksHandler := NewTasksHandler(tasks.NewService(tasks.NewRepository(runtime.DB)))
			r.Get("/tasks", tasksHandler.List)
			r.Delete("/tasks", tasksHandler.Delete)
			r.Get("/users", NewUsersHandler(users.NewService(users.NewRepository(runtime.DB))).List)
			r.Get("/apps", NewAppsHandler(apps.NewService(apps.NewRepository(runtime.DB))).List)
			r.Get("/chats", NewChatsHandler(chats.NewService(chats.NewRepository(runtime.DB))).ListConversations)
			r.Get("/wiki/pages", wikiPages.ListPages)
			r.Post("/wiki/pages", wikiPages.CreatePage)
			r.Get("/wiki/pages/{id}", wikiPages.GetPage)
			r.Put("/wiki/pages/{id}", wikiPages.UpdatePage)
			r.Delete("/wiki/pages/{id}", wikiPages.DeletePage)
			r.Get("/wiki/pages/{id}/revisions", wikiPages.ListRevisions)
			r.Post("/wiki/attachments", wikiPages.UploadAttachment)
			r.Get("/wiki/attachments/{id}/raw", wikiPages.AttachmentRaw)
			r.Get("/proxy-settings", proxySettings.Get)
			r.Put("/proxy-settings", proxySettings.Update)
			r.Get("/telegram-notifications/settings", telegramNotifications.GetSettings)
			r.Put("/telegram-notifications/settings", telegramNotifications.UpdateSettings)
			r.Get("/telegram-notifications/users", telegramNotifications.ListUserSettings)
			r.Put("/telegram-notifications/users/{userID}", telegramNotifications.UpdateUserSettings)
			r.Get("/server-connections", serverConnections.List)
			r.Post("/server-connections/monitor/touch", serverConnections.TouchMonitor)
			r.Post("/server-connections", serverConnections.Save)
			r.Get("/server-connections/{id}/metrics", serverConnections.Metrics)
			r.Post("/server-connections/{id}/agent/update", serverConnections.UpdateAgent)
			r.Post("/server-connections/{id}/ssh/command", serverConnections.RunCommand)
			r.Get("/server-connections/{id}/ssh/ws", serverConnections.Shell)
			r.Put("/server-connections/{id}", serverConnections.Save)
			r.Delete("/server-connections/{id}", serverConnections.Delete)
		})
	})

	return r
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
