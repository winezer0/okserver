package server

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestGenerateDefaultConfigurationAndExit(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := Run("okserver", []string{"-g"}); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig("server.yaml", true)
	if err != nil || !reflect.DeepEqual(config, defaultConfig()) {
		t.Fatalf("generated configuration does not match runtime defaults: %+v %v", config, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "server.yaml" {
		t.Fatalf("generation started a service or created unrelated files: %v %v", entries, err)
	}
}

func TestGenerateCustomConfigurationNeverOverwrites(t *testing.T) {
	name := filepath.Join(t.TempDir(), "custom config.yaml")
	if err := Run("okserver", []string{"-g", "-c", name}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run("okserver", []string{"-g", "-c", name}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing configuration must be rejected: %v", err)
	}
	after, err := os.ReadFile(name)
	if err != nil || string(after) != string(before) {
		t.Fatal("existing configuration changed")
	}
	if err := generateConfig(filepath.Join(name, "invalid.yaml")); err == nil {
		t.Fatal("invalid destination accepted")
	}
}

func TestConcurrentConfigurationGeneration(t *testing.T) {
	name := filepath.Join(t.TempDir(), "server.yaml")
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); results <- generateConfig(name) }()
	}
	workers.Wait()
	close(results)
	created := 0
	for err := range results {
		if err == nil {
			created++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if created != 1 {
		t.Fatalf("created %d configurations, want 1", created)
	}
	if _, err := loadConfig(name, true); err != nil {
		t.Fatal(err)
	}
}
