package client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/digest"
)

func TestSingleDownloadAutomaticallyResumesTruncatedBody(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		current, _ := os.ReadFile(target)
		if string(current) != "original" {
			t.Error("target changed before final verification")
		}
		w.Header().Set("ETag", "\"v1\"")
		w.Header().Set("X-File-SHA256", hashBytes([]byte("abcdef")))
		if calls == 1 {
			w.Header().Set("Content-Length", "6")
			w.Write([]byte("abc"))
			return
		}
		if r.Header.Get("Range") != "bytes=3-" {
			t.Error("missing resume offset", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 3-5/6")
		w.WriteHeader(206)
		w.Write([]byte("def"))
	}))
	defer server.Close()
	client := &fileClient{client: server.Client(), httpUrl: server.URL, output: target, buf: make([]byte, 1024)}
	if err := client.get(); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(target)
	if string(actual) != "abcdef" || calls != 2 {
		t.Fatal(string(actual), calls)
	}
}

func TestDownloadProtectsExistingFile(t *testing.T) {
	for _, status := range []int{401, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "file")
			os.WriteFile(target, []byte("original"), 0600)
			client := &fileClient{client: server.Client(), httpUrl: server.URL, output: target, buf: make([]byte, 1024)}
			if err := client.get(); err == nil {
				t.Fatal("expected failure")
			}
			got, _ := os.ReadFile(target)
			if string(got) != "original" {
				t.Fatalf("existing file changed: %q", got)
			}
		})
	}
}

func TestDownloadRangeValidation(t *testing.T) {
	for _, scenario := range []string{"resume", "ignored", "wrong-offset", "wrong-identity", "416-wrong", "416-complete", "short"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "bytes=3-" || r.Header.Get("If-Range") != "\"v1\"" {
					t.Error("missing conditional range")
				}
				w.Header().Set("ETag", "\"v1\"")
				switch scenario {
				case "ignored":
					w.Write([]byte("abcdef"))
					return
				case "416-complete":
					w.Header().Set("Content-Range", "bytes */3")
					w.WriteHeader(416)
					return
				case "416-wrong":
					w.Header().Set("Content-Range", "bytes */2")
					w.WriteHeader(416)
					return
				case "wrong-offset":
					w.Header().Set("Content-Range", "bytes 2-5/6")
				default:
					w.Header().Set("Content-Range", "bytes 3-5/6")
				}
				if scenario == "wrong-identity" {
					w.Header().Set("ETag", "\"v2\"")
				}
				w.Header().Set("Content-Length", "3")
				w.WriteHeader(206)
				if scenario == "short" {
					w.Write([]byte("d"))
					return
				}
				w.Write([]byte("def"))
			}))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "file")
			os.WriteFile(target, []byte("original"), 0600)
			partial, statePath, pathErr := downloadPaths(target)
			if pathErr != nil {
				t.Fatal(pathErr)
			}
			os.WriteFile(partial, []byte("abc"), 0600)
			size := int64(6)
			if scenario == "416-complete" {
				size = 3
			}
			state, _ := json.Marshal(downloadState{URL: server.URL, Validator: "\"v1\"", Size: size})
			os.WriteFile(statePath, state, 0600)
			client := &fileClient{client: server.Client(), httpUrl: server.URL, output: target, point: true, buf: make([]byte, 1024)}
			err := client.getOnce()
			success := scenario == "resume" || scenario == "ignored" || scenario == "416-complete"
			if (err == nil) != success {
				t.Fatalf("error = %v, success = %v", err, success)
			}
			got, _ := os.ReadFile(target)
			want := "original"
			if success {
				want = "abcdef"
			}
			if scenario == "416-complete" {
				want = "abc"
			}
			if string(got) != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestDownloadLockAndStagingIsolation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file")
	os.WriteFile(target+".part", []byte("user data"), 0600)
	partial, _, release, err := prepareDownload(target)
	if err != nil {
		t.Fatal(err)
	}
	if partial == target+".part" {
		t.Fatal("staging collides with user file")
	}
	if _, _, secondRelease, lockErr := prepareDownload(target); lockErr == nil {
		secondRelease()
		t.Fatal("concurrent download accepted")
	}
	release()
	_, _, release, err = prepareDownload(target)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	release()
	got, _ := os.ReadFile(target + ".part")
	if string(got) != "user data" {
		t.Fatal("user file changed")
	}
}

func TestWeakDownloadValidatorIsRejected(t *testing.T) {
	response := &http.Response{Header: make(http.Header)}
	response.Header.Set("ETag", "W/\"weak\"")
	response.Header.Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
	if responseValidator(response) != "" {
		t.Fatal("weak validator accepted")
	}
}

func TestDownloadRejectsCorruptLocalPrefix(t *testing.T) {
	checksum := sha256.Sum256([]byte("abcdef"))
	checksumHex := hex.EncodeToString(checksum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"v1\"")
		w.Header().Set("X-File-Checksum-Algorithm", digest.AlgorithmSHA256)
		w.Header().Set("X-File-Checksum", checksumHex)
		w.Header().Set("Content-Range", "bytes 3-5/6")
		w.WriteHeader(206)
		w.Write([]byte("def"))
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "file")
	os.WriteFile(target, []byte("original"), 0600)
	partial, statePath, err := downloadPaths(target)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(partial, []byte("BAD"), 0600)
	state, _ := json.Marshal(downloadState{URL: server.URL, Validator: "\"v1\"", Size: 6, ChecksumAlgorithm: digest.AlgorithmSHA256, Checksum: checksumHex})
	os.WriteFile(statePath, state, 0600)
	client := &fileClient{client: server.Client(), httpUrl: server.URL, output: target, point: true, buf: make([]byte, 1024)}
	if err = client.get(); err == nil {
		t.Fatal("corrupted prefix accepted")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original" {
		t.Fatal("original overwritten")
	}
}
