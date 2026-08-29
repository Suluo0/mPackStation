// Package instlock 提供数据目录级单实例锁，
// 避免两个服务进程同时写同一个 SQLite 数据库。
//
// 实现方案：在数据目录放置 server.lock 文件，以 O_CREATE|O_EXCL 独占创建并
// 写入当前 pid。若文件已存在则读取其中的 pid，判断对应进程是否存活：
//   - 存活：说明另一实例正在运行，返回错误拒绝启动；
//   - 不存活或内容无法解析：视为 stale 锁（上次进程异常退出遗留），删除后接管。
//
// 进程存活判断按平台分文件实现：Windows 上使用 syscall.OpenProcess
// （os.FindProcess + Signal(0) 在 Windows 上不可靠，FindProcess 总是成功）；
// 其他平台使用 Signal(0) 探测。
package instlock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LockFileName 是数据目录内锁文件的名称。
const LockFileName = "server.lock"

// Lock 代表已持有的单实例锁。
type Lock struct {
	path string
}

// Acquire 尝试在 dir 下获取单实例锁。成功返回 *Lock；
// 若另一存活实例已持有锁则返回错误。
func Acquire(dir string) (*Lock, error) {
	path := filepath.Join(dir, LockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		alive, pid := checkExisting(path)
		if alive {
			return nil, fmt.Errorf("another mpackstation server instance is already running (pid %d)", pid)
		}
		// stale 锁：删除后重试一次独占创建。重试仍失败说明有并发启动竞争，直接报错。
		if rmErr := os.Remove(path); rmErr != nil {
			return nil, fmt.Errorf("remove stale lock file: %w", rmErr)
		}
		f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return nil, fmt.Errorf("acquire instance lock: %w", err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("write lock file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("close lock file: %w", err)
	}
	return &Lock{path: path}, nil
}

// Release 释放锁并删除锁文件。进程退出（含优雅退出）前应调用。
func (l *Lock) Release() error {
	err := os.Remove(l.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove lock file: %w", err)
	}
	return nil
}

// checkExisting 读取已有锁文件中的 pid 并判断进程是否存活。
// 内容缺失或无法解析时一律视为 stale（返回 alive=false）。
func checkExisting(path string) (alive bool, pid int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, 0
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false, 0
	}
	return processAlive(pid), pid
}
