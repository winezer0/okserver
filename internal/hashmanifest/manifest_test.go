package hashmanifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/winezer0/okserver/internal/digest"
)

func TestStorePersistsAndReplacesEntries(t *testing.T) {
	root := t.TempDir()
	first := Entry{
		Path: "nested/file.bin", Size: 3, Fingerprint: "fingerprint-1",
		Hashes: digest.Hashes{
			SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
	}
	store := New(root)
	if err := store.Put(first); err != nil {
		t.Fatal(err)
	}
	reloaded := New(root)
	got, found, err := reloaded.Lookup(first.Path, first.Fingerprint)
	if err != nil || !found || got.Hashes != first.Hashes {
		t.Fatalf("lookup after reload = %+v, found=%v, err=%v", got, found, err)
	}
	if _, found, err = reloaded.Lookup(first.Path, "stale-fingerprint"); err != nil || found {
		t.Fatalf("stale entry accepted: found=%v, err=%v", found, err)
	}

	second := first
	second.Fingerprint = "fingerprint-2"
	file, err := os.CreateTemp(root, "replacement-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString("xyz"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	second.Hashes, err = digest.OpenFileHashes(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = reloaded.Put(second); err != nil {
		t.Fatal(err)
	}
	got, found, err = store.Lookup(second.Path, second.Fingerprint)
	if err != nil || !found || got.Hashes != second.Hashes {
		t.Fatalf("updated lookup = %+v, found=%v, err=%v", got, found, err)
	}
	if _, found, err = store.Lookup(first.Path, first.Fingerprint); err != nil || found {
		t.Fatalf("replaced fingerprint remained valid: found=%v, err=%v", found, err)
	}
	if _, err = os.Stat(filepath.Join(root, "file-hashes.json")); err != nil {
		t.Fatal("manifest file was not persisted:", err)
	}
}
