package server

import (
	"net/http"
	"os"

	"github.com/winezer0/okserver/internal/storage"
)

func (fs *fileServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/readyz" {
		if _, err := os.Stat(fs.path); err != nil {
			writeAPIError(w, 503, "storage unavailable", nil)
			return
		}
		if fs.minFreeBytes > 0 {
			if free, err := storage.FreeSpace(fs.path); err != nil || free < fs.minFreeBytes {
				writeAPIError(w, 503, "insufficient storage", nil)
				return
			}
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
