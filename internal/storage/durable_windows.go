package storage

import (
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

// Windows MoveFileEx with WRITE_THROUGH waits for the move to reach storage.
func DurableReplace(source, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	var moveErr error
	for attempt := 0; attempt < 8; attempt++ {
		moveErr = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if moveErr == nil {
			return nil
		}
		if isCrossDeviceLink(moveErr) {
			return replaceCopiedFile(source, target)
		}
		if !errors.Is(moveErr, windows.ERROR_SHARING_VIOLATION) && !errors.Is(moveErr, windows.ERROR_ACCESS_DENIED) {
			return moveErr
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return moveErr
}

// Windows persists file replacement via DurableReplace rather than directory fsync.
func SyncDirectory(name string) error { return nil }
