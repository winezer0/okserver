package storage

import (
	"os"
	"path/filepath"
)

// PublishNew uses an atomic no-replace link, so even an external writer cannot
// be overwritten between an existence check and publication.
func PublishNew(source, target string) error {
	if err := os.Link(source, target); err != nil {
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
