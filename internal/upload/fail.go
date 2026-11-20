package upload

import "time"

// Fail records unrecoverable publication errors without deleting the original file.
func (m *Manager) Fail(id string) error {
	return m.FailWithCode(id, "")
}

func (m *Manager) FailWithCode(id, code string) error {
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
	if s.metadata.Completed || s.metadata.Canceled {
		return ErrConflict
	}
	s.metadata.Failed = true
	s.metadata.Finalizing = false
	s.metadata.ErrorCode = code
	s.metadata.UpdatedAt = time.Now().UTC()
	return m.persist(s)
}
