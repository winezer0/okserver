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
	actualPaths := []string{config.FilesDir, config.StatesDir, config.CacheDir}
	wantPaths := []string{
		filepath.Join(root, "files"),
		filepath.Join(root, "okserver_states"),
		filepath.Join(root, "okserver_cache"),
	}
	for index, wantPath := range wantPaths {
		resolvedWantPath, err := filepath.EvalSymlinks(wantPath)
		if err != nil {
			t.Fatal(err)
		}
		if actualPaths[index] != resolvedWantPath {
			t.Errorf("prepared path = %q, want %q", actualPaths[index], resolvedWantPath)
		}
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
