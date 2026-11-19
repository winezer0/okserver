package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxManagementBody = 64 << 10
const maxDeletePaths = 500

func (fs *fileServer) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		AllowCreateDir  bool `json:"allow_create_dir"`
		AllowDeleteFile bool `json:"allow_delete_file"`
	}{fs.allowCreateDir, fs.allowDeleteFile})
}

func decodeManagementJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxManagementBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", nil)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "request body must contain one JSON value", nil)
		return false
	}
	return true
}

func (fs *fileServer) handleCreateDirectory(w http.ResponseWriter, r *http.Request) {
	if !fs.allowCreateDir {
		writeAPIError(w, http.StatusForbidden, "directory creation is disabled", nil)
		return
	}
	var input struct {
		Parent string `json:"parent"`
		Name   string `json:"name"`
	}
	if !decodeManagementJSON(w, r, &input) {
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || name == "." || name == ".." || strings.TrimRight(name, " .") != name || strings.ContainsAny(name, "/\\<>|?*") || strings.ContainsRune(name, ':') || filepath.Base(name) != name {
		writeAPIError(w, http.StatusBadRequest, "name must be a single directory name", nil)
		return
	}
	_, parent, err := fs.resolvePath(input.Parent, true)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid parent directory", nil)
		return
	}
	info, err := os.Stat(parent)
	if err != nil {
		writeManagementPathError(w, err)
		return
	}
	if !info.IsDir() {
		writeAPIError(w, http.StatusBadRequest, "parent path is not a directory", nil)
		return
	}
	parentRelative := strings.ReplaceAll(input.Parent, "\\", "/")
	firstParent := strings.Split(parentRelative, "/")[0]
	if strings.EqualFold(firstParent, ".okserver") {
		writeAPIError(w, http.StatusBadRequest, "protected path", nil)
		return
	}
	child := filepath.Join(parent, name)
	if _, _, err = fs.resolvePath(filepath.ToSlash(filepath.Join(input.Parent, name)), false); err != nil {
		writeAPIError(w, http.StatusBadRequest, "directory path is outside the storage root", nil)
		return
	}
	if err = os.Mkdir(child, 0755); err != nil {
		writeManagementPathError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Path string `json:"path"`
	}{filepath.ToSlash(filepath.Join(input.Parent, name))})
}

func (fs *fileServer) handleDeletePaths(w http.ResponseWriter, r *http.Request) {
	if !fs.allowDeleteFile {
		writeAPIError(w, http.StatusForbidden, "file deletion is disabled", nil)
		return
	}
	var input struct {
		Paths []string `json:"paths"`
	}
	if !decodeManagementJSON(w, r, &input) {
		return
	}
	if len(input.Paths) == 0 || len(input.Paths) > maxDeletePaths {
		writeAPIError(w, http.StatusBadRequest, "paths must contain between 1 and 500 entries", nil)
		return
	}
	resolved := make([]string, 0, len(input.Paths))
	seen := make(map[string]struct{}, len(input.Paths))
	for _, value := range input.Paths {
		relative, full, err := fs.resolvePath(value, false)
		first := strings.Split(filepath.ToSlash(relative), "/")[0]
		if err != nil || relative == "" || relative == "." || strings.EqualFold(first, ".okserver") {
			writeAPIError(w, http.StatusBadRequest, "invalid or protected path", nil)
			return
		}
		if _, ok := seen[full]; ok {
			continue
		}
		seen[full] = struct{}{}
		if _, err = os.Lstat(full); err != nil {
			writeManagementPathError(w, err)
			return
		}
		resolved = append(resolved, full)
	}
	deleted := make([]string, 0, len(resolved))
	for _, full := range resolved {
		if err := os.RemoveAll(full); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "one or more paths could not be deleted", nil)
			return
		}
		deleted = append(deleted, filepath.ToSlash(strings.TrimPrefix(full, fs.path+string(os.PathSeparator))))
	}
	writeJSON(w, http.StatusOK, struct {
		Deleted []string `json:"deleted"`
	}{deleted})
}

func writeManagementPathError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		writeAPIError(w, http.StatusNotFound, "path not found", nil)
	case errors.Is(err, os.ErrExist):
		writeAPIError(w, http.StatusConflict, "path already exists", nil)
	case errors.Is(err, os.ErrPermission):
		writeAPIError(w, http.StatusForbidden, "filesystem operation denied", nil)
	default:
		writeAPIError(w, http.StatusInternalServerError, "filesystem operation failed", nil)
	}
}
