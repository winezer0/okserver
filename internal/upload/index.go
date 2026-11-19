package upload

import (
	"os"
	"path/filepath"
)

// Derived indexes are rebuilt from durable metadata. Directory discovery is
// cross-process safe; historical JSON is read only once, never for every chunk.
func (m *Manager) indexRecord(record Metadata) {
	m.index[record.ID] = record
	previous := m.index[m.paths[record.Path]]
	if previous.ID == "" || !record.CreatedAt.Before(previous.CreatedAt) {
		m.paths[record.Path] = record.ID
	}
}

func (m *Manager) discoverIndex() error {
	entries, err := os.ReadDir(m.sessionDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		if _, known := m.index[entry.Name()]; known {
			continue
		}
		if record, err := readMetadata(filepath.Join(m.sessionDir, entry.Name())); err == nil {
			m.indexRecord(record)
		}
	}
	return nil
}

func (m *Manager) indexedPath(path string) (Metadata, error) {
	m.indexMu.Lock()
	defer m.indexMu.Unlock()
	if err := m.discoverIndex(); err != nil {
		return Metadata{}, err
	}
	id := m.paths[path]
	if id == "" {
		return Metadata{}, ErrNotFound
	}
	record, err := readMetadata(filepath.Join(m.sessionDir, id))
	if os.IsNotExist(err) {
		delete(m.index, id)
		delete(m.paths, path)
		return Metadata{}, ErrNotFound
	}
	if err != nil {
		return Metadata{}, err
	}
	m.indexRecord(record)
	return record, nil
}
