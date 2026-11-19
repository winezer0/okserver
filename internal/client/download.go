package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"crypto/sha256"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/progress"
	"github.com/winezer0/okserver/internal/storage"
	"github.com/winezer0/okserver/internal/transfer"
)

type downloadState struct {
	URL               string
	Validator         string
	Size              int64
	ChecksumAlgorithm string
	Checksum          string
}

func (fc *fileClient) get() error {
	resume := fc.point
	defer func() { fc.point = resume }()
	for attempt := 0; ; attempt++ {
		err := fc.getOnce()
		var network net.Error
		if err == nil || attempt == 5 || fc.output == "-" || !(errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network)) {
			return err
		}
		if err = fc.waitRetry(attempt); err != nil {
			return err
		}
		fc.point = true
	}
}

func (fc *fileClient) getOnce() error {
	req, err := fc.newRequest(http.MethodGet, downloadURL(fc.httpUrl), nil, nil)
	if err != nil {
		return err
	}
	if fc.output == "" {
		fc.output = filepath.Base(req.URL.Path)
	}
	var partial, statePath string
	if fc.output != "-" {
		var release func()
		partial, statePath, release, err = prepareDownload(fc.output)
		if err != nil {
			return err
		}
		defer release()
	}
	var saved downloadState
	var offset int64
	if fc.point && fc.output != "-" {
		data, _ := os.ReadFile(statePath)
		if json.Unmarshal(data, &saved) == nil && saved.URL == fc.httpUrl && saved.Validator != "" {
			if info, statErr := os.Stat(partial); statErr == nil && info.Mode().IsRegular() && info.Size() <= saved.Size {
				offset = info.Size()
			}
		}
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		req.Header.Set("If-Range", saved.Validator)
	}
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := fc.queryMetadata(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	total := resp.ContentLength
	appendMode := false
	switch resp.StatusCode {
	case http.StatusOK:
		offset = 0
	case http.StatusPartialContent:
		var first, last int64
		first, last, total, err = transfer.ScanRangeSize(resp.Header)
		if err != nil ||
			offset <= 0 || first != offset || last < first || last != total-1 || (resp.ContentLength >= 0 && resp.ContentLength != last-first+1) {
			return errors.New("invalid download Content-Range")
		}
		if responseValidator(resp) != saved.Validator || total != saved.Size {
			return errors.New("download source changed")
		}
		appendMode = true
	case http.StatusRequestedRangeNotSatisfiable:
		size, parseErr := strconv.ParseInt(strings.TrimPrefix(resp.Header.Get("Content-Range"), "bytes */"), 10, 64)
		if parseErr != nil || offset <= 0 || size != offset || size != saved.Size || responseValidator(resp) != saved.Validator {
			return errors.New("download range is unsatisfied; local file is not verified complete")
		}
		if err = verifyDownloadedFile(partial, saved.ChecksumAlgorithm, saved.Checksum); err != nil {
			return err
		}
		if err = storage.DurableReplace(partial, fc.output); err != nil {
			return err
		}
		_ = os.Remove(statePath)
		return nil
	default:
		return responseError(resp)
	}
	var reader io.Reader = resp.Body
	checksumAlgorithm := resp.Header.Get("X-File-Checksum-Algorithm")
	checksum := resp.Header.Get("X-File-Checksum")
	if value := resp.Header.Get("X-File-SHA256"); value != "" {
		checksum, checksumAlgorithm = value, digest.AlgorithmSHA256
	}
	if checksum != "" && checksumAlgorithm != digest.AlgorithmSHA256 {
		return errors.New("unsupported download checksum algorithm")
	}

	if fc.output == "-" {
		hash := sha256.New()
		count, copyErr := io.CopyBuffer(io.MultiWriter(os.Stdout, hash), reader, fc.buf)
		if copyErr != nil {
			return copyErr
		}
		if total >= 0 && count != total {
			return io.ErrUnexpectedEOF
		}
		if checksum != "" && fmt.Sprintf("%x", hash.Sum(nil)) != strings.ToLower(checksum) {
			return errors.New("download SHA-256 checksum mismatch")
		}
		return nil
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendMode {
		flags = os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(partial, flags, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	validator := responseValidator(resp)
	if appendMode && (checksum != saved.Checksum || checksumAlgorithm != saved.ChecksumAlgorithm) {
		return errors.New("download checksum changed")
	}
	state := downloadState{URL: fc.httpUrl, Validator: validator, Size: total, ChecksumAlgorithm: checksumAlgorithm, Checksum: checksum}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err = writeDownloadState(statePath, data); err != nil {
		return err
	}
	var destination io.Writer = file
	if fc.pBar != nil {
		progress := progress.NewProgressWriter(file, progress.NewProgressBar(fc.pBar, http.MethodGet, fc.output, total))
		progress.SetCurrent(offset)
		defer progress.Close()
		destination = progress
	}
	count, err := io.CopyBuffer(destination, reader, fc.buf)
	if err != nil {
		return err
	}
	if total >= 0 && count+offset != total {
		return io.ErrUnexpectedEOF
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = verifyDownloadedFile(partial, checksumAlgorithm, checksum); err != nil {
		return err
	}
	if err = storage.DurableReplace(partial, fc.output); err != nil {
		return err
	}
	_ = os.Remove(statePath)
	return nil
}

func verifyDownloadedFile(name, algorithm, expected string) error {
	if expected == "" {
		return nil
	} // External HTTP servers may not provide a digest.
	if len(expected) != 64 {
		return errors.New("invalid download SHA-256 checksum")
	}
	if algorithm != digest.AlgorithmSHA256 {
		return errors.New("unsupported download checksum algorithm")
	}
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != strings.ToLower(expected) {
		return errors.New("download SHA-256 checksum mismatch; original target preserved")
	}
	return nil
}

func responseValidator(response *http.Response) string {
	if tag := response.Header.Get("ETag"); tag != "" && !strings.HasPrefix(tag, "W/") {
		return tag
	}
	return ""
}
