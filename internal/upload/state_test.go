package upload

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/winezer0/okserver/internal/digest"
)

func TestSessionStateTransitionsAndResume(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	checksum := hashString([]byte("data"))
	task, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if task.State() != StateReceiving {
		t.Fatal(task.State())
	}
	if err = task.beginFinalizing(checksum, digest.Hashes{}, time.Now()); !errors.Is(err, ErrIncomplete) {
		t.Fatal("incomplete transition allowed")
	}
	task, err = manager.WriteChunk(task.ID, 0, 3, 4, checksum, bytes.NewBufferString("data"))
	if err != nil {
		t.Fatal(err)
	}
	if task.State() != StateReady {
		t.Fatal(task.State())
	}
	restored, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	task, err = restored.Complete(task.ID, checksum)
	if err != nil {
		t.Fatal(err)
	}
	if task.State() != StateCompleted {
		t.Fatal(task.State())
	}
	if _, err = restored.Complete(task.ID, checksum); err != nil {
		t.Fatal("completion is not idempotent", err)
	}
}
