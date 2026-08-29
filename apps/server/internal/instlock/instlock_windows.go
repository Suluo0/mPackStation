//go:build windows

package instlock

import "syscall"

// processAlive 在 Windows 上通过 OpenProcess 探测进程是否存在。
// os.FindProcess 在 Windows 上不会校验进程存在性，Signal(0) 也不可用，
// 因此必须用 syscall。句柄打开成功即说明进程存活。
func processAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = syscall.CloseHandle(h)
	return true
}
