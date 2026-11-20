package diskbudget

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/winezer0/okserver/internal/storage"
)

func TestReservationsPreventOvercommit(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	stateDir := filepath.Join(root, "states")
	os.MkdirAll(cacheDir, 0700)
	free, err := storage.FreeSpace(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Reserve(cacheDir, stateDir, free*3/4, 0)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := Reserve(cacheDir, stateDir, free/2, 0); !errors.Is(err, storage.ErrInsufficientSpace) {
		if next != nil {
			next.Close()
		}
		t.Fatalf("overcommit accepted: %v", err)
	}
	first.Close()
	next, err := Reserve(cacheDir, stateDir, free/2, 0)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
func TestBudgetIncludesUnwrittenSessionBytes(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	stateDir := filepath.Join(root, "states")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	free, err := storage.FreeSpace(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(stateDir, "uploads", "task")
	os.MkdirAll(directory, 0700)
	data, _ := json.Marshal(map[string]any{"size": free / 2, "offset": 0, "completed": false})
	os.WriteFile(filepath.Join(directory, "metadata.json"), data, 0600)
	release, err := Admit(cacheDir, stateDir, free*3/4, 0)
	if err == nil {
		release()
		t.Fatal("pending session ignored")
	}
}
