package upload

import (
	"crypto/rand"
	"encoding/hex"
	"hash"
	"path/filepath"
	"sync"
	"time"

	"github.com/winezer0/okserver/internal/digest"
)

// CreateRequest is the public creation contract; business validation remains here.
type CreateRequest struct {
	Owner             string `json:"-"`
	Token             string `json:"-"`
	IfMatch           string `json:"if_match,omitempty"`
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	IdempotencyKey    string `json:"idempotency_key,omitempty"`
}

type Metadata struct {
	PublishedFingerprint string         `json:"published_fingerprint,omitempty"`
	Owner                string         `json:"owner,omitempty"`
	TokenSHA256          string         `json:"token_sha256,omitempty"`
	Canceled             bool           `json:"canceled,omitempty"`
	Failed               bool           `json:"failed,omitempty"`
	ExpectedSHA256       string         `json:"expected_sha256,omitempty"`
	ErrorCode            string         `json:"error_code,omitempty"`
	BaseChecksum         string         `json:"base_checksum,omitempty"`
	ID                   string         `json:"id"`
	Path                 string         `json:"path"`
	Size                 int64          `json:"size"`
	Offset               int64          `json:"offset"`
	ChecksumAlgorithm    string         `json:"checksum_algorithm"`
	Checksum             string         `json:"checksum,omitempty"`
	Hashes               *digest.Hashes `json:"hashes,omitempty"`
	ChunkSize            int64          `json:"chunk_size"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	Completed            bool           `json:"completed,omitempty"`
	Finalizing           bool           `json:"finalizing,omitempty"`
	IdempotencyKey       string         `json:"idempotency_key,omitempty"`
}

type session struct {
	whole           hash.Hash
	hashedOffset    int64
	hashFingerprint string
	mu              sync.Mutex
	metadata        Metadata
	dir             string
	done            bool
}

func (m *Manager) lookup(id string) (*session, error) {
	m.cleanupExpired()
	if !validID(id) {
		return nil, ErrNotFound
	}
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s == nil {
		dir := filepath.Join(m.sessionDir, id)
		metadata, readErr := readMetadata(dir)
		if readErr != nil {
			return nil, ErrNotFound
		}
		if metadata.ID != id {
			return nil, ErrNotFound
		}
		loaded := &session{metadata: metadata, dir: dir}
		m.mu.Lock()
		s = m.sessions[id]
		if s == nil {
			s = loaded
			m.sessions[id] = s
		}
		m.mu.Unlock()
	}
	return s, nil
}

func newID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
