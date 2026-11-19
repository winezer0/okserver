package upload

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/diskbudget"
	"github.com/winezer0/okserver/internal/storage"
)

func (m *Manager) Create(request CreateRequest) (Metadata, error) {
	if request.Token != "" && (len(request.Token) < 32 || len(request.Token) > 256) {
		return Metadata{}, ErrInvalid
	}
	m.cleanupExpired()
	idempotencyKey, err := normalizeIdempotencyKey(request.IdempotencyKey)
	if err != nil {
		return Metadata{}, ErrInvalid
	}
	relativePath, err := CleanRelativePath(request.Path)
	if err != nil || request.Size < 0 {
		return Metadata{}, ErrInvalid
	}
	if m.maxFileSize > 0 && request.Size > m.maxFileSize {
		return Metadata{}, ErrTooLarge
	}

	if request.ChecksumAlgorithm != digest.AlgorithmSHA256 {
		return Metadata{}, ErrInvalid
	}
	m.createMu.Lock()
	defer m.createMu.Unlock()
	admissionRelease, admissionErr := storage.TryFileLock(filepath.Join(m.sessionDir, "admission.lock"))
	if admissionErr != nil {
		return Metadata{}, ErrBusy
	}
	defer admissionRelease()
	if idempotencyKey != "" {
		// Another process may have created this key since this manager started.
		entries, _ := os.ReadDir(m.sessionDir)
		for _, entry := range entries {
			if !entry.IsDir() || !validID(entry.Name()) {
				continue
			}
			disk, readErr := readMetadata(filepath.Join(m.sessionDir, entry.Name()))
			if readErr != nil {
				continue
			}
			if disk.Canceled || disk.Failed || disk.IdempotencyKey != idempotencyKey {
				continue
			}
			m.mu.Lock()
			m.keys[idempotencyKey] = disk.ID
			if m.sessions[disk.ID] == nil {
				m.sessions[disk.ID] = &session{metadata: disk, dir: filepath.Join(m.sessionDir, disk.ID)}
			}
			m.mu.Unlock()
			break
		}
		m.mu.RLock()
		existing := m.sessions[m.keys[idempotencyKey]]
		m.mu.RUnlock()
		if existing != nil {
			existing.mu.Lock()
			sessionRelease, readErr := m.lockSession(existing)
			metadata := existing.metadata
			if metadata.Canceled || metadata.Failed {
				readErr = ErrNotFound
			}
			if sessionRelease != nil {
				sessionRelease()
			}
			existing.mu.Unlock()
			if readErr == ErrNotFound {
				m.mu.Lock()
				delete(m.keys, idempotencyKey)
				delete(m.sessions, metadata.ID)
				m.mu.Unlock()
			} else {
				if readErr != nil {
					return Metadata{}, readErr
				}
				if metadata.Path != relativePath || metadata.Size != request.Size || metadata.ChecksumAlgorithm != request.ChecksumAlgorithm {
					return metadata, ErrConflict
				}
				return metadata, nil
			}
		}
	}
	if err := m.checkResources(request.Size, request.Owner); err != nil {
		return Metadata{}, err
	}
	budgetRelease, budgetErr := diskbudget.Admit(m.root, uint64(request.Size), m.minFreeBytes)
	if budgetErr != nil {
		return Metadata{}, budgetErr
	}
	defer budgetRelease()
	id, err := newID()
	if err != nil {
		return Metadata{}, err
	}
	target, err := m.destinationPath(relativePath)
	if err != nil {
		return Metadata{}, err
	}
	baseDigest, err := digest.FileDigest(target)
	if err != nil {
		return Metadata{}, err
	}
	if baseDigest != "missing" && request.IfMatch != "\"sha256:"+baseDigest+"\"" {
		if request.IfMatch != "" {
			return Metadata{}, ErrPrecondition
		}
		return Metadata{}, ErrConflict
	}
	dir := filepath.Join(m.sessionDir, id)
	if err = os.Mkdir(dir, 0o700); err != nil {
		return Metadata{}, err
	}
	if err = storage.SyncDirectory(m.sessionDir); err != nil {
		return Metadata{}, err
	}

	part, err := os.OpenFile(filepath.Join(dir, "payload.part"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o666)
	if err != nil {
		_ = os.RemoveAll(dir)
		return Metadata{}, err
	}
	if err = part.Close(); err != nil {
		_ = os.RemoveAll(dir)
		return Metadata{}, err
	}

	now := time.Now().UTC()
	metadata := Metadata{
		Owner:             request.Owner,
		BaseChecksum:      baseDigest,
		ID:                id,
		Path:              relativePath,
		Size:              request.Size,
		ChecksumAlgorithm: request.ChecksumAlgorithm,
		ChunkSize:         m.chunkSize,
		CreatedAt:         now,
		UpdatedAt:         now,
		IdempotencyKey:    idempotencyKey,
	}
	if request.Token != "" {
		metadata.TokenSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(request.Token)))
	}
	s := &session{metadata: metadata, dir: dir}
	if err = m.persist(s); err != nil {
		_ = os.RemoveAll(dir)
		return Metadata{}, err
	}
	m.mu.Lock()
	m.sessions[id] = s
	if idempotencyKey != "" {
		m.keys[idempotencyKey] = id
	}
	m.mu.Unlock()
	return metadata, nil
}
