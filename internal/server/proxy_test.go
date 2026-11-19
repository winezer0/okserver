package server

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/upload"
)

func TestHTTPSReverseProxyPreservesAuthSSEChunksAndRanges(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(&fileServer{path: root, param: "token:secret", uploads: manager, uploadIdleTimeout: 0})
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	client := proxy.Client()
	endpoint := func(path string) string {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		return proxy.URL + path + separator + "token=secret"
	}

	unauthorized, err := client.Get(proxy.URL + "/api/v1/files")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthorized.StatusCode)
	}

	payload := []byte("proxy transfer with verified range")
	session, err := manager.Create(upload.CreateRequest{Path: "nested/proxy.bin", Size: int64(len(payload)), ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	var request *http.Request
	events, err := client.Get(endpoint("/api/v1/events"))
	if err != nil {
		t.Fatal(err)
	}
	defer events.Body.Close()
	if events.StatusCode != http.StatusOK || !strings.HasPrefix(events.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE through proxy: %d %q", events.StatusCode, events.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(events.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, session.ID) {
				t.Fatalf("SSE snapshot omitted upload: %s", line)
			}
			break
		}
	}
	for start := 0; start < len(payload); start += 4 {
		end := min(start+4, len(payload))
		chunk := payload[start:end]
		chunkHash := sha256.Sum256(chunk)
		request, _ = http.NewRequest(http.MethodPut, endpoint("/upload/nested/proxy.bin"), bytes.NewReader(chunk))
		request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, len(payload)))
		request.Header.Set("X-Chunk-SHA256", hex.EncodeToString(chunkHash[:]))
		response, requestErr := client.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != 204 && response.StatusCode != 201 {
			t.Fatalf("chunk through proxy: %d", response.StatusCode)
		}
	}
	fileURL := endpoint("/download/nested/proxy.bin")
	request, _ = http.NewRequest(http.MethodGet, fileURL, nil)
	request.Header.Set("Range", "bytes=7-18")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	ranged, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusPartialContent || string(ranged) != string(payload[7:19]) {
		t.Fatalf("range through proxy: %d %q %v", response.StatusCode, ranged, readErr)
	}

	if _, err := os.Stat(filepath.Join(root, "nested", "proxy.bin")); err != nil {
		t.Fatal(err)
	}
}
