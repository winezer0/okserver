package server

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/winezer0/okserver/internal/diskbudget"
	"github.com/winezer0/okserver/internal/observability"
	"github.com/winezer0/okserver/internal/protocol"
	"github.com/winezer0/okserver/internal/storage"
	"github.com/winezer0/okserver/internal/upload"
)

type apiError = protocol.Error

func (fs *fileServer) writeUploadError(w http.ResponseWriter, err error, metadata *upload.Metadata) {
	observability.RecordError(w, err)
	if metadata != nil {
		observability.RecordTask(w, metadata.ID)
	}
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, upload.ErrPrecondition):
		status = http.StatusPreconditionFailed
	case errors.Is(err, diskbudget.ErrBusy):
		status = http.StatusTooManyRequests
	case errors.Is(err, upload.ErrBusy):
		status = http.StatusLocked
	case errors.Is(err, storage.ErrInsufficientSpace):
		status = http.StatusInsufficientStorage
	case errors.Is(err, upload.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, upload.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, upload.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, upload.ErrOffset), errors.Is(err, upload.ErrIncomplete):
		status = http.StatusConflict
	case errors.Is(err, upload.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, os.ErrExist):
		status = http.StatusConflict
	case errors.Is(err, upload.ErrChecksum):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		status = http.StatusBadRequest
	case errors.Is(err, os.ErrDeadlineExceeded):
		status = http.StatusRequestTimeout
	default:
		var networkErr net.Error
		if errors.As(err, &networkErr) && networkErr.Timeout() {
			status = http.StatusRequestTimeout
		}
	}
	if status == 429 || status == 423 {
		w.Header().Set("Retry-After", "1")
	}
	writeAPIError(w, status, err.Error(), metadata)
}

func writeAPIError(w http.ResponseWriter, status int, message string, metadata *upload.Metadata) {
	observability.RecordError(w, errors.New(message))
	var public *protocol.Upload
	if metadata != nil {
		value := publicUpload(*metadata)
		public = &value
	}
	writeJSON(w, status, apiError{Error: message, Upload: public})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
