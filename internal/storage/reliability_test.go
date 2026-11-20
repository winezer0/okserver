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
	name := os.Getenv("OKSERVER_LOCK_CHILD")
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
	command.Env = append(os.Environ(), "OKSERVER_LOCK_CHILD="+name)
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

func TestSessionLockWaitsForCollidingShard(t *testing.T) {
	stateDir := t.TempDir()
	firstID := "session-first"
	secondID := ""
	for candidate := 0; candidate < 1000; candidate++ {
		id := fmt.Sprintf("session-%d", candidate)
		if id != firstID && lockShard("session", id) == lockShard("session", firstID) {
			secondID = id
			break
		}
	}
	if secondID == "" {
		t.Fatal("could not find colliding session IDs")
	}

	release, err := SessionLock(stateDir, firstID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	started := make(chan struct{})
	acquired := make(chan error, 1)
	go func() {
		close(started)
		releaseSecond, lockErr := SessionLock(stateDir, secondID)
		if lockErr == nil {
			releaseSecond()
		}
		acquired <- lockErr
	}()
	<-started
	select {
	case err = <-acquired:
		t.Fatalf("colliding session lock did not wait: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err = <-acquired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("colliding session lock remained blocked after release")
	}

	entries, err := os.ReadDir(filepath.Join(stateDir, "locks"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("session locks are not bounded to one shard: %d files, %v", len(entries), err)
	}
}
