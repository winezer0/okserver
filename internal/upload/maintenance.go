package upload

import (
	"os"
	"path/filepath"
	"time"

	"github.com/winezer0/okserver/internal/storage"
)

func (m *Manager) Tasks() []Metadata {
	m.indexMu.Lock()
	defer m.indexMu.Unlock()
	_ = m.discoverIndex()
	result := make([]Metadata, 0)
	for id, cached := range m.index {
		if cached.Completed || cached.Canceled || cached.Failed {
			continue
		}
		metadata, err := readMetadata(filepath.Join(m.sessionDir, id))
		if err != nil {
			continue
		}
		m.indexRecord(metadata)
		if !metadata.Completed && !metadata.Canceled && !metadata.Failed {
			result = append(result, metadata)
		}
	}
	return result
}
func (m *Manager) PruneExpired() {
	m.cleanupMu.Lock()
	m.lastCleanup = time.Time{}
	m.cleanupMu.Unlock()
	m.cleanupExpired()
	m.cleanupOrphans()
}
func (m *Manager) cleanupOrphans() {
	entries, err := os.ReadDir(m.sessionDir)
	if err != nil {
		m.emit("cleanup.upload_failed", "", 0, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		directory := filepath.Join(m.sessionDir, entry.Name())
		if _, err := os.Stat(filepath.Join(directory, "metadata.json")); !os.IsNotExist(err) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < time.Minute {
			continue
		}
		release, err := storage.TryDestinationLock(m.root, filepath.Join(m.root, "session-"+entry.Name()))
		if err != nil {
			continue
		}
		if _, err = os.Stat(filepath.Join(directory, "metadata.json")); os.IsNotExist(err) {
			m.emit("cleanup.orphan", entry.Name(), 0, os.RemoveAll(directory))
		}
		release()
	}
}

func (m *Manager) cleanupExpired() {
	if m.sessionTTL < 0 {
		return
	}
	now := time.Now()
	m.cleanupMu.Lock()
	interval := time.Minute
	if m.sessionTTL > 0 && m.sessionTTL/2 < interval {
		interval = m.sessionTTL / 2
		if interval <= 0 {
			interval = time.Millisecond
		}
	}
	if now.Sub(m.lastCleanup) < interval {
		m.cleanupMu.Unlock()
		return
	}
	m.lastCleanup = now
	m.cleanupMu.Unlock()

	m.mu.RLock()
	sessions := make([]struct {
		id string
		s  *session
	}, 0, len(m.sessions))
	for id, current := range m.sessions {
		sessions = append(sessions, struct {
			id string
			s  *session
		}{id: id, s: current})
	}
	m.mu.RUnlock()

	for _, current := range sessions {
		if !current.s.mu.TryLock() {
			continue
		}
		release, lockErr := m.lockSession(current.s)
		if lockErr != nil {
			current.s.mu.Unlock()
			continue
		}
		updatedAt := current.s.metadata.UpdatedAt
		ttl := m.sessionTTL
		if current.s.metadata.Completed || current.s.metadata.Canceled || current.s.metadata.Failed {
			ttl = m.terminalTTL
		}
		if ttl >= 0 && !updatedAt.IsZero() && now.Sub(updatedAt) > ttl {
			if err := os.RemoveAll(current.s.dir); err != nil {
				m.emit("cleanup.upload_failed", current.id, current.s.metadata.Offset, err)
				current.s.mu.Unlock()
				release()
				continue
			}
			current.s.done = true
			m.mu.Lock()
			if m.sessions[current.id] == current.s {
				delete(m.sessions, current.id)
				key := current.s.metadata.IdempotencyKey
				if key != "" && m.keys[key] == current.id {
					delete(m.keys, key)
				}
			}
			m.mu.Unlock()
			m.emit("upload.expired", current.id, current.s.metadata.Offset, nil)
		}
		current.s.mu.Unlock()
		release()
	}
}
