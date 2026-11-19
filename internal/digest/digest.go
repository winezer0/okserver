package digest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	AlgorithmSHA256 = "sha256"
)

var ErrChanged = errors.New("file changed while hashing")

type Hashes struct {
	SHA256 string `json:"sha256"`
}

func (h Hashes) Value(algorithm string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case AlgorithmSHA256:
		return h.SHA256, true
	default:
		return "", false
	}
}

func (h Hashes) Valid() bool {
	return validHex(h.SHA256, 64)
}

func ValidAlgorithm(algorithm string) bool {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case AlgorithmSHA256:
		return true
	default:
		return false
	}
}

func validHex(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type digestEntry struct {
	info   os.FileInfo
	change string
	hashes Hashes
}

var digests = struct {
	sync.Mutex
	entries map[string]digestEntry
}{entries: make(map[string]digestEntry)}

func OpenFileDigest(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	stamp, stampErr := changeStamp(file, info)
	if stampErr == nil {
		digests.Lock()
		previous, found := digests.entries[file.Name()]
		digests.Unlock()
		if found && previous.hashes.SHA256 != "" && previous.change == stamp && previous.info.Size() == info.Size() && previous.info.ModTime().Equal(info.ModTime()) && os.SameFile(previous.info, info) {
			return previous.hashes.SHA256, nil
		}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	value := hex.EncodeToString(hash.Sum(nil))
	current, err := file.Stat()
	if err != nil {
		return "", err
	}
	after, afterErr := changeStamp(file, current)
	if current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) || (stampErr == nil && afterErr == nil && stamp != after) {
		return "", ErrChanged
	}
	if stampErr == nil && afterErr == nil {
		digests.Lock()
		if len(digests.entries) >= 1024 {
			digests.entries = make(map[string]digestEntry)
		}
		digests.entries[file.Name()] = digestEntry{info: current, change: after, hashes: Hashes{SHA256: value}}
		digests.Unlock()
	}
	return value, nil
}

// OpenFileHashes calculates all supported hashes in one streaming pass.
func OpenFileHashes(file *os.File) (Hashes, error) {
	info, err := file.Stat()
	if err != nil {
		return Hashes{}, err
	}
	stamp, stampErr := changeStamp(file, info)
	digests.Lock()
	previous, found := digests.entries[file.Name()]
	digests.Unlock()
	if stampErr == nil && found && previous.hashes.Valid() && previous.change == stamp && previous.info.Size() == info.Size() && previous.info.ModTime().Equal(info.ModTime()) && os.SameFile(previous.info, info) {
		return previous.hashes, nil
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return Hashes{}, err
	}
	sha256Hash := sha256.New()
	if _, err = io.Copy(sha256Hash, file); err != nil {
		return Hashes{}, err
	}
	hashes := Hashes{
		SHA256: hex.EncodeToString(sha256Hash.Sum(nil)),
	}
	current, err := file.Stat()
	if err != nil {
		return Hashes{}, err
	}
	after, afterErr := changeStamp(file, current)
	if current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) || (stampErr == nil && afterErr == nil && stamp != after) {
		return Hashes{}, ErrChanged
	}
	if stampErr == nil && afterErr == nil {
		digests.Lock()
		if len(digests.entries) >= 1024 {
			digests.entries = make(map[string]digestEntry)
		}
		digests.entries[file.Name()] = digestEntry{info: current, change: after, hashes: hashes}
		digests.Unlock()
	}
	return hashes, nil
}

// Fingerprint returns a cache key when the filesystem exposes a change stamp.
// Callers must bypass persistent caching when reliable is false.
func Fingerprint(file *os.File) (value string, reliable bool, err error) {
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	stamp, err := changeStamp(file, info)
	if err != nil {
		return "", false, nil
	}
	return strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + stamp, true, nil
}

// Carry verified staging hashes across our own rename. The staging file is
// private; external modifications after publication invalidate its change stamp.
func RememberPublishedDigest(source, target string) {
	digests.Lock()
	entry, ok := digests.entries[source]
	delete(digests.entries, source)
	digests.Unlock()
	if !ok {
		return
	}
	file, err := os.Open(target)
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(entry.info, info) || entry.info.Size() != info.Size() || !entry.info.ModTime().Equal(info.ModTime()) {
		return
	}
	stamp, err := changeStamp(file, info)
	if err != nil {
		return
	}
	digests.Lock()
	digests.entries[target] = digestEntry{info: info, change: stamp, hashes: entry.hashes}
	digests.Unlock()
}

// RememberVerifiedFile is called only for a privately staged, verified file
// published by this process. External changes invalidate its change stamp.
func RememberVerifiedFile(target string, size int64, hashes Hashes) {
	if !hashes.Valid() {
		return
	}
	file, err := os.Open(target)
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != size {
		return
	}
	stamp, err := changeStamp(file, info)
	if err != nil {
		return
	}
	digests.Lock()
	defer digests.Unlock()
	if len(digests.entries) >= 1024 {
		digests.entries = make(map[string]digestEntry)
	}
	digests.entries[target] = digestEntry{info: info, change: stamp, hashes: hashes}
}
