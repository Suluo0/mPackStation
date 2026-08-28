//go:build windows

package service

import (
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
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
	path, err := syscall.UTF16PtrFromString(filepath.Clean(dir))
	if err != nil {
		return true, 0
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(path, &free, &total, &totalFree); err != nil {
		return true, 0
	}
	return true, int64(free)
}
