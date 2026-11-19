package server

import (
	"net/http"
	"strings"

	"github.com/winezer0/okserver/internal/webui"
)

func (fs *fileServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	webui.Handler().ServeHTTP(w, r)
}

func serveStatic(w http.ResponseWriter, r *http.Request) bool {
	stable := r.URL.Path == "/app.js" || r.URL.Path == "/app.css"
	hashed := strings.HasPrefix(r.URL.Path, "/assets/") && webui.HasAsset(r.URL.Path)
	if !stable && !hashed {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeAPIError(w, 405, "method not allowed", nil)
		return true
	}
	if hashed {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	webui.Handler().ServeHTTP(w, r)
	return true
}
