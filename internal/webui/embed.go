package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:generate npm --prefix ../../web run build
//go:embed assets/*
var embedded embed.FS

func Handler() http.Handler {
	assets, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(assets))
}

func HasAsset(urlPath string) bool {
	name := "assets/" + strings.TrimPrefix(urlPath, "/")
	if !fs.ValidPath(name) {
		return false
	}
	info, err := fs.Stat(embedded, name)
	return err == nil && !info.IsDir()
}

// Index serves the SPA shell for client-side deep links without rewriting file URLs.
func Index(w http.ResponseWriter, r *http.Request) {
	request := r.Clone(r.Context())
	request.URL.Path = "/"
	w.Header().Set("Cache-Control", "no-cache")
	Handler().ServeHTTP(w, request)
}
