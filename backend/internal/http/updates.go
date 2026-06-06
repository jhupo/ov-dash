package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"ov-dash/backend/internal/modules/updates"
)

type UpdatesHandler struct {
	service *updates.Service
}

func NewUpdatesHandler(service *updates.Service) *UpdatesHandler {
	return &UpdatesHandler{service: service}
}

func (h *UpdatesHandler) Status(w http.ResponseWriter, r *http.Request) {
	result, update, err := h.service.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update_status_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": result, "update": update})
}

func (h *UpdatesHandler) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	result, err := h.service.Check(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		code := "update_check_failed"
		if errors.Is(err, updates.ErrUpdateDisabled) || errors.Is(err, updates.ErrNotGitRepo) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		writeJSON(w, status, map[string]string{"error": code, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": result})
}

func (h *UpdatesHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := h.service.Update(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		code := "update_start_failed"
		if errors.Is(err, updates.ErrUpdateDisabled) ||
			errors.Is(err, updates.ErrUpdateRunning) ||
			errors.Is(err, updates.ErrNoNewVersion) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		writeJSON(w, status, map[string]string{"error": code, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"update": result})
}

func (h *UpdatesHandler) Restart(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	result, err := h.service.Restart(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		code := "update_restart_failed"
		if errors.Is(err, updates.ErrUpdateNotReady) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		writeJSON(w, status, map[string]string{"error": code, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"update": result})
}
