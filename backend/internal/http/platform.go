package http

import (
	"context"
	"net/http"
	"time"

	"ov-dash/backend/internal/platform"
)

type PlatformHandler struct {
	runtime *platform.Runtime
}

func NewPlatformHandler(runtime *platform.Runtime) *PlatformHandler {
	return &PlatformHandler{runtime: runtime}
}

func (h *PlatformHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	components := map[string]any{
		"database": map[string]string{
			"kind":       "postgres",
			"management": "pool,migrations",
			"status":     statusFromError(h.runtime.DB.Ping(ctx)),
		},
		"cache": map[string]string{
			"kind":   "redis",
			"status": statusFromError(h.runtime.Cache.Ping(ctx)),
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

	proxySettings, err := h.runtime.Proxy.Get(ctx)
	components["proxy"] = map[string]any{
		"kind":    "socks5",
		"status":  statusFromError(err),
		"enabled": err == nil && proxySettings.Enabled,
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
