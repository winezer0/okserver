package hashmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
)

const manifestVersion = 1

type Entry struct {
	Path        string        `json:"path"`
	Size        int64         `json:"size"`
	Fingerprint string        `json:"fingerprint"`
	Hashes      digest.Hashes `json:"hashes"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type document struct {
	Version int              `json:"version"`
	Files   map[string]Entry `json:"files"`
}

type Store struct {
	path       string
	mu         sync.Mutex
	cached     document
	cachedInfo os.FileInfo
}

func New(stateDir string) *Store {
	return &Store{path: filepath.Join(stateDir, "file-hashes.json")}
}

func (s *Store) Lookup(path, fingerprint string) (Entry, bool, error) {
	if s == nil || path == "" || fingerprint == "" {
		return Entry{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.lookupDocument()
	if err != nil {
		return Entry{}, false, err
	}
	entry, ok := doc.Files[path]
	if !ok || entry.Path != path || entry.Fingerprint != fingerprint || !entry.Hashes.Valid() {
		return Entry{}, false, nil
	}
	return entry, true, nil
}

func (s *Store) Put(entry Entry) error {
	if s == nil {
		return errors.New("hash manifest is unavailable")
	}
	if entry.Path == "" || entry.Size < 0 || entry.Fingerprint == "" || !entry.Hashes.Valid() {
		return errors.New("invalid hash manifest entry")
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	release, err := storage.TryFileLock(s.path + ".lock")
	if err != nil {
		return err
	}
	defer release()
	doc, err := readDocument(s.path)
	if err != nil {
		return err
	}
	doc.Files[entry.Path] = entry
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".file-hashes-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(append(encoded, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = storage.DurableReplace(tempName, s.path); err != nil {
		return fmt.Errorf("publish hash manifest: %w", err)
	}
	s.cachedInfo = nil
	return nil
}

// Writers replace the manifest atomically. Cache only a stable opened version;
// inode identity, size and mtime also invalidate another process's replacement.
func (s *Store) lookupDocument() (document, error) {
	before, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return document{Version: manifestVersion, Files: make(map[string]Entry)}, nil
	}
	if err != nil {
		return document{}, err
	}
	if s.cachedInfo != nil && os.SameFile(before, s.cachedInfo) && before.Size() == s.cachedInfo.Size() && before.ModTime().Equal(s.cachedInfo.ModTime()) {
		return s.cached, nil
	}
	doc, err := readDocument(s.path)
	if err != nil {
		return document{}, err
	}
	after, err := os.Stat(s.path)
	if err == nil && os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) {
		s.cached, s.cachedInfo = doc, after
	}
	return doc, nil
}

func readDocument(path string) (document, error) {
	doc := document{Version: manifestVersion, Files: make(map[string]Entry)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return document{}, err
	}
	if err = json.Unmarshal(data, &doc); err != nil {
		return document{}, fmt.Errorf("decode hash manifest: %w", err)
	}
	if doc.Version != manifestVersion || doc.Files == nil {
		return document{}, errors.New("unsupported hash manifest format")
	}
	return doc, nil
}
