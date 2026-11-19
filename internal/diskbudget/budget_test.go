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
	free, err := storage.FreeSpace(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Reserve(root, free*3/4, 0)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := Reserve(root, free/2, 0); !errors.Is(err, storage.ErrInsufficientSpace) {
		if next != nil {
			next.Close()
		}
		t.Fatalf("overcommit accepted: %v", err)
	}
	first.Close()
	next, err := Reserve(root, free/2, 0)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
func TestBudgetIncludesUnwrittenSessionBytes(t *testing.T) {
	root := t.TempDir()
	free, err := storage.FreeSpace(root)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, ".okserver", "uploads", "task")
	os.MkdirAll(directory, 0700)
	data, _ := json.Marshal(map[string]any{"size": free / 2, "offset": 0, "completed": false})
	os.WriteFile(filepath.Join(directory, "metadata.json"), data, 0600)
	release, err := Admit(root, free*3/4, 0)
	if err == nil {
		release()
		t.Fatal("pending session ignored")
	}
}
