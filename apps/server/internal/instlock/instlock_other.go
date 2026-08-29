//go:build !windows

package instlock

import (
	"os"
	"syscall"
)

// processAlive 在类 Unix 平台上以 Signal(0) 探测进程是否存在。
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
