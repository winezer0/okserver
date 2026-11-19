package upload

import (
	"errors"
	"strings"
	"testing"
)

func TestSessionIsolationAndExplicitTakeover(t *testing.T) {
	manager, _ := NewManager(Config{Root: t.TempDir()})
	one, two := strings.Repeat("a", 32), strings.Repeat("b", 32)
	task, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: "sha256", Owner: "alice", Token: one})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Authorize(task.ID, "alice", two, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("shared account bypassed session: %v", err)
	}
	if _, err = manager.Authorize(task.ID, "bob", two, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("takeover crossed owners: %v", err)
	}
	updated, err := manager.Authorize(task.ID, "alice", two, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckAccess(updated, "alice", one); !errors.Is(err, ErrConflict) {
		t.Fatal("old token remains valid")
	}
	if err = CheckAccess(updated, "alice", two); err != nil {
		t.Fatal(err)
	}
	manager, err = NewManager(Config{Root: manager.root})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := manager.Snapshot(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckAccess(restored, "alice", two); err != nil {
		t.Fatal("takeover was not durable", err)
	}
}
