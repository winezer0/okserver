package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrepareDirectoriesResolvesDefaultsFromWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	config := defaultConfig().Server
	if err := prepareDirectories(&config); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{config.FilesDir, config.StatesDir, config.CacheDir} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("directory not prepared at %q: %v", path, err)
		}
	}
	if config.FilesDir != filepath.Join(root, "files") || config.StatesDir != filepath.Join(root, "okserver_states") || config.CacheDir != filepath.Join(root, "okserver_cache") {
		t.Fatalf("unexpected prepared paths: %+v", config)
	}
}

func TestPrepareDirectoriesRejectsOverlappingPaths(t *testing.T) {
	root := t.TempDir()
	config := httpConfig{
		FilesDir:  filepath.Join(root, "files"),
		StatesDir: filepath.Join(root, "files", "states"),
		CacheDir:  filepath.Join(root, "cache"),
	}
	if err := prepareDirectories(&config); err == nil {
		t.Fatal("overlapping service directories accepted")
	}
}

func TestValidateDirectorySeparationAllowsSeparateWindowsVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows volume paths are only meaningful on Windows")
	}
	config := httpConfig{
		FilesDir:  `C:\server\files`,
		StatesDir: `D:\server\states`,
		CacheDir:  `D:\server\cache`,
	}
	if err := validateDirectorySeparation(config); err != nil {
		t.Fatalf("separate volumes rejected: %v", err)
	}
}
