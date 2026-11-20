package upload

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

const DefaultChunkSize int64 = 8 << 20

type Config struct {
	Observe         func(Event)
	MaxSessions     int
	MaxPerOwner     int
	MaxPendingBytes int64
	MinFreeBytes    uint64
	Root            string
	StateDir        string
	CacheDir        string
	ChunkSize       int64
	MaxFileSize     int64
	SessionTTL      time.Duration
	TerminalTTL     time.Duration
}

type Manager struct {
	observe         func(Event)
	maxSessions     int
	maxPerOwner     int
	maxPendingBytes int64
	minFreeBytes    uint64
	root            string
	stateDir        string
	cacheDir        string
	sessionDir      string
	cacheSessionDir string
	chunkSize       int64
	maxFileSize     int64
	mu              sync.RWMutex
	sessions        map[string]*session
	keys            map[string]string
	createMu        sync.Mutex
	cleanupMu       sync.Mutex
	lastCleanup     time.Time
	sessionTTL      time.Duration
	terminalTTL     time.Duration
	indexMu         sync.Mutex
	index           map[string]Metadata
	paths           map[string]string
}

func NewManager(config Config) (*Manager, error) {
	if config.MaxPerOwner < 0 || config.MaxSessions < 0 || config.MaxPendingBytes < 0 || config.MaxFileSize < 0 || config.ChunkSize < 0 || config.ChunkSize == 1<<63-1 {
		return nil, ErrInvalid
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	stateDir := config.StateDir
	if stateDir == "" {
		stateDir = root + ".states"
	}
	stateDir, err = filepath.Abs(stateDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	cacheDir := config.CacheDir
	if cacheDir == "" {
		cacheDir = root + ".cache"
	}
	cacheDir, err = filepath.Abs(cacheDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	stateDir, err = filepath.EvalSymlinks(stateDir)
	if err != nil {
		return nil, err
	}
	cacheDir, err = filepath.EvalSymlinks(cacheDir)
	if err != nil {
		return nil, err
	}
	sessionDir := filepath.Join(stateDir, "uploads")
	cacheSessionDir := filepath.Join(cacheDir, "uploads")
	if err = os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(cacheSessionDir, 0o700); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(stateDir, "locks"), 0o700); err != nil {
		return nil, err
	}
	chunkSize := config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	sessionTTL := config.SessionTTL
	if sessionTTL == 0 {
		sessionTTL = 7 * 24 * time.Hour
	}
	manager := &Manager{
		terminalTTL:     config.TerminalTTL,
		observe:         config.Observe,
		maxSessions:     config.MaxSessions,
		maxPerOwner:     config.MaxPerOwner,
		maxPendingBytes: config.MaxPendingBytes,
		minFreeBytes:    config.MinFreeBytes,
		root:            root,
		stateDir:        stateDir,
		cacheDir:        cacheDir,
		sessionDir:      sessionDir,
		cacheSessionDir: cacheSessionDir,
		chunkSize:       chunkSize,
		maxFileSize:     config.MaxFileSize,
		sessions:        make(map[string]*session),
		keys:            make(map[string]string),
		index:           make(map[string]Metadata),
		paths:           make(map[string]string),
		sessionTTL:      sessionTTL,
	}
	if manager.terminalTTL == 0 {
		manager.terminalTTL = sessionTTL
	}
	manager.cleanupOrphans()
	if err = manager.loadSessions(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) StateDir() string {
	return m.stateDir
}

func (m *Manager) CacheDir() string {
	return m.cacheDir
}

func (m *Manager) payloadPath(id string) string {
	return filepath.Join(m.cacheSessionDir, id, "payload.part")
}

func (m *Manager) payloadSessionDir(id string) string {
	return filepath.Join(m.cacheSessionDir, id)
}

func (m *Manager) ChunkSize() int64 {
	return m.chunkSize
}
