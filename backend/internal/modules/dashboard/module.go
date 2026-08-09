package dashboard

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

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "dashboard",
		Title:       "Dashboard",
		Description: "Dashboard snapshot API.",
		Kind:        "service",
		Tags:        []string{"dashboard", "overview"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.HTTP(func(ctx platformmodule.Context) {
		handler := &Handler{service: NewService()}
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.DashboardRead)).Get("/dashboard", handler.Snapshot)
	}); err != nil {
		return err
	}
	return reg.Capabilities(capability.DashboardRead)
}

func (h *Handler) Snapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.service.Snapshot(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "dashboard_snapshot_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, snapshot)
}
