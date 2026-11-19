package server

import (
	"bytes"
	"errors"
	"io"

	"github.com/winezer0/okserver/internal/upload"
)

const curlWriteBuffer = 8 << 20

func (fs *fileServer) appendCurlBody(id string, metadata upload.Metadata, body io.Reader, bodyLength int64) (upload.Metadata, error) {
	remaining := metadata.Size - metadata.Offset
	if bodyLength < -1 || bodyLength > remaining {
		return metadata, upload.ErrTooLarge
	}
	toRead := bodyLength
	if toRead < 0 {
		toRead = remaining
	}
	blockSize := fs.uploads.ChunkSize()
	if blockSize > curlWriteBuffer {
		blockSize = curlWriteBuffer
	}
	if blockSize <= 0 {
		return metadata, upload.ErrInvalid
	}
	buffer := make([]byte, int(blockSize))
	for toRead > 0 {
		length := toRead
		if length > blockSize {
			length = blockSize
		}
		n, readErr := io.ReadFull(body, buffer[:int(length)])
		if n > 0 {
			updated, writeErr := fs.uploads.WriteChunkCalculated(
				id, metadata.Offset, metadata.Offset+int64(n)-1, metadata.Size,
				bytes.NewReader(buffer[:n]),
			)
			if writeErr != nil {
				if updated.ID == "" {
					updated = metadata
				}
				return updated, writeErr
			}
			metadata = updated
			toRead -= int64(n)
		}
		if readErr != nil {
			return metadata, readErr
		}
	}
	if bodyLength < 0 && metadata.Offset == metadata.Size {
		var extra [1]byte
		n, err := io.ReadFull(body, extra[:])
		if n > 0 {
			return metadata, upload.ErrTooLarge
		}
		if !errors.Is(err, io.EOF) {
			return metadata, err
		}
	}
	return metadata, nil
}
