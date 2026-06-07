package chats

import (
	"net/http"

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

func (Module) Name() string {
	return "chats"
}

func (Module) RegisterRoutes(ctx platformmodule.Context) {
	handler := &Handler{service: NewService(NewRepository(ctx.DB))}
	ctx.ProtectedRouter.Get("/chats", handler.ListConversations)
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListConversations(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "chats_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
