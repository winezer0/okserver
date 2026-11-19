package upload

import (
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"crypto/sha256"
)

func (m *Manager) destinationPath(relativePath string) (string, error) {
	relativePath, err := CleanRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	target := filepath.Join(m.root, filepath.FromSlash(relativePath))
	parent := filepath.Dir(target)
	rootResolved, err := filepath.EvalSymlinks(m.root)
	if err != nil {
		return "", err
	}
	if _, err = resolveWithinRoot(rootResolved, parent); err != nil {
		return "", err
	}
	if err = os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	parentResolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootResolved, parentResolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrInvalid
	}
	if rel != "." {
		if _, err = CleanRelativePath(filepath.ToSlash(rel)); err != nil {
			return "", err
		}
	}
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalid
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	return target, nil
}

func resolveWithinRoot(root, candidate string) (string, error) {
	ancestor := candidate
	var suffix []string
	var resolved string
	for {
		var err error
		resolved, err = filepath.EvalSymlinks(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", ErrInvalid
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
	}
	for _, part := range suffix {
		resolved = filepath.Join(resolved, part)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", ErrInvalid
	}
	if relative != "." {
		if _, err = CleanRelativePath(filepath.ToSlash(relative)); err != nil {
			return "", err
		}
	}
	return resolved, nil
}

func CleanRelativePath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	localPath := filepath.FromSlash(value)
	if value == "" || strings.HasPrefix(value, "/") || path.IsAbs(value) || filepath.IsAbs(localPath) || filepath.VolumeName(localPath) != "" {
		return "", ErrInvalid
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrInvalid
	}
	for _, part := range strings.Split(cleaned, "/") {
		if strings.ContainsAny(part, ":\x00") || strings.TrimRight(part, " .") != part {
			return "", ErrInvalid
		}
		if strings.EqualFold(strings.TrimRight(part, " ."), ".okserver") || strings.EqualFold(strings.TrimRight(part, " ."), ".okserver-downloads") {
			return "", ErrInvalid
		}
		device := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		switch device {
		case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
			return "", ErrInvalid
		}
	}
	return cleaned, nil
}

func normalizeChecksum(value string, allowEmpty bool) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" && allowEmpty {
		return "", nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(sha256.Sum256(nil)) {
		return "", ErrInvalid
	}
	return value, nil
}

func normalizeIdempotencyKey(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if len(value) != 32 && len(value) != 64 {
		return "", ErrInvalid
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", ErrInvalid
	}
	return value, nil
}
