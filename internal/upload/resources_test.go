package upload

import (
	"bytes"
	"testing"

	"github.com/winezer0/okserver/internal/digest"
)

func TestResourceCeilings(t *testing.T) {
	for _, configuration := range []Config{
		{MaxSessions: 1},
		{MaxPendingBytes: 4},
		{MinFreeBytes: ^uint64(0)},
	} {
		configuration.Root = t.TempDir()
		manager, err := NewManager(configuration)
		if err != nil {
			t.Fatal(err)
		}
		_, err = manager.Create(CreateRequest{Path: "a", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
		if configuration.MinFreeBytes > 0 {
			if err == nil {
				t.Fatal("disk floor not enforced")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = manager.Create(CreateRequest{Path: "b", Size: 1, ChecksumAlgorithm: digest.AlgorithmSHA256}); err == nil {
			t.Fatal("resource ceiling not enforced")
		}
	}
}

func TestPerIdentityLimit(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), MaxPerOwner: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []CreateRequest{{Path: "one", Owner: "alice"}, {Path: "two", Owner: "bob"}} {
		request.ChecksumAlgorithm = digest.AlgorithmSHA256
		if _, err := manager.Create(request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = manager.Create(CreateRequest{Path: "three", Owner: "alice", ChecksumAlgorithm: digest.AlgorithmSHA256}); err != ErrTooLarge {
		t.Fatalf("identity ceiling: %v", err)
	}
}
func TestDestinationConflictAndSharedSessionRecovery(t *testing.T) {
	root := t.TempDir()
	one, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	first, err := one.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	second, err := one.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []Metadata{first, second} {
		payload := []byte("test")
		if _, err = two.WriteChunk(session.ID, 0, 3, 4, hashString(payload), bytes.NewReader(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = one.Complete(first.ID, hashString([]byte("test"))); err != nil {
		t.Fatal(err)
	}
	// Make the competing payload different so the target version has changed.
	if _, err = two.Complete(second.ID, hashString([]byte("test"))); err != ErrPrecondition {
		t.Fatalf("expected failed destination precondition, got %v", err)
	}
}

func TestCancelReleasesResourceReservation(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), MaxPendingBytes: 4, MaxSessions: 1})
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Create(CreateRequest{Path: "first", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Cancel(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Create(CreateRequest{Path: "second", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256}); err != nil {
		t.Fatalf("reservation leaked: %v", err)
	}
}

func TestIdempotencyAfterAnotherProcessCancels(t *testing.T) {
	root := t.TempDir()
	first, err := NewManager(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRequest{Path: "file", Size: 1, ChecksumAlgorithm: digest.AlgorithmSHA256, IdempotencyKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	original, err := first.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err = second.Cancel(original.ID); err != nil {
		t.Fatal(err)
	}
	recreated, err := first.Create(request)
	if err != nil || recreated.ID == original.ID {
		t.Fatalf("stale cancelled session: %+v %v", recreated, err)
	}
}
