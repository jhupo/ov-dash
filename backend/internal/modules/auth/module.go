package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
)

const sessionCookieName = "ovdash_session"

type Module struct{}

type Handler struct {
	service *Service
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type currentUserContextKey struct{}

func NewModule() Module {
	return Module{}
}

func (Module) ID() string {
	return "auth"
}

func (Module) RegisterHTTP(ctx platformmodule.Context) {
	handler := &Handler{service: NewService(NewRepository(ctx.DB))}

	ctx.PublicRouter.Post("/auth/login", handler.Login)
	ctx.ProtectedRouter.Post("/auth/logout", handler.Logout)
	ctx.ProtectedRouter.Get("/auth/me", handler.Me)
	ctx.ProtectedRouter.Put("/auth/password", handler.ChangePassword)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var payload loginRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := h.service.Login(r.Context(), LoginInput{
		Email:     payload.Email,
		Password:  payload.Password,
		UserAgent: r.UserAgent(),
		IPAddress: clientIP(r),
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "login_failed"
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrInactiveUser) {
			status = http.StatusUnauthorized
			code = "invalid_credentials"
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code})
		return
	}
	setSessionCookie(w, r, result.Token, result.ExpiresAt)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": result.User, "expiresAt": result.ExpiresAt})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.service.Logout(r.Context(), TokenFromRequest(r))
	clearSessionCookie(w, r)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var payload changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := h.service.ChangePassword(r.Context(), user.ID, payload.CurrentPassword, payload.NewPassword); err != nil {
		status := http.StatusInternalServerError
		code := "password_change_failed"
		if errors.Is(err, ErrInvalidCredentials) {
			status = http.StatusUnauthorized
			code = "invalid_credentials"
		}
		if errors.Is(err, ErrPasswordTooShort) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func ContextWithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, currentUserContextKey{}, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(currentUserContextKey{}).(User)
	return user, ok
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration().Seconds()),
		Expires:  expiresAt,
	}
	http.SetCookie(w, cookie)
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func TokenFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}

func clientIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		return strings.TrimSpace(strings.Split(forwardedFor, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
