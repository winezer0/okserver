package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func secretEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (fs *fileServer) authenticate(w http.ResponseWriter, r *http.Request) bool {
	if fs.auth == "" && fs.param == "" {
		return true
	}
	if fs.auth != "" {
		user, password, ok := r.BasicAuth()
		if ok && secretEqual(user+":"+password, fs.auth) {
			return true
		}
	}
	if fs.param != "" {
		key, value, ok := strings.Cut(fs.param, ":")
		values := r.URL.Query()[key]
		if ok && key != "" && value != "" && len(values) == 1 && secretEqual(values[0], value) {
			return true
		}
	} else {
		w.Header().Set("WWW-Authenticate", `Basic realm="Please Authenticate"`)
	}
	writeAPIError(w, http.StatusUnauthorized, "Authenticate Error", nil)
	return false
}
