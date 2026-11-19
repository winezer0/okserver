package upload

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/winezer0/okserver/internal/digest"
)

func TestRestoreAndCleanupEvents(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.Create(CreateRequest{Path: "file", Size: 8, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.WriteChunk(task.ID, 0, 3, 8, hashString([]byte("test")), bytes.NewBufferString("test")); err != nil {
		t.Fatal(err)
	}
	invalidID := strings.Repeat("f", 32)
	dir := filepath.Join(root, ".okserver", "uploads", invalidID)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	var events []Event
	restored, err := NewManager(Config{Root: root, MaxSessions: 1, MaxPendingBytes: 8, Observe: func(event Event) { events = append(events, event) }})
	if err != nil {
		t.Fatal(err)
	}
	found, corrupt := false, false
	for _, event := range events {
		if event.Name == "upload.restored" && event.TaskID == task.ID && event.Offset == 4 && event.Err == nil {
			found = true
		}
		if event.Name == "upload.restore_failed" && event.TaskID == invalidID && errors.Is(event.Err, ErrInvalid) {
			corrupt = true
		}
	}
	if !found || !corrupt {
		t.Fatalf("missing events: %+v", events)
	}
	if _, err := restored.Create(CreateRequest{Path: "another", Size: 1, ChecksumAlgorithm: digest.AlgorithmSHA256}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("restored session lost resource reservation: %v", err)
	}
	s := restored.sessions[task.ID]
	s.metadata.UpdatedAt = time.Now().Add(-8 * 24 * time.Hour)
	if err := restored.persist(s); err != nil {
		t.Fatal(err)
	}
	restored.PruneExpired()
	if _, err := restored.Get(task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session remains: %v", err)
	}
	if _, err := os.Stat(s.dir); !os.IsNotExist(err) {
		t.Fatalf("expired payload remains: %v", err)
	}
	last := events[len(events)-1]
	if last.Name != "upload.expired" || last.TaskID != task.ID || last.Err != nil {
		t.Fatalf("missing expiry event: %+v", last)
	}
	if _, err := restored.Create(CreateRequest{Path: "another", Size: 8, ChecksumAlgorithm: digest.AlgorithmSHA256}); err != nil {
		t.Fatalf("expired quota was not released: %v", err)
	}
}
