package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"
)

func TestRequestLogSeverityAndCause(t *testing.T) {
	for _, row := range []struct {
		status int
		level  string
	}{{200, "INFO"}, {422, "WARN"}, {507, "ERROR"}} {
		var output bytes.Buffer
		var metrics Metrics
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		request := httptest.NewRequest("POST", "/api/v1/uploads", nil)
		w, finish := metrics.Observe(httptest.NewRecorder(), request, logger)
		RecordTask(w, "new-task")
		if row.status >= 400 {
			RecordError(w, errors.New("checksum mismatch"))
			RecordError(w, errors.New("generic error"))
		}
		w.WriteHeader(row.status)
		finish()
		var entry map[string]any
		if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["level"] != row.level || entry["task_id"] != "new-task" {
			t.Fatalf("log: %v", entry)
		}
		if row.status >= 400 && entry["error"] != "checksum mismatch" {
			t.Fatalf("lost root cause: %v", entry)
		}
	}
}
