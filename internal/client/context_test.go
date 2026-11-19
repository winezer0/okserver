package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanceledClientStopsHashRetryAndRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &fileClient{ctx: ctx}
	started := time.Now()
	if err := client.waitRetry(5); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation waited for backoff")
	}
	request, err := client.newRequest(http.MethodGet, "http://127.0.0.1/", nil, nil)
	if err != nil || !errors.Is(request.Context().Err(), context.Canceled) {
		t.Fatalf("request: %v", err)
	}
	name := filepath.Join(t.TempDir(), "source")
	if err = os.WriteFile(name, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = hashFilePrefixContext(ctx, file, 7, make([]byte, 2)); !errors.Is(err, context.Canceled) {
		t.Fatalf("hash: %v", err)
	}
}

func TestRetryAfterRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	response := &http.Response{Header: http.Header{"Retry-After": []string{"60"}}}
	client := &fileClient{}
	if err := client.waitRetryResponse(ctx, 0, response); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retry-after ignored context: %v", err)
	}
}

func TestTransportErrorRedactsCredentials(t *testing.T) {
	err := redactTransferError(&url.Error{Op: "Put", URL: "https://user:password@example.test/upload/file?token=secret&sha256=abc", Err: context.Canceled})
	for _, secret := range []string{"user", "password", "token", "secret", "sha256"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("credential in transport error: %s", err)
		}
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("redaction lost cancellation cause")
	}
}
