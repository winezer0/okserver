package upload

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Snapshot reads atomically persisted status without waiting for verification.
func (m *Manager) Snapshot(id string) (Metadata, error) {
	if !validID(id) {
		return Metadata{}, ErrInvalid
	}
	metadata, err := readMetadata(filepath.Join(m.sessionDir, id))
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, ErrNotFound
	}
	return metadata, err
}

func (m *Manager) Get(id string) (Metadata, error) {
	s, err := m.lookup(id)
	if err != nil {
		return Metadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := m.lockSession(s)
	if err != nil {
		return Metadata{}, err
	}
	defer release()
	return s.metadata, nil
}

func (m Metadata) String() string {
	return fmt.Sprintf("%s %d/%d", m.ID, m.Offset, m.Size)
}
