package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCLIUploadDownloadAndResume(t *testing.T) {
	binary := buildTestBinary(t)
	serverRoot := t.TempDir()
	serverURL := startTestServer(t, binary, serverRoot)

	fullPayload := testPayload(2 << 20)
	fullSource := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(fullSource, fullPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	fullURL := serverURL + "/upload/full.bin"
	runCLI(t, binary, "-d", "@"+fullSource, fullURL)

	stored, err := os.ReadFile(filepath.Join(serverRoot, "full.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, fullPayload) {
		t.Fatal("CLI upload contents do not match source")
	}

	downloadPath := filepath.Join(t.TempDir(), "download.bin")
	runCLI(t, binary, "-o", downloadPath, serverURL+"/download/full.bin")
	downloaded, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloaded, fullPayload) {
		t.Fatal("CLI download contents do not match uploaded file")
	}

	resumePayload := testPayload(512 << 10)
	resumeSource := filepath.Join(t.TempDir(), "resume-source.bin")
	if err = os.WriteFile(resumeSource, resumePayload, 0o600); err != nil {
		t.Fatal(err)
	}
	resumePath := filepath.Join(serverRoot, "resume.bin")
	resumeOffset := 12345
	request, _ := http.NewRequest("PUT", serverURL+"/upload/resume.bin", bytes.NewReader(resumePayload[:resumeOffset]))
	request.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", resumeOffset-1, len(resumePayload)))
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal(response.StatusCode)
	}
	runCLI(t, binary, "-c", "-d", "@"+resumeSource, serverURL+"/upload/resume.bin")

	resumed, err := os.ReadFile(resumePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resumed, resumePayload) {
		t.Fatal("CLI resumed upload contents do not match source")
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	binary := filepath.Join(t.TempDir(), "okserver")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", binary, "./cmd/okserver")
	cmd.Dir = moduleRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building CLI test binary: %v\n%s", err, output)
	}
	return binary
}

func startTestServer(t *testing.T, binary, root string) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}

	missingConfig := filepath.Join(t.TempDir(), "server.yaml")
	config := fmt.Sprintf("server:\n  files_dir: %q\n  states_dir: %q\n  cache_dir: %q\n", root, root+".states", root+".cache")
	if err := os.WriteFile(missingConfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-c", missingConfig, "-s", address, "-p", root)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	baseURL := "http://" + address
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		resp, requestErr := client.Get(baseURL + "/")
		if requestErr == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return baseURL
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("test server did not become ready")
	return ""
}

func runCLI(t *testing.T, binary string, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cliArgs := append([]string{"cli", "-t", "20s"}, args...)
	cmd := exec.CommandContext(ctx, binary, cliArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running CLI %v: %v\n%s", args, err, output)
	}
}

func testPayload(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i*37 + 11) % 251)
	}
	return data
}
