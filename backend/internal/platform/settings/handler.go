package settings

import (
	"encoding/json"
	"errors"
	"net/http"

	"ov-dash/backend/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	registry *Registry
	store    *Store
}

func NewHandler(registry *Registry, store *Store) *Handler {
	return &Handler{registry: registry, store: store}
}

func (h *Handler) Schema(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": h.registry.Schemas()})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	schema, ok := h.registry.Get(chi.URLParam(r, "key"))
	if !ok {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "setting_not_found"})
		return
	}
	value, err := h.store.Get(r.Context(), schema)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "setting_get_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	schema, ok := h.registry.Get(chi.URLParam(r, "key"))
	if !ok {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "setting_not_found"})
		return
	}
	if !schema.Writable {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "setting_not_writable"})
		return
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	raw, ok := payload["value"]
	if !ok {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "setting_value_required"})
		return
	}

	value, err := h.store.Put(r.Context(), schema, raw)
	if err != nil {
		status := http.StatusInternalServerError
		code := "setting_update_failed"
		if errors.Is(err, ErrInvalidValue) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}
