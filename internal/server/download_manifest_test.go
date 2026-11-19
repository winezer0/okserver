package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/hashmanifest"
)

func TestDownloadUsesFingerprintValidatedManifest(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "file")
	if err := os.WriteFile(filename, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, _ := file.Stat()
	fingerprint, reliable, err := digest.Fingerprint(file)
	if err != nil {
		t.Fatal(err)
	}
	if !reliable {
		t.Skip("filesystem has no reliable fingerprint")
	}
	store := hashmanifest.New(root)
	// A sentinel distinguishes manifest reuse from recalculation; manifests are
	// trusted internal state and may only be written with verified upload hashes.
	sentinel := strings.Repeat("a", 64)
	if err = store.Put(hashmanifest.Entry{Path: "file", Size: 3, Fingerprint: fingerprint, Hashes: digest.Hashes{SHA256: sentinel}}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("HEAD", "/download/file?path=file", nil)
	recorder := httptest.NewRecorder()
	if err = serveDownload(recorder, request, file, info, store); err != nil {
		t.Fatal(err)
	}
	if recorder.Header().Get("X-File-SHA256") != sentinel {
		t.Fatal("manifest was ignored")
	}
	if err = store.Put(hashmanifest.Entry{Path: "file", Size: 3, Fingerprint: "stale", Hashes: digest.Hashes{SHA256: sentinel}}); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	if err = serveDownload(recorder, request, file, info, store); err != nil {
		t.Fatal(err)
	}
	if recorder.Header().Get("X-File-SHA256") == sentinel {
		t.Fatal("stale manifest was trusted")
	}
}
