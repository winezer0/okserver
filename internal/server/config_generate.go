package server

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// generateConfig uses the runtime defaults as its single source of truth.
func generateConfig(name string) (err error) {
	var data bytes.Buffer
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2)
	if err := encoder.Encode(defaultConfig()); err != nil {
		return fmt.Errorf("encode default configuration: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("encode default configuration: %w", err)
	}
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create configuration %q (existing files are never overwritten): %w", name, err)
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err = file.Write(data.Bytes()); err == nil {
		err = file.Sync()
	}
	if err == nil {
		err = file.Close()
	}
	if err != nil {
		return fmt.Errorf("write configuration %q: %w", name, err)
	}
	return nil
}
