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

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "apps",
		Title:       "Apps",
		Description: "Application inventory API.",
		Kind:        "service",
		Tags:        []string{"apps", "inventory"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.HTTP(func(ctx platformmodule.Context) {
		handler := &Handler{service: NewService(NewRepository(ctx.DB))}
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.AppsRead)).Get("/apps", handler.List)
	}); err != nil {
		return err
	}
	return reg.Capabilities(capability.AppsRead)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "apps_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
