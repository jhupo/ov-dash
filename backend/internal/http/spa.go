package http

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type spaHandler struct {
	root string
}

func newSPAHandler(root string) spaHandler {
	return spaHandler{root: filepath.Clean(root)}
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	cleanPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	requested := filepath.Join(h.root, filepath.FromSlash(cleanPath))
	if info, err := os.Stat(requested); err == nil && info.Mode().IsRegular() {
		if strings.HasPrefix(cleanPath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, requested)
		return
	}
	if path.Ext(cleanPath) != "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(h.root, "index.html"))
}
