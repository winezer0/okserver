package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// PublishNew uses an atomic no-replace link, so even an external writer cannot
// be overwritten between an existence check and publication. When source and
// destination are on different volumes, it copies beside the destination and
// links that temporary file into place with the same no-replace guarantee.
func PublishNew(source, target string) error {
	if err := os.Link(source, target); err != nil {
		if isCrossDeviceLink(err) {
			return publishCopiedFile(source, target)
		}
		// A crash may leave both hard links after publication but before cleanup.
		from, first := os.Stat(source)
		to, second := os.Stat(target)
		if first != nil || second != nil || !os.SameFile(from, to) {
			return err
		}
	}
	if err := SyncDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(source))
}

func isCrossDeviceLink(err error) bool {
	if runtime.GOOS == "windows" {
		// ERROR_NOT_SAME_DEVICE from CreateHardLinkW.
		return errors.Is(err, syscall.Errno(17))
	}
	return errors.Is(err, syscall.EXDEV)
}

// Stage on the destination volume before atomic replacement; keep the source
// until the replacement has reached storage.
func replaceCopiedFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".okserver-replace-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() { _ = temp.Close(); _ = os.Remove(name) }()
	if _, err = io.Copy(temp, input); err != nil {
		return err
	}
	if err = temp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = input.Close(); err != nil {
		return err
	}
	if err = DurableReplace(name, target); err != nil {
		return err
	}
	if err = os.Remove(source); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(source))
}

// publishCopiedFile keeps the no-replace publication guarantee when the
// staging file and destination live on different volumes. The temporary file
// is created beside the destination, then linked into place atomically.
func publishCopiedFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(target), ".okserver-publish-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if _, err = io.Copy(temp, input); err != nil {
		return err
	}
	if err = temp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = input.Close(); err != nil {
		return err
	}

	if err = os.Link(tempPath, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			if same, compareErr := sameFileContents(source, target); compareErr == nil && same {
				if err = SyncDirectory(filepath.Dir(target)); err != nil {
					return err
				}
				if err = os.Remove(source); err != nil {
					return err
				}
				return SyncDirectory(filepath.Dir(source))
			}
		}
		return err
	}
	if err = SyncDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	if err = os.Remove(source); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(source))
}

func sameFileContents(firstPath, secondPath string) (bool, error) {
	firstInfo, err := os.Lstat(firstPath)
	if err != nil {
		return false, err
	}
	secondInfo, err := os.Lstat(secondPath)
	if err != nil {
		return false, err
	}
	if !firstInfo.Mode().IsRegular() || !secondInfo.Mode().IsRegular() || firstInfo.Size() != secondInfo.Size() {
		return false, nil
	}
	first, err := os.Open(firstPath)
	if err != nil {
		return false, err
	}
	defer first.Close()
	second, err := os.Open(secondPath)
	if err != nil {
		return false, err
	}
	defer second.Close()

	const chunkSize = 64 * 1024
	firstChunk := make([]byte, chunkSize)
	secondChunk := make([]byte, chunkSize)
	remaining := firstInfo.Size()
	for remaining > 0 {
		count := int64(chunkSize)
		if remaining < count {
			count = remaining
		}
		firstBytes := firstChunk[:int(count)]
		secondBytes := secondChunk[:int(count)]
		if _, err = io.ReadFull(first, firstBytes); err != nil {
			return false, err
		}
		if _, err = io.ReadFull(second, secondBytes); err != nil {
			return false, err
		}
		if !bytes.Equal(firstBytes, secondBytes) {
			return false, nil
		}
		remaining -= count
	}
	return true, nil
}
