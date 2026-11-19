package upload

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
)

func CheckAccess(metadata Metadata, owner, token string) error {
	if metadata.Owner != "" && metadata.Owner != owner {
		return ErrConflict
	}
	if metadata.TokenSHA256 != "" {
		sum := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
		if subtle.ConstantTimeCompare([]byte(sum), []byte(metadata.TokenSHA256)) != 1 {
			return ErrConflict
		}
	}
	return nil
}

// Authorize binds managed clients to a durable token; simple curl uploads keep
// their authentication and destination lock without requiring extra headers.
func (m *Manager) Authorize(id, owner, token string, takeover bool) (Metadata, error) {
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
	if s.metadata.Owner != "" && s.metadata.Owner != owner {
		return Metadata{}, ErrConflict
	}
	sum := ""
	if token != "" {
		if len(token) < 32 || len(token) > 256 {
			return Metadata{}, ErrInvalid
		}
		sum = fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	}
	if s.metadata.TokenSHA256 != "" && subtle.ConstantTimeCompare([]byte(sum), []byte(s.metadata.TokenSHA256)) != 1 && !takeover {
		return Metadata{}, ErrConflict
	}
	if s.metadata.Owner == "" || (sum != "" && (s.metadata.TokenSHA256 == "" || takeover)) {
		s.metadata.Owner = owner
		s.metadata.TokenSHA256 = sum
		if err = m.persist(s); err != nil {
			return Metadata{}, err
		}
	}
	return s.metadata, nil
}
