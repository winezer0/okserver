package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/winezer0/okserver/internal/upload"
)

func TestStartupHelpShowsUnifiedTransferExamples(t *testing.T) {
	var output bytes.Buffer
	err := printStartupHelp(&output, map[string]any{"exec": "okserver", "addr": "http://localhost:28080", "example": "example.txt"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"/upload/", "/download/", "curl", "wget", "sha256", "Content-Range"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("help missing %s", value)
		}
	}
}
func TestOffsetQueryAndQueryAuthentication(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(&fileServer{path: root, uploads: manager, param: "token:secret"})
	defer ts.Close()
	request, _ := http.NewRequest("PUT", ts.URL+"/upload/file?token=secret", strings.NewReader("ab"))
	request.Header.Set("Content-Range", "bytes 0-1/4")
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal(response.StatusCode)
	}
	request, _ = http.NewRequest("GET", ts.URL+"/upload/file?token=secret", nil)
	request.Header.Set("Content-Type", "application/offset")
	response, err = ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !strings.HasPrefix(string(body), "offset=2\n") || !strings.Contains(string(body), "curl -C") {
		t.Fatal(response.StatusCode, string(body))
	}
	for _, path := range []string{"/upload/file", "/download/file", "/api/v1/uploads", "/api/v1/events"} {
		response, err = ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatal("authentication bypass", path)
		}
	}
}
