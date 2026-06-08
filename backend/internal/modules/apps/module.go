package apps

import (
	"net/http"

	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type Module struct{}

type Handler struct {
	service *Service
}

func NewModule() Module {
	return Module{}
}

func (Module) ID() string {
	return "apps"
}

func (Module) RegisterHTTP(ctx platformmodule.Context) {
	handler := &Handler{service: NewService(NewRepository(ctx.DB))}
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.AppsRead)).Get("/apps", handler.List)
}

func (Module) Capabilities() []capability.Capability {
	return []capability.Capability{capability.AppsRead}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "apps_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
