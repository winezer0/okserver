package server

import (
	"context"
	"errors"
	"os"

	"github.com/winezer0/okserver/internal/digest"
	"github.com/winezer0/okserver/internal/hashmanifest"
	"github.com/winezer0/okserver/internal/upload"
)

func (fs *fileServer) rememberUploadedHashes(ctx context.Context, metadata upload.Metadata) {
	if fs.hashes == nil {
		return
	}
	relative, fullPath, err := fs.resolvePath(metadata.Path, false)
	if err != nil {
		fs.warnHashManifest(ctx, "resolve uploaded file", metadata.Path, err)
		return
	}
	file, err := os.Open(fullPath)
	if err != nil {
		fs.warnHashManifest(ctx, "open uploaded file", relative, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		fs.warnHashManifest(ctx, "stat uploaded file", relative, err)
		return
	}
	if info.IsDir() {
		fs.warnHashManifest(ctx, "hash uploaded file", relative, errors.New("upload destination is a directory"))
		return
	}
	fingerprint, reliable, err := digest.Fingerprint(file)
	if err != nil {
		fs.warnHashManifest(ctx, "fingerprint uploaded file", relative, err)
		return
	}
	if !reliable {
		return
	}
	hashes, err := digest.OpenFileHashes(file)
	if err != nil {
		fs.warnHashManifest(ctx, "hash uploaded file", relative, err)
		return
	}
	currentInfo, err := file.Stat()
	if err != nil {
		fs.warnHashManifest(ctx, "stat uploaded file", relative, err)
		return
	}
	currentFingerprint, currentReliable, err := digest.Fingerprint(file)
	if err != nil {
		fs.warnHashManifest(ctx, "fingerprint uploaded file", relative, err)
		return
	}
	if !sameFileAtPath(file, fullPath) || !currentReliable || currentFingerprint != fingerprint || currentInfo.Size() != info.Size() {
		fs.warnHashManifest(ctx, "cache uploaded file", relative, digest.ErrChanged)
		return
	}
	if err := fs.hashes.Put(hashmanifest.Entry{
		Path: relative, Size: currentInfo.Size(), Fingerprint: currentFingerprint, Hashes: hashes,
	}); err != nil {
		fs.warnHashManifest(ctx, "write", relative, err)
	}
}

func sameFileAtPath(file *os.File, path string) bool {
	opened, err := file.Stat()
	if err != nil {
		return false
	}
	current, err := os.Stat(path)
	return err == nil && os.SameFile(opened, current)
}

func (fs *fileServer) warnHashManifest(ctx context.Context, operation, path string, err error) {
	if fs.logger != nil {
		fs.logger.WarnContext(ctx, "file.hash_manifest", "operation", operation, "path", path, "error", err.Error())
	}
}
