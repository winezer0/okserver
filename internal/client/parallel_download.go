package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/winezer0/okserver/internal/protocol"
	"github.com/winezer0/okserver/internal/storage"
)

type parallelState struct {
	URL, ETag, SHA256 string
	Size, Chunk       int64
	Done              []bool
}

func (fc *fileClient) parallelDownload() error {
	endpoint := downloadURL(fc.httpUrl)
	req, err := fc.newRequest("HEAD", endpoint, nil, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "identity")
	response, err := fc.queryMetadata(req)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("download metadata: HTTP %d", response.StatusCode)
	}
	sum, tag := response.Header.Get("X-File-SHA256"), responseValidator(response)
	size, e := strconv.ParseInt(response.Header.Get("X-File-Size"), 10, 64)
	if e != nil || size < 0 || len(sum) != 64 || tag == "" || response.Header.Get("Accept-Ranges") != "bytes" {
		return fc.get()
	}
	if fc.output == "" {
		fc.output = filepath.Base(req.URL.Path)
	}
	partial, statePath, release, err := prepareDownload(fc.output)
	if err != nil {
		return err
	}
	defer release()
	statePath += ".ranges"
	const block int64 = 8 << 20
	count := (size + block - 1) / block
	if count > 10000000 {
		return errors.New("too many download ranges")
	}
	state := parallelState{URL: fc.httpUrl, ETag: tag, SHA256: sum, Size: size, Chunk: block, Done: make([]bool, int(count))}
	if fc.point {
		data, _ := os.ReadFile(statePath)
		var saved parallelState
		if json.Unmarshal(data, &saved) == nil && saved.URL == state.URL && saved.ETag == tag && saved.SHA256 == sum && saved.Size == size && saved.Chunk == block && len(saved.Done) == len(state.Done) {
			if info, e := os.Stat(partial); e == nil && info.Size() == size {
				state = saved
			}
		}
	}
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = file.Truncate(size); err != nil {
		return err
	}
	save := func() error {
		data, e := json.Marshal(state)
		if e != nil {
			return e
		}
		return writeDownloadState(statePath, data)
	}
	if err = save(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(fc.context())
	defer cancel()
	jobs := make(chan int)
	failures := make(chan error, 1)
	var workers sync.WaitGroup
	var mu sync.Mutex
	fail := func(e error) {
		select {
		case failures <- e:
		default:
		}
		cancel()
	}
	for n := 0; n < fc.downloadConcurrency; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				start := int64(index) * block
				end := min(start+block, size) - 1
				var last error
				for attempt := 0; attempt < 6; attempt++ {
					request, e := fc.newRequest("GET", endpoint, nil, nil)
					if e != nil {
						last = e
						break
					}
					request = request.WithContext(ctx)
					request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
					request.Header.Set("If-Range", tag)
					request.Header.Set("Accept-Encoding", "identity")
					resp, e := fc.client.Do(request)
					if e == nil {
						if protocol.RetryableStatus(resp.StatusCode, false) {
							last = responseError(resp)
							resp.Body.Close()
							if e = fc.waitRetryResponse(ctx, attempt, resp); e != nil {
								last = e
								break
							}
							continue
						}
						expected := fmt.Sprintf("bytes %d-%d/%d", start, end, size)
						if resp.StatusCode != 206 || resp.Header.Get("Content-Range") != expected || resp.Header.Get("ETag") != tag || resp.Header.Get("X-File-SHA256") != sum {
							resp.Body.Close()
							last = errors.New("download range or version changed")
							break
						}
						data, copyErr := io.ReadAll(io.LimitReader(resp.Body, end-start+2))
						resp.Body.Close()
						if int64(len(data)) > end-start+1 {
							last = errors.New("oversized download range")
							break
						}
						if copyErr == nil && int64(len(data)) == end-start+1 {
							_, copyErr = file.WriteAt(data, start)
							if copyErr == nil {
								last = nil
								break
							}
						}
						last = copyErr
						if last == nil {
							last = io.ErrUnexpectedEOF
						}
					} else {
						last = e
					}
					if ctx.Err() != nil {
						break
					}
					if e = fc.waitRetryResponse(ctx, attempt, nil); e != nil {
						last = e
						break
					}
				}
				if last != nil {
					fail(last)
					return
				}
				mu.Lock()
				e := file.Sync()
				if e == nil {
					state.Done[index] = true
					e = save()
				}
				mu.Unlock()
				if e != nil {
					fail(e)
					return
				}
			}
		}()
	}
dispatch:
	for index, done := range state.Done {
		if done {
			continue
		}
		select {
		case jobs <- index:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	select {
	case err = <-failures:
		return err
	default:
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = verifyDownloadedFile(partial, "sha256", sum); err != nil {
		_ = os.Remove(statePath)
		return err
	}
	if err = storage.DurableReplace(partial, fc.output); err != nil {
		return err
	}
	_ = os.Remove(statePath)
	_ = os.Remove(strings.TrimSuffix(statePath, ".ranges"))
	return nil
}
