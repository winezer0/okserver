package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

func validOrigin(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// Cross-origin data requests require an explicit exact origin allowlist.
// Preflight exposes protocol capabilities only; actual data requests authenticate.
func (fs *fileServer) allowOrigin(w http.ResponseWriter, r *http.Request) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 || !validOrigin(values[0]) {
		writeAPIError(w, 403, "invalid request origin", nil)
		return false
	}
	origin := values[0]
	u, _ := url.Parse(origin)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	same := strings.EqualFold(u.Host, r.Host) && u.Scheme == scheme
	if !same && !slices.Contains(fs.allowedOrigins, origin) {
		writeAPIError(w, 403, "request origin is not allowed", nil)
		return false
	}
	w.Header().Add("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Expose-Headers", "X-File-SHA256, X-File-Size, ETag, Content-Range, Content-Disposition, Upload-Offset, Upload-Length, Upload-Chunk-Size, Upload-State, Upload-Error-Code, Upload-ID, Upload-Prefix-SHA256, Offset-Length, Retry-After")
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		if !slices.Contains([]string{"GET", "HEAD", "POST", "PUT", "DELETE"}, r.Header.Get("Access-Control-Request-Method")) {
			writeAPIError(w, 405, "preflight method is not allowed", nil)
			return false
		}
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Content-Range, X-File-SHA256, X-Chunk-SHA256, Upload-Token, Upload-Takeover, If-Match, If-Range, Range")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.WriteHeader(204)
		return false
	}
	return true
}
