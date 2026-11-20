package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
	"github.com/winezer0/okserver/internal/transfer"
	"github.com/winezer0/okserver/internal/upload"
)

func expectedSHA256(r *http.Request) (string, error) {
	values, present := r.URL.Query()["sha256"]
	if !present {
		values = r.Header.Values("X-File-SHA256")
	}
	if len(values) == 0 {
		return "", nil
	}
	if len(values) != 1 || len(values[0]) != 64 {
		return "", upload.ErrInvalid
	}
	value := strings.ToLower(values[0])
	_, err := hex.DecodeString(value)
	return value, err
}

func (fs *fileServer) directDownload(w http.ResponseWriter, r *http.Request) {
	if fs.downloadSlots != nil {
		select {
		case fs.downloadSlots <- struct{}{}:
			defer func() { <-fs.downloadSlots }()
		default:
			w.Header().Set("Retry-After", "1")
			writeAPIError(w, 429, "too many concurrent downloads", nil)
			return
		}
	}
	request := r.Clone(r.Context())
	copied := *r.URL
	request.URL = &copied
	q := request.URL.Query()
	q.Set("path", strings.TrimPrefix(r.URL.Path, "/download/"))
	request.URL.RawQuery = q.Encode()
	fs.handleFileDownload(w, request)
}

