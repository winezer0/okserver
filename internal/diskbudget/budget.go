// Package diskbudget coordinates remaining upload space across server processes.
package diskbudget

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/winezer0/okserver/internal/storage"
)

var ErrBusy = errors.New("disk budget is busy; retry later")

type Reservation struct {
	name    string
	release func()
}

func gate(stateDir string) (string, func(), error) {
	directory := filepath.Join(stateDir, "budget")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", nil, err
	}
	release, err := storage.TryFileLock(filepath.Join(directory, "admission.lock"))
	if err != nil {
		return "", nil, ErrBusy
	}
	return directory, release, nil
}
func add(total *uint64, value uint64) bool {
	if value > ^uint64(0)-*total {
		return false
	}
	*total += value
	return true
}
func check(cacheDir, stateDir, directory string, bytes, minimum uint64) error {
	free, err := storage.FreeSpace(cacheDir)
	if err != nil {
		return err
	}
	requested := minimum
	if !add(&requested, bytes) {
		return storage.ErrInsufficientSpace
	}
	// Existing destination files and already written prefixes are already reflected in free space.
	sessionDir := filepath.Join(stateDir, "uploads")
	sessions, _ := os.ReadDir(sessionDir)
	for _, session := range sessions {
		if !session.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionDir, session.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		var state struct {
			Size, Offset     int64
			Completed        bool
			Canceled, Failed bool
		}
		if json.Unmarshal(data, &state) == nil && !state.Completed && !state.Canceled && !state.Failed && state.Size > state.Offset {
			if !add(&requested, uint64(state.Size-state.Offset)) {
				return storage.ErrInsufficientSpace
			}
		}
	}
	records, _ := os.ReadDir(directory)
	for _, record := range records {
		if filepath.Ext(record.Name()) != ".json" {
			continue
		}
		name := filepath.Join(directory, record.Name())
		release, lockErr := storage.TryFileLock(name + ".lock")
		if lockErr == nil {
			_ = os.Remove(name)
			release()
			_ = os.Remove(name + ".lock")
			continue
		}
		data, readErr := os.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		var size uint64
		if json.Unmarshal(data, &size) != nil {
			return storage.ErrInsufficientSpace
		}
		if !add(&requested, size) {
			return storage.ErrInsufficientSpace
		}
	}
	if requested > free {
		return storage.ErrInsufficientSpace
	}
	return nil
}

// Admit holds the global admission lock until the caller persists the new session.
func Admit(cacheDir, stateDir string, bytes, minimum uint64) (func(), error) {
	directory, release, err := gate(stateDir)
	if err != nil {
		return nil, err
	}
	if err = check(cacheDir, stateDir, directory, bytes, minimum); err != nil {
		release()
		return nil, err
	}
	return release, nil
}
func Reserve(cacheDir, stateDir string, bytes, minimum uint64) (*Reservation, error) {
	directory, release, err := gate(stateDir)
	if err != nil {
		return nil, err
	}
	defer release()
	if err = check(cacheDir, stateDir, directory, bytes, minimum); err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, err
	}
	name := filepath.Join(directory, hex.EncodeToString(random[:])+".json")
	owner, err := storage.TryFileLock(name + ".lock")
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(bytes)
	if err = os.WriteFile(name, data, 0600); err != nil {
		owner()
		return nil, err
	}
	return &Reservation{name: name, release: owner}, nil
}
func (reservation *Reservation) Close() {
	_ = os.Remove(reservation.name)
	reservation.release()
	_ = os.Remove(reservation.name + ".lock")
}
