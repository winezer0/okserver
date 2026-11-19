//go:build windows

package uploadstore

import (
	"errors"

	"golang.org/x/sys/windows"
)

func retryableMetadataReadError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
