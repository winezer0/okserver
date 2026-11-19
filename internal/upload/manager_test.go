package upload

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
)

func TestCompletionUsesSharedDestinationLock(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("test")
	if _, err = manager.WriteChunk(metadata.ID, 0, 3, 4, hashString(payload), bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	release := storage.LockPath(filepath.Join(root, "file"))
	done := make(chan error, 1)
	go func() { _, err := manager.Complete(metadata.ID, hashString(payload)); done <- err }()
	select {
	case err := <-done:
		release()
		t.Fatalf("completion bypassed lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("completion did not resume")
	}
}

func TestCalculatedUploadDoesNotRequireClientChecksums(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("curl-without-client-hash")
	metadata, err := manager.Create(CreateRequest{Path: "curl.bin", Size: int64(len(payload)), ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(payload); {
		end := offset + 4
		if end > len(payload) {
			end = len(payload)
		}
		metadata, err = manager.WriteChunkCalculated(metadata.ID, int64(offset), int64(end-1), int64(len(payload)), bytes.NewReader(payload[offset:end]))
		if err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	metadata, err = manager.CompleteCalculated(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.Completed || metadata.Checksum != hashString(payload) {
		t.Fatalf("calculated completion metadata = %+v", metadata)
	}
	retried, err := manager.CompleteCalculated(metadata.ID)
	if err != nil || retried.Checksum != metadata.Checksum {
		t.Fatalf("calculated completion retry = %+v, %v", retried, err)
	}
	stored, err := os.ReadFile(filepath.Join(root, "curl.bin"))
	if err != nil || !bytes.Equal(stored, payload) {
		t.Fatalf("stored file = %q, %v", stored, err)
	}
}

func TestCompletedUploadCreateRetryWithoutInitialChecksum(t *testing.T) {
	config := Config{Root: t.TempDir(), ChunkSize: 4}
	manager, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRequest{Path: "retry.bin", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256, IdempotencyKey: strings.Repeat("a", 32)}
	session, err := manager.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("test")
	if _, err = manager.WriteChunk(session.ID, 0, 3, 4, hashString(payload), bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Complete(session.ID, hashString(payload)); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		got, retryErr := manager.Create(request)
		if retryErr != nil || got.ID != session.ID || !got.Completed {
			t.Fatalf("completion retry: %+v, %v", got, retryErr)
		}
		manager, err = NewManager(config)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestUploadSessionResumesAfterManagerRestart(t *testing.T) {
	root := t.TempDir()
	config := Config{Root: root, ChunkSize: 4}
	manager, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("abcdefghij")
	fullHash := hashString(payload)
	metadata, err := manager.Create(CreateRequest{Path: "nested/file.bin", Size: int64(len(payload)), ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}

	first := payload[:4]
	metadata, err = manager.WriteChunk(metadata.ID, 0, 3, int64(len(payload)), hashString(first), bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Offset != 4 {
		t.Fatalf("offset after first chunk = %d, want 4", metadata.Offset)
	}

	manager, err = NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err = manager.Get(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Offset != 4 {
		t.Fatalf("restored offset = %d, want 4", metadata.Offset)
	}

	if _, err = manager.WriteChunk(metadata.ID, 8, 9, int64(len(payload)), hashString(payload[8:]), bytes.NewReader(payload[8:])); !errors.Is(err, ErrOffset) {
		t.Fatalf("out-of-order chunk error = %v, want ErrOffset", err)
	}
	metadata, err = manager.WriteChunk(metadata.ID, 4, 7, int64(len(payload)), hashString(payload[4:8]), bytes.NewReader(payload[4:8]))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err = manager.WriteChunk(metadata.ID, 8, 9, int64(len(payload)), hashString(payload[8:]), bytes.NewReader(payload[8:]))
	if err != nil {
		t.Fatal(err)
	}

	completed, err := manager.Complete(metadata.ID, fullHash)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Offset != int64(len(payload)) || completed.Checksum != fullHash {
		t.Fatalf("completion metadata = %#v", completed)
	}
	stored, err := os.ReadFile(filepath.Join(root, "nested", "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored data = %q, want %q", stored, payload)
	}
}

func TestBadChunkChecksumDoesNotAdvanceOffset(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), ChunkSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create(CreateRequest{Path: "file.bin", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.WriteChunk(metadata.ID, 0, 3, 4, hashString([]byte("nope")), bytes.NewReader([]byte("data"))); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad checksum error = %v, want ErrChecksum", err)
	}
	metadata, err = manager.Get(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Offset != 0 {
		t.Fatalf("offset after rejected chunk = %d, want 0", metadata.Offset)
	}
}

func TestCreateRejectsInvalidPathAndOversizedFile(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), MaxFileSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Create(CreateRequest{Path: "../outside.bin", Size: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("path traversal error = %v, want ErrInvalid", err)
	}
	for _, invalidPath := range []string{"C:/outside.bin", ".UPDOWNFILE/metadata.json", "file:stream", "CON.txt", "name."} {
		if _, err = manager.Create(CreateRequest{Path: invalidPath, Size: 1}); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid path %q error = %v, want ErrInvalid", invalidPath, err)
		}
	}
	if _, err = manager.Create(CreateRequest{Path: "file.bin", Size: 11}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized file error = %v, want ErrTooLarge", err)
	}
}

func TestCompletedUploadCanBeRetriedAfterRestart(t *testing.T) {
	root := t.TempDir()
	config := Config{Root: root, ChunkSize: 64}
	manager, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("recover completion response")
	metadata, err := manager.Create(CreateRequest{Path: "complete.bin", Size: int64(len(payload)), ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err = manager.WriteChunk(metadata.ID, 0, int64(len(payload))-1, int64(len(payload)), hashString(payload), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := manager.Complete(metadata.ID, hashString(payload))
	if err != nil || !completed.Completed {
		t.Fatalf("first completion = %#v, %v", completed, err)
	}

	manager, err = NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	status, err := manager.Get(metadata.ID)
	if err != nil || !status.Completed {
		t.Fatalf("restored status = %#v, %v", status, err)
	}
	completed, err = manager.Complete(metadata.ID, hashString(payload))
	if err != nil || !completed.Completed || completed.Checksum != hashString(payload) {
		t.Fatalf("repeated completion = %#v, %v", completed, err)
	}
}

func TestWriteChunkRejectsExtraBytesWithoutAdvancingOffset(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), ChunkSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create(CreateRequest{Path: "file.bin", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.WriteChunk(metadata.ID, 0, 3, 4, hashString([]byte("data")), bytes.NewReader([]byte("data!"))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("extra chunk bytes error = %v, want ErrInvalid", err)
	}
	metadata, err = manager.Get(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Offset != 0 {
		t.Fatalf("offset after rejected oversized chunk = %d, want 0", metadata.Offset)
	}
}

func TestChecksumFailureResetsSessionForRetry(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), ChunkSize: 16})
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("correct data")
	wrong := []byte("wrong data!!")
	metadata, err := manager.Create(CreateRequest{Path: "file.bin", Size: int64(len(expected)), ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err = manager.WriteChunk(metadata.ID, 0, int64(len(wrong))-1, int64(len(expected)), hashString(wrong), bytes.NewReader(wrong))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Complete(metadata.ID, hashString(expected)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("completion checksum error = %v, want ErrChecksum", err)
	}
	metadata, err = manager.Get(metadata.ID)
	if err != nil || metadata.Offset != 0 {
		t.Fatalf("session after checksum reset = %#v, %v", metadata, err)
	}
	if !metadata.Failed {
		t.Fatal("missing failed terminal state")
	}
	if _, err = manager.WriteChunk(metadata.ID, 0, int64(len(expected))-1, int64(len(expected)), hashString(expected), bytes.NewReader(expected)); !errors.Is(err, ErrConflict) {
		t.Fatal("failed task accepted more data")
	}
	metadata, err = manager.Create(CreateRequest{Path: "file.bin", Size: int64(len(expected)), ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err = manager.WriteChunk(metadata.ID, 0, int64(len(expected))-1, int64(len(expected)), hashString(expected), bytes.NewReader(expected))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Complete(metadata.ID, hashString(expected)); err != nil {
		t.Fatalf("completion after retry: %v", err)
	}
}

func TestExpiredSessionIsRemoved(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), SessionTTL: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create(CreateRequest{Path: "file.bin", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err = manager.Get(metadata.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session lookup error = %v, want ErrNotFound", err)
	}
}

func TestCreateIsIdempotentAcrossRetriesAndRestart(t *testing.T) {
	root := t.TempDir()
	config := Config{Root: root}
	manager, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRequest{Path: "retry.bin", Size: 12, ChecksumAlgorithm: digest.AlgorithmSHA256, IdempotencyKey: strings.Repeat("a", 64)}
	first, err := manager.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := manager.Create(request)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("repeated create = %#v, %v; want session %s", retry, err, first.ID)
	}
	conflict := request
	conflict.Size++
	if _, err = manager.Create(conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting idempotency key error = %v, want ErrConflict", err)
	}

	manager, err = NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	retry, err = manager.Create(request)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("create after restart = %#v, %v; want session %s", retry, err, first.ID)
	}
}

func hashString(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
