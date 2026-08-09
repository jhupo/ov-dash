package notifications

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ov-dash/backend/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type TelegramNotificationsHandler struct {
	service *Service
}

func NewTelegramNotificationsHandler(service *Service) *TelegramNotificationsHandler {
	return &TelegramNotificationsHandler{service: service}
}

type updateTelegramSettingsRequest struct {
	Enabled           bool    `json:"enabled"`
	BotToken          *string `json:"bot_token"`
	InboundToken      *string `json:"inbound_token"`
	GroupEnabled      bool    `json:"group_enabled"`
	GroupChatID       string  `json:"group_chat_id"`
	ClearBotToken     bool    `json:"clear_bot_token"`
	ClearInboundToken bool    `json:"clear_inbound_token"`
}

type updateUserTelegramSettingsRequest struct {
	Enabled bool   `json:"enabled"`
	ChatID  string `json:"chat_id"`
}

type incomingMessageRequest struct {
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	Title          string `json:"title"`
	Message        string `json:"message"`
	Source         string `json:"source"`
	DeliverToGroup bool   `json:"deliver_to_group"`
}

func (h *TelegramNotificationsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.GetTelegramSettings(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "telegram_settings_get_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settings.Public())
}

func (h *TelegramNotificationsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var payload updateTelegramSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	settings, err := h.service.UpdateTelegramSettings(r.Context(), UpdateTelegramSettingsInput{
		Enabled:           payload.Enabled,
		BotToken:          payload.BotToken,
		InboundToken:      payload.InboundToken,
		GroupEnabled:      payload.GroupEnabled,
		GroupChatID:       payload.GroupChatID,
		ClearBotToken:     payload.ClearBotToken,
		ClearInboundToken: payload.ClearInboundToken,
	})
	if err != nil {
		writeNotificationError(w, err, "telegram_settings_update_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settings.Public())
}

func (h *TelegramNotificationsHandler) ListUserSettings(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListUserTelegramSettings(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "telegram_user_settings_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *TelegramNotificationsHandler) UpdateUserSettings(w http.ResponseWriter, r *http.Request) {
	var payload updateUserTelegramSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	settings, err := h.service.UpdateUserTelegramSettings(r.Context(), UpdateUserTelegramSettingsInput{
		UserID:  strings.TrimSpace(chi.URLParam(r, "userID")),
		Enabled: payload.Enabled,
		ChatID:  payload.ChatID,
	})
	if err != nil {
		writeNotificationError(w, err, "telegram_user_settings_update_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settings)
}

func (h *TelegramNotificationsHandler) IncomingMessage(w http.ResponseWriter, r *http.Request) {
	var payload incomingMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	result, err := h.service.DeliverIncomingMessage(r.Context(), inboundTokenFromRequest(r), IncomingMessageInput{
		UserID:         payload.UserID,
		Username:       payload.Username,
		Title:          payload.Title,
		Message:        payload.Message,
		Source:         payload.Source,
		DeliverToGroup: payload.DeliverToGroup,
	})
	if err != nil {
		writeNotificationError(w, err, "incoming_message_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, result)
}

func inboundTokenFromRequest(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get("X-OV-Dash-Token")); token != "" {
		return token
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}

func writeNotificationError(w http.ResponseWriter, err error, fallback string) {
	status := http.StatusInternalServerError
	code := fallback

	switch {
	case errors.Is(err, ErrInvalidInboundToken):
		status = http.StatusUnauthorized
		code = err.Error()
	case errors.Is(err, ErrTelegramBotTokenRequired),
		errors.Is(err, ErrInboundTokenRequired),
		errors.Is(err, ErrRecipientRequired),
		errors.Is(err, ErrTelegramChatIDRequired),
		errors.Is(err, ErrTelegramGroupDisabled),
		errors.Is(err, ErrTelegramGroupIDRequired),
		errors.Is(err, ErrMessageRequired),
		errors.Is(err, ErrNotificationsDisabled),
		errors.Is(err, ErrRecipientDisabled):
		status = http.StatusBadRequest
		code = err.Error()
	case errors.Is(err, ErrRecipientNotFound):
		status = http.StatusNotFound
		code = err.Error()
	}

	httpx.WriteJSON(w, status, map[string]string{"error": code})
}
