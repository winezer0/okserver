package client

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/winezer0/okserver/internal/storage"
)

func downloadPaths(output string) (string, string, error) {
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "", "", err
	}
	if parent, resolveErr := filepath.EvalSymlinks(filepath.Dir(absolute)); resolveErr == nil {
		absolute = filepath.Join(parent, filepath.Base(absolute))
	}
	// Keep staging on the destination volume so replacement remains a rename.
	if runtime.GOOS == "windows" {
		absolute = strings.ToLower(absolute)
	}
	digest := sha256.Sum256([]byte(absolute))
	directory := filepath.Join(filepath.Dir(absolute), ".okserver-downloads")
	if info, statErr := os.Lstat(directory); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return "", "", errors.New("invalid download staging directory")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", "", err
	}
	if err = PruneDownloadCache(directory, 7*24*time.Hour); err != nil {
		return "", "", err
	}
	name := filepath.Join(directory, fmt.Sprintf("%x", digest))
	return name + ".part", name + ".json", nil
}

func prepareDownload(output string) (string, string, func(), error) {
	partial, state, err := downloadPaths(output)
	if err != nil {
		return "", "", nil, err
	}
	if info, statErr := os.Lstat(partial + ".lock"); statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return "", "", nil, errors.New("invalid download lock file")
	}
	lock, err := os.OpenFile(partial+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", "", nil, err
	}
	if err = lockDownload(lock); err != nil {
		lock.Close()
		return "", "", nil, fmt.Errorf("download destination is in use: %w", err)
	}
	for _, name := range []string{partial, state, state + ".ranges"} {
		if info, statErr := os.Lstat(name); statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			lock.Close()
			return "", "", nil, errors.New("invalid download staging file")
		}
	}
	return partial, state, func() { lock.Close() }, nil
}

func writeDownloadState(name string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(name), "state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return storage.DurableReplace(file.Name(), name)
}
