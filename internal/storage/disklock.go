package storage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const lockShardCount = 64

func TryDestinationLock(stateDir, target string) (func(), error) {
	canonical, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	directory, err := lockDirectory(stateDir)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(canonical))
	return TryFileLock(filepath.Join(directory, fmt.Sprintf("%x.lock", key)))
}

func SessionLock(stateDir, id string) (func(), error) {
	directory, err := lockDirectory(stateDir)
	if err != nil {
		return nil, err
	}
	shard := lockShard("session", strings.ToLower(id))
	name := filepath.Join(directory, fmt.Sprintf("session-%02x.lock", shard))
	return WaitFileLock(name)
}

func TrySpoolLock(_ string, name string) (func(), error) {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = trySpoolFileLock(file); err != nil {
		file.Close()
		return nil, err
	}
	return func() { file.Close() }, nil
}

func lockShard(scope, key string) byte {
	return sha256.Sum256([]byte(scope + "\x00" + key))[0] % lockShardCount
}

func lockDirectory(stateDir string) (string, error) {
	directory := filepath.Join(stateDir, "locks")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return directory, nil
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

func WaitFileLock(name string) (func(), error) {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		err = tryFileLock(file)
		if err == nil {
			return func() { file.Close() }, nil
		}
		if !isFileLockBusy(err) {
			file.Close()
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
