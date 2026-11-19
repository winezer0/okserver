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
	ChunkDir        string
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
	sessionDir      string
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

	sessionDir := config.ChunkDir
	if sessionDir == "" {
		sessionDir = filepath.Join(root, ".okserver", "uploads")
	}
	sessionDir, err = filepath.Abs(sessionDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	sessionDir, err = filepath.EvalSymlinks(sessionDir)
	if err != nil {
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
		sessionDir:      sessionDir,
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

func (m *Manager) ChunkSize() int64 {
	return m.chunkSize
}
