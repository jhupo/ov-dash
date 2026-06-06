package http

import (
	"net/http"

	"ov-dash/backend/internal/modules/chats"
)

type ChatsHandler struct {
	service *chats.Service
}

func NewChatsHandler(service *chats.Service) *ChatsHandler {
	return &ChatsHandler{service: service}
}

func (h *ChatsHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListConversations(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "chats_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
