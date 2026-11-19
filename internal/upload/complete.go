package upload

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
)

func (m *Manager) Complete(id, expectedChecksum string) (Metadata, error) {
	expectedChecksum, err := normalizeChecksum(expectedChecksum, false)
	if err != nil {
		return Metadata{}, err
	}
	return m.CompleteContext(context.Background(), id, expectedChecksum)
}

// CompleteCalculated finalizes an upload using the digest calculated from the
// server-side staging file. It is intended for clients that do not send hashes.
func (m *Manager) CompleteCalculated(id string) (Metadata, error) {
	return m.CompleteContext(context.Background(), id, "")
}

func (m *Manager) CompleteContext(ctx context.Context, id, expectedChecksum string) (Metadata, error) {
	verifyExpected := expectedChecksum != ""
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
	if s.metadata.Canceled || s.metadata.Failed {
		return s.metadata, ErrConflict
	}
	if s.done {
		if verifyExpected && subtle.ConstantTimeCompare([]byte(s.metadata.Checksum), []byte(expectedChecksum)) != 1 {
			return s.metadata, ErrChecksum
		}
		return s.metadata, nil
	}
	if s.metadata.Completed {
		if verifyExpected && subtle.ConstantTimeCompare([]byte(s.metadata.Checksum), []byte(expectedChecksum)) != 1 {
			return s.metadata, ErrChecksum
		}
		return s.metadata, nil
	}
	if s.metadata.Finalizing {
		if verifyExpected && subtle.ConstantTimeCompare([]byte(s.metadata.Checksum), []byte(expectedChecksum)) != 1 {
			return s.metadata, ErrChecksum
		}
		return m.finalizeCompleted(s, true)
	}
	if s.metadata.Offset != s.metadata.Size {
		return s.metadata, ErrIncomplete
	}

	partPath := filepath.Join(s.dir, "payload.part")
	part, err := os.Open(partPath)
	if err != nil {
		return s.metadata, err
	}
	info, err := part.Stat()
	if err != nil {
		_ = part.Close()
		return s.metadata, err
	}
	if info.Size() != s.metadata.Size {
		_ = part.Close()
		return s.metadata, ErrIncomplete
	}
	var hashes digest.Hashes
	var hashErr error
	fingerprint, reliable, err := digest.Fingerprint(part)
	if err != nil {
		part.Close()
		return s.metadata, err
	}
	if s.whole != nil && s.hashedOffset == s.metadata.Size && reliable && fingerprint == s.hashFingerprint {
		hashes.SHA256 = hex.EncodeToString(s.whole.Sum(nil))
	} else {
		h := sha256.New()
		_, hashErr = io.Copy(h, contextReader{ctx: ctx, reader: part})
		hashes.SHA256 = hex.EncodeToString(h.Sum(nil))
	}
	if hashErr != nil {
		_ = part.Close()
		return s.metadata, hashErr
	}
	if err = part.Close(); err != nil {
		return s.metadata, err
	}
	actualChecksum := hashes.SHA256
	if err = ctx.Err(); err != nil {
		return s.metadata, err
	}
	if verifyExpected && subtle.ConstantTimeCompare([]byte(actualChecksum), []byte(expectedChecksum)) != 1 {
		resetFile, resetErr := os.OpenFile(partPath, os.O_RDWR, 0o600)
		if resetErr == nil {
			resetErr = resetFile.Truncate(0)
		}
		if resetErr == nil {
			resetErr = resetFile.Sync()
		}
		if resetFile != nil {
			if closeErr := resetFile.Close(); resetErr == nil {
				resetErr = closeErr
			}
		}
		if resetErr != nil {
			return s.metadata, resetErr
		}
		previous := s.metadata
		s.whole = nil
		s.hashedOffset = 0
		s.metadata.Offset = 0
		s.metadata.Failed = true
		s.metadata.UpdatedAt = time.Now().UTC()
		if resetErr = m.persist(s); resetErr != nil {
			s.metadata = previous
			return s.metadata, resetErr
		}
		return s.metadata, ErrChecksum
	}

	previous := s.metadata
	if err = s.metadata.beginFinalizing(actualChecksum, hashes, time.Now()); err != nil {
		return s.metadata, err
	}
	if err = m.persist(s); err != nil {
		s.metadata = previous
		return s.metadata, err
	}
	return m.finalizeCompleted(s, false)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (m *Manager) finalizeCompleted(s *session, verify bool) (Metadata, error) {
	target, err := m.destinationPath(s.metadata.Path)
	if err != nil {
		return s.metadata, err
	}
	unlock := storage.LockPath(target)
	defer unlock()
	release, lockErr := storage.TryDestinationLock(m.root, target)
	if lockErr != nil {
		return s.metadata, ErrBusy
	}
	defer release()
	partPath := filepath.Join(s.dir, "payload.part")
	if _, err = os.Stat(partPath); err == nil {
		if s.metadata.BaseChecksum != "" {
			current, digestErr := digest.FileDigest(target)
			if digestErr != nil {
				return s.metadata, digestErr
			}
			if current != s.metadata.BaseChecksum {
				return s.metadata, ErrPrecondition
			}
		}
		if s.metadata.Hashes == nil {
			hashes, hashErr := hashFile(partPath)
			if hashErr != nil {
				return s.metadata, hashErr
			}
			if hashes.SHA256 != s.metadata.Checksum {
				return s.metadata, ErrChecksum
			}
			s.metadata.Hashes = &hashes
		} else if verify {
			if err = verifyFile(partPath, s.metadata.Size, s.metadata.Checksum); err != nil {
				return s.metadata, err
			}
		}
		if s.metadata.BaseChecksum == "missing" {
			err = storage.PublishNew(partPath, target)
		} else {
			err = storage.DurableReplace(partPath, target)
		}
		if errors.Is(err, os.ErrExist) {
			return s.metadata, ErrConflict
		}
		if err != nil {
			return s.metadata, err
		}
		digest.RememberPublishedDigest(partPath, target)
		if s.metadata.Hashes != nil {
			digest.RememberVerifiedFile(target, s.metadata.Size, *s.metadata.Hashes)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return s.metadata, err
	} else if s.metadata.Hashes == nil {
		hashes, hashErr := hashFile(target)
		if hashErr != nil {
			return s.metadata, hashErr
		}
		if hashes.SHA256 != s.metadata.Checksum {
			return s.metadata, ErrChecksum
		}
		s.metadata.Hashes = &hashes
	} else if err = verifyFile(target, s.metadata.Size, s.metadata.Checksum); err != nil {
		return s.metadata, err
	}
	previous := s.metadata
	if err = s.metadata.finish(time.Now()); err != nil {
		return s.metadata, err
	}
	if published, e := os.Open(target); e == nil {
		s.metadata.PublishedFingerprint, _, _ = digest.Fingerprint(published)
		published.Close()
	}
	if err = m.persist(s); err != nil {
		s.metadata = previous
		return s.metadata, err
	}
	s.done = true
	return s.metadata, nil
}

func hashFile(name string) (digest.Hashes, error) {
	file, err := os.Open(name)
	if err != nil {
		return digest.Hashes{}, err
	}
	defer file.Close()
	return digest.OpenFileHashes(file)
}

func verifyFile(name string, size int64, expectedChecksum string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != size {
		return ErrIncomplete
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(hash.Sum(nil))), []byte(expectedChecksum)) != 1 {
		return ErrChecksum
	}
	return nil
}
