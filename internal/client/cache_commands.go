package client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CacheTask struct {
	ID      string    `json:"id"`
	URL     string    `json:"url"`
	Bytes   int64     `json:"bytes"`
	Updated time.Time `json:"updated"`
}

func validCacheID(id string) bool {
	data, err := hex.DecodeString(id)
	return err == nil && len(data) == 32 && id == strings.ToLower(id)
}

func ListDownloadCache(directory string) ([]CacheTask, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []CacheTask{}, nil
	}
	if err != nil {
		return nil, err
	}
	tasks := make([]CacheTask, 0)
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".ranges")
		id := strings.TrimSuffix(name, ".json")
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || !validCacheID(id) {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			continue
		}
		var state downloadState
		if json.Unmarshal(data, &state) != nil {
			continue
		}
		partial, statErr := os.Stat(filepath.Join(directory, id+".part"))
		if statErr != nil {
			continue
		}
		remote, parseErr := url.Parse(state.URL)
		address := ""
		if parseErr == nil {
			remote.RawQuery = ""
			remote.User = nil
			address = remote.String()
		}
		tasks = append(tasks, CacheTask{ID: id, URL: address, Bytes: partial.Size(), Updated: partial.ModTime()})
	}
	return tasks, nil
}

func CancelCachedDownload(directory, id string) error {
	if !validCacheID(id) {
		return fmt.Errorf("invalid cache task ID")
	}
	if info, err := os.Lstat(directory); err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid download staging directory")
	}
	if info, err := os.Lstat(filepath.Join(directory, id+".part.lock")); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("invalid download lock file")
	}
	// Use the same OS lock as an active download.
	lock, err := os.OpenFile(filepath.Join(directory, id+".part.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = lockDownload(lock); err != nil {
		return fmt.Errorf("download is active: %w", err)
	}
	for _, suffix := range []string{".part", ".json", ".json.ranges"} {
		if err = os.Remove(filepath.Join(directory, id+suffix)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func PruneDownloadCache(directory string, age time.Duration) error {
	tasks, err := ListDownloadCache(directory)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if time.Since(task.Updated) > age {
			_ = CancelCachedDownload(directory, task.ID)
		}
	}
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		suffix := filepath.Ext(entry.Name())
		if strings.HasSuffix(entry.Name(), ".json.ranges") {
			suffix = ".json.ranges"
		}
		if (suffix == ".part" || suffix == ".json" || suffix == ".json.ranges") && validCacheID(strings.TrimSuffix(entry.Name(), suffix)) {
			if suffix != ".part" {
				if _, err := os.Stat(filepath.Join(directory, strings.TrimSuffix(entry.Name(), suffix)+".part")); !os.IsNotExist(err) {
					continue
				}
			}
			if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > age {
				_ = CancelCachedDownload(directory, strings.TrimSuffix(entry.Name(), suffix))
			}
			continue
		}
		if !strings.HasPrefix(entry.Name(), "state-") {
			continue
		}
		if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > age {
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
	return nil
}
