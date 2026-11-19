package server

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	digestpkg "github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/hashmanifest"
)

// Atomic replacements keep the opened inode stable. Detect in-place external
// writes during a response and terminate it rather than send a mixed version.
type versionedDownload struct {
	file        *os.File
	fingerprint string
	changed     bool
}

func (v *versionedDownload) Seek(offset int64, whence int) (int64, error) {
	return v.file.Seek(offset, whence)
}
func (v *versionedDownload) Read(p []byte) (int, error) {
	before, reliable, err := digestpkg.Fingerprint(v.file)
	if err != nil {
		return 0, err
	}
	if reliable && before != v.fingerprint {
		v.changed = true
		return 0, digestpkg.ErrChanged
	}
	n, err := v.file.Read(p)
	after, reliable, checkErr := digestpkg.Fingerprint(v.file)
	if checkErr != nil {
		return 0, checkErr
	}
	if reliable && after != v.fingerprint {
		v.changed = true
		return 0, digestpkg.ErrChanged
	}
	return n, err
}

// Hash the opened file rather than a pathname so the validator and body refer
// to the same file even when another upload replaces the destination.
func serveDownload(w http.ResponseWriter, r *http.Request, file *os.File, info os.FileInfo, stores ...*hashmanifest.Store) error {
	initial, reliable, err := digestpkg.Fingerprint(file)
	if err != nil {
		return err
	}
	digest := ""
	if reliable && len(stores) > 0 && stores[0] != nil {
		if entry, ok, _ := stores[0].Lookup(r.URL.Query().Get("path"), initial); ok && entry.Size == info.Size() {
			digest = entry.Hashes.SHA256
		}
	}
	if digest == "" {
		digest, err = digestpkg.OpenFileDigest(file)
	}
	if err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	current, _, err := digestpkg.Fingerprint(file)
	if err != nil {
		return err
	}
	if reliable && current != initial {
		return digestpkg.ErrChanged
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(file.Name())}))
	w.Header().Set("ETag", "\"sha256:"+digest+"\"")
	w.Header().Set("X-File-Checksum-Algorithm", digestpkg.AlgorithmSHA256)
	w.Header().Set("X-File-Checksum", digest)
	w.Header().Set("X-File-SHA256", digest)
	w.Header().Set("X-File-Size", strconv.FormatInt(info.Size(), 10))
	fingerprint, _, err := digestpkg.Fingerprint(file)
	if err != nil {
		return err
	}
	content := &versionedDownload{file: file, fingerprint: fingerprint}
	http.ServeContent(w, r, file.Name(), info.ModTime(), content)
	if content.changed {
		panic(http.ErrAbortHandler)
	}
	return nil
}
