package observability

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNestedLogConfiguration(t *testing.T) {
	var config struct{ Log LogConfig }
	err := yaml.Unmarshal([]byte("log:\n  console:\n    format: off\n    level: error\n  file:\n    path: request.log\n    format: json\n    level: debug\n    max_size: 12\n    max_age: 4\n    max_backups: 7\n    localtime: true\n    compress: true\n"), &config)
	if err != nil {
		t.Fatal(err)
	}
	c := config.Log.slogsConfig()
	if c.LogFilePath != "request.log" || c.MaxSize != 12 || c.MaxAge != 4 || c.MaxBackups != 7 || !c.LocalTime || !c.Compress || c.ConsoleFormat != "off" || c.ConsoleLevel != "error" || c.FileLevel != "debug" || c.LogFileFormat != "json" {
		t.Fatalf("configuration lost: %+v", c)
	}
	c = DefaultLogConfig().slogsConfig()
	if c.ConsoleFormat != "text" || c.ConsoleLevel != "info" || c.LogFilePath != "" || c.LocalTime {
		t.Fatalf("bad defaults: %+v", c)
	}
}

func TestSlogsConcurrentRequestLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "requests.log")
	logger, err := NewLogger(LogConfig{Console: OutputConfig{Format: "off"}, File: FileLogConfig{Path: path, LocalTime: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	var metrics Metrics
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			req := httptest.NewRequest("PUT", "/api/v1/uploads/task?token=secret", nil)
			req.Header.Set("Authorization", "Bearer hidden")
			w, finish := metrics.Observe(httptest.NewRecorder(), req, logger)
			w.WriteHeader(204)
			finish()
		}()
	}
	workers.Wait()
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "hidden") || strings.Contains(string(data), "token") {
		t.Fatal("credentials leaked into logs")
	}
	ids := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		var entry struct {
			Message   string `json:"msg"`
			RequestID string `json:"request_id"`
			TaskID    string `json:"task_id"`
			Status    int    `json:"status"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Message != "http.request" || entry.TaskID != "task" || entry.Status != 204 || entry.RequestID == "" || ids[entry.RequestID] {
			t.Fatalf("invalid request log: %+v", entry)
		}
		ids[entry.RequestID] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 32 {
		t.Fatalf("got %d logs, want 32", len(ids))
	}
}

func TestLoggerRejectsInvalidFile(t *testing.T) {
	if logger, err := NewLogger(LogConfig{Console: OutputConfig{Format: "off"}, File: FileLogConfig{Path: t.TempDir()}}); err == nil {
		_ = logger.Close()
		t.Fatal("expected invalid file error")
	}
}
