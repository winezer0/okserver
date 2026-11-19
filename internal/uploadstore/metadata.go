// Package uploadstore owns the durable JSON format, independently of upload rules.
package uploadstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/winezer0/okserver/internal/storage"
)

const Version = 1
const maxMetadataBytes = 64 << 10

var ErrVersion = errors.New("unsupported upload metadata version")

// Read accepts only the current explicit session format.
func Read[T any](directory string) (T, error) {
	var value T
	file, err := os.Open(filepath.Join(directory, "metadata.json"))
	if err != nil {
		return value, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMetadataBytes+1))
	if err != nil {
		return value, err
	}
	if len(data) > maxMetadataBytes {
		return value, fmt.Errorf("upload metadata exceeds size limit")
	}
	var header *struct {
		Version int `json:"version"`
	}
	if err = json.Unmarshal(data, &header); err != nil {
		return value, err
	}
	if header == nil {
		return value, fmt.Errorf("upload metadata must be an object")
	}
	if header.Version != Version {
		return value, ErrVersion
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

// Write requires the caller's session lock. Data is synced before atomic replace.
func Write(directory string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("upload metadata must be an object")
	}
	fields["version"] = json.RawMessage("1")
	data, err = json.Marshal(fields)
	if err != nil {
		return err
	}
	if len(data) > maxMetadataBytes {
		return fmt.Errorf("upload metadata exceeds size limit")
	}
	temporary := filepath.Join(directory, "metadata.tmp")
	defer os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err = errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return storage.DurableReplace(temporary, filepath.Join(directory, "metadata.json"))
}
