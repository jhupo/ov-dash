package platforminfo

import (
	"context"
	"net/http"
	"time"

	"ov-dash/backend/internal/cache"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/modules/proxy"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type Module struct{}

type Handler struct {
	cache *cache.Cache
	db    *db.Pool
	proxy *proxy.Service
}

func NewModule() Module {
	return Module{}
}

func (Module) Name() string {
	return "platform"
}

func (Module) RegisterRoutes(ctx platformmodule.Context) {
	handler := &Handler{
		cache: ctx.Cache,
		db:    ctx.DB,
		proxy: proxy.NewService(proxy.NewRepository(ctx.DB)),
	}
	ctx.ProtectedRouter.Get("/platform", handler.Status)
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	components := map[string]any{
		"database": map[string]string{
			"kind":       "postgres",
			"management": "pool,migrations",
			"status":     statusFromError(h.db.Ping(ctx)),
		},
		"cache": map[string]string{
			"kind":   "redis",
			"status": statusFromError(h.cache.Ping(ctx)),
		},
		"events": map[string]string{
			"kind":   "in-memory",
			"status": "ok",
		},
		"logger": map[string]string{
			"kind":   "zap",
			"status": "ok",
		},
	}

	proxySettings, err := h.proxy.Get(ctx)
	components["proxy"] = map[string]any{
		"kind":    "socks5",
		"status":  statusFromError(err),
		"enabled": err == nil && proxySettings.Enabled,
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"components": components,
	})
}

func statusFromError(err error) string {
	if err != nil {
		return "down"
	}
	return "ok"
}
