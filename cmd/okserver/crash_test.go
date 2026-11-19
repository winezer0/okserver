package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"crypto/sha256"
)

func crashServer(t *testing.T, binary, root, address string) (*exec.Cmd, string) {
	t.Helper()
	if address == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address = listener.Addr().String()
		listener.Close()
	}
	config := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(config, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "-c", config, "-s", address, "-p", root)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	base := "http://" + address
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(base + "/api/v1/uploads")
		if err == nil {
			response.Body.Close()
			return command, base
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return nil, ""
}
func TestServerCrashKeepsAcknowledgedChunk(t *testing.T) {
	binary := buildTestBinary(t)
	root := t.TempDir()
	process, base := crashServer(t, binary, root, "")
	request, _ := http.NewRequest("PUT", base+"/upload/crash.bin", bytes.NewBufferString("ab"))
	request.Header.Set("Content-Range", "bytes 0-1/4")
	chunkChecksum := sha256.Sum256([]byte("ab"))
	request.Header.Set("X-Chunk-SHA256", hex.EncodeToString(chunkChecksum[:]))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatalf("chunk status %d", response.StatusCode)
	}
	process.Process.Kill()
	process.Wait()
	_, base = crashServer(t, binary, root, "")
	request, _ = http.NewRequest("HEAD", base+"/upload/crash.bin", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Upload-Offset") != "2" {
		t.Fatal("lost acknowledged offset")
	}
}
