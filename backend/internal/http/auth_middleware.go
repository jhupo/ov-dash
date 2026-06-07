package http

import (
	"context"
	"net/http"

	"ov-dash/backend/internal/modules/auth"
)

func authMiddleware(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := service.CurrentUser(r.Context(), auth.TokenFromRequest(r))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			ctx := auth.ContextWithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserFromContext(ctx context.Context) (auth.User, bool) {
	return auth.UserFromContext(ctx)
}
