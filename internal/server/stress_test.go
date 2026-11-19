package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"crypto/sha256"
)

func TestHTTPTransferStress(t *testing.T) {
	seconds := 2
	if value := os.Getenv("OKSERVER_STRESS_SECONDS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 3600 {
			t.Fatal("invalid stress duration")
		}
		seconds = parsed
	}
	server, _ := newTestHTTPServerWithChunkSize(t, "", 64<<10)
	context, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 15 * time.Second}
	var workers sync.WaitGroup
	var completed atomic.Uint64
	errors := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for sequence := 0; context.Err() == nil; sequence++ {
				if err := stressRound(client, server.URL, worker, sequence); err != nil {
					errors <- err
					cancel()
					return
				}
				completed.Add(1)
			}
		}(worker)
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if completed.Load() == 0 {
		t.Fatal("no completed transfer")
	}
	t.Logf("8 workers, %d seconds, %d verified upload/download transactions", seconds, completed.Load())
}
func stressRound(client *http.Client, base string, worker, sequence int) error {
	payload := bytes.Repeat([]byte{byte(worker)}, 64<<10)
	copy(payload, fmt.Sprintf("%d:%d", worker, sequence))
	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))
	path := fmt.Sprintf("worker-%d-%d.bin", worker, sequence)
	request, _ := http.NewRequest("PUT", base+"/upload/"+path+"?sha256="+checksum, bytes.NewReader(payload))
	request.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(payload)-1, len(payload)))
	request.Header.Set("X-Chunk-SHA256", checksum)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 201 {
		return fmt.Errorf("upload status %d", response.StatusCode)
	}
	response, err = client.Get(base + "/download/" + path)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	actual, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, payload) || response.Header.Get("X-File-SHA256") != checksum {
		return fmt.Errorf("content mismatch")
	}
	return nil
}