func directStatus(w http.ResponseWriter, metadata upload.Metadata) {
	w.Header().Set("Upload-Offset", strconv.FormatInt(metadata.Offset, 10))
	w.Header().Set("Offset-Length", strconv.FormatInt(metadata.Offset, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(metadata.Size, 10))
	w.Header().Set("Upload-Chunk-Size", strconv.FormatInt(metadata.ChunkSize, 10))
	w.Header().Set("Upload-ID", metadata.ID)
	state := "uploading"
	if metadata.Offset == metadata.Size && !metadata.Completed {
		state = "verifying"
	}
	if metadata.Finalizing {
		state = "verifying"
	}
	if metadata.Completed {
		state = "completed"
	}
	if metadata.Failed {
		state = "failed"
	}
	if metadata.Canceled {
		state = "canceled"
	}
	w.Header().Set("Upload-State", state)
	w.Header().Set("Cache-Control", "no-store")
	if metadata.Completed {
		w.Header().Set("X-File-SHA256", metadata.Checksum)
	}
	if metadata.ErrorCode != "" {
		w.Header().Set("Upload-Error-Code", metadata.ErrorCode)
	}
}

func (fs *fileServer) directUpload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/upload/")
	if name == "" || strings.HasSuffix(name, "/") {
		writeAPIError(w, 400, "a complete destination filename is required", nil)
		return
	}
	relative, target, err := fs.resolvePath(name, false)
	if err != nil {
		writeAPIError(w, 400, "invalid destination", nil)
		return
	}
	release, err := storage.TryDestinationLock(fs.uploads.StateDir(), target+".upload-request")
	if err != nil {
		writeAPIError(w, 409, "another request is writing this destination", nil)
		return
	}
	defer release()
	expected, err := expectedSHA256(r)
	if err != nil {
		writeAPIError(w, 400, "invalid sha256", nil)
		return
	}
	metadata, lookupErr := fs.uploads.FindPath(relative)
	overwrite := (r.Method == http.MethodPost || r.Method == http.MethodPut) && r.URL.Query().Get("overwrite") == "true" && r.Header.Get("If-Match") != ""
	if overwrite && errors.Is(lookupErr, upload.ErrConflict) && metadata.Completed {
		// Explicit conditional replacement validates the current target version in Create.
		lookupErr = nil
	}
	owner := "anonymous"
	if user, password, ok := r.BasicAuth(); ok && fs.auth != "" && secretEqual(user+":"+password, fs.auth) {
		owner = "basic:" + user
	} else if fs.param != "" {
		owner = "parameter"
	}
	owner = fmt.Sprintf("%x", sha256.Sum256([]byte(owner)))
	if lookupErr == nil && !metadata.Canceled && !metadata.Failed {
		if r.Method == "HEAD" || r.Method == "GET" || r.Method == "DELETE" {
			err = upload.CheckAccess(metadata, owner, r.Header.Get("Upload-Token"))
		} else {
			metadata, err = fs.uploads.Authorize(metadata.ID, owner, r.Header.Get("Upload-Token"), r.Header.Get("Upload-Takeover") == "true" && owner != fmt.Sprintf("%x", sha256.Sum256([]byte("anonymous"))))
		}
		if err != nil {
			fs.writeUploadError(w, err, nil)
			return
		}
	}
	if r.Method == http.MethodHead || r.Method == http.MethodGet {
		w.Header().Set("Upload-Chunk-Size", strconv.FormatInt(fs.uploads.ChunkSize(), 10))
		if lookupErr != nil {
			fs.writeUploadError(w, lookupErr, nil)
			return
		}
		directStatus(w, metadata)
		if r.URL.Query().Get("prefix") == "sha256" && !metadata.Canceled && !metadata.Failed {
			value, err := fs.uploads.PrefixSHA256Context(r.Context(), metadata.ID)
			if err != nil {
				fs.writeUploadError(w, err, nil)
				return
			}
			w.Header().Set("Upload-Prefix-SHA256", value)
		}
		if r.Method == http.MethodGet {
			if r.Header.Get("Content-Type") != "application/offset" {
				writeAPIError(w, 405, "use HEAD to query upload status", nil)
				return
			}
			fmt.Fprintf(w, "offset=%d\n", metadata.Offset)
			fmt.Fprintln(w, "Continue with curl -C <Upload-Offset> -T <local-file> <this-upload-URL>")
		}
		return
	}
	if r.Method == http.MethodDelete {
		if lookupErr != nil {
			fs.writeUploadError(w, lookupErr, nil)
			return
		}
		if metadata.Completed {
			writeAPIError(w, 409, "file already published", nil)
			return
		}
		if cancel, ok := fs.verificationCancel.Load(metadata.ID); ok {
			cancel.(context.CancelFunc)()
		}
		if err := fs.uploads.Cancel(metadata.ID); err != nil {
			fs.writeUploadError(w, err, nil)
			return
		}
		w.WriteHeader(204)
		return
	}
	var body io.Reader = r.Body
	total, start, end := r.ContentLength, int64(0), r.ContentLength-1
	chunked := r.Method == http.MethodPut && r.Header.Get("Content-Range") != ""
	if chunked {
		start, end, total, err = transfer.ScanRangeSize(r.Header)
		if err != nil {
			writeAPIError(w, 400, "invalid Content-Range", nil)
			return
		}
		if r.ContentLength >= 0 && r.ContentLength != end-start+1 {
			writeAPIError(w, 400, "length mismatch", nil)
			return
		}
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media == "multipart/form-data" {
		if r.Method != http.MethodPost {
			writeAPIError(w, 400, "multipart requires POST", nil)
			return
		}
		reader, err := r.MultipartReader()
		if err != nil {
			writeAPIError(w, 400, "invalid multipart", nil)
			return
		}
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "file" || part.FileName() == "" {
			writeAPIError(w, 400, "one file field is required", nil)
			return
		}
		temp, length, cleanup, err := fs.spool(part)
		if err != nil {
			fs.writeSpoolError(w, err)
			return
		}
		defer cleanup()
		total = length
		if _, err = reader.NextPart(); !errors.Is(err, io.EOF) {
			writeAPIError(w, 400, "only one file is allowed", nil)
			return
		}
		if _, err = temp.Seek(0, io.SeekStart); err != nil {
			fs.writeUploadError(w, err, nil)
			return
		}
		body = temp
		end = total - 1
	}
	if total < 0 {
		temp, length, cleanup, err := fs.spool(body)
		if err != nil {
			fs.writeSpoolError(w, err)
			return
		}
		defer cleanup()
		total, body, end = length, temp, length-1
	}
	if values, ok := r.URL.Query()["size"]; ok {
		if len(values) != 1 {
			writeAPIError(w, 400, "duplicate size", nil)
			return
		}
		size, e := strconv.ParseInt(values[0], 10, 64)
		if e != nil || size < 0 || size != total {
			writeAPIError(w, 400, "file size mismatch", nil)
			return
		}
	}
	if _, err := os.Stat(target); err == nil && !overwrite {
		writeAPIErrorWithCode(w, 409, "destination already exists", nil, "destination_exists")
		return
	}
	if lookupErr == nil && !metadata.Canceled && !metadata.Failed && !(metadata.Completed && overwrite) {
		directStatus(w, metadata)
		if !chunked || metadata.Completed || total != metadata.Size || start != metadata.Offset {
			writeAPIError(w, 409, "upload conflicts with current offset", nil)
			return
		}
	} else {
		if lookupErr != nil && !errors.Is(lookupErr, upload.ErrNotFound) {
			fs.writeUploadError(w, lookupErr, nil)
			return
		}
		if start != 0 {
			writeAPIError(w, 409, "upload must start at zero", nil)
			return
		}
		metadata, err = fs.uploads.Create(upload.CreateRequest{Path: relative, Size: total, ChecksumAlgorithm: digest.AlgorithmSHA256, IfMatch: r.Header.Get("If-Match"), Owner: owner, Token: r.Header.Get("Upload-Token")})
		if err != nil {
			fs.writeUploadError(w, err, nil)
			return
		}
	}
	metadata, err = fs.uploads.BindSHA256(metadata.ID, expected)
	if err != nil {
		fs.writeUploadError(w, err, &metadata)
		return
	}
	expected = metadata.ExpectedSHA256
	metadata, err = fs.uploads.Authorize(metadata.ID, owner, r.Header.Get("Upload-Token"), false)
	if err != nil {
		fs.writeUploadError(w, err, nil)
		return
	}
	if chunked && total > 0 {
		checksum := r.Header.Get("X-Chunk-SHA256")
		if checksum != "" {
			metadata, err = fs.uploads.WriteChunk(metadata.ID, start, end, total, checksum, body)
		} else if end-start+1 > fs.uploads.ChunkSize() {
			metadata, err = fs.appendCurlBody(metadata.ID, metadata, body, end-start+1)
		} else {
			metadata, err = fs.uploads.WriteChunkCalculated(metadata.ID, start, end, total, body)
		}
	} else if total > 0 {
		metadata, err = fs.appendCurlBody(metadata.ID, metadata, body, total)
	}
	if err != nil {
		fs.writeUploadError(w, err, &metadata)
		return
	}
	if metadata.Offset == metadata.Size {
		if fs.verificationQueue != nil {
			fs.enqueueVerification(metadata)
			directStatus(w, metadata)
			w.Header().Set("Upload-State", "verifying")
			writeJSON(w, 202, publicUpload(metadata))
			return
		}
		if expected != "" {
			metadata, err = fs.uploads.Complete(metadata.ID, expected)
		} else {
			metadata, err = fs.uploads.CompleteCalculated(metadata.ID)
		}
		if err != nil {
			fs.writeUploadError(w, err, &metadata)
			return
		}
		fs.rememberUploadedHashes(r.Context(), metadata)
	}
	directStatus(w, metadata)
	if metadata.Completed {
		writeJSON(w, 201, publicUpload(metadata))
	} else {
		w.WriteHeader(204)
	}
}
