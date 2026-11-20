package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/winezer0/okserver/internal/upload"
)

// Ready tasks are persisted before enqueue; scanning recovers lost queue entries.
func (fs *fileServer) startVerification(ctx context.Context, concurrency int) func() {
	if concurrency < 1 {
		concurrency = 2
	}
	fs.verificationQueue = make(chan upload.Metadata, 128)
	var workers sync.WaitGroup
	for n := 0; n < concurrency; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			failures := make(map[string]int)
			for {
				select {
				case <-ctx.Done():
					return
				case task := <-fs.verificationQueue:
					state, err := fs.uploads.Get(task.ID)
					if err != nil || state.Completed || state.Failed || state.Canceled || state.Offset != state.Size {
						fs.verificationPending.Delete(task.ID)
						continue
					}
					jobCtx, cancel := context.WithCancel(ctx)
					fs.verificationCancel.Store(task.ID, cancel)
					state, err = fs.uploads.CompleteContext(jobCtx, state.ID, state.ExpectedSHA256)
					cancel()
					fs.verificationCancel.Delete(task.ID)
					fs.verificationPending.Delete(task.ID)
					if err != nil && !errors.Is(err, context.Canceled) {
						failures[task.ID]++
						if fs.logger != nil {
							fs.logger.ErrorContext(ctx, "upload.verification_failed", "task_id", task.ID, "path", state.Path, "attempt", failures[task.ID], "error", err.Error())
						}
						if failures[task.ID] >= 5 {
							if failErr := fs.uploads.Fail(task.ID); failErr == nil {
								delete(failures, task.ID)
							}
						}
					} else {
						delete(failures, task.ID)
					}
					if errors.Is(err, upload.ErrPrecondition) {
						_ = fs.uploads.FailWithCode(task.ID, "destination_changed")
					} else if errors.Is(err, upload.ErrConflict) || errors.Is(err, upload.ErrIncomplete) {
						_ = fs.uploads.Fail(task.ID)
					}
					if err == nil {
						fs.rememberUploadedHashes(ctx, state)
					}
				}
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			for _, task := range fs.uploads.Tasks() {
				if task.Offset == task.Size && !task.Canceled && !task.Failed {
					fs.enqueueVerification(task)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return workers.Wait
}

func (fs *fileServer) enqueueVerification(task upload.Metadata) {
	if _, loaded := fs.verificationPending.LoadOrStore(task.ID, true); loaded {
		return
	}
	select {
	case fs.verificationQueue <- task:
	default:
		fs.verificationPending.Delete(task.ID)
	}
}
