//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package storage

import (
	"os"
	"path/filepath"
)

func SyncDirectory(name string) error {
	directory, err := os.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// DurableReplace publishes an already synced file, then persists both directory entries.
func DurableReplace(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	if err := SyncDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	if filepath.Dir(source) != filepath.Dir(target) {
		return SyncDirectory(filepath.Dir(source))
	}
	return nil
}
