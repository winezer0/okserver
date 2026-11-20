package upload

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIndexDiscoversOtherManagerAndRebuilds(t *testing.T) {
	config := Config{Root: t.TempDir()}
	first, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	task, err := second.Create(CreateRequest{Path: "file", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := first.FindPath("file"); err != nil || found.ID != task.ID {
		t.Fatal(found, err)
	}
	if len(first.Tasks()) != 1 {
		t.Fatal("missing external active task")
	}
	if err = second.Cancel(task.ID); err != nil {
		t.Fatal(err)
	}
	if len(first.Tasks()) != 0 {
		t.Fatal("stale active task")
	}
	replacement, err := second.Create(CreateRequest{Path: "file", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := first.FindPath("file"); err != nil || found.ID != replacement.ID {
		t.Fatal(found, err)
	}
	rebuilt, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := rebuilt.FindPath("file"); err != nil || found.ID != replacement.ID {
		t.Fatal(found, err)
	}
}

func TestDeletedPublishedFileCanBeUploadedAgain(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("test")
	first, err := manager.Create(CreateRequest{Path: "file", Size: int64(len(payload)), ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.WriteChunk(first.ID, 0, int64(len(payload)-1), int64(len(payload)), hashString(payload), bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Complete(first.ID, hashString(payload)); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, "file")); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.FindPath("file"); err != ErrNotFound {
		t.Fatalf("FindPath after deleting published file = %v, want ErrNotFound", err)
	}
	second, err := manager.Create(CreateRequest{Path: "file", Size: int64(len(payload)), ChecksumAlgorithm: "sha256"})
	if err != nil || second.ID == first.ID {
		t.Fatalf("reupload session = %+v, %v", second, err)
	}
}

func TestStateAndCacheDirectoriesAreIndependentOfFilesRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "files")
	stateDir := filepath.Join(base, "states")
	cacheDir := filepath.Join(base, "cache")
	manager, err := NewManager(Config{Root: root, StateDir: stateDir, CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	wantSessionDir, err := filepath.EvalSymlinks(filepath.Join(stateDir, "uploads"))
	if err != nil {
		t.Fatal(err)
	}
	if manager.sessionDir != wantSessionDir {
		t.Fatalf("session dir = %q, want %q", manager.sessionDir, wantSessionDir)
	}
	wantStateDir, err := filepath.EvalSymlinks(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	wantCacheDir, err := filepath.EvalSymlinks(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if manager.cacheDir != wantCacheDir || manager.stateDir != wantStateDir {
		t.Fatalf("state/cache paths = %q, %q; want %q, %q", manager.stateDir, manager.cacheDir, wantStateDir, wantCacheDir)
	}
	if _, err := os.Stat(filepath.Join(root, "okserver_states")); !os.IsNotExist(err) {
		t.Fatalf("state directory was created inside upload root: %v", err)
	}
}

func TestIndexDoesNotRereadHistoricalMetadata(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	old, err := manager.Create(CreateRequest{Path: "old", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Cancel(old.ID); err != nil {
		t.Fatal(err)
	}
	current, err := manager.Create(CreateRequest{Path: "current", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(manager.sessionDir, old.ID, "metadata.json"), []byte("corrupted historical record"), 0600); err != nil {
		t.Fatal(err)
	}
	if found, err := manager.FindPath("current"); err != nil || found.ID != current.ID {
		t.Fatal(found, err)
	}
	if len(manager.Tasks()) != 1 {
		t.Fatal("historical JSON disturbed active index")
	}
}
