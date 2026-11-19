package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDurableReplacePreservesOriginalOnMissingSource(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	os.WriteFile(target, []byte("original"), 0600)
	if err := DurableReplace(filepath.Join(root, "absent"), target); err == nil {
		t.Fatal("missing source accepted")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original" {
		t.Fatal("original changed")
	}
	source := filepath.Join(root, "new")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	file.Write([]byte("replacement"))
	file.Sync()
	file.Close()
	if err = DurableReplace(source, target); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(target)
	if string(got) != "replacement" {
		t.Fatal("commit failed")
	}
}
