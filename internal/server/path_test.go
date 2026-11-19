package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathRecreatesMissingUploadRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uploads")
	fs := &fileServer{path: root}
	if relative, target, err := fs.resolvePath("", true); err != nil || relative != "" || target != root {
		t.Fatalf("resolve root = (%q, %q, %v)", relative, target, err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("upload root was not recreated: info=%v err=%v", info, err)
	}
}

func TestResolvePathRecreatesMissingRootForDestination(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uploads")
	fs := &fileServer{path: root}
	relative, target, err := fs.resolvePath("file.bin", false)
	if err != nil || relative != "file.bin" || target != filepath.Join(root, "file.bin") {
		t.Fatalf("resolve destination = (%q, %q, %v)", relative, target, err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("upload root was not recreated: info=%v err=%v", info, err)
	}
}
