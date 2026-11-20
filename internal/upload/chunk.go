package upload

import (
	"crypto/subtle"
	"encoding"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"time"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
)

func (m *Manager) WriteChunk(id string, start, end, total int64, expectedChecksum string, body io.Reader) (Metadata, error) {
	return m.writeChunk(id, start, end, total, expectedChecksum, true, body)
}

// WriteChunkCalculated appends a chunk without requiring a client checksum.
// The completed file is still hashed by CompleteCalculated before publication.
func (m *Manager) WriteChunkCalculated(id string, start, end, total int64, body io.Reader) (Metadata, error) {
	return m.writeChunk(id, start, end, total, "", false, body)
}

func (m *Manager) writeChunk(id string, start, end, total int64, expectedChecksum string, verifyExpected bool, body io.Reader) (Metadata, error) {
	if err := m.checkFreeSpace(); err != nil {
		return Metadata{}, err
	}
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
	if s.done {
		return s.metadata, ErrNotFound
	}
	if s.metadata.Completed {
		return s.metadata, ErrOffset
	}
	if s.metadata.Canceled || s.metadata.Failed {
		return s.metadata, ErrConflict
	}

	length := end - start + 1
	if start < 0 || end < start || total != s.metadata.Size || length <= 0 || length > m.chunkSize || end >= total {
		return s.metadata, ErrInvalid
	}
	if start != s.metadata.Offset {
		return s.metadata, ErrOffset
	}
	if verifyExpected {
		checksum, err := normalizeChecksum(expectedChecksum, false)
		if err != nil {
			return s.metadata, ErrInvalid
		}
		expectedChecksum = checksum
	}

	part, err := os.OpenFile(m.payloadPath(s.metadata.ID), os.O_RDWR, 0o600)
	if err != nil {
		return s.metadata, err
	}
	defer part.Close()
	fingerprint, reliable, err := digest.Fingerprint(part)
	if err != nil {
		return s.metadata, err
	}
	if s.whole == nil || s.hashedOffset != start || !reliable || fingerprint != s.hashFingerprint {
		s.whole = sha256.New()
		if _, err = io.CopyN(s.whole, io.NewSectionReader(part, 0, start), start); err != nil {
			return s.metadata, err
		}
		s.hashedOffset = start
	}
	snapshot, err := s.whole.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return s.metadata, err
	}
	rollback := func(cause error) (Metadata, error) {
		restoreErr := s.whole.(encoding.BinaryUnmarshaler).UnmarshalBinary(snapshot)
		s.hashedOffset = start
		truncateErr := part.Truncate(start)
		syncErr := part.Sync()
		if restoreErr != nil {
			s.whole = nil
		}
		if truncateErr != nil || syncErr != nil {
			s.whole = nil
		}
		s.hashFingerprint, _, _ = digest.Fingerprint(part)
		return s.metadata, errors.Join(cause, restoreErr, truncateErr, syncErr)
	}

	var writer io.Writer = storage.SpaceWriter{Writer: io.NewOffsetWriter(part, start), Root: m.cacheDir, Minimum: m.minFreeBytes}
	writer = io.MultiWriter(writer, s.whole)
	var hash hash.Hash
	if verifyExpected {
		hash = sha256.New()
		writer = io.MultiWriter(writer, hash)
	}
	written, err := io.CopyN(writer, body, length+1)
	if written != length {
		if written < length {
			return rollback(io.ErrUnexpectedEOF)
		}
		return rollback(ErrInvalid)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return rollback(err)
	}
	if verifyExpected {
		actualHash := hex.EncodeToString(hash.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedChecksum)) != 1 {
			return rollback(ErrChecksum)
		}
	}
	if err = part.Sync(); err != nil {
		return rollback(err)
	}

	previous := s.metadata
	s.metadata.Offset = start + length
	s.metadata.UpdatedAt = time.Now().UTC()
	if err = m.persist(s); err != nil {
		s.metadata = previous
		return rollback(err)
	}
	s.hashedOffset = s.metadata.Offset
	s.hashFingerprint, _, err = digest.Fingerprint(part)
	if err != nil {
		s.whole = nil
	}
	return s.metadata, nil
}
