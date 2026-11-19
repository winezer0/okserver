package upload

import (
	"os"
	"path/filepath"
	"time"
)

func (m *Manager) Cancel(id string) error {
	s, err := m.lookup(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := m.lockSession(s)
	if err != nil {
		return err
	}
	defer release()
	if s.metadata.Completed {
		return ErrConflict
	}
	if s.metadata.Canceled {
		return nil
	}
	if err := os.Remove(filepath.Join(s.dir, "payload.part")); err != nil && !os.IsNotExist(err) {
		m.emit("upload.cancel_failed", id, s.metadata.Offset, err)
		return err
	}
	s.metadata.Canceled = true
	s.metadata.UpdatedAt = time.Now().UTC()
	if err = m.persist(s); err != nil {
		return err
	}
	s.done = true
	m.mu.Lock()
	delete(m.sessions, id)
	if s.metadata.IdempotencyKey != "" && m.keys[s.metadata.IdempotencyKey] == id {
		delete(m.keys, s.metadata.IdempotencyKey)
	}
	m.mu.Unlock()
	return nil
}
