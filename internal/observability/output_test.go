package observability

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/winezer0/slogs"
)

// None of these tests run in parallel: stdout is captured during logger creation.
func captureLogger(t *testing.T, config LogConfig, log func(*slogs.Logger)) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	previous := os.Stdout
	os.Stdout = writer
	logger, err := NewLogger(config)
	os.Stdout = previous
	if err != nil {
		t.Fatal(err)
	}
	log(logger)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestIndependentOutputFormatsAndLevels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	config := DefaultLogConfig()
	config.Console = OutputConfig{Format: "json", Level: "error"}
	config.File.Path = path
	config.File.Format = "text"
	config.File.Level = "debug"
	console := captureLogger(t, config, func(logger *slogs.Logger) {
		logger.Debug("debug-message")
		logger.Info("info-message")
		logger.Warn("warn-message")
		logger.Error("error-message", "task_id", "task")
	})
	var record map[string]any
	if err := json.Unmarshal([]byte(console), &record); err != nil {
		t.Fatal(err)
	}
	if record["msg"] != "error-message" || record["level"] != "ERROR" || record["task_id"] != "task" {
		t.Fatalf("console: %s", console)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"debug-message", "info-message", "warn-message", "error-message"} {
		if !strings.Contains(string(data), "msg="+message) {
			t.Fatalf("file dropped %s: %s", message, data)
		}
	}
}

func TestOutputFormatsAndOff(t *testing.T) {
	for _, format := range []string{"text", "json", "TLCM", "off"} {
		t.Run(format, func(t *testing.T) {
			config := DefaultLogConfig()
			config.Console.Format = format
			config.Console.Level = "warn"
			output := captureLogger(t, config, func(logger *slogs.Logger) { logger.Info("hidden"); logger.Warn("shown") })
			if format == "off" {
				if output != "" {
					t.Fatalf("disabled output: %s", output)
				}
				return
			}
			if !strings.Contains(output, "shown") || strings.Contains(output, "hidden") {
				t.Fatalf("level filtering: %s", output)
			}
			if format == "json" && !json.Valid([]byte(output)) {
				t.Fatalf("invalid JSON: %s", output)
			}
		})
	}
	config := DefaultLogConfig()
	config.File.Path = filepath.Join(t.TempDir(), "disabled", "file.log")
	config.File.Format = "off"
	captureLogger(t, config, func(logger *slogs.Logger) { logger.Info("console-only") })
	if _, err := os.Stat(filepath.Dir(config.File.Path)); !os.IsNotExist(err) {
		t.Fatalf("disabled file output created a directory: %v", err)
	}
}

func TestFileLevelAndJSON(t *testing.T) {
	config := DefaultLogConfig()
	config.Console.Format = "off"
	config.File.Path = filepath.Join(t.TempDir(), "file.jsonl")
	config.File.Level = "warn"
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hidden")
	logger.Warn("warning")
	logger.Error("failure")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(config.File.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		if !json.Valid(scanner.Bytes()) || strings.Contains(scanner.Text(), "hidden") {
			t.Fatalf("invalid JSON log: %s", scanner.Text())
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("got %d records, want 2", count)
	}
}
