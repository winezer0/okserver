package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/winezer0/okserver/internal/diskbudget"
	"github.com/winezer0/okserver/internal/storage"
)

var errSpoolLimit = errors.New("file exceeds size limit")

func (fs *fileServer) writeSpoolError(w http.ResponseWriter, err error) {
	if errors.Is(err, errSpoolLimit) {
		writeAPIError(w, 413, err.Error(), nil)
		return
	}
	fs.writeUploadError(w, err, nil)
}

// spool keeps a cross-process ownership lock until the request releases its file.
func (fs *fileServer) spool(reader io.Reader) (*os.File, int64, func(), error) {
	file, err := os.CreateTemp(filepath.Join(fs.path, ".okserver"), "stream-*")
	if err != nil {
		return nil, 0, nil, err
	}
	release, err := storage.TryFileLock(file.Name() + ".lock")
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, 0, nil, err
	}
	cleanup := func() { file.Close(); os.Remove(file.Name()); release(); os.Remove(file.Name() + ".lock") }
	limit := fs.uploads.SpoolLimit()
	written, err := io.CopyBuffer(spoolWriter{fs: fs, file: file}, io.LimitReader(reader, limit+1), make([]byte, 64*1024))
	if err == nil && written > limit {
		err = errSpoolLimit
	}
	if err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		cleanup()
		return nil, written, nil, err
	}
	return file, written, cleanup, nil
}

type spoolWriter struct {
	fs   *fileServer
	file *os.File
}

func (writer spoolWriter) Write(data []byte) (int, error) {
	release, err := diskbudget.Admit(writer.fs.path, uint64(len(data)), writer.fs.minFreeBytes)
	if err != nil {
		return 0, err
	}
	defer release()
	return writer.file.Write(data)
}

// Only orphaned, aged scratch files are removed. Live requests retain an OS lock.
func (fs *fileServer) pruneSpools(now time.Time) {
	directory := filepath.Join(fs.path, ".okserver")
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".lock") || !(strings.HasPrefix(entry.Name(), "stream-") || strings.HasPrefix(entry.Name(), "multipart-")) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < time.Hour {
			continue
		}
		name := filepath.Join(directory, entry.Name())
		release, err := storage.TryFileLock(name + ".lock")
		if err != nil {
			continue
		}
		_ = os.Remove(name)
		release()
		_ = os.Remove(name + ".lock")
	}
}
