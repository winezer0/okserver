package server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func prepareDirectories(config *httpConfig) error {
	directories := []*string{&config.FilesDir, &config.StatesDir, &config.CacheDir}
	for _, directory := range directories {
		absolute, err := filepath.Abs(*directory)
		if err != nil {
			return err
		}
		*directory = filepath.Clean(absolute)
	}
	if err := validateDirectorySeparation(*config); err != nil {
		return err
	}
	for index, directory := range directories {
		mode := os.FileMode(0o700)
		if index == 0 {
			mode = 0o755
		}
		if err := os.MkdirAll(*directory, mode); err != nil {
			return fmt.Errorf("create %s: %w", []string{"server.files_dir", "server.states_dir", "server.cache_dir"}[index], err)
		}
		resolved, err := filepath.EvalSymlinks(*directory)
		if err != nil {
			return err
		}
		*directory = filepath.Clean(resolved)
	}
	return validateDirectorySeparation(*config)
}

func validateDirectorySeparation(config httpConfig) error {
	paths := []struct{ name, value string }{
		{"server.files_dir", config.FilesDir},
		{"server.states_dir", config.StatesDir},
		{"server.cache_dir", config.CacheDir},
	}
	for left := 0; left < len(paths); left++ {
		for right := left + 1; right < len(paths); right++ {
			if directoryContains(paths[left].value, paths[right].value) || directoryContains(paths[right].value, paths[left].value) {
				return fmt.Errorf("%s and %s must be separate, non-overlapping directories", paths[left].name, paths[right].name)
			}
		}
	}
	return nil
}

func directoryContains(parent, child string) bool {
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.VolumeName(parent), filepath.VolumeName(child)) {
			return false
		}
		parent = strings.ToLower(parent)
		child = strings.ToLower(child)
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return true
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
