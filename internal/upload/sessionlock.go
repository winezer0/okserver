package upload

import (
	"os"
	"path/filepath"

	"github.com/winezer0/okserver/internal/storage"
)

func (m *Manager) lockSession(s *session) (func(), error) {
	release, err := storage.TryDestinationLock(m.root, filepath.Join(m.root, "session-"+s.metadata.ID))
	if err != nil {
		return nil, ErrBusy
	}
	metadata, err := readMetadata(s.dir)
	if err != nil {
		release()
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	s.metadata = metadata
	s.done = metadata.Completed
	return release, nil
}
