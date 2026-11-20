package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/winezer0/okserver/internal/observability"
)

type certificateConfig struct {
	Domain string `yaml:"domain"`
	Cert   string `yaml:"cert"`
	Key    string `yaml:"key"`
	Ca     string `yaml:"ca"`
}

type httpConfig struct {
	AllowedOrigins    []string          `yaml:"allowed_origins,omitempty"`
	Listen            string            `yaml:"listen"`
	FilesDir          string            `yaml:"files_dir"`
	StatesDir         string            `yaml:"states_dir"`
	CacheDir          string            `yaml:"cache_dir"`
	Auth              string            `yaml:"auth"`
	Param             string            `yaml:"param"`
	DisableFrontUI    bool              `yaml:"disable_front_ui"`
	AllowCreateDir    bool              `yaml:"allow_create_dir"`
	AllowDeleteFile   bool              `yaml:"allow_delete_file"`
	ReadHeaderTimeout string            `yaml:"read_header_timeout"`
	Certificate       certificateConfig `yaml:"certificate"`
}

type uploadConfig struct {
	IdleTimeout      string           `yaml:"idle_timeout"`
	SessionTTL       string           `yaml:"session_ttl"`
	TerminalTTL      string           `yaml:"terminal_ttl"`
	VerifyWorkers    int              `yaml:"verify_workers"`
	ChunkSize        byteSize         `yaml:"chunk_size"`
	MaxFileSize      byteSize         `yaml:"max_file_size"`
	MaxConcurrent    int              `yaml:"max_concurrent"`
	MaxPerIdentity   int              `yaml:"max_per_identity"`
	BufferBudget     byteSize         `yaml:"buffer_budget_bytes"`
	MaxSessions      int              `yaml:"max_sessions"`
	MaxPendingBytes  byteSize         `yaml:"max_pending_bytes"`
	MinFreeDiskBytes unsignedByteSize `yaml:"min_free_disk_bytes"`
}

type serverConfig struct {
	Download struct {
		MaxConcurrent int `yaml:"max_concurrent"`
	} `yaml:"download"`
	Server httpConfig              `yaml:"server"`
	Upload uploadConfig            `yaml:"upload"`
	Log    observability.LogConfig `yaml:"log"`
}

func defaultConfig() serverConfig {
	c := serverConfig{
		Server: httpConfig{
			Listen: ":28080", FilesDir: "./files", StatesDir: "./okserver_states", CacheDir: "./okserver_cache",
			ReadHeaderTimeout: "10s",
		},
		Upload: uploadConfig{
			IdleTimeout: "2m", SessionTTL: "24h", TerminalTTL: "168h", VerifyWorkers: 2, ChunkSize: 8 << 20,
			MaxConcurrent: 4, MaxPerIdentity: 32, BufferBudget: 64 << 20, MaxSessions: 128, MaxPendingBytes: 100 << 30, MinFreeDiskBytes: 1 << 30,
		},
		Log: observability.DefaultLogConfig(),
	}
	c.Download.MaxConcurrent = 16
	return c
}

func displayAuth(auth string) string {
	if auth != "" {
		return "<username>:<password>"
	}
	return ""
}

func loadConfig(name string, explicit bool) (serverConfig, error) {
	config := defaultConfig()
	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("read configuration: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err = decoder.Decode(&config); err != nil && !errors.Is(err, io.EOF) {
		if strings.Contains(err.Error(), "field path not found in type server.httpConfig") {
			return config, errors.New("server.path is no longer supported; use server.files_dir")
		}
		if strings.Contains(err.Error(), "field chunk_dir not found in type server.uploadConfig") {
			return config, errors.New("upload.chunk_dir is no longer supported; configure server.states_dir and server.cache_dir")
		}
		return config, fmt.Errorf("decode configuration (expected server, upload, download and log sections): %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return config, errors.New("configuration must contain one YAML document")
	}
	config.applyDirectoryDefaults()
	return config, config.validate()
}

func (c *serverConfig) applyDirectoryDefaults() {
	defaults := defaultConfig().Server
	if c.Server.FilesDir == "" {
		c.Server.FilesDir = defaults.FilesDir
	}
	if c.Server.StatesDir == "" {
		c.Server.StatesDir = defaults.StatesDir
	}
	if c.Server.CacheDir == "" {
		c.Server.CacheDir = defaults.CacheDir
	}
}

func durationSetting(name, value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func (c serverConfig) validate() error {
	for _, origin := range c.Server.AllowedOrigins {
		if !validOrigin(origin) {
			return errors.New("server.allowed_origins must contain exact http/https origins without a path")
		}
	}
	if c.Server.Param != "" {
		key, _, _ := strings.Cut(c.Server.Param, ":")
		switch key {
		case "sha256", "size", "prefix", "overwrite", "takeover", "directory", "page", "search", "sort", "direction", "limit", "path":
			return errors.New("server.param uses a reserved protocol parameter")
		}
	}
	if strings.TrimSpace(c.Server.Listen) == "" {
		return errors.New("server.listen must not be empty")
	}
	for _, directory := range []struct{ name, value string }{
		{"server.files_dir", c.Server.FilesDir},
		{"server.states_dir", c.Server.StatesDir},
		{"server.cache_dir", c.Server.CacheDir},
	} {
		if strings.TrimSpace(directory.value) == "" {
			return fmt.Errorf("%s must not be empty", directory.name)
		}
	}
	for _, s := range []struct {
		name  string
		value int64
	}{
		{"upload.chunk_size", int64(c.Upload.ChunkSize)}, {"upload.max_file_size", int64(c.Upload.MaxFileSize)},
		{"upload.max_concurrent", int64(c.Upload.MaxConcurrent)}, {"upload.max_sessions", int64(c.Upload.MaxSessions)},
		{"upload.max_pending_bytes", int64(c.Upload.MaxPendingBytes)},
		{"upload.max_per_identity", int64(c.Upload.MaxPerIdentity)}, {"upload.buffer_budget_bytes", int64(c.Upload.BufferBudget)},
		{"upload.verify_workers", int64(c.Upload.VerifyWorkers)}, {"download.max_concurrent", int64(c.Download.MaxConcurrent)},
	} {
		if s.value < 0 {
			return fmt.Errorf("%s must be non-negative", s.name)
		}
	}
	if c.Upload.ChunkSize == 1<<63-1 {
		return errors.New("upload.chunk_size is too large")
	}
	if c.Upload.BufferBudget > 0 && c.Upload.BufferBudget < 8<<20 {
		return errors.New("upload.buffer_budget_bytes must be zero or at least 8MiB")
	}
	for _, s := range []struct{ name, value string }{
		{"server.read_header_timeout", c.Server.ReadHeaderTimeout},
		{"upload.idle_timeout", c.Upload.IdleTimeout}, {"upload.session_ttl", c.Upload.SessionTTL},
		{"upload.terminal_ttl", c.Upload.TerminalTTL},
	} {
		if _, err := durationSetting(s.name, s.value, time.Second); err != nil {
			return err
		}
	}
	for _, s := range []struct{ name, value string }{{"server.auth", c.Server.Auth}, {"server.param", c.Server.Param}} {
		if s.value != "" {
			key, value, ok := strings.Cut(s.value, ":")
			if !ok || key == "" || value == "" {
				return fmt.Errorf("%s must contain a non-empty key:value authentication pair", s.name)
			}
		}
	}
	if (c.Server.Certificate.Cert == "") != (c.Server.Certificate.Key == "") {
		return errors.New("server.certificate.cert and server.certificate.key must be configured together")
	}
	return c.Log.Validate()
}
