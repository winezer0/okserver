package digest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFileHashesKnownValues(t *testing.T) {
	name := filepath.Join(t.TempDir(), "abc.txt")
	if err := os.WriteFile(name, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	want := Hashes{
		SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	if got, err := OpenFileDigest(file); err != nil || got != want.SHA256 {
		t.Fatalf("SHA-256 digest = %q, %v; want %q", got, err, want.SHA256)
	}
	hashes, err := OpenFileHashes(file)
	if err != nil {
		t.Fatal(err)
	}
	if hashes != want {
		t.Fatalf("hashes = %+v, want %+v", hashes, want)
	}
	if !hashes.Valid() {
		t.Fatal("calculated hashes failed validation")
	}
}

func TestFingerprintDetectsSameSizeSameTimeModification(t *testing.T) {
	name := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(name, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	stamp := info.ModTime()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	first, reliable, err := Fingerprint(file)
	file.Close()
	if err != nil || !reliable {
		t.Fatalf("initial fingerprint = %q, reliable=%v, err=%v", first, reliable, err)
	}
	if err = os.WriteFile(name, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(name, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	second, reliable, err := Fingerprint(file)
	if err != nil || !reliable {
		t.Fatalf("updated fingerprint = %q, reliable=%v, err=%v", second, reliable, err)
	}
	if first == second {
		t.Fatal("same-size content change with restored mtime kept the fingerprint")
	}
}
