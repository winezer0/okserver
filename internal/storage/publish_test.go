package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishCopiedFile(t *testing.T) {
	payload := bytes.Repeat([]byte("verified upload data"), 7000)

	t.Run("publishes without replacing", func(t *testing.T) {
		source, target := publishTestPaths(t)
		if err := os.WriteFile(source, payload, 0o640); err != nil {
			t.Fatal(err)
		}

		if err := publishCopiedFile(source, target); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staging file still exists: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("published file content differs from staging file")
		}
	})

	t.Run("recovers a matching destination", func(t *testing.T) {
		source, target := publishTestPaths(t)
		for _, name := range []string{source, target} {
			if err := os.WriteFile(name, payload, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		if err := publishCopiedFile(source, target); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staging file still exists: %v", err)
		}
	})

	t.Run("preserves a conflicting destination", func(t *testing.T) {
		source, target := publishTestPaths(t)
		conflict := append([]byte(nil), payload...)
		conflict[len(conflict)-1] ^= 0xff
		if err := os.WriteFile(source, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, conflict, 0o600); err != nil {
			t.Fatal(err)
		}

		if err := publishCopiedFile(source, target); !errors.Is(err, os.ErrExist) {
			t.Fatalf("expected destination conflict, got %v", err)
		}
		gotSource, err := os.ReadFile(source)
		if err != nil || !bytes.Equal(gotSource, payload) {
			t.Fatalf("staging file changed: err=%v", err)
		}
		gotTarget, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(gotTarget, conflict) {
			t.Fatalf("existing destination changed: err=%v", err)
		}
	})
}

func publishTestPaths(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	destination := filepath.Join(root, "files")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(state, "payload.part"), filepath.Join(destination, "payload.bin")
}
