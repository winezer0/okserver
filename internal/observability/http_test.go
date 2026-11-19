package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAndLogsExcludeSecrets(t *testing.T) {
	var log bytes.Buffer
	var metrics Metrics
	request := httptest.NewRequest("PUT", "http://localhost/api/v1/uploads/task?password=secret", nil)
	response := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(&log, nil))
	wrapped, finish := metrics.Observe(response, request, logger)
	if RequestID(request.Context()) != response.Header().Get("X-Request-ID") {
		t.Fatal("request context lost its ID")
	}
	wrapped.WriteHeader(507)
	wrapped.Write([]byte("full"))
	finish()
	var entry map[string]any
	if err := json.Unmarshal(log.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["task_id"] != "task" || entry["status"] != float64(507) || entry["request_id"] != RequestID(request.Context()) {
		t.Fatalf("missing request fields: %v", entry)
	}
	if strings.Contains(log.String(), "secret") || strings.Contains(log.String(), "password") {
		t.Fatal("secret leaked")
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request ID")
	}
	output := httptest.NewRecorder()
	metrics.ServeHTTP(output, request)
	for _, expected := range []string{"okserver_requests_total 1", "okserver_upload_failures_total 1", "okserver_storage_failures_total 1", "okserver_response_bytes_total 4"} {
		if !strings.Contains(output.Body.String(), expected) {
			t.Fatalf("missing %s", expected)
		}
	}
	if err := http.NewResponseController(wrapped).Flush(); err != nil {
		t.Fatal("response controller lost underlying writer")
	}
}
