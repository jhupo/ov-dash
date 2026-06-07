package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ov-dash/backend/internal/modules/wiki"

	"github.com/go-chi/chi/v5"
)

type WikiHandler struct {
	service *wiki.Service
}

func NewWikiHandler(service *wiki.Service) *WikiHandler {
	return &WikiHandler{service: service}
}

type saveWikiPageRequest struct {
	ParentID        string `json:"parent_id"`
	Title           string `json:"title"`
	PageType        string `json:"page_type"`
	Category        string `json:"category"`
	Summary         string `json:"summary"`
	ContentMD       string `json:"content_md"`
	LinkLabel       string `json:"link_label"`
	LinkURL         string `json:"link_url"`
	MachineHost     string `json:"machine_host"`
	MachinePort     string `json:"machine_port"`
	MachineUsername string `json:"machine_username"`
	Tags            string `json:"tags"`
}

func (h *WikiHandler) ListPages(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "wiki_pages_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *WikiHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	page, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeWikiError(w, err, "wiki_page_get_failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *WikiHandler) CreatePage(w http.ResponseWriter, r *http.Request) {
	input, ok := h.saveInputFromRequest(w, r)
	if !ok {
		return
	}
	page, err := h.service.Create(r.Context(), input)
	if err != nil {
		writeWikiError(w, err, "wiki_page_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, page)
}

func (h *WikiHandler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	input, ok := h.saveInputFromRequest(w, r)
	if !ok {
		return
	}
	input.ID = strings.TrimSpace(chi.URLParam(r, "id"))
	page, err := h.service.Update(r.Context(), input)
	if err != nil {
		writeWikiError(w, err, "wiki_page_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *WikiHandler) DeletePage(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeWikiError(w, err, "wiki_page_delete_failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WikiHandler) ListRevisions(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListRevisions(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeWikiError(w, err, "wiki_page_revisions_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *WikiHandler) saveInputFromRequest(w http.ResponseWriter, r *http.Request) (wiki.SavePageInput, bool) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return wiki.SavePageInput{}, false
	}

	var payload saveWikiPageRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return wiki.SavePageInput{}, false
	}

	return wiki.SavePageInput{
		ParentID:        payload.ParentID,
		Title:           payload.Title,
		PageType:        payload.PageType,
		Category:        payload.Category,
		Summary:         payload.Summary,
		ContentMD:       payload.ContentMD,
		LinkLabel:       payload.LinkLabel,
		LinkURL:         payload.LinkURL,
		MachineHost:     payload.MachineHost,
		MachinePort:     payload.MachinePort,
		MachineUsername: payload.MachineUsername,
		Tags:            payload.Tags,
		ActorID:         user.ID,
	}, true
}

func writeWikiError(w http.ResponseWriter, err error, fallback string) {
	status := http.StatusInternalServerError
	code := fallback

	switch {
	case errors.Is(err, wiki.ErrTitleRequired),
		errors.Is(err, wiki.ErrIDRequired):
		status = http.StatusBadRequest
		code = err.Error()
	case wiki.IsNotFound(err):
		status = http.StatusNotFound
		code = "wiki_page_not_found"
	}

	writeJSON(w, status, map[string]string{"error": code})
}
