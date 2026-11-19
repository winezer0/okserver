package server

import (
	"net/http"
	"strings"
)

func (fs *fileServer) middleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w, finish := fs.metrics.Observe(w, r, fs.logger)
	defer finish()

	if r.Body != nil {
		//goland:noinspection GoUnhandledErrorResult
		defer r.Body.Close()
	}
	if fs.disableFrontUI && isFrontendRequest(r.URL.Path) {
		writeAPIError(w, http.StatusForbidden, "frontend access is disabled", nil)
		return
	}
	if serveStatic(w, r) {
		return
	}

	if r.RequestURI == "/favicon.ico" {
		//goland:noinspection GoUnhandledErrorResult
		w.Write(icoData) // 返回网页的图标
		return
	}

	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) && fs.uploadIdleTimeout > 0 {
		r.Body = &idleTimeoutBody{ReadCloser: r.Body, controller: http.NewResponseController(w), timeout: fs.uploadIdleTimeout}
	}

	if !fs.allowOrigin(w, r) || !fs.authenticate(w, r) {
		return
	}

	next(w, r)
}

func isFrontendRequest(path string) bool {
	return path == "/" || path == "/files" || strings.HasPrefix(path, "/files/") || path == "/tasks" ||
		path == "/app.js" || path == "/app.css" || path == "/favicon.ico" || strings.HasPrefix(path, "/assets/")
}
