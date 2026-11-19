package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/winezer0/okserver/internal/storage"
)

func (fs *fileServer) uploadEndpoint(next http.HandlerFunc, write bool) gin.HandlerFunc {
	return fs.endpoint(func(w http.ResponseWriter, r *http.Request) {
		if write {
			fs.admitUpload(w, r, next)
			return
		}
		next(w, r)
	}, true)
}

func (fs *fileServer) admitUpload(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if fs.uploadSlots != nil {
		select {
		case fs.uploadSlots <- struct{}{}:
			defer func() { <-fs.uploadSlots }()
		default:
			w.Header().Set("Retry-After", "1")
			writeAPIError(w, http.StatusTooManyRequests, "too many concurrent uploads", nil)
			return
		}
	}
	if fs.minFreeBytes > 0 {
		free, err := storage.FreeSpace(fs.path)
		if err != nil || free < fs.minFreeBytes {
			writeAPIError(w, http.StatusInsufficientStorage, "insufficient free disk space", nil)
			return
		}
	}
	next(w, r)
}
