package server

import (
	"errors"
	"net/http"
	"os"

	"github.com/winezer0/okserver/internal/filebrowser"
	"github.com/winezer0/okserver/internal/protocol"
)

type fileEntry = protocol.FileEntry
type fileList = protocol.FileList

func (fs *fileServer) handleFileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	relative, fullPath, err := fs.resolvePath(r.URL.Query().Get("path"), true)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	result, err := filebrowser.ReadPage(fullPath, relative, r.URL.Query())
	if err != nil {
		if errors.Is(err, filebrowser.ErrInvalidQuery) {
			writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		} else if errors.Is(err, os.ErrNotExist) {
			writeAPIError(w, http.StatusNotFound, "directory not found", nil)
		} else {
			writeAPIError(w, http.StatusInternalServerError, err.Error(), nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (fs *fileServer) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	_, fullPath, err := fs.resolvePath(r.URL.Query().Get("path"), false)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	f, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeAPIError(w, http.StatusNotFound, "file not found", nil)
		} else {
			writeAPIError(w, http.StatusForbidden, err.Error(), nil)
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if info.IsDir() {
		writeAPIError(w, http.StatusBadRequest, "path is a directory", nil)
		return
	}
	if err = serveDownload(w, r, f, info, fs.hashes); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error(), nil)
	}
}
