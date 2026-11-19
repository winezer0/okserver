package uploadstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataVersionAndAtomicReplacement(t *testing.T) {
	directory := t.TempDir()
	name := filepath.Join(directory, "metadata.json")
	if err := os.WriteFile(name, []byte(`{"version":1,"id":"current","offset":4}`), 0600); err != nil {
		t.Fatal(err)
	}
	type record struct {
		ID     string `json:"id"`
		Offset int64  `json:"offset"`
	}
	old, err := Read[record](directory)
	if err != nil || old.ID != "current" || old.Offset != 4 {
		t.Fatalf("current record: %+v %v", old, err)
	}
	if err = Write(directory, record{ID: "current", Offset: 8}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Version int
		Offset  int64
	}
	if err = json.Unmarshal(data, &saved); err != nil || saved.Version != Version || saved.Offset != 8 {
		t.Fatalf("saved record: %s %v", data, err)
	}
	if _, err = os.Stat(filepath.Join(directory, "metadata.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains: %v", err)
	}
	// Serialization failure must not replace the last committed record.
	if err = Write(directory, make(chan int)); err == nil {
		t.Fatal("invalid value accepted")
	}
	current, err := Read[record](directory)
	if err != nil || current.Offset != 8 {
		t.Fatalf("committed record changed: %+v %v", current, err)
	}
}

func TestRejectUnsupportedCorruptAndOversizedMetadata(t *testing.T) {
	for _, input := range []string{`{}`, `{"version":0}`, `null`, `{"version":2}`, `{"version":-1}`, `{"version":"1"}`, `broken`, strings.Repeat("x", maxMetadataBytes+1)} {
		t.Run(input[:min(len(input), 32)], func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "metadata.json"), []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Read[map[string]any](directory); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "metadata.json"), []byte(`{"version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read[map[string]any](directory); !errors.Is(err, ErrVersion) {
		t.Fatalf("version error: %v", err)
	}
}
