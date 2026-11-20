package upload

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/winezer0/okserver/internal/digest"
)

func TestUnversionedSessionsAreNotRestored(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.Create(CreateRequest{Path: "file", Size: 4, ChecksumAlgorithm: digest.AlgorithmSHA256})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(manager.sessionDir, task.ID, "metadata.json")
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]json.RawMessage
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	delete(record, "version")
	data, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := NewManager(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Tasks()) != 0 {
		t.Fatal("unversioned session restored")
	}
	if _, err = restored.Get(task.ID); err == nil {
		t.Fatal("unversioned session accessible")
	}
	if _, err = os.Stat(name); err != nil {
		t.Fatal("unsupported session was deleted", err)
	}
}
