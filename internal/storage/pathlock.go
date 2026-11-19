package storage

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type pathLock struct {
	mu    sync.Mutex
	users int
}

var pathLocks = struct {
	sync.Mutex
	entries map[string]*pathLock
}{entries: make(map[string]*pathLock)}

// LockPath coordinates commits to the same destination. Complete replacements
// use last successful commit wins; append operations read their prefix while locked.
func LockPath(name string) func() {
	name, _ = filepath.Abs(name)
	if parent, err := filepath.EvalSymlinks(filepath.Dir(name)); err == nil {
		name = filepath.Join(parent, filepath.Base(name))
	}
	name = filepath.Clean(name)
	if runtime.GOOS == "windows" {
		name = strings.ToLower(name)
	}
	pathLocks.Lock()
	entry := pathLocks.entries[name]
	if entry == nil {
		entry = &pathLock{}
		pathLocks.entries[name] = entry
	}
	entry.users++
	pathLocks.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		pathLocks.Lock()
		entry.users--
		if entry.users == 0 {
			delete(pathLocks.entries, name)
		}
		pathLocks.Unlock()
	}
}
