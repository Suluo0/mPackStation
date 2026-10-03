// Package launcher integrates the mPackLauncher Rust binary as an external
// process. It owns the exec plumbing and JSON Lines protocol parsing; the
// task layer only sees phase events and a final result.
package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Event is one line of stdout from mPackLauncher.
type Event struct {
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Phase   string          `json:"phase"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

// PhaseHandler is called for each phase event emitted by the binary.
type PhaseHandler func(phase, message string)

// Runner executes mPackLauncher commands and streams events.
type Runner struct {
	BinPath string // path to mpack-launcher binary
	// Available is false when no binary was found. Callers must check it before
	// queueing work: a 202 whose task dies on "fork/exec …: no such file or
	// directory" is worse for the user than a synchronous 503.
	Available bool
}

// NewRunner resolves the launcher binary: MPACK_LAUNCHER_BIN wins, then the
// workbench-local copy under .tools/launcher/, then PATH. The Windows build is
// named mpack-launcher.exe; on macOS/Linux there is no extension, and the old
// hardcoded .exe path made every install/launch task fail on this machine.
func NewRunner(workbenchRoot string) *Runner {
	name := "mpack-launcher"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fallback := filepath.Join(workbenchRoot, ".tools", "launcher", name)
	for _, candidate := range []string{os.Getenv("MPACK_LAUNCHER_BIN"), fallback} {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return &Runner{BinPath: candidate, Available: true}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return &Runner{BinPath: p, Available: true}
	}
	// Keep the not-found path in BinPath so the task error still names it.
	return &Runner{BinPath: fallback, Available: false}
}

// Install runs `mpack-launcher install --mc <version> --dir <dir> [--loader <loader>] [--loader-version <v>] [--mirror <mirror>]`.
// phaseFn receives each phase event as it arrives. Returns the final result data.
//
// 旗标名以 launcherCore/src/cli.rs 的 InstallArgs 为准：字段是 `mc`，clap 生成的长选项
// 就是 --mc，并且没有 --version。之前这里发 `install --version 1.20.1`，真实二进制会被
// clap 判成「未知参数 + 缺少必填 --mc」直接退出 2 —— 安装任务在链路测试里之所以全绿，
// 是因为桩只校验版本号、不校验旗标（见 docs/tests 报告 D9）。
func (r *Runner) Install(ctx context.Context, version, minecraftDir string, loader, loaderVersion, mirror, javaPath string, phaseFn PhaseHandler) (json.RawMessage, error) {
	args := []string{"install", "--mc", version, "--dir", minecraftDir}
	if loader != "" {
		args = append(args, "--loader", loader)
	}
	if loaderVersion != "" {
		args = append(args, "--loader-version", loaderVersion)
	}
	if mirror != "" {
		args = append(args, "--mirror", mirror)
	}
	if javaPath != "" {
		args = append(args, "--java", javaPath)
	}
	return r.run(ctx, args, phaseFn)
}

// InstallFromMrpack runs `mpack-launcher install --mrpack <file> --dir <dir>`.
//
// 整合包安装与「装一个空版本」的区别：mc/loader 版本与模组清单都由包内
// modrinth.index.json 决定，调用方只给制品路径。返回的 data 里除 version_id
// 还有 mods/overrides 计数，供任务结果展示。
func (r *Runner) InstallFromMrpack(ctx context.Context, mrpackPath, minecraftDir, mirror, javaPath string, phaseFn PhaseHandler) (json.RawMessage, error) {
	args := []string{"install", "--mrpack", mrpackPath, "--dir", minecraftDir}
	if mirror != "" {
		args = append(args, "--mirror", mirror)
	}
	if javaPath != "" {
		args = append(args, "--java", javaPath)
	}
	return r.run(ctx, args, phaseFn)
}

// Launch runs `mpack-launcher launch --version <version> --dir <dir> --username <username> [--java <path>] [--xmx <mb>]`.
func (r *Runner) Launch(ctx context.Context, version, minecraftDir, username string, javaPath string, xmxMB int, phaseFn PhaseHandler) (json.RawMessage, error) {
	args := []string{"launch", "--version", version, "--dir", minecraftDir, "--username", username}
	if javaPath != "" {
		args = append(args, "--java", javaPath)
	}
	if xmxMB > 0 {
		args = append(args, "--xmx", fmt.Sprintf("%d", xmxMB))
	}
	return r.run(ctx, args, phaseFn)
}

// run executes the binary and parses JSON Lines from stdout.
func (r *Runner) run(ctx context.Context, args []string, phaseFn PhaseHandler) (json.RawMessage, error) {
	cmd := exec.CommandContext(ctx, r.BinPath, args...)
	// 取消/异常时兜底：启动器会派生 java 子进程并让它继承 stdout 管道，
	// 只杀启动器不会让管道 EOF，Wait 于是永远阻塞、任务卡在「启动中」。
	// WaitDelay 保证进程退出后最多再等 3 秒就强制关闭描述符。
	cmd.WaitDelay = 3 * time.Second // 必须早于 Start 设置
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	// stderr goes to the server log for debugging.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mpack-launcher: %w", err)
	}

	// Read stderr in background (tracing logs go there).
	var stderrMu sync.Mutex
	var stderrBuf []byte
	go func() {
		b, _ := io.ReadAll(stderr)
		stderrMu.Lock()
		stderrBuf = b
		stderrMu.Unlock()
	}()

	// Parse JSON Lines on its own goroutine: the scanner blocks on read, and a
	// blocking read cannot be interrupted when the task is canceled.
	events := make(chan scanResult, 1)
	go func() { events <- scanStdout(stdout, phaseFn) }()

	select {
	case ev := <-events:
		waitErr := cmd.Wait()
		if ev.err != nil {
			return nil, ev.err
		}
		if waitErr != nil {
			stderrMu.Lock()
			msg := string(stderrBuf)
			stderrMu.Unlock()
			return nil, fmt.Errorf("mpack-launcher exited: %w (stderr: %s)", waitErr, msg)
		}
		return ev.data, nil
	case <-ctx.Done():
		// 取消路径：杀进程 + Wait。Wait 会关闭父进程侧的管道描述符，扫描协程随之
		// 结束；如果游戏进程还持有写端（--wait 模式下 GameProcess::spawn 会继承
		// stdout），WaitDelay 保证最多 3 秒后强制收摊。
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		<-events
		return nil, fmt.Errorf("mpack-launcher 已取消: %w", ctx.Err())
	}
}

type scanResult struct {
	data json.RawMessage
	err  error
}

// scanStdout parses the launcher's JSON Lines protocol (launcherCore/src/protocol.rs):
// {"type":"phase",...} events stream out as they arrive and the single
// {"type":"result",...} line decides the task outcome.
func scanStdout(stdout io.Reader, phaseFn PhaseHandler) scanResult {
	var res scanResult
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			// Non-JSON line (e.g. cargo output in debug builds) — skip.
			continue
		}
		switch ev.Type {
		case "phase":
			if phaseFn != nil {
				phaseFn(ev.Phase, ev.Message)
			}
		case "result":
			if ev.Success {
				res.data = ev.Data
			} else {
				res.err = fmt.Errorf("%s: %s", ev.Error, ev.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return scanResult{err: fmt.Errorf("read stdout: %w", err)}
	}
	return res
}
