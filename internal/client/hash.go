package client

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"crypto/sha256"
)

const maxUploadChunkSize int64 = 8 << 20

func copyFilePrefixContext(ctx context.Context, file *os.File, size int64, buffer []byte, hash io.Writer) error {
	if size < 0 {
		return errors.New("hash prefix size must not be negative")
	}
	written, err := io.CopyBuffer(hash, contextReader{ctx, io.NewSectionReader(file, 0, size)}, buffer)
	if err != nil {
		return err
	}
	if written != size {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func hashFilePrefix(file *os.File, size int64, buffer []byte) (string, error) {
	return hashFilePrefixContext(context.Background(), file, size, buffer)
}

func hashFilePrefixContext(ctx context.Context, file *os.File, size int64, buffer []byte) (string, error) {
	hash := sha256.New()
	if err := copyFilePrefixContext(ctx, file, size, buffer, hash); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readFileRangeContext(ctx context.Context, file *os.File, start, length int64) ([]byte, error) {
	if start < 0 || length < 0 || length > maxUploadChunkSize {
		return nil, errors.New("invalid upload chunk range")
	}
	payload := make([]byte, int(length))
	read, err := io.ReadFull(contextReader{ctx, io.NewSectionReader(file, start, length)}, payload)
	if err != nil {
		return nil, err
	}
	if int64(read) != length {
		return nil, io.ErrUnexpectedEOF
	}
	return payload, nil
}

func sha256Digest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
