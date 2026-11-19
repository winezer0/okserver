package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/winezer0/okserver/internal/upload"
)

func TestRouteMethodsAndPublicUploadContract(t *testing.T) {
	server, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	response, err := http.Post(server.URL+"/upload/file", "application/octet-stream", strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]json.RawMessage
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil || response.StatusCode != 201 {
		t.Fatal("upload contract", err)
	}
	for _, private := range []string{"base_checksum", "request_sha256", "version", "owner", "token_sha256"} {
		if _, ok := result[private]; ok {
			t.Fatalf("exposed %s", private)
		}
	}
	for _, item := range []struct {
		method, path string
		status       int
	}{{"PATCH", "/upload/file", 405}, {"POST", "/download/file", 405}, {"POST", "/api/v1/uploads", 405}, {"GET", "/unknown", 404}} {
		r, _ := http.NewRequest(item.method, server.URL+item.path, nil)
		resp, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != item.status {
			t.Fatalf("%s: %d", item.path, resp.StatusCode)
		}
	}
}

func TestRemovedProtocolsReturnNotFound(t *testing.T) {
	server, root := newTestHTTPServer(t, "")
	for _, endpoint := range []string{"/api/files", "/api/uploads", "/api/events", "/file.bin", "/.okserver/uploads"} {
		for _, method := range []string{"GET", "POST", "PUT"} {
			request, _ := http.NewRequest(method, server.URL+endpoint, bytes.NewBufferString("data"))
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("%s %s: %d", method, endpoint, response.StatusCode)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "file.bin")); !os.IsNotExist(err) {
		t.Fatal("removed transport wrote a file")
	}
}

func TestVersionedDownloadSupportsHeadAndConditionalRanges(t *testing.T) {
	server, root := newTestHTTPServer(t, "")
	if err := os.WriteFile(filepath.Join(root, "file.bin"), []byte("abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/download/file.bin"
	response, err := server.Client().Head(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	tag := response.Header.Get("ETag")
	response.Body.Close()
	if response.StatusCode != 200 || response.ContentLength != 6 || tag == "" || response.Header.Get("X-File-SHA256") == "" {
		t.Fatal("invalid HEAD response")
	}
	request, _ := http.NewRequest("GET", endpoint, nil)
	request.Header.Set("Range", "bytes=2-4")
	request.Header.Set("If-Range", tag)
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 206 || string(body) != "cde" {
		t.Fatalf("range response: %d %q %v", response.StatusCode, body, err)
	}
}

func TestCleanupRemainsAvailableWhenStorageAndUploadSlotsAreFull(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	server := &fileServer{path: root, uploads: manager, minFreeBytes: ^uint64(0), uploadSlots: make(chan struct{}, 1)}
	server.uploadSlots <- struct{}{}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("POST", "/api/v1/uploads/cleanup", nil))
	if response.Code != 200 {
		t.Fatalf("cleanup blocked: %d %s", response.Code, response.Body.String())
	}
	if len(server.uploadSlots) != 1 {
		t.Fatal("cleanup changed active upload slots")
	}
}

func TestAuthenticationPrecedesUploadAdmission(t *testing.T) {
	server, _ := newTestHTTPServerWithChunkSize(t, "user:secret", 4)
	response, err := http.Post(server.URL+"/api/v1/uploads", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("authentication returned %d", response.StatusCode)
	}
}
