package updates

import (
	"context"
	"errors"
	"net/http"
	"time"

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
	return "updates"
}

func (Module) RegisterHTTP(ctx platformmodule.Context) {
	handler := &Handler{service: NewService(ctx.Config, ctx.Logger)}
	readUpdates := ctx.RequireCapability(capability.UpdatesRead)
	applyUpdates := ctx.RequireCapability(capability.UpdatesApply)

	ctx.ProtectedRouter.With(readUpdates).Get("/updates", handler.Status)
	ctx.ProtectedRouter.With(readUpdates).Post("/updates/check", handler.Check)
	ctx.ProtectedRouter.With(applyUpdates).Post("/updates/apply", handler.Update)
	ctx.ProtectedRouter.With(applyUpdates).Post("/updates/restart", handler.Restart)
}

func (Module) Capabilities() []capability.Capability {
	return []capability.Capability{
		capability.UpdatesRead,
		capability.UpdatesApply,
	}
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	result, update, err := h.service.Status(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "update_status_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": result, "update": update})
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	result, err := h.service.Check(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		code := "update_check_failed"
		if errors.Is(err, ErrUpdateDisabled) || errors.Is(err, ErrNotGitRepo) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": result})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := h.service.Update(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		code := "update_start_failed"
		if errors.Is(err, ErrUpdateDisabled) ||
			errors.Is(err, ErrUpdateRunning) ||
			errors.Is(err, ErrNoNewVersion) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"update": result})
}

func (h *Handler) Restart(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	result, err := h.service.Restart(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		code := "update_restart_failed"
		if errors.Is(err, ErrUpdateNotReady) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"update": result})
}
