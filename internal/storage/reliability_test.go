package storage

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/winezer0/okserver/internal/digest"
)

func TestDigestCacheInvalidatesSameSizeSameTimeChange(t *testing.T) {
	name := filepath.Join(t.TempDir(), "file")
	os.WriteFile(name, []byte("old"), 0600)
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.Chtimes(name, stamp, stamp)
	first, err := digest.FileDigest(name)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := digest.FileDigest(name)
	if err != nil || repeat != first {
		t.Fatal("cached digest mismatch")
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	file.Seek(1, 0)
	if _, err = digest.OpenFileDigest(file); err != nil {
		t.Fatal(err)
	}
	position, _ := file.Seek(0, 1)
	file.Close()
	if position != 1 {
		t.Fatal("cache hit reread the entire file")
	}
	os.WriteFile(name, []byte("new"), 0600)
	os.Chtimes(name, stamp, stamp)
	second, err := digest.FileDigest(name)
	if err != nil || first == second {
		t.Fatalf("cache accepted changed content: %v", err)
	}
}
func TestFileLockChild(t *testing.T) {
	name := os.Getenv("UPDOWNFILE_LOCK_CHILD")
	if name == "" {
		return
	}
	release, err := TryFileLock(name)
	if err != nil {
		os.Exit(3)
	}
	defer release()
	fmt.Println("locked")
	time.Sleep(time.Hour)
}
func TestProcessLockReleasedAfterCrash(t *testing.T) {
	name := filepath.Join(t.TempDir(), "lock")
	command := exec.Command(os.Args[0], "-test.run=^TestFileLockChild$")
	command.Env = append(os.Environ(), "UPDOWNFILE_LOCK_CHILD="+name)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { command.Process.Kill(); command.Wait() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatal("child did not acquire lock")
	}
	if release, lockErr := TryFileLock(name); lockErr == nil {
		release()
		t.Fatal("cross-process lock bypassed")
	}
	command.Process.Kill()
	command.Wait()
	release, err := TryFileLock(name)
	if err != nil {
		t.Fatalf("crash left stale lock: %v", err)
	}
	release()
}
