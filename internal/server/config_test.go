package server

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestConfigRejectsLegacyAndInvalidSettings(t *testing.T) {
	for _, body := range []string{
		"listen: ':28080'", "path: uploads", "auth: user:pass", "timeout: 10s",
		"upload_chunk_size: 8", "log:\n  logger:\n    filename: log.txt",
		"server:\n  listen: ':28080'\nlisten: ':9090'",
		"server:\n  timeout: 10s", "upload:\n  max_concurrent: -1",
		"upload:\n  max_sessions: -1", "upload:\n  max_file_size: -1",
		"upload:\n  max_pending_bytes: -1", "upload:\n  chunk_size: -1",
		"upload:\n  chunk_size: 9223372036854775807", "upload:\n  min_free_disk_bytes: -1",
		"server:\n  read_header_timeout: typo", "upload:\n  idle_timeout: 0s",
		"upload:\n  session_ttl: -1s", "server:\n  param: secret", "server:\n  auth: user",
		"server:\n  certificate:\n    cert: file.pem", "log:\n  file:\n    max_size: -1",
		"log:\n  console:\n    format: jsno", "log:\n  file:\n    level: verbose",
		"log:\n  console:\n    unknown: true", "log:\n  file:\n    filename: log.txt",
		"server:\n  listen: ''", "server:\n  path: ''", "upload:\n  chunck_size: 123",
		"server:\n  listen: a\n  listen: b", "{}\n---\n{}", "server: [",
		"server:\n  param: sha256:secret", "server:\n  param: directory:secret",
		"upload:\n  max_per_identity: -1", "upload:\n  buffer_budget_bytes: 1",
		"upload:\n  verify_workers: -1", "download:\n  max_concurrent: -1",
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := loadConfig(writeConfig(t, body), true); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestConfigDefaultsAndReadErrors(t *testing.T) {
	name := filepath.Join(t.TempDir(), "absent.yaml")
	config, err := loadConfig(name, false)
	if err != nil || !reflect.DeepEqual(config, defaultConfig()) {
		t.Fatalf("missing defaults: %+v %v", config, err)
	}
	if _, err := loadConfig(name, true); err == nil {
		t.Fatal("explicit missing file was ignored")
	}
	if _, err := loadConfig(filepath.Dir(name), false); err == nil {
		t.Fatal("read error was ignored")
	}
	for _, body := range []string{"", "{}", "server: {}\nupload: {}\nlog: {}"} {
		config, err := loadConfig(writeConfig(t, body), true)
		if err != nil || !reflect.DeepEqual(config, defaultConfig()) {
			t.Fatalf("empty defaults: %+v %v", config, err)
		}
	}
	config, err = loadConfig(writeConfig(t, "server:\n  auth: user:pass\nupload:\n  max_concurrent: 0\n  max_sessions: 0\n  max_pending_bytes: 0\n  min_free_disk_bytes: 0\nlog:\n  file:\n    max_age: 0\n    max_backups: 0"), true)
	if err != nil {
		t.Fatal(err)
	}
	if config.Server.Auth != "user:pass" || config.Server.Path != "./uploads" || config.Upload.MaxConcurrent != 0 || config.Upload.MaxSessions != 0 || config.Upload.MaxPendingBytes != 0 || config.Upload.MinFreeDiskBytes != 0 || config.Upload.ChunkSize != 8<<20 || config.Log.File.MaxAge != 0 || config.Log.File.MaxBackups != 0 {
		t.Fatalf("partial configuration or explicit zero lost: %+v", config)
	}
	if strings.Contains(displayAuth("user:secret"), "secret") {
		t.Fatal("startup example leaks credentials")
	}
}

func TestExampleConfigMatchesDefaults(t *testing.T) {
	config, err := loadConfig(filepath.Join("..", "..", "server.yaml"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config, defaultConfig()) {
		t.Fatalf("example and defaults differ: %+v", config)
	}
}
