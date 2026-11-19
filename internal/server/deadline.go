package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

type idleTimeoutBody struct {
	io.ReadCloser
	controller *http.ResponseController
	timeout    time.Duration
}

func (body *idleTimeoutBody) Read(buffer []byte) (int, error) {
	useTimer := body.controller == nil
	if body.controller != nil {
		if err := body.controller.SetReadDeadline(time.Now().Add(body.timeout)); err != nil {
			if !errors.Is(err, http.ErrNotSupported) {
				return 0, err
			}
			useTimer = true
		}
	}
	var timer *time.Timer
	if useTimer {
		timer = time.AfterFunc(body.timeout, func() { _ = body.ReadCloser.Close() })
	}
	n, err := body.ReadCloser.Read(buffer)
	if timer != nil && !timer.Stop() {
		return n, os.ErrDeadlineExceeded
	}
	if err != nil && body.controller != nil {
		_ = body.controller.SetReadDeadline(time.Time{})
	}
	return n, err
}
