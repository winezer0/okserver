package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func managementServer(t *testing.T, create, remove bool) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	ts := httptest.NewServer(&fileServer{path: root, allowCreateDir: create, allowDeleteFile: remove, auth: "user:pass"})
	t.Cleanup(ts.Close)
	return ts, root
}

func managementRequest(t *testing.T, ts *httptest.Server, endpoint, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, ts.URL+endpoint, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("user", "pass")
	request.Header.Set("Content-Type", "application/json")
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func TestCreateDirectorySingleLevelAndRootConfinement(t *testing.T) {
	ts, root := managementServer(t, true, false)
	if response := managementRequest(t, ts, "/api/v1/create", "{\"parent\":\"\",\"name\":\"one\"}"); response.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", response.StatusCode)
	}
	if response := managementRequest(t, ts, "/api/v1/create", "{\"parent\":\"one\",\"name\":\"two/three\"}"); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("nested name status=%d", response.StatusCode)
	}
	if response := managementRequest(t, ts, "/api/v1/create", "{\"parent\":\"../outside\",\"name\":\"x\"}"); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("traversal status=%d", response.StatusCode)
	}
	if info, err := os.Stat(filepath.Join(root, "one")); err != nil || !info.IsDir() {
		t.Fatalf("created directory: %v", err)
	}
}

func TestDeleteFilesAndDirectoriesBatchAndConfigGate(t *testing.T) {
	ts, root := managementServer(t, false, true)
	if err := os.MkdirAll(filepath.Join(root, "tree", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tree", "child", "file"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	response := managementRequest(t, ts, "/api/v1/delete", "{\"paths\":[\"tree\",\"file\"]}")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("delete status=%d", response.StatusCode)
	}
	for _, path := range []string{filepath.Join(root, "tree"), filepath.Join(root, "file")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("path remains: %s", path)
		}
	}
	if response := managementRequest(t, ts, "/api/v1/delete", "{\"paths\":[\"../outside\"]}"); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("traversal status=%d", response.StatusCode)
	}
	if response := managementRequest(t, ts, "/api/v1/delete", "{\"paths\":[\".\"]}"); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("root delete status=%d", response.StatusCode)
	}
}

func TestManagementDisabledByDefault(t *testing.T) {
	ts, _ := managementServer(t, false, false)
	if response := managementRequest(t, ts, "/api/v1/create", "{\"parent\":\"\",\"name\":\"no\"}"); response.StatusCode != http.StatusForbidden {
		t.Fatalf("create disabled status=%d", response.StatusCode)
	}
	if response := managementRequest(t, ts, "/api/v1/delete", "{\"paths\":[\"file\"]}"); response.StatusCode != http.StatusForbidden {
		t.Fatalf("delete disabled status=%d", response.StatusCode)
	}
}

func TestManagementAndCapabilitiesRequireAuthentication(t *testing.T) {
	ts, _ := managementServer(t, true, true)
	for _, endpoint := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/capabilities", ""},
		{http.MethodPost, "/api/v1/create", "{\"parent\":\"\",\"name\":\"unauthorized\"}"},
		{http.MethodPost, "/api/v1/delete", "{\"paths\":[\"file\"]}"},
	} {
		request, err := http.NewRequest(endpoint.method, ts.URL+endpoint.path, bytes.NewBufferString(endpoint.body))
		if err != nil {
			t.Fatal(err)
		}
		response, err := ts.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated %s %s status=%d, want 401", endpoint.method, endpoint.path, response.StatusCode)
		}
	}
}
