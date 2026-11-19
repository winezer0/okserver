package client

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crypto/sha256"
)

func TestHashFilePrefix(t *testing.T) {
	payload := []byte("0123456789abcdefg")
	filename := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(filename, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	whole, err := hashFilePrefix(file, int64(len(payload)), make([]byte, 3))
	if err != nil {
		t.Fatal(err)
	}
	wholeHash := sha256.Sum256(payload)
	if whole != hex.EncodeToString(wholeHash[:]) {
		t.Fatalf("whole hash = %s, want %x", whole, wholeHash)
	}
	prefix, err := hashFilePrefix(file, 8, make([]byte, 3))
	if err != nil || prefix != hashBytes(payload[:8]) {
		t.Fatalf("prefix digest = %s, %v", prefix, err)
	}
}

func hashBytes(data []byte) string { return sha256Digest(data) }
func TestSerialUploadRecovery(t *testing.T) {
	for _, mode := range []string{"normal", "lost-chunk", "lost-final", "async", "resume", "bad-prefix", "bad-final", "failed", "canceled", "query-retry"} {
		t.Run(mode, func(t *testing.T) {
			payload := []byte("serial upload recovery with SHA-256")
			stored := []byte{}
			created := false
			completed := false
			dropped := false
			queried := false
			if mode == "resume" || mode == "bad-prefix" {
				stored = append(stored, payload[:4]...)
				created = true
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/upload/file.bin" {
					http.Error(w, "wrong endpoint", 400)
					return
				}
				w.Header().Set("Upload-Chunk-Size", "4")
				w.Header().Set("Upload-Length", fmt.Sprint(len(payload)))
				w.Header().Set("Upload-Offset", fmt.Sprint(len(stored)))
				if r.Header.Get("Upload-Token") == "" {
					http.Error(w, "missing token", 400)
					return
				}
				if r.Method == "HEAD" {
					if !queried {
						queried = true
						if mode == "failed" || mode == "canceled" {
							w.Header().Set("Upload-State", mode)
							w.Header().Set("Upload-Length", "999")
							return
						}
						if mode == "query-retry" {
							w.WriteHeader(503)
							return
						}
					}
					if !created {
						w.WriteHeader(404)
						return
					}
					if mode == "async" && len(stored) == len(payload) {
						completed = true
					}
					w.Header().Set("Upload-Prefix-SHA256", sha256Digest(stored))
					if mode == "bad-prefix" {
						w.Header().Set("Upload-Prefix-SHA256", strings.Repeat("0", 64))
					}
					if completed {
						w.Header().Set("Upload-State", "completed")
						sum := sha256Digest(payload)
						if mode == "bad-final" {
							sum = strings.Repeat("0", 64)
						}
						w.Header().Set("X-File-SHA256", sum)
					}
					return
				}
				part, _ := io.ReadAll(r.Body)
				if r.Header.Get("X-Chunk-SHA256") != sha256Digest(part) {
					http.Error(w, "bad chunk", 422)
					return
				}
				stored = append(stored, part...)
				created = true
				final := len(stored) == len(payload)
				completed = final && mode != "async"
				w.Header().Set("Upload-Offset", fmt.Sprint(len(stored)))
				if (mode == "lost-chunk" && !final || mode == "lost-final" && final) && !dropped {
					dropped = true
					w.WriteHeader(503)
					return
				}
				if final {
					sum := sha256Digest(payload)
					if mode == "bad-final" {
						sum = strings.Repeat("0", 64)
					}
					w.Header().Set("X-File-SHA256", sum)
					if mode == "async" {
						w.WriteHeader(202)
					} else {
						w.WriteHeader(201)
					}
				} else {
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			source := filepath.Join(t.TempDir(), "source.bin")
			os.WriteFile(source, payload, 0600)
			err := Execute(Options{URL: server.URL + "/upload/file.bin", Data: "@" + source, Resume: true, Timeout: 10 * time.Second})
			if mode == "bad-prefix" || mode == "bad-final" {
				if err == nil {
					t.Fatal("integrity error accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stored, payload) {
				t.Fatal("replayed or missing bytes")
			}
		})
	}
}
