package server

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/winezer0/okserver/internal/storage"
	"github.com/winezer0/okserver/internal/upload"
)

func TestSpoolLimitAndCleanup(t *testing.T) {
	root := t.TempDir()
	manager, err := upload.NewManager(upload.Config{Root: root, MaxFileSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	server := &fileServer{path: root, uploads: manager}
	if _, _, _, err = server.spool(bytes.NewBufferString("12345")); !errors.Is(err, errSpoolLimit) {
		t.Fatalf("limit: %v", err)
	}
	file, size, cleanup, err := server.spool(bytes.NewBufferString("1234"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || size != 4 || string(data) != "1234" {
		t.Fatalf("spool %q %d %v", data, size, err)
	}
	name := file.Name()
	cleanup()
	if _, err = os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scratch retained: %v", err)
	}
}

func TestPruneSpoolsPreservesActiveAndFresh(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, ".okserver")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, name := range []string{"stream-orphan", "stream-active", "stream-fresh", "unrelated"} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if name != "stream-fresh" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	release, err := storage.TryFileLock(filepath.Join(directory, "stream-active.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	server := &fileServer{path: root}
	server.pruneSpools(time.Now())
	for _, name := range []string{"stream-active", "stream-fresh", "unrelated"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("removed %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(directory, "stream-orphan")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan retained: %v", err)
	}
}
