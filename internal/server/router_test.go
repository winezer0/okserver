package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/winezer0/okserver/internal/upload"
)

func TestVersionedAPIAndSPALinks(t *testing.T) {
	server, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	for _, endpoint := range []string{"/api/v1/files"} {
		response, err := http.Get(server.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		var data fileList
		err = json.NewDecoder(response.Body).Decode(&data)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || data.Page != 1 || data.Limit != 100 {
			t.Fatalf("contract mismatch: %+v %v", data, err)
		}
	}
	response, err := http.Get(server.URL + "/files/nested")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	page, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !bytes.Contains(page, []byte("/app.js")) {
		t.Fatal("SPA deep link failed")
	}
	for _, body := range []string{`{"path":"","size":1}`, `{"path":"file","size":-1}`, `{"path":"file","size":1,"sha256":"bad"}`, `{"path":"file","size":1,"unknown":true}`} {
		response, err := http.Post(server.URL+"/api/v1/uploads", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 405 {
			t.Fatalf("invalid metadata accepted: %s", body)
		}
	}
}

func TestDenyDisablesFrontendPagesAndAssets(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	fs := &fileServer{path: root, uploads: manager, disableFrontUI: true}
	server := httptest.NewServer(fs)
	defer server.Close()
	for _, endpoint := range []string{"/", "/files", "/files/nested", "/tasks", "/app.js", "/app.css", "/favicon.ico"} {
		response, err := http.Get(server.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("%s status %d, want %d", endpoint, response.StatusCode, http.StatusForbidden)
		}
	}
	response, err := http.Get(server.URL + "/api/v1/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("API status %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestEventStreamSnapshotAndDisconnect(t *testing.T) {
	server, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/events", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("bad event stream: %v", response)
	}
	reader := bufio.NewReader(response.Body)
	found := false
	for count := 0; count < 4; count++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data: ") {
			var snapshot struct {
				Version int               `json:"version"`
				Uploads []json.RawMessage `json:"uploads"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snapshot); err != nil || snapshot.Version != 1 {
				t.Fatal("invalid snapshot")
			}
			found = true
		}
		if line == "\n" {
			break
		}
	}
	if !found {
		t.Fatal("no task snapshot")
	}
	cancel()
	response.Body.Close()
	// The server must finish a canceled stream so httptest.Close does not hang.
}

func TestEventStreamRequiresAuthenticationAndHasBound(t *testing.T) {
	server, _ := newTestHTTPServerWithChunkSize(t, "user:secret", 4)
	response, err := http.Get(server.URL + "/api/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("event authentication bypassed")
	}
	fs := &fileServer{}
	fs.eventClients.Store(16)
	// An unavailable upload service is rejected before reserving an event slot.
	recorder := httptest.NewRecorder()
	fs.handleEvents(recorder, httptest.NewRequest("GET", "/api/v1/events", nil))
	if recorder.Code != 503 || fs.eventClients.Load() != 16 {
		t.Fatal("event slot leaked")
	}
	manager, err := upload.NewManager(upload.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	fs.uploads = manager
	recorder = httptest.NewRecorder()
	fs.handleEvents(recorder, httptest.NewRequest("GET", "/api/v1/events", nil))
	if recorder.Code != 429 || fs.eventClients.Load() != 16 {
		t.Fatal("event connection bound was not enforced")
	}
}

type deadlineResponse struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (writer *deadlineResponse) SetReadDeadline(value time.Time) error {
	writer.deadline = value
	return nil
}
func TestGinTransportPreservesDeadlineControl(t *testing.T) {
	original := &deadlineResponse{ResponseRecorder: httptest.NewRecorder()}
	_, router := gin.CreateTestContext(original)
	router.GET("/", func(c *gin.Context) {
		writer := &transportWriter{ResponseWriter: c.Writer, original: original}
		if err := http.NewResponseController(writer).SetReadDeadline(time.Now()); err != nil {
			t.Fatal(err)
		}
	})
	router.ServeHTTP(original, httptest.NewRequest("GET", "/", nil))
	if original.deadline.IsZero() {
		t.Fatal("Gin lost original connection controls")
	}
}

func TestEmbeddedAssetsPublicButDataProtected(t *testing.T) {
	server, root := newTestHTTPServerWithChunkSize(t, "user:secret", 4)
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "private.txt"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/app.js", "/app.css"} {
		response, err := http.Get(server.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("public frontend code unavailable: %s", endpoint)
		}
	}
	for _, endpoint := range []string{"/api/v1/files", "/assets/private.txt"} {
		response, err := http.Get(server.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("data authentication bypass: %s", endpoint)
		}
	}
}

func TestQueryAuthenticationWorksWithModuleAssets(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	fs := &fileServer{path: root, param: "token:secret", uploads: manager}
	server := httptest.NewServer(fs)
	defer server.Close()
	for endpoint, status := range map[string]int{"/?token=secret": 200, "/app.js": 200, "/app.css": 200, "/api/v1/files?token=secret": 200, "/api/v1/files": 401} {
		response, err := http.Get(server.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("%s status %d, want %d", endpoint, response.StatusCode, status)
		}
	}
}

func TestInvalidQueryAuthenticationConfigRejected(t *testing.T) {
	config := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(config, []byte("server:\n  param: invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run("okserver", []string{"-c", config}); err == nil || !strings.Contains(err.Error(), "server.param must") {
		t.Fatalf("invalid authentication configuration accepted: %v", err)
	}
}
