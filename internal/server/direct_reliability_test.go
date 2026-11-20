package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/winezer0/okserver/internal/upload"
)

func TestDirectPostValidationAndNoOverwrite(t *testing.T) {
	for _, tc := range []struct {
		name, query, header string
		want                int
	}{
		{"optional", "", "", 201}, {"correct", "?sha256=" + fmt.Sprintf("%x", sha256.Sum256([]byte("abc"))), "invalid", 201},
		{"bad", "?sha256=" + strings.Repeat("0", 64), "", 422}, {"empty", "?sha256=", "", 400}, {"duplicate", "?sha256=a&sha256=b", "", 400}, {"size", "?size=2", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, root := newTestHTTPServerWithChunkSize(t, "", 4)
			request, _ := http.NewRequest("POST", ts.URL+"/upload/file"+tc.query, strings.NewReader("abc"))
			if tc.header != "" {
				request.Header.Set("X-File-SHA256", tc.header)
			}
			response, err := ts.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != tc.want {
				t.Fatalf("status %d want %d", response.StatusCode, tc.want)
			}
			if tc.want != 201 {
				if _, err = os.Stat(filepath.Join(root, "file")); !os.IsNotExist(err) {
					t.Fatal("invalid upload was published")
				}
			} else {
				request, _ = http.NewRequest("POST", ts.URL+"/upload/file", strings.NewReader("new"))
				response, err = ts.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != 409 {
					t.Fatal("overwrite allowed")
				}
				var apiError struct {
					Code string `json:"code"`
				}
				if err = json.NewDecoder(response.Body).Decode(&apiError); err != nil || apiError.Code != "destination_exists" {
					t.Fatalf("same-name upload error code = %q, %v", apiError.Code, err)
				}
				response.Body.Close()
			}
		})
	}
}

func TestDirectMultipartAndUnknownLength(t *testing.T) {
	ts, root := newTestHTTPServerWithChunkSize(t, "", 4)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "ignored-name")
	part.Write([]byte("form-data"))
	writer.Close()
	request, _ := http.NewRequest("POST", ts.URL+"/upload/form.bin", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("multipart %d", response.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(root, "form.bin"))
	if string(got) != "form-data" {
		t.Fatal("multipart content differs")
	}
	request, _ = http.NewRequest("POST", ts.URL+"/upload/stream.bin", io.NopCloser(strings.NewReader("stream")))
	request.ContentLength = -1
	response, err = ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("stream %d", response.StatusCode)
	}
}

func TestDirectTokenIsolationCancelAndRestore(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	makeServer := func(m *upload.Manager) *httptest.Server {
		return httptest.NewServer(&fileServer{path: root, uploads: m, param: "token:secret"})
	}
	ts := makeServer(manager)
	call := func(method, token, body, rangeHeader string) int {
		request, _ := http.NewRequest(method, ts.URL+"/upload/file?token=secret", strings.NewReader(body))
		request.Header.Set("Upload-Token", token)
		if rangeHeader != "" {
			request.Header.Set("Content-Range", rangeHeader)
		}
		response, err := ts.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	token := strings.Repeat("a", 32)
	if got := call("PUT", token, "abcd", "bytes 0-3/8"); got != 204 {
		t.Fatalf("first %d", got)
	}
	if got := call("PUT", strings.Repeat("b", 32), "efgh", "bytes 4-7/8"); got != 409 {
		t.Fatalf("token bypass %d", got)
	}
	ts.Close()
	restored, err := upload.NewManager(upload.Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	ts = makeServer(restored)
	defer ts.Close()
	if got := call("PUT", token, "efgh", "bytes 4-7/8"); got != 201 {
		t.Fatalf("restore %d", got)
	}
	if got := call("DELETE", token, "", ""); got != 409 {
		t.Fatalf("deleted published file %d", got)
	}
}

func TestDirectChunkCorruptionAndOffsetRecovery(t *testing.T) {
	ts, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	call := func(rangeHeader, hash string) int {
		request, _ := http.NewRequest("PUT", ts.URL+"/upload/file", strings.NewReader("abcd"))
		request.Header.Set("Content-Range", rangeHeader)
		request.Header.Set("X-Chunk-SHA256", hash)
		response, err := ts.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if got := call("bytes 0-3/8", strings.Repeat("0", 64)); got != 422 {
		t.Fatal(got)
	}
	response, err := ts.Client().Head(ts.URL + "/upload/file")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.Header.Get("Upload-Offset") != "0" {
		t.Fatal("corrupt chunk advanced offset")
	}
	if got := call("bytes 0-3/8", fmt.Sprintf("%x", sha256.Sum256([]byte("abcd")))); got != 204 {
		t.Fatal(got)
	}
	if got := call("bytes 0-3/8", ""); got != 409 {
		t.Fatalf("duplicate range appended %d", got)
	}
}

func TestRemovedUploadProtocolsAndDirectoryAddress(t *testing.T) {
	ts, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	for _, path := range []string{"/api/v1/uploads", "/api/v1/curl/upload", "/api/v1/curl/uploads", "/upload/directory/"} {
		request, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader("abc"))
		response, err := ts.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode < 400 {
			t.Fatalf("legacy/directory endpoint accepted %s", path)
		}
	}
}
