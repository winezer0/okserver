package server

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	client "github.com/winezer0/okserver/internal/client"
)

func TestDirectCLIUploadAndVerifiedDownload(t *testing.T) {
	ts, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	source := filepath.Join(t.TempDir(), "source.bin")
	payload := []byte("cli serial upload and verified download")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.Execute(client.Options{URL: ts.URL + "/upload/cli.bin", Data: "@" + source, Timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "download.bin")
	if err := client.Execute(client.Options{URL: ts.URL + "/download/cli.bin", Output: target, Timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download differs: %v", err)
	}
}

func TestDirectSerialUploadAndDownload(t *testing.T) {
	ts, _ := newTestHTTPServerWithChunkSize(t, "", 4)
	payload := []byte("abcdefgh")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	endpoint := ts.URL + "/upload/nested/file.bin?sha256=" + sum
	for start := 0; start < len(payload); start += 4 {
		req, _ := http.NewRequest("PUT", endpoint, bytes.NewReader(payload[start:start+4]))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/8", start, start+3))
		req.Header.Set("X-Chunk-SHA256", fmt.Sprintf("%x", sha256.Sum256(payload[start:start+4])))
		response, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		want := 204
		if start == 4 {
			want = 201
		}
		if response.StatusCode != want {
			t.Fatalf("chunk status %d want %d", response.StatusCode, want)
		}
	}
	req, _ := http.NewRequest("GET", ts.URL+"/download/nested/file.bin", nil)
	req.Header.Set("Range", "bytes=2-5")
	response, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 206 || string(body) != "cdef" || response.Header.Get("X-File-SHA256") != sum {
		t.Fatalf("range response %d %q %v", response.StatusCode, body, response.Header)
	}
}

func TestDirectHashParameterPrecedence(t *testing.T) {
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte("abc")))
	req, _ := http.NewRequest("POST", "http://localhost/upload/a?sha256="+sum, nil)
	req.Header.Set("X-File-SHA256", "invalid")
	got, err := expectedSHA256(req)
	if err != nil || got != sum {
		t.Fatalf("precedence: %s %v", got, err)
	}
	req, _ = http.NewRequest("POST", "http://localhost/upload/a?sha256=", nil)
	req.Header.Set("X-File-SHA256", sum)
	if _, err = expectedSHA256(req); err == nil {
		t.Fatal("invalid parameter fell back to header")
	}
}
