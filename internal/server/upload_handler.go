package server

import (
	"net/http"
)

func (fs *fileServer) handleUploadList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"chunk_size": fs.uploads.ChunkSize(), "uploads": publicUploads(fs.uploads.Tasks())})
}

func (fs *fileServer) handleUploadCleanup(w http.ResponseWriter, r *http.Request) {
	if fs.uploads != nil {
		fs.uploads.PruneExpired()
	}

	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (fs *fileServer) handleUploadGet(w http.ResponseWriter, r *http.Request, id string) {

	metadata, err := fs.uploads.Snapshot(id)
	if err != nil {
		fs.writeUploadError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, publicUpload(metadata))

}

func (fs *fileServer) handleUploadCancel(w http.ResponseWriter, r *http.Request, id string) {

	metadata, err := fs.uploads.Snapshot(id)
	if err != nil {
		fs.writeUploadError(w, err, nil)
		return
	}
	request := r.Clone(r.Context())
	copied := *r.URL
	request.URL = &copied
	request.URL.Path = "/upload/" + metadata.Path
	request.URL.RawPath = ""
	fs.directUpload(w, request)

}
