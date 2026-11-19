package server

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/winezer0/okserver/internal/hashmanifest"
	"github.com/winezer0/okserver/internal/observability"
	"github.com/winezer0/okserver/internal/upload"
)

type fileServer struct {
	allowedOrigins      []string
	routerOnce          sync.Once
	router              http.Handler
	eventClients        atomic.Int64
	metrics             observability.Metrics
	minFreeBytes        uint64
	uploadSlots         chan struct{}
	downloadSlots       chan struct{}
	path                string
	auth                string
	param               string
	disableFrontUI      bool
	allowCreateDir      bool
	allowDeleteFile     bool
	logger              observability.RequestLogger
	uploads             *upload.Manager
	hashes              *hashmanifest.Store
	uploadIdleTimeout   time.Duration
	verificationQueue   chan upload.Metadata
	verificationPending sync.Map
	verificationCancel  sync.Map
}

const (
	schemeHttp  = "http"
	schemeHttps = "https"
)
