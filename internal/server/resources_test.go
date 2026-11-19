package server

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/upload"
)

func TestHTTPResourceLimitsRecoverAndLogFailures(t *testing.T) {
	var logs bytes.Buffer
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root, MaxSessions: 1, MaxPendingBytes: 4, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	server := &fileServer{path: root, uploads: manager, uploadSlots: make(chan struct{}, 1), logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	call := func(path, hash string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", path, strings.NewReader("te"))
		r.Header.Set("Content-Range", "bytes 0-1/4")
		r.Header.Set("X-Chunk-SHA256", hash)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	server.minFreeBytes = ^uint64(0)
	if got := call("/upload/file", "").Code; got != 507 {
		t.Fatalf("disk: %d", got)
	}
	server.minFreeBytes = 0
	server.uploadSlots <- struct{}{}
	if got := call("/upload/file", "").Code; got != 429 {
		t.Fatalf("slots: %d", got)
	}
	<-server.uploadSlots
	if got := call("/upload/file", strings.Repeat("0", 64)).Code; got != 422 {
		t.Fatalf("hash: %d", got)
	}
	task, err := manager.FindPath("file")
	if err != nil || task.Offset != 0 {
		t.Fatal("corrupt chunk committed", err)
	}
	if got := call("/upload/second", "").Code; got != 413 {
		t.Fatalf("quota: %d", got)
	}
	if got := call("/upload/file", fmt.Sprintf("%x", sha256.Sum256([]byte("te")))).Code; got != 204 {
		t.Fatalf("retry: %d", got)
	}
	if len(server.uploadSlots) != 0 || !strings.Contains(logs.String(), `"error":"checksum mismatch"`) {
		t.Fatal("slots or logging failed")
	}
}
