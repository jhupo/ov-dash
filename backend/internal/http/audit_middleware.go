package http

import (
	"net/http"
	"strings"

	"ov-dash/backend/internal/platform/audit"

	"github.com/go-chi/chi/v5"
)

func auditMiddleware(recorder *audit.Recorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if recorder == nil || !auditableMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			ww := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)

			user, _ := UserFromContext(r.Context())
			resource, action := auditResourceAction(r)
			result := "success"
			message := ""
			if ww.status >= http.StatusBadRequest {
				result = "failure"
				message = http.StatusText(ww.status)
			}
			_ = recorder.Record(r.Context(), audit.Entry{
				Actor: audit.Actor{
					ID:    user.ID,
					Email: user.Email,
					Role:  user.Role,
				},
				Action:     action,
				Resource:   resource,
				ResourceID: auditResourceID(r),
				Result:     result,
				Message:    message,
				Metadata: map[string]any{
					"method": r.Method,
					"path":   r.URL.Path,
					"status": ww.status,
				},
				IP:        audit.RequestIP(r),
				UserAgent: r.UserAgent(),
			})
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func auditableMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func auditResourceAction(r *http.Request) (string, string) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1"), "/")
	segment := path
	if index := strings.Index(segment, "/"); index >= 0 {
		segment = segment[:index]
	}

	resource := segment
	switch segment {
	case "server-connections":
		resource = "servers"
	case "proxy-settings":
		resource = "proxy"
	case "telegram-notifications":
		resource = "notifications"
	}

	action := strings.ToLower(r.Method)
	switch {
	case strings.Contains(path, "/ssh/"):
		action = "ssh"
	case path == "updates/apply":
		action = "apply"
	case strings.HasSuffix(path, "/cancel"):
		action = "cancel"
	case r.Method == http.MethodPost:
		action = "create"
	case r.Method == http.MethodPut || r.Method == http.MethodPatch:
		action = "update"
	case r.Method == http.MethodDelete:
		action = "delete"
	}

	return resource, action
}

func auditResourceID(r *http.Request) string {
	for _, key := range []string{"id", "userID"} {
		if value := strings.TrimSpace(chi.URLParam(r, key)); value != "" {
			return value
		}
	}
	return ""
}
