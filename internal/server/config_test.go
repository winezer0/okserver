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
		"server:\n  listen: ''", "server:\n  path: ''", "upload:\n  chunk_dir: old", "upload:\n  chunck_size: 123",
		"server:\n  listen: a\n  listen: b", "{}\n---\n{}", "server: [",
		"server:\n  param: sha256:secret", "server:\n  param: directory:secret",
		"upload:\n  max_per_identity: -1", "upload:\n  buffer_budget_bytes: 1",
		"upload:\n  chunk_size: 8MB", "upload:\n  buffer_budget_bytes: 1.5MiB",
		"upload:\n  max_pending_bytes: 9223372036854775807KiB",
		"upload:\n  min_free_disk_bytes: 18446744073709551615KiB",
		"upload:\n  verify_workers: -1", "download:\n  max_concurrent: -1",
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := loadConfig(writeConfig(t, body), true); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestConfigAcceptsBinarySizeSuffixesAndLegacyByteValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		upload string
	}{
		{
			name:   "binary suffixes",
			upload: "upload:\n  chunk_size: 8MiB\n  max_file_size: 2GiB\n  buffer_budget_bytes: 64MiB\n  max_pending_bytes: 100GiB\n  min_free_disk_bytes: 1GiB",
		},
		{
			name:   "legacy byte integers",
			upload: "upload:\n  chunk_size: 8388608\n  max_file_size: 2147483648\n  buffer_budget_bytes: 67108864\n  max_pending_bytes: 107374182400\n  min_free_disk_bytes: 1073741824",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := loadConfig(writeConfig(t, test.upload), true)
			if err != nil {
				t.Fatal(err)
			}
			if config.Upload.ChunkSize != 8<<20 || config.Upload.MaxFileSize != 2<<30 ||
				config.Upload.BufferBudget != 64<<20 || config.Upload.MaxPendingBytes != 100<<30 ||
				config.Upload.MinFreeDiskBytes != 1<<30 {
				t.Fatalf("unexpected parsed upload sizes: %+v", config.Upload)
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
	if config.Server.Auth != "user:pass" || config.Server.FilesDir != "./files" || config.Server.StatesDir != "./okserver_states" || config.Server.CacheDir != "./okserver_cache" || config.Upload.MaxConcurrent != 0 || config.Upload.MaxSessions != 0 || config.Upload.MaxPendingBytes != 0 || config.Upload.MinFreeDiskBytes != 0 || config.Upload.ChunkSize != 8<<20 || config.Log.File.MaxAge != 0 || config.Log.File.MaxBackups != 0 {
		t.Fatalf("partial configuration or explicit zero lost: %+v", config)
	}
	if strings.Contains(displayAuth("user:secret"), "secret") {
		t.Fatal("startup example leaks credentials")
	}
}

func TestExampleConfigLoads(t *testing.T) {
	config, err := loadConfig(filepath.Join("..", "..", "server.yaml"), true)
	if err != nil {
		t.Fatal(err)
	}
	defaults := defaultConfig()
	if config.Server.Listen != defaults.Server.Listen || config.Server.FilesDir != defaults.Server.FilesDir ||
		config.Server.StatesDir != defaults.Server.StatesDir || config.Server.CacheDir != defaults.Server.CacheDir ||
		config.Upload.ChunkSize != defaults.Upload.ChunkSize || config.Download.MaxConcurrent != defaults.Download.MaxConcurrent {
		t.Fatalf("example config has unexpected server defaults: %+v", config)
	}
}

func TestEmptyDirectorySettingsUseDefaults(t *testing.T) {
	config, err := loadConfig(writeConfig(t, "server:\n  files_dir: ''\n  states_dir: ''\n  cache_dir: ''"), true)
	if err != nil {
		t.Fatal(err)
	}
	defaults := defaultConfig().Server
	if config.Server.FilesDir != defaults.FilesDir || config.Server.StatesDir != defaults.StatesDir || config.Server.CacheDir != defaults.CacheDir {
		t.Fatalf("empty directory settings did not use defaults: %+v", config.Server)
	}
}

func TestLegacyDirectorySettingsGiveMigrationErrors(t *testing.T) {
	for _, test := range []struct{ body, message string }{
		{"server:\n  path: ./uploads", "server.path is no longer supported; use server.files_dir"},
		{"upload:\n  chunk_dir: ./chunks", "upload.chunk_dir is no longer supported; configure server.states_dir and server.cache_dir"},
	} {
		_, err := loadConfig(writeConfig(t, test.body), true)
		if err == nil || err.Error() != test.message {
			t.Fatalf("legacy setting %q returned %v", test.body, err)
		}
	}
}
