package http

import (
	"context"
	"net/http"

	"ov-dash/backend/internal/modules/auth"
)

type authContextKey struct{}

func authMiddleware(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := service.CurrentUser(r.Context(), tokenFromRequest(r))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), authContextKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserFromContext(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(authContextKey{}).(auth.User)
	return user, ok
}
