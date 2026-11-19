package upload

import (
	"path/filepath"

	"github.com/winezer0/okserver/internal/uploadstore"
)

func (m *Manager) persist(s *session) error {
	if err := uploadstore.Write(s.dir, s.metadata); err != nil {
		return err
	}
	m.indexMu.Lock()
	m.indexRecord(s.metadata)
	m.indexMu.Unlock()
	return nil
}

// readMetadata is the boundary between durable records and trusted domain state.
func readMetadata(directory string) (Metadata, error) {
	metadata, err := uploadstore.Read[Metadata](directory)
	if err != nil {
		return metadata, err
	}
	if err = metadata.validate(); err != nil {
		return Metadata{}, err
	}
	if metadata.ID != filepath.Base(directory) {
		return Metadata{}, ErrInvalid
	}
	return metadata, nil
}
