package server

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/winezer0/okserver/internal/buildinfo"
	"github.com/winezer0/okserver/internal/hashmanifest"
	"github.com/winezer0/okserver/internal/observability"
	"github.com/winezer0/okserver/internal/upload"
)

func Run(exe string, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return RunContext(ctx, exe, args)
}

// RunContext owns all server resources until requests and maintenance have stopped.
func RunContext(parent context.Context, exe string, args []string) (runErr error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	fs := flag.NewFlagSet(exe, flag.ContinueOnError)
	cnf := fs.String("c", "server.yaml", "config file")
	generate := fs.Bool("g", false, "generate default configuration and exit (never overwrite)")
	path := fs.String("p", "", "path")
	listen := fs.String("s", "", "ip:port")
	err := fs.Parse(args)
	if err != nil {
		return err
	}
	if *generate {
		if err := generateConfig(*cnf); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Configuration written to %s\n", *cnf)
		return nil
	}

	explicitConfig := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "c" {
			explicitConfig = true
		}
	})
	config, err := loadConfig(*cnf, explicitConfig)
	if err != nil {
		return err
	}
	if *path != "" {
		config.Server.Path = *path
	}
	if *listen != "" {
		config.Server.Listen = *listen
	}

	headerTimeout, _ := durationSetting("server.read_header_timeout", config.Server.ReadHeaderTimeout, 10*time.Second)
	uploadIdleTimeout, _ := durationSetting("upload.idle_timeout", config.Upload.IdleTimeout, 2*time.Minute)
	uploadSessionTTL, _ := durationSetting("upload.session_ttl", config.Upload.SessionTTL, 7*24*time.Hour)
	terminalTTL, _ := durationSetting("upload.terminal_ttl", config.Upload.TerminalTTL, 7*24*time.Hour)
	logger, err := observability.NewLogger(config.Log)
	if err != nil {
		return fmt.Errorf("initialize logging: %w", err)
	}
	defer logger.Close()
	defer func() {
		if runErr != nil {
			logger.Error("server.stopped", "error", runErr.Error())
		}
	}()
	tcpAddr, err := net.ResolveTCPAddr("tcp", config.Server.Listen)
	if err != nil {
		return err
	}

	uri := &url.URL{Scheme: schemeHttp}
	if config.Server.Certificate.Cert != "" || config.Server.Certificate.Key != "" {
		if config.Server.Certificate.Cert == "" {
			return errors.New("cert file is null")
		}
		if config.Server.Certificate.Key == "" {
			return errors.New("key file is null")
		}
		uri.Scheme = schemeHttps
	}

	addr, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	defer addr.Close()
	bound := addr.Addr().(*net.TCPAddr)
	tcpAddr = &net.TCPAddr{IP: append(net.IP(nil), bound.IP...), Port: bound.Port, Zone: bound.Zone}
	var urls []string
	if len(tcpAddr.IP) == 0 || tcpAddr.IP.IsUnspecified() {
		if ips := internalIP(); len(ips) > 0 {
			for _, v := range ips {
				tcpAddr.IP = v // 拼接本机所有ip:port
				uri.Host = tcpAddr.String()
				urls = append(urls, uri.String())
			}
		}

		tcpAddr.IP = net.IPv4(127, 0, 0, 1)
	}

	if config.Server.Certificate.Domain == "" {
		uri.Host = tcpAddr.IP.String()
	} else {
		uri.Host = config.Server.Certificate.Domain
	}

	if !((uri.Scheme == schemeHttp && tcpAddr.Port == 80) ||
		(uri.Scheme == schemeHttps && tcpAddr.Port == 443)) {
		// 不是特殊协议和端口,需要拼接端口,特殊协议不需要带上端口
		uri.Host = net.JoinHostPort(uri.Host, strconv.Itoa(tcpAddr.Port))
	}
	urls = append(urls, uri.String())

	addrStr := uri.String()

	config.Server.Path, err = filepath.Abs(config.Server.Path)
	fmt.Fprintf(os.Stderr, "Current Web Root Abs is %v\n", config.Server.Path)
	if err != nil {
		return err
	}

	uploads, err := upload.NewManager(upload.Config{
		MaxSessions:     config.Upload.MaxSessions,
		MaxPerOwner:     config.Upload.MaxPerIdentity,
		MaxPendingBytes: config.Upload.MaxPendingBytes,
		MinFreeBytes:    config.Upload.MinFreeDiskBytes,
		Root:            config.Server.Path,
		Observe:         observability.UploadObserver(logger),
		ChunkSize:       config.Upload.ChunkSize,
		MaxFileSize:     config.Upload.MaxFileSize,
		SessionTTL:      uploadSessionTTL,
		TerminalTTL:     terminalTTL,
	})
	if err != nil {
		return fmt.Errorf("initialize upload manager: %w", err)
	}

	err = printStartupHelp(os.Stderr, map[string]any{
		"exec":    exe,
		"listen":  addr.Addr().String(),
		"urls":    urls,
		"addr":    addrStr,
		"domain":  config.Server.Certificate.Domain,
		"example": "example.txt",
		"dir":     config.Server.Path,
		"config":  *cnf,
		"cert":    config.Server.Certificate.Cert,
		"key":     config.Server.Certificate.Key,
		"ca":      config.Server.Certificate.Ca,
		"auth":    displayAuth(config.Server.Auth),
	})
	if err != nil {
		return err
	}

	srh := &fileServer{
		allowedOrigins:    config.Server.AllowedOrigins,
		minFreeBytes:      config.Upload.MinFreeDiskBytes,
		path:              config.Server.Path,
		auth:              config.Server.Auth,
		param:             config.Server.Param,
		deny:              config.Server.Deny,
		logger:            logger,
		uploads:           uploads,
		hashes:            hashmanifest.New(config.Server.Path),
		uploadIdleTimeout: uploadIdleTimeout,
	}
	if config.Upload.MaxConcurrent > 0 {
		srh.uploadSlots = make(chan struct{}, config.Upload.MaxConcurrent)
	}
	if config.Upload.BufferBudget > 0 {
		slots := int(config.Upload.BufferBudget / (8 << 20))
		if srh.uploadSlots == nil || slots < cap(srh.uploadSlots) {
			srh.uploadSlots = make(chan struct{}, slots)
		}
	}
	if config.Download.MaxConcurrent > 0 {
		srh.downloadSlots = make(chan struct{}, config.Download.MaxConcurrent)
	}
	stopMaintenance := srh.startMaintenance(ctx)
	defer stopMaintenance()
	workerCtx, stopWorkers := context.WithCancel(ctx)
	waitWorkers := srh.startVerification(workerCtx, config.Upload.VerifyWorkers)
	defer func() { stopWorkers(); waitWorkers() }()

	srv := &http.Server{
		Addr:    addrStr,
		Handler: srh,

		ReadHeaderTimeout: headerTimeout,
		IdleTimeout:       2 * time.Minute,
	}

	logger.Info("server.started", "listen", addr.Addr().String(), "root", config.Server.Path, "tls", uri.Scheme == schemeHttps, "build", buildinfo.String())
	return serve(ctx, srv, addr, config.Server.Certificate.Cert, config.Server.Certificate.Key)
}
