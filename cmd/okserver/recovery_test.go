package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/upload"
)

func TestGeneratedLargeFileRecovery(t *testing.T) {
	megabytes := 64
	if configured := os.Getenv("OKSERVER_LARGE_TEST_MB"); configured != "" {
		value, err := strconv.Atoi(configured)
		if err != nil || value < 1 || value > 4096 {
			t.Fatal("invalid OKSERVER_LARGE_TEST_MB")
		}
		megabytes = value
	}
	binary := buildTestBinary(t)
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "generated.bin")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 1<<20)
	for index := range block {
		block[index] = byte(index*31 + 17)
	}
	hash := sha256.New()
	for index := 0; index < megabytes; index++ {
		if _, err = io.MultiWriter(file, hash).Write(block); err != nil {
			t.Fatal(err)
		}
	}
	file.Close()
	checksum := hex.EncodeToString(hash.Sum(nil))
	size := int64(megabytes) << 20
	// Persist an acknowledged chunk, then reopen the manager as a crashed server would.
	manager, err := upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.Create(upload.CreateRequest{Path: "resumed.bin", Size: size, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	chunkHash := sha256.Sum256(block)
	if _, err = manager.WriteChunk(session.ID, 0, int64(len(block))-1, size, hex.EncodeToString(chunkHash[:]), bytes.NewReader(block)); err != nil {
		t.Fatal(err)
	}
	manager, err = upload.NewManager(upload.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	status, err := manager.Get(session.ID)
	if err != nil || status.Offset != int64(len(block)) {
		t.Fatalf("restart offset: %+v %v", status, err)
	}
	// Resume the persisted session through real HTTP after server startup.
	base := startTestServer(t, binary, root)
	input, _ := os.Open(source)
	defer input.Close()
	for offset := status.Offset; offset < size; {
		length := int64(8 << 20)
		if size-offset < length {
			length = size - offset
		}
		chunk := make([]byte, length)
		if _, err = input.ReadAt(chunk, offset); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(chunk)
		request, _ := http.NewRequest("PUT", base+"/upload/resumed.bin?sha256="+checksum, bytes.NewReader(chunk))
		request.Header.Set("Content-Range", "bytes "+strconv.FormatInt(offset, 10)+"-"+strconv.FormatInt(offset+length-1, 10)+"/"+strconv.FormatInt(size, 10))
		request.Header.Set("X-Chunk-SHA256", hex.EncodeToString(digest[:]))
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != 204 && response.StatusCode != 202 && response.StatusCode != 201 {
			t.Fatalf("chunk status %d", response.StatusCode)
		}
		offset += length
	}
	deadline := time.Now().Add(time.Minute)
	for {
		response, e := http.Head(base + "/upload/resumed.bin")
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.Header.Get("Upload-State") == "completed" {
			if response.Header.Get("X-File-SHA256") != checksum {
				t.Fatal("hash differs")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("verification timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Exercise the actual CLI uploader separately.
	runCLI(t, binary, "-d", "@"+source, base+"/upload/cli.bin")
	// Drop a download response after 1 MiB. Retry must resume and validate the file.
	var dropResponses atomic.Bool
	dropResponses.Store(true)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := http.NewRequest(r.Method, base+"/download/cli.bin", nil)
		request.Header = r.Header.Clone()
		upstream, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			http.Error(w, requestErr.Error(), 502)
			return
		}
		defer upstream.Body.Close()
		for key, values := range upstream.Header {
			w.Header()[key] = values
		}
		w.WriteHeader(upstream.StatusCode)
		if r.Method == "GET" && dropResponses.Load() {
			io.CopyN(w, upstream.Body, 1<<20)
			return
		}
		io.Copy(w, upstream.Body)
	}))
	defer proxy.Close()
	destination := filepath.Join(t.TempDir(), "download.bin")
	os.WriteFile(destination, []byte("original"), 0600)
	failed := exec.Command(binary, "cli", "-parallel", "1", "-c", "-o", destination, proxy.URL+"/file")
	if err = failed.Run(); err == nil {
		t.Fatal("truncated response accepted")
	}
	original, _ := os.ReadFile(destination)
	if string(original) != "original" {
		t.Fatal("failed download replaced original")
	}
	dropResponses.Store(false)
	runCLI(t, binary, "-parallel", "1", "-c", "-o", destination, proxy.URL+"/file")
	downloaded, _ := os.Open(destination)
	defer downloaded.Close()
	actual := sha256.New()
	io.Copy(actual, downloaded)
	if hex.EncodeToString(actual.Sum(nil)) != checksum {
		t.Fatal("recovered download checksum mismatch")
	}
	t.Logf("verified %d MiB upload, restart resume and interrupted download, SHA-256 %s", megabytes, checksum)
}
