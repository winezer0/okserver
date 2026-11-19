package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

type blockingReadCloser struct {
	closed chan struct{}
	once   sync.Once
}

func (body *blockingReadCloser) Read([]byte) (int, error) {
	<-body.closed
	return 0, io.ErrClosedPipe
}

func (body *blockingReadCloser) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

func TestIdleTimeoutBodyStopsUnsupportedReadDeadline(t *testing.T) {
	reader := &blockingReadCloser{closed: make(chan struct{})}
	body := &idleTimeoutBody{
		ReadCloser: reader,
		controller: http.NewResponseController(httptest.NewRecorder()),
		timeout:    10 * time.Millisecond,
	}
	started := time.Now()
	_, err := body.Read(make([]byte, 1))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read() error = %v, want deadline exceeded", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("idle read timeout took too long")
	}
}
