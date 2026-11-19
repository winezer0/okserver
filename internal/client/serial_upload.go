package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/winezer0/okserver/internal/protocol"
)

func (fc *fileClient) serialUpload(source string) (result error) {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source must be a regular file")
	}
	defer func() {
		if result != nil {
			return
		}
		current, err := file.Stat()
		if err != nil {
			result = err
			return
		}
		if current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
			result = errors.New("source changed during upload")
		}
	}()
	remote, err := url.Parse(fc.httpUrl)
	if err != nil || remote.Host == "" || !strings.HasPrefix(remote.Path, "/upload/") || strings.HasSuffix(remote.Path, "/") {
		return errors.New("upload URL must be /upload/<destination filename>")
	}
	statusURL := *remote
	cache := os.Getenv("OKSERVER_CACHE_DIR")
	var e error
	if cache == "" {
		cache, e = os.UserCacheDir()
		if e != nil {
			return e
		}
	}
	directory := filepath.Join(cache, "okserver", "uploads")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	key := sha256Digest([]byte(remote.String() + "\n" + source))
	tokenPath := filepath.Join(directory, key+".token")
	if data, e := os.ReadFile(tokenPath); e == nil {
		fc.uploadToken = string(data)
	} else {
		fc.uploadToken, e = randomUploadKey()
		if e != nil {
			return e
		}
		if e = os.WriteFile(tokenPath, []byte(fc.uploadToken), 0600); e != nil {
			return e
		}
	}
	q := statusURL.Query()
	q.Set("prefix", "sha256")
	statusURL.RawQuery = q.Encode()
	offset, chunkSize := int64(0), maxUploadChunkSize
	request, err := fc.newRequest("HEAD", statusURL.String(), nil, nil)
	if err != nil {
		return err
	}
	response, err := fc.queryMetadata(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if value, e := strconv.ParseInt(response.Header.Get("Upload-Chunk-Size"), 10, 64); e == nil && value > 0 && value < chunkSize {
		chunkSize = value
	}
	whole := sha256.New()
	terminal := response.Header.Get("Upload-State") == "failed" || response.Header.Get("Upload-State") == "canceled"
	if response.StatusCode == 200 && !terminal {
		offset, err = strconv.ParseInt(response.Header.Get("Upload-Offset"), 10, 64)
		if err != nil || offset < 0 || offset > info.Size() {
			return errors.New("invalid remote offset")
		}
		size, err := strconv.ParseInt(response.Header.Get("Upload-Length"), 10, 64)
		if err != nil || size != info.Size() {
			return errors.New("remote upload size differs")
		}
		if !fc.point && response.Header.Get("Upload-State") != "completed" {
			return errors.New("upload exists; use -c to resume")
		}
		if err = copyFilePrefixContext(fc.context(), file, offset, fc.buf, whole); err != nil {
			return err
		}
		if hex.EncodeToString(whole.Sum(nil)) != response.Header.Get("Upload-Prefix-SHA256") {
			return errors.New("remote prefix differs from local file")
		}
		if response.Header.Get("Upload-State") == "completed" {
			return nil
		}
		if value, e := strconv.ParseInt(response.Header.Get("Upload-Chunk-Size"), 10, 64); e == nil && value > 0 && value < chunkSize {
			chunkSize = value
		}
	} else if response.StatusCode != 404 && !terminal {
		return fmt.Errorf("query upload: HTTP %d", response.StatusCode)
	}
	if offset == info.Size() && response.StatusCode == 200 && !terminal {
		return fc.awaitUpload(remote.String(), hex.EncodeToString(whole.Sum(nil)))
	}
	for offset < info.Size() || info.Size() == 0 {
		length := min(chunkSize, info.Size()-offset)
		payload, err := readFileRangeContext(fc.context(), file, offset, length)
		if err != nil {
			return err
		}
		whole.Write(payload)
		final := offset+length == info.Size()
		accepted := false
		for attempt := 0; attempt < 6; attempt++ {
			method := "PUT"
			if info.Size() == 0 {
				method = "POST"
			}
			request, err = fc.newRequest(method, remote.String(), bytes.NewReader(payload), nil)
			if err != nil {
				return err
			}
			if info.Size() > 0 {
				request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, info.Size()))
				request.Header.Set("X-Chunk-SHA256", sha256Digest(payload))
			}
			if final {
				request.Header.Set("X-File-SHA256", hex.EncodeToString(whole.Sum(nil)))
			}
			response, err = fc.client.Do(request)
			if err == nil {
				if response.StatusCode == 202 && final {
					response.Body.Close()
					return fc.awaitUpload(remote.String(), hex.EncodeToString(whole.Sum(nil)))
				}
				io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
				response.Body.Close()
				if response.StatusCode == 204 || response.StatusCode == 201 {
					if final && response.Header.Get("X-File-SHA256") != hex.EncodeToString(whole.Sum(nil)) {
						return errors.New("upload verification failed")
					}
					accepted = true
					break
				}
				if !protocol.RetryableStatus(response.StatusCode, true) && response.StatusCode != 422 {
					return fmt.Errorf("upload: HTTP %d", response.StatusCode)
				}
			}
			probe, e := fc.newRequest("HEAD", remote.String(), nil, nil)
			if e != nil {
				return e
			}
			state, e := fc.queryMetadata(probe)
			if e == nil {
				state.Body.Close()
				if state.StatusCode == 404 && offset == 0 {
					if err = fc.waitRetryResponse(fc.context(), attempt, response); err != nil {
						return err
					}
					continue
				}
				if state.StatusCode != 200 {
					return fmt.Errorf("upload recovery query: HTTP %d", state.StatusCode)
				}
				if state.Header.Get("Upload-State") == "failed" || state.Header.Get("Upload-State") == "canceled" {
					return errors.New("upload verification failed")
				}
				next, _ := strconv.ParseInt(state.Header.Get("Upload-Offset"), 10, 64)
				if final && next == info.Size() {
					return fc.awaitUpload(remote.String(), hex.EncodeToString(whole.Sum(nil)))
				}
				if next == offset+length && (!final || state.Header.Get("Upload-State") == "completed") {
					accepted = true
					break
				}
				if next != offset {
					return errors.New("unexpected remote upload progress")
				}
			}
			if err = fc.waitRetryResponse(fc.context(), attempt, response); err != nil {
				return err
			}
		}
		if !accepted {
			return errors.New("upload retry limit exceeded")
		}
		offset += length
		if final {
			break
		}
	}
	return nil
}

func (fc *fileClient) awaitUpload(endpoint, checksum string) error {
	for poll := 0; poll < 86400; poll++ {
		request, err := fc.newRequest("HEAD", endpoint, nil, nil)
		if err != nil {
			return err
		}
		response, err := fc.queryMetadata(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != 200 {
				return fmt.Errorf("upload verification query: HTTP %d", response.StatusCode)
			}
			switch response.Header.Get("Upload-State") {
			case "completed":
				if response.Header.Get("X-File-SHA256") != checksum {
					return errors.New("upload verification failed")
				}
				return nil
			case "failed", "canceled":
				return errors.New("upload verification failed")
			}
		}
		if err = fc.waitRetry(0); err != nil {
			return err
		}
	}
	return errors.New("upload verification timed out")
}
