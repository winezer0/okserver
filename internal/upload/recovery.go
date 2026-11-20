package upload

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
)

// loadSessions restores only validated metadata under the session's process lock.
func (m *Manager) loadSessions() error {
	entries, err := os.ReadDir(m.sessionDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		dir := filepath.Join(m.sessionDir, entry.Name())
		release, lockErr := storage.SessionLock(m.stateDir, entry.Name())
		if lockErr != nil {
			continue
		}
		metadata, readErr := readMetadata(dir)
		if readErr != nil {
			release()
			m.emit("upload.restore_failed", entry.Name(), 0, errors.Join(ErrInvalid, readErr))
			continue
		}
		if metadata.ID != entry.Name() || metadata.Size < 0 || metadata.Offset < 0 || metadata.Offset > metadata.Size {
			release()
			m.emit("upload.restore_failed", entry.Name(), 0, ErrInvalid)
			continue
		}
		idempotencyKey, keyErr := normalizeIdempotencyKey(metadata.IdempotencyKey)
		if keyErr != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, keyErr)
			release()
			continue
		}
		metadata.IdempotencyKey = idempotencyKey
		relativePath, pathErr := CleanRelativePath(metadata.Path)
		if pathErr != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, pathErr)
			release()
			continue
		}
		if metadata.ChecksumAlgorithm != digest.AlgorithmSHA256 {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, ErrInvalid)
			release()
			continue
		}
		if _, checksumErr := normalizeChecksum(metadata.Checksum, true); checksumErr != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, checksumErr)
			release()
			continue
		}
		if m.maxFileSize > 0 && metadata.Size > m.maxFileSize {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, ErrTooLarge)
			release()
			continue
		}
		metadata.Path = relativePath
		metadata.ChunkSize = m.chunkSize
		if metadata.UpdatedAt.IsZero() {
			metadata.UpdatedAt = metadata.CreatedAt
		}
		ttl := m.sessionTTL
		if metadata.Completed || metadata.Canceled || metadata.Failed {
			ttl = m.terminalTTL
		}
		if ttl >= 0 && !metadata.UpdatedAt.IsZero() && time.Since(metadata.UpdatedAt) > ttl {
			removeErr := os.RemoveAll(dir)
			m.emit("upload.expired", metadata.ID, metadata.Offset, removeErr)
			release()
			continue
		}
		if metadata.Canceled || metadata.Failed {
			m.sessions[metadata.ID] = &session{metadata: metadata, dir: dir, done: true}
			release()
			continue
		}
		if metadata.Completed || metadata.Finalizing {
			if metadata.Offset != metadata.Size || metadata.Checksum == "" {
				m.emit("upload.restore_failed", metadata.ID, metadata.Offset, ErrInvalid)
				release()
				continue
			}
			s := &session{metadata: metadata, dir: dir}
			if metadata.Completed {
				s.done = true
				m.sessions[metadata.ID] = s
				release()
				continue
			}
			if _, completeErr := m.finalizeCompleted(s, true); completeErr != nil {
				release()
				m.emit("upload.restore_failed", metadata.ID, metadata.Offset, completeErr)
				continue
			}
			m.sessions[metadata.ID] = s
			m.emit("upload.restored", metadata.ID, s.metadata.Offset, nil)
			if idempotencyKey != "" {
				m.keys[idempotencyKey] = metadata.ID
			}
			release()
			continue
		}
		partPath := m.payloadPath(metadata.ID)
		if err = os.MkdirAll(m.payloadSessionDir(metadata.ID), 0o700); err != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, err)
			release()
			continue
		}
		part, openErr := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0o666)
		if openErr != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, openErr)
			release()
			continue
		}
		info, statErr := part.Stat()
		if statErr != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, statErr)
			_ = part.Close()
			release()
			continue
		}
		switch {
		case info.Size() < metadata.Offset:
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, ErrIncomplete)
			_ = part.Close()
			release()
			continue
		case info.Size() > metadata.Offset:
			if err = part.Truncate(metadata.Offset); err != nil {
				m.emit("upload.restore_failed", metadata.ID, metadata.Offset, err)
				_ = part.Close()
				release()
				continue
			}
		}
		if metadata.Offset > metadata.Size {
			metadata.Offset = 0
			if err = part.Truncate(0); err != nil {
				m.emit("upload.restore_failed", metadata.ID, metadata.Offset, err)
				_ = part.Close()
				release()
				continue
			}
		}
		syncErr := part.Sync()
		closeErr := part.Close()
		if err := errors.Join(syncErr, closeErr); err != nil {
			release()
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, err)
			continue
		}
		s := &session{metadata: metadata, dir: dir}
		if err = m.persist(s); err != nil {
			m.emit("upload.restore_failed", metadata.ID, metadata.Offset, err)
			release()
			continue
		}
		m.sessions[metadata.ID] = s
		m.emit("upload.restored", metadata.ID, metadata.Offset, nil)
		if idempotencyKey != "" {
			m.keys[idempotencyKey] = metadata.ID
		}
		release()
	}
	return nil
}
