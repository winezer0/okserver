package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCachePrunePreservesActiveAndUserFiles(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target+".part", []byte("user"), 0600)
	partial, state, release, err := prepareDownload(target)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(downloadState{URL: "http://user:secret@example.com/file?token=secret"})
	os.WriteFile(state, data, 0600)
	os.WriteFile(partial, []byte("partial"), 0600)
	old := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(partial, old, old)
	tasks, err := ListDownloadCache(filepath.Dir(partial))
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks: %+v %v", tasks, err)
	}
	if strings.Contains(tasks[0].URL, "secret") {
		t.Fatal("cache list exposed credentials")
	}
	if err = PruneDownloadCache(filepath.Dir(partial), 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(partial); err != nil {
		t.Fatal("active task pruned")
	}
	release()
	if err = PruneDownloadCache(filepath.Dir(partial), 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("expired task retained")
	}
	user, _ := os.ReadFile(target + ".part")
	if string(user) != "user" {
		t.Fatal("user file changed")
	}
}

func TestCachePruneOrphanState(t *testing.T) {
	partial, state, err := downloadPaths(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(state, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err = os.Chtimes(state, old, old); err != nil {
		t.Fatal(err)
	}
	if err = PruneDownloadCache(filepath.Dir(partial), 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("orphan state retained")
	}
}
