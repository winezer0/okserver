package upload

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/storage"
)

func TestRecoverFinalizingTransaction(t *testing.T) {
	for _, alreadyRenamed := range []bool{false, true} {
		root := t.TempDir()
		manager, err := NewManager(Config{Root: root, ChunkSize: 4})
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte("test")
		metadata, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = manager.WriteChunk(metadata.ID, 0, 3, 4, hashString(payload), bytes.NewReader(payload)); err != nil {
			t.Fatal(err)
		}
		session, err := manager.lookup(metadata.ID)
		if err != nil {
			t.Fatal(err)
		}
		part, err := os.Open(manager.payloadPath(session.metadata.ID))
		if err != nil {
			t.Fatal(err)
		}
		hashes, err := digest.OpenFileHashes(part)
		if closeErr := part.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = session.metadata.beginFinalizing(hashString(payload), hashes, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err = manager.persist(session); err != nil {
			t.Fatal(err)
		}
		if alreadyRenamed {
			if err = storage.DurableReplace(manager.payloadPath(session.metadata.ID), filepath.Join(root, "file")); err != nil {
				t.Fatal(err)
			}
		}
		manager, err = NewManager(Config{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := manager.Get(metadata.ID)
		if err != nil || !recovered.Completed {
			t.Fatalf("recovery failed: %+v %v", recovered, err)
		}
		got, _ := os.ReadFile(filepath.Join(root, "file"))
		if !bytes.Equal(got, payload) {
			t.Fatal("recovered file mismatch")
		}
	}
}
func TestMetadataFailureDoesNotAcknowledgeChunk(t *testing.T) {
	manager, err := NewManager(Config{Root: t.TempDir(), ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.lookup(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	os.Mkdir(filepath.Join(session.dir, "metadata.tmp"), 0700)
	if _, err = manager.WriteChunk(metadata.ID, 0, 3, 4, hashString([]byte("test")), bytes.NewBufferString("test")); err == nil {
		t.Fatal("failed metadata write acknowledged")
	}
	info, _ := os.Stat(manager.payloadPath(session.metadata.ID))
	if info.Size() != 0 || session.metadata.Offset != 0 {
		t.Fatal("failed chunk not rolled back")
	}
}
func TestUploadRejectsPrivateDirectoryAlias(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(root, ".okserver"), filepath.Join(root, "alias")); err != nil {
		t.Skip(err)
	}
	if _, err = manager.Create(CreateRequest{Path: "alias/overwrite", Size: 1, ChecksumAlgorithm: digest.AlgorithmSHA256}); err == nil {
		t.Fatal("private alias accepted")
	}
}
func FuzzCleanRelativePath(f *testing.F) {
	for _, value := range []string{"file", "../escape", ".okserver/uploads", "alias/../file", "C:/file", "nested/file", "a\x00b"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		clean, err := CleanRelativePath(value)
		if err == nil {
			again, recheck := CleanRelativePath(clean)
			if recheck != nil || again != clean {
				t.Fatalf("path normalization is unstable: %q", clean)
			}
		}
	})
}
