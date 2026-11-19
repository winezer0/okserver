package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/winezer0/okserver/internal/upload"
)

func (fs *fileServer) resolvePath(value string, allowRoot bool) (string, string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" && allowRoot {
		return "", fs.path, nil
	}
	relative, err := upload.CleanRelativePath(value)
	if err != nil {
		return "", "", err
	}
	fullPath := filepath.Join(fs.path, filepath.FromSlash(relative))
	root, err := filepath.EvalSymlinks(fs.path)
	if err != nil {
		return "", "", err
	}
	ancestor := fullPath
	var suffix []string
	var resolved string
	for {
		resolved, err = filepath.EvalSymlinks(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", "", err
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
	}
	for _, part := range suffix {
		resolved = filepath.Join(resolved, part)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", upload.ErrInvalid
	}
	if rel != "." {
		if _, err = upload.CleanRelativePath(filepath.ToSlash(rel)); err != nil {
			return "", "", err
		}
	}
	return relative, fullPath, nil
}
