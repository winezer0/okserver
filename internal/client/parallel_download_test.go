package client

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelDownloadIntegrityAndConcurrency(t *testing.T) {
	payload := bytes.Repeat([]byte("parallel"), 3<<20)
	checksum := sha256Digest(payload)
	for _, mode := range []string{"normal", "corrupt", "wrong-version", "oversized", "retry"} {
		t.Run(mode, func(t *testing.T) {
			var active, peak, attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-File-SHA256", checksum)
				w.Header().Set("X-File-Size", fmt.Sprint(len(payload)))
				w.Header().Set("ETag", `"sha256:`+checksum+`"`)
				w.Header().Set("Accept-Ranges", "bytes")
				if r.Method == "HEAD" {
					w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
					return
				}
				if mode == "retry" && attempts.Add(1) == 1 {
					w.WriteHeader(503)
					return
				}
				current := active.Add(1)
				defer active.Add(-1)
				for {
					old := peak.Load()
					if current <= old || peak.CompareAndSwap(old, current) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				var start, end int
				if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				data := append([]byte(nil), payload[start:end+1]...)
				if mode == "corrupt" {
					data[0] ^= 1
				}
				if mode == "oversized" {
					data = append(data, 0)
				}
				if mode == "wrong-version" {
					w.Header().Set("ETag", `"different"`)
				}
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
				w.WriteHeader(206)
				w.Write(data)
			}))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "target")
			os.WriteFile(target, []byte("original"), 0600)
			err := Execute(Options{URL: server.URL + "/download/file", Output: target, DownloadConcurrency: 4, Timeout: time.Minute})
			stored, _ := os.ReadFile(target)
			if mode == "normal" || mode == "retry" {
				if err != nil || !bytes.Equal(stored, payload) {
					t.Fatalf("download: %v", err)
				}
				if peak.Load() < 2 {
					t.Fatal("range requests were serial")
				}
			} else if err == nil || string(stored) != "original" {
				t.Fatalf("invalid download published: %v", err)
			}
		})
	}
}

func TestParallelDownloadResumeCacheCleanup(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	partial, state, release, err := prepareDownload(target)
	if err != nil {
		t.Fatal(err)
	}
	release()
	os.WriteFile(partial, []byte("partial"), 0600)
	os.WriteFile(state+".ranges", []byte(`{"URL":"https://example.test/download/file?secret=hidden"}`), 0600)
	tasks, err := ListDownloadCache(filepath.Dir(partial))
	if err != nil || len(tasks) != 1 {
		t.Fatal("range cache not listed", err)
	}
	if tasks[0].URL != "https://example.test/download/file" {
		t.Fatal("cache exposed query credentials")
	}
	if err = CancelCachedDownload(filepath.Dir(partial), tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(state + ".ranges"); !os.IsNotExist(err) {
		t.Fatal("range state not removed")
	}
}
