package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/hashmanifest"
	"github.com/winezer0/okserver/internal/upload"
)

func newTestHTTPServer(t *testing.T, auth string) (*httptest.Server, string) {
	return newTestHTTPServerWithChunkSize(t, auth, upload.DefaultChunkSize)
}

func newTestHTTPServerWithChunkSize(t *testing.T, auth string, chunkSize int64) (*httptest.Server, string) {
	t.Helper()

	root := t.TempDir()
	stateDir, cacheDir := root+".states", root+".cache"
	manager, err := upload.NewManager(upload.Config{Root: root, StateDir: stateDir, CacheDir: cacheDir, ChunkSize: chunkSize})
	if err != nil {
		t.Fatal(err)
	}
	fs := &fileServer{
		path:              root,
		auth:              auth,
		uploads:           manager,
		hashes:            hashmanifest.New(stateDir),
		uploadIdleTimeout: 2 * time.Minute,
	}
	ts := httptest.NewServer(fs)
	t.Cleanup(func() {
		ts.Close()
	})
	return ts, root
}

func TestDownloadETagDetectsSameSizeSameTimeReplacement(t *testing.T) {
	ts, root := newTestHTTPServer(t, "")
	target := filepath.Join(root, "file.bin")
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.WriteFile(target, []byte("old"), 0600)
	os.Chtimes(target, stamp, stamp)
	for _, endpoint := range []string{"/download/file.bin"} {
		os.WriteFile(target, []byte("old"), 0600)
		os.Chtimes(target, stamp, stamp)
		response, err := ts.Client().Get(ts.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		oldTag := response.Header.Get("ETag")
		response.Body.Close()
		if oldTag == "" || strings.HasPrefix(oldTag, "W/") {
			t.Fatalf("missing strong ETag: %q", oldTag)
		}
		os.WriteFile(target, []byte("new"), 0600)
		os.Chtimes(target, stamp, stamp)
		request, _ := http.NewRequest("GET", ts.URL+endpoint, nil)
		request.Header.Set("Range", "bytes=1-")
		request.Header.Set("If-Range", oldTag)
		response, err = ts.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || string(body) != "new" || response.Header.Get("ETag") == oldTag {
			t.Fatalf("replacement not detected: %d %q", response.StatusCode, body)
		}
	}
}

func TestBasicAuthentication(t *testing.T) {
	ts, _ := newTestHTTPServer(t, "user:pass")
	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("user", "pass")
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestResumableUploadAPI(t *testing.T) {
	ts, root := newTestHTTPServerWithChunkSize(t, "", 4)
	payload := []byte("abcdefghij")
	for _, start := range []int{0, 4, 8} {
		end := min(start+4, len(payload))
		response := putUploadChunk(t, ts, "nested/file.bin", int64(start), payload[start:end], payload, checksumHex(payload[start:end]))
		response.Body.Close()
		want := 204
		if end == len(payload) {
			want = 201
		}
		if response.StatusCode != want {
			t.Fatalf("chunk: %d", response.StatusCode)
		}
	}
	stored, err := os.ReadFile(filepath.Join(root, "nested", "file.bin"))
	if err != nil || !bytes.Equal(stored, payload) {
		t.Fatalf("content mismatch: %v", err)
	}
	response, err := ts.Client().Head(ts.URL + "/upload/nested/file.bin?prefix=sha256")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Upload-State") != "completed" || response.Header.Get("Upload-Prefix-SHA256") != checksumHex(payload) {
		t.Fatal("completion not recoverable")
	}
}

func TestResumableUploadRejectsTraversalAndServesEmbeddedUI(t *testing.T) {
	ts, _ := newTestHTTPServer(t, "")
	body := bytes.NewBufferString(`{"path":"../outside.bin","size":1}`)
	resp, err := ts.Client().Post(ts.URL+"/upload/%2e%2e/outside.bin", "application/octet-stream", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("path traversal status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp, err = ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !bytes.Contains(page, []byte("/app.js")) || bytes.Contains(page, []byte("/assets/app.js")) {
		t.Fatalf("embedded UI route returned status %d or invalid asset links", resp.StatusCode)
	}
}

func putUploadChunk(t *testing.T, ts *httptest.Server, id string, start int64, chunk, full []byte, checksum string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPut, ts.URL+"/upload/"+id, bytes.NewReader(chunk))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(start+int64(len(chunk))-1, 10)+"/"+strconv.Itoa(len(full)))
	request.Header.Set("X-Chunk-SHA256", checksum)
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeUploadStatus(t *testing.T, response *http.Response, wantStatus int, wantOffset int64) {
	t.Helper()
	defer response.Body.Close()
	var metadata upload.Metadata
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus || metadata.Offset != wantOffset {
		t.Fatalf("upload status/offset = %d/%d, want %d/%d", response.StatusCode, metadata.Offset, wantStatus, wantOffset)
	}
}

func checksumHex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
