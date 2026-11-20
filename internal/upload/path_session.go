package upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/winezer0/okserver/internal/digest"
)

func (m *Manager) MaxFileSize() int64 { return m.maxFileSize }

// Unknown-length bodies have a finite temporary-space ceiling even when the
// administrator leaves the managed-file size unlimited.
func (m *Manager) SpoolLimit() int64 {
	limit := int64(100 << 30)
	if m.maxPendingBytes > 0 {
		limit = min(limit, m.maxPendingBytes)
	}
	if m.maxFileSize > 0 {
		limit = min(limit, m.maxFileSize)
	}
	return limit
}

// FindPath resolves the reconstructible index, then reads only current state.
func (m *Manager) FindPath(path string) (Metadata, error) {
	m.cleanupExpired()
	newest, err := m.indexedPath(path)
	if err != nil {
		return Metadata{}, err
	}
	if newest.ID == "" {
		return Metadata{}, ErrNotFound
	}
	if newest.Completed && newest.PublishedFingerprint != "" {
		target, err := m.destinationPath(newest.Path)
		if err != nil {
			return Metadata{}, err
		}
		file, err := os.Open(target)
		if err != nil {
			if os.IsNotExist(err) {
				// The published file may have been deleted independently of its
				// durable session metadata. Treat the old session as absent so a
				// client can upload the destination again.
				return Metadata{}, ErrNotFound
			}
			return newest, ErrConflict
		}
		value, reliable, err := digest.Fingerprint(file)
		file.Close()
		if err != nil || reliable && value != newest.PublishedFingerprint {
			return newest, ErrConflict
		}
	}
	// Status reads must not wait behind a potentially long verification hash.
	return newest, nil
}

func (m *Manager) PrefixSHA256(id string) (string, error) {
	return m.PrefixSHA256Context(context.Background(), id)
}

func (m *Manager) PrefixSHA256Context(ctx context.Context, id string) (string, error) {
	s, err := m.lookup(id)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := m.lockSession(s)
	if err != nil {
		return "", err
	}
	defer release()
	if s.metadata.Completed {
		return s.metadata.Checksum, nil
	}
	file, err := os.Open(m.payloadPath(s.metadata.ID))
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err = io.CopyN(h, contextReader{ctx: ctx, reader: file}, s.metadata.Offset); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BindSHA256 persists the full-file expectation across requests and restarts.
func (m *Manager) BindSHA256(id, expected string) (Metadata, error) {
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
	if expected != "" && s.metadata.ExpectedSHA256 != "" && expected != s.metadata.ExpectedSHA256 {
		return s.metadata, ErrConflict
	}
	if expected != "" && s.metadata.ExpectedSHA256 == "" {
		s.metadata.ExpectedSHA256 = expected
		if err = m.persist(s); err != nil {
			return Metadata{}, err
		}
	}
	return s.metadata, nil
}
