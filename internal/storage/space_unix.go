//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package storage

import "golang.org/x/sys/unix"

func FreeSpace(path string) (uint64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	return stat.Bavail * uint64(stat.Bsize), err
}
