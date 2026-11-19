package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

const shutdownTimeout = 30 * time.Second

// serve joins shutdown so logging and storage resources outlive active requests.
func serve(ctx context.Context, server *http.Server, listener net.Listener, cert, key string) error {
	requests, cancel := context.WithCancel(ctx)
	defer cancel()
	server.BaseContext = func(net.Listener) context.Context { return requests }
	finished := make(chan struct{})
	shutdown := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancel()
			deadline, stop := context.WithTimeout(context.Background(), shutdownTimeout)
			defer stop()
			err := server.Shutdown(deadline)
			if err != nil {
				err = errors.Join(err, server.Close())
			}
			shutdown <- err
		case <-finished:
			shutdown <- nil
		}
	}()
	var err error
	if cert != "" || key != "" {
		err = server.ServeTLS(listener, cert, key)
	} else {
		err = server.Serve(listener)
	}
	close(finished)
	shutdownErr := <-shutdown
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}

// startMaintenance returns an idempotent stop function that joins the worker.
func (fs *fileServer) startMaintenance(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		fs.pruneSpools(time.Now())
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:

				fs.uploads.PruneExpired()
				fs.pruneSpools(time.Now())
			}
		}
	}()
	return func() { cancel(); <-done }
}
