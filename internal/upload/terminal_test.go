package upload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanceledTerminalRetentionAndNoPollExtension(t *testing.T) {
	root := t.TempDir()
	config := Config{Root: root, SessionTTL: time.Hour, TerminalTTL: 40 * time.Millisecond}
	manager, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Cancel(task.ID); err != nil {
		t.Fatal(err)
	}
	before, err := manager.Get(task.ID)
	if err != nil || !before.Canceled {
		t.Fatal("cancel state lost", err)
	}
	time.Sleep(time.Millisecond)
	after, err := manager.Get(task.ID)
	if err != nil || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("poll extends terminal lifetime")
	}
	time.Sleep(50 * time.Millisecond)
	manager, err = NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Get(task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired terminal restored", err)
	}
}

func TestCanceledVerificationDoesNotPublish(t *testing.T) {
	manager, _ := NewManager(Config{Root: t.TempDir()})
	task, err := manager.Create(CreateRequest{Path: "file", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.WriteChunkCalculated(task.ID, 0, 2, 3, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = manager.CompleteContext(ctx, task.ID, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("verification ignored cancel", err)
	}
	state, err := manager.Get(task.ID)
	if err != nil || state.Completed {
		t.Fatal("canceled verification published", err)
	}
}

func TestCleanupDoesNotWaitForActiveSession(t *testing.T) {
	manager, _ := NewManager(Config{Root: t.TempDir()})
	task, err := manager.Create(CreateRequest{Path: "file", Size: 3, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.lookup(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	done := make(chan struct{})
	go func() { manager.PruneExpired(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup waited behind active task")
	}
	if snapshot, err := manager.Snapshot(task.ID); err != nil || snapshot.ID != task.ID {
		t.Fatalf("snapshot blocked or failed: %v", err)
	}
}

func TestPrefixHashCancellation(t *testing.T) {
	manager, _ := NewManager(Config{Root: t.TempDir()})
	task, err := manager.Create(CreateRequest{Path: "file", Size: 6, ChecksumAlgorithm: "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.WriteChunkCalculated(task.ID, 0, 2, 6, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = manager.PrefixSHA256Context(ctx, task.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("prefix hashing ignored cancellation: %v", err)
	}
}

func TestModifiedStagingInvalidatesIncrementalHash(t *testing.T) {
	for _, stage := range []string{"between-chunks", "before-completion"} {
		t.Run(stage, func(t *testing.T) {
			manager, _ := NewManager(Config{Root: t.TempDir(), ChunkSize: 3})
			task, err := manager.Create(CreateRequest{Path: "file", Size: 6, ChecksumAlgorithm: "sha256"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.WriteChunkCalculated(task.ID, 0, 2, 6, strings.NewReader("abc")); err != nil {
				t.Fatal(err)
			}
			if stage == "before-completion" {
				if _, err = manager.WriteChunkCalculated(task.ID, 3, 5, 6, strings.NewReader("def")); err != nil {
					t.Fatal(err)
				}
			}
			part, err := os.OpenFile(filepath.Join(manager.sessionDir, task.ID, "payload.part"), os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = part.WriteAt([]byte("XYZ"), 0); err != nil {
				t.Fatal(err)
			}
			part.Sync()
			part.Close()
			if stage == "between-chunks" {
				if _, err = manager.WriteChunkCalculated(task.ID, 3, 5, 6, strings.NewReader("def")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = manager.Complete(task.ID, hashString([]byte("abcdef"))); !errors.Is(err, ErrChecksum) {
				t.Fatalf("stale incremental hash accepted: %v", err)
			}
		})
	}
}
