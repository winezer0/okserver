package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/winezer0/okserver/internal/upload"
)

func TestVerificationWorkerRestoresReadyTasks(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.Create(upload.CreateRequest{Path: "file", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.WriteChunkCalculated(task.ID, 0, 2, 3, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	restored, err := upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	fs := &fileServer{path: root, uploads: restored}
	ctx, cancel := context.WithCancel(context.Background())
	wait := fs.startVerification(ctx, 1)
	defer func() { cancel(); wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err := restored.Get(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Completed {
			if state.Checksum != checksumHex([]byte("abc")) {
				t.Fatal("invalid final hash")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ready task not recovered")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDirectAsynchronousCompletion(t *testing.T) {
	root := t.TempDir()
	manager, _ := upload.NewManager(upload.Config{Root: root})
	fs := &fileServer{path: root, uploads: manager}
	ctx, cancel := context.WithCancel(context.Background())
	wait := fs.startVerification(ctx, 1)
	defer func() { cancel(); wait() }()
	ts := httptest.NewServer(fs)
	defer ts.Close()
	response, err := http.Post(ts.URL+"/upload/file?sha256="+checksumHex([]byte("abc")), "", strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatal(response.StatusCode)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err = ts.Client().Head(ts.URL + "/upload/file")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.Header.Get("Upload-State") == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("verification timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDirectConditionalOverwrite(t *testing.T) {
	ts, _ := newTestHTTPServer(t, "")
	response, err := http.Post(ts.URL+"/upload/file", "", strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for _, tc := range []struct {
		tag  string
		want int
	}{{`"sha256:wrong"`, 412}, {fmt.Sprintf(`"sha256:%s"`, checksumHex([]byte("old"))), 201}} {
		request, _ := http.NewRequest("POST", ts.URL+"/upload/file?overwrite=true", strings.NewReader("new"))
		request.Header.Set("If-Match", tc.tag)
		response, err = ts.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != tc.want {
			t.Fatalf("conditional overwrite: %d want %d", response.StatusCode, tc.want)
		}
		if tc.want == 412 {
			var apiError struct {
				Code string `json:"code"`
			}
			if err = json.NewDecoder(response.Body).Decode(&apiError); err != nil || apiError.Code != "destination_changed" {
				t.Fatalf("stale ETag error code = %q, %v", apiError.Code, err)
			}
		}
		response.Body.Close()
	}
}

func TestConditionalOverwriteAfterExternalReplacement(t *testing.T) {
	ts, root := newTestHTTPServer(t, "")
	response, err := http.Post(ts.URL+"/upload/file", "", strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if err = os.WriteFile(filepath.Join(root, "file"), []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("POST", ts.URL+"/upload/file?overwrite=true", strings.NewReader("new"))
	request.Header.Set("If-Match", fmt.Sprintf(`"sha256:%s"`, checksumHex([]byte("external"))))
	response, err = ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("conditional external replacement: %d", response.StatusCode)
	}
}
