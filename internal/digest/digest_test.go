package digest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"crypto/sha256"
)

func TestDigestInvalidatesSameSizeSameTimeContent(t *testing.T) {
	name := filepath.Join(t.TempDir(), "file.bin")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(content string) {
		t.Helper()
		got, err := FileDigest(name)
		if want := fmt.Sprintf("%x", sha256.Sum256([]byte(content))); err != nil || got != want {
			t.Fatalf("digest = %q, %v; want %s", got, err, want)
		}
	}
	write("original")
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	check("original")
	check("original") // Exercise the cache before changing the same inode.
	write("modified")
	if err := os.Chtimes(name, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	check("modified")
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	write("replaced")
	if err := os.Chtimes(name, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	check("replaced")
}

func TestPublishedDigestAndConcurrentReaders(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "staging"), filepath.Join(root, "published")
	if err := os.WriteFile(source, []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	want, err := FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, target); err != nil {
		t.Fatal(err)
	}
	RememberPublishedDigest(source, target)
	var readers sync.WaitGroup
	for range 16 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			if got, err := FileDigest(target); err != nil || got != want {
				t.Errorf("published digest = %q, %v", got, err)
			}
		}()
	}
	readers.Wait()
	if got, err := FileDigest(source); err != nil || got != "missing" {
		t.Fatalf("missing digest = %q, %v", got, err)
	}
	closed, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if _, err := OpenFileDigest(closed); err == nil {
		t.Fatal("closed file accepted")
	}
}
