package observability

import (
	"fmt"
	"strings"

	"github.com/winezer0/slogs"
)

type OutputConfig struct {
	Format string `yaml:"format"`
	Level  string `yaml:"level"`
}

type FileLogConfig struct {
	Path       string `yaml:"path"`
	Format     string `yaml:"format"`
	Level      string `yaml:"level"`
	MaxSize    int    `yaml:"max_size"`
	MaxAge     int    `yaml:"max_age"`
	MaxBackups int    `yaml:"max_backups"`
	LocalTime  bool   `yaml:"localtime"`
	Compress   bool   `yaml:"compress"`
}

type LogConfig struct {
	Console OutputConfig  `yaml:"console"`
	File    FileLogConfig `yaml:"file"`
}

// DefaultLogConfig logs to the console without creating a file.
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Console: OutputConfig{Format: "text", Level: "info"},
		File:    FileLogConfig{Format: "json", Level: "info", MaxSize: 100, MaxAge: 1, MaxBackups: 10},
	}
}

func outputDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func validateOutput(name, format, level string) error {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("%s.level must be debug, info, warn or error", name)
	}
	switch strings.ToLower(format) {
	case "text", "json", "off":
		return nil
	}
	// slogs also supports compact masks made of time, level, caller and message.
	if format != "" && strings.Trim(format, "TLCM") == "" {
		return nil
	}
	return fmt.Errorf("%s.format must be text, json, off or a TLCM mask", name)
}

func (c LogConfig) slogsConfig() slogs.LogConfig {
	return slogs.LogConfig{
		ConsoleFormat: outputDefault(c.Console.Format, "text"),
		ConsoleLevel:  outputDefault(c.Console.Level, "info"),
		LogFileFormat: outputDefault(c.File.Format, "json"),
		FileLevel:     outputDefault(c.File.Level, "info"),
		LogFilePath:   c.File.Path, MaxSize: c.File.MaxSize, MaxAge: c.File.MaxAge,
		MaxBackups: c.File.MaxBackups, LocalTime: c.File.LocalTime, Compress: c.File.Compress,
	}
}

func (c LogConfig) Validate() error {
	mapped := c.slogsConfig()
	if err := validateOutput("log.console", mapped.ConsoleFormat, mapped.ConsoleLevel); err != nil {
		return err
	}
	if err := validateOutput("log.file", mapped.LogFileFormat, mapped.FileLevel); err != nil {
		return err
	}
	if c.File.MaxSize < 0 || c.File.MaxAge < 0 || c.File.MaxBackups < 0 {
		return fmt.Errorf("log.file.max_size, max_age and max_backups must be non-negative")
	}
	if c.File.Path != "" && (strings.TrimSpace(c.File.Path) == "" || c.File.Path == "-") {
		return fmt.Errorf("log.file.path must be a file path; use console.format for console output")
	}
	return nil
}

// NewLogger creates a server-owned logger; callers must close it on shutdown.
func NewLogger(config LogConfig) (*slogs.Logger, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return slogs.NewLogger(config.slogsConfig())
}
