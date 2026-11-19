package upload

import (
	"os"
	"path/filepath"

	"github.com/winezer0/okserver/internal/storage"
)

func (m *Manager) checkFreeSpace() error {
	if m.minFreeBytes == 0 {
		return nil
	}
	free, err := storage.FreeSpace(m.root)
	if err != nil {
		return err
	}
	if free < m.minFreeBytes {
		return storage.ErrInsufficientSpace
	}
	return nil
}
func (m *Manager) checkResources(size int64, owner string) error {
	if err := m.checkFreeSpace(); err != nil {
		return err
	}
	entries, err := os.ReadDir(m.sessionDir)
	if err != nil {
		return err
	}
	count := 0
	owned := 0
	pending := int64(0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metadata, readErr := readMetadata(filepath.Join(m.sessionDir, entry.Name()))
		if readErr != nil {
			continue
		}
		if metadata.Completed || metadata.Canceled || metadata.Failed {
			continue
		}
		count++
		if metadata.Owner == owner {
			owned++
		}
		if metadata.Size > 0 && pending <= (1<<63-1)-metadata.Size {
			pending += metadata.Size
		} else if metadata.Size > 0 {
			return ErrTooLarge
		}
	}
	if m.maxSessions > 0 && count >= m.maxSessions {
		return ErrTooLarge
	}
	if m.maxPerOwner > 0 && owned >= m.maxPerOwner {
		return ErrTooLarge
	}
	if m.maxPendingBytes > 0 && (size > m.maxPendingBytes || pending > m.maxPendingBytes-size) {
		return ErrTooLarge
	}
	return nil
}
