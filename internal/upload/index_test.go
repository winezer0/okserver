package upload

import (
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
