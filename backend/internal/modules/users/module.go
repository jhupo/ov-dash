package users

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
		ID:          "users",
		Title:       "Users",
		Description: "User directory API.",
		Kind:        "service",
		Tags:        []string{"users", "identity"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.HTTP(func(ctx platformmodule.Context) {
		handler := &Handler{service: NewService(NewRepository(ctx.DB))}
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.UsersRead)).Get("/users", handler.List)
	}); err != nil {
		return err
	}
	return reg.Capabilities(capability.UsersRead)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "users_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
