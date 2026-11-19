package storage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func TryDestinationLock(root, target string) (func(), error) {
	canonical, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	directory := filepath.Join(root, ".okserver", "locks")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(canonical))
	return TryFileLock(filepath.Join(directory, fmt.Sprintf("%x.lock", key)))
}

func TryFileLock(name string) (func(), error) {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = tryFileLock(file); err != nil {
		file.Close()
		return nil, err
	}
	return func() { file.Close() }, nil
}
