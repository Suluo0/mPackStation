//go:build !windows

package service

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func storageInfo(dir string) (bool, int64) {
	if dir == "" {
		return false, 0
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, 0
	}
	f, err := os.CreateTemp(filepath.Clean(dir), ".health-")
	if err != nil {
		return false, 0
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return true, 0
	}
	return true, int64(stat.Bavail) * int64(stat.Bsize)
}
