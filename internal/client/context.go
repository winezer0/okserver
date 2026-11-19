package client

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

func (fc *fileClient) context() context.Context {
	if fc.ctx != nil {
		return fc.ctx
	}
	return context.Background()
}

func (fc *fileClient) waitRetry(attempt int) error {
	return fc.waitRetryResponse(fc.context(), attempt, nil)
}

func (fc *fileClient) waitRetryResponse(ctx context.Context, attempt int, response *http.Response) error {
	backoff := min(30*time.Second, 500*time.Millisecond*time.Duration(1<<min(max(attempt, 0), 6)))
	wait := backoff/2 + time.Duration(rand.Int64N(int64(backoff/2)+1))
	if response != nil {
		value := response.Header.Get("Retry-After")
		if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
			wait = max(wait, time.Duration(seconds)*time.Second)
		} else if date, err := http.ParseTime(value); err == nil {
			wait = max(wait, time.Until(date))
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
