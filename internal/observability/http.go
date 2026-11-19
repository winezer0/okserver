package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Metrics struct {
	requests      atomic.Uint64
	failures      atomic.Uint64
	uploadsFailed atomic.Uint64
	storageFailed atomic.Uint64
	bytes         atomic.Uint64
	active        atomic.Int64
	nanoseconds   atomic.Uint64
}
type response struct {
	http.ResponseWriter
	status int
	bytes  uint64
	cause  error
	taskID string
}

func RecordError(w http.ResponseWriter, err error) {
	if recorder, ok := w.(*response); ok && recorder.cause == nil {
		recorder.cause = err
	}
}

func RecordTask(w http.ResponseWriter, id string) {
	if recorder, ok := w.(*response); ok {
		recorder.taskID = id
	}
}

func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *response) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *response) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += uint64(n)
	return n, err
}

type RequestLogger interface {
	InfoContext(context.Context, string, ...any)
	WarnContext(context.Context, string, ...any)
	ErrorContext(context.Context, string, ...any)
}

func (m *Metrics) Observe(w http.ResponseWriter, r *http.Request, logger RequestLogger) (http.ResponseWriter, func()) {
	started := time.Now()
	var id [12]byte
	_, _ = rand.Read(id[:])
	requestID := hex.EncodeToString(id[:])
	w.Header().Set("X-Request-ID", requestID)
	*r = *r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
	recorder := &response{ResponseWriter: w}
	m.active.Add(1)
	return recorder, func() {
		duration := time.Since(started)
		m.active.Add(-1)
		m.requests.Add(1)
		m.bytes.Add(recorder.bytes)
		m.nanoseconds.Add(uint64(duration))
		status := recorder.status
		if status == 0 {
			status = 200
		}
		if status >= 400 {
			m.failures.Add(1)
			if r.Method == "PUT" || r.Method == "POST" {
				m.uploadsFailed.Add(1)
			}
		}
		if status == 507 {
			m.storageFailed.Add(1)
		}
		if logger != nil {
			taskID := recorder.taskID
			for _, prefix := range []string{"/api/v1/uploads/", "/api/v1/uploads/"} {
				if strings.HasPrefix(r.URL.Path, prefix) {
					taskID = strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")[0]
					break
				}
			}
			fields := []any{
				"request_id", requestID, "method", r.Method, "path", r.URL.Path,
				"task_id", taskID, "status", status, "bytes", recorder.bytes,
				"duration_ms", float64(duration) / float64(time.Millisecond),
			}
			if recorder.cause != nil {
				fields = append(fields, "error", recorder.cause.Error())
			}
			switch {
			case status >= 500:
				logger.ErrorContext(r.Context(), "http.request", fields...)
			case status >= 400:
				logger.WarnContext(r.Context(), "http.request", fields...)
			default:
				logger.InfoContext(r.Context(), "http.request", fields...)
			}
		}
	}
}

var _ RequestLogger = (*slog.Logger)(nil)

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "okserver_requests_total %d\nokserver_failures_total %d\nokserver_upload_failures_total %d\nokserver_storage_failures_total %d\nokserver_response_bytes_total %d\nokserver_active_requests %d\nokserver_request_duration_seconds_sum %.9f\n", m.requests.Load(), m.failures.Load(), m.uploadsFailed.Load(), m.storageFailed.Load(), m.bytes.Load(), m.active.Load(), float64(m.nanoseconds.Load())/float64(time.Second))
}

type requestIDKey struct{}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}
