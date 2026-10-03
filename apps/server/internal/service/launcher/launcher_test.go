// Package launcher_test — the Go side of the launcher process boundary.
//
// Why this file exists: mpack-launcher is a Rust binary and this machine has no
// cargo, so the only way to prove the exec plumbing and the JSON Lines protocol
// parser is to run a fake launcher script that speaks the same protocol. These
// tests cover the parser contract (phase/result/non-JSON/exit code), not the
// Rust implementation — that remains verified only against the protocol stub.
package launcher_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"mpackstation/internal/service/launcher"
)

const scriptTmpl = `#!/bin/sh
%s
`

// fakeLauncher writes an executable /bin/sh script that emits the given lines,
// in order, to stdout, then exits with code. stderr lines go to stderr.
func fakeLauncher(t *testing.T, stdout []string, stderr []string, code int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher uses /bin/sh")
	}
	var b strings.Builder
	for _, l := range stderr {
		b.WriteString("printf '%s\\n' " + shQuote(l) + " >&2\n")
	}
	for _, l := range stdout {
		b.WriteString("printf '%s\\n' " + shQuote(l) + "\n")
	}
	if code != 0 {
		b.WriteString("exit " + strconv.Itoa(code) + "\n")
	}
	path := filepath.Join(t.TempDir(), "mpack-launcher")
	if err := os.WriteFile(path, []byte(scriptTmpl+b.String()), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}
	return path
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func phaseLine(phase, message string) string {
	return `{"type":"phase","phase":"` + phase + `","message":"` + message + `"}`
}

func resultLine(data string) string {
	return `{"type":"result","success":true,"data":` + data + `}`
}

// TestInstall_StreamsPhasesThenResult: phases arrive in emission order and the
// result payload is returned verbatim.
func TestInstall_StreamsPhasesThenResult(t *testing.T) {
	launcherBin := fakeLauncher(t, []string{
		"not json at all — must be skipped",
		"",
		phaseLine("preparing", "准备版本目录"),
		resultLine(`{"version":"1.20.1","files":42}`),
	}, nil, 0)
	r := &launcher.Runner{BinPath: launcherBin, Available: true}

	var got [][2]string
	data, err := r.Install(context.Background(), "1.20.1", t.TempDir(), "fabric", "", "", "",
		func(phase, message string) { got = append(got, [2]string{phase, message}) })
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(got) != 1 || got[0][0] != "preparing" || got[0][1] != "准备版本目录" {
		t.Fatalf("phases = %v, want one preparing event", got)
	}
	var out struct {
		Version string `json:"version"`
		Files   int    `json:"files"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("result data %s: %v", data, err)
	}
	if out.Version != "1.20.1" || out.Files != 42 {
		t.Fatalf("result = %+v, want version=1.20.1 files=42", out)
	}
}

// TestInstall_FailureResultWinsOverExitCode: a success=false result must surface
// as an error even when the process exits 0 — otherwise a failed install would
// be reported to the UI as done.
func TestInstall_FailureResultWinsOverExitCode(t *testing.T) {
	bin := fakeLauncher(t, []string{
		phaseLine("downloading_assets", "开始下载"),
		`{"type":"result","success":false,"error":"version_not_found","message":"没有 1.99.0"}`,
	}, nil, 0)
	r := &launcher.Runner{BinPath: bin, Available: true}

	_, err := r.Install(context.Background(), "1.99.0", t.TempDir(), "", "", "", "", nil)
	if err == nil {
		t.Fatal("want error for success=false result, got nil")
	}
	if !strings.Contains(err.Error(), "version_not_found") || !strings.Contains(err.Error(), "没有 1.99.0") {
		t.Fatalf("err = %v, want it to carry the launcher error code and message", err)
	}
}

// TestRun_NonZeroExitWithoutResult: a crash must report the exit code plus
// stderr, so the task log has something actionable.
func TestRun_NonZeroExitWithoutResult(t *testing.T) {
	bin := fakeLauncher(t, []string{phaseLine("launching", "启动中")},
		[]string{"panic: java not found"}, 2)
	r := &launcher.Runner{BinPath: bin, Available: true}

	_, err := r.Launch(context.Background(), "1.20.1", t.TempDir(), "Player", "", 0, nil)
	if err == nil {
		t.Fatal("want error when the binary exits non-zero without a result")
	}
	if !strings.Contains(err.Error(), "java not found") {
		t.Fatalf("err = %v, want stderr included", err)
	}
}

// TestInstall_ForwardsMcFlag: 回归护栏 —— Rust 侧 install 子命令的必填旗标是 --mc
// （launcherCore/src/cli.rs InstallArgs），并且根本没有 --version。
// 之前 Go 发的是 `install --version`，真实二进制会被 clap 直接拒掉（退出码 2），
// 而协议桩不校验旗标，所以链路测试当时全绿 —— 这个用例锁住 argv。
func TestInstall_ForwardsMcFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher uses /bin/sh")
	}
	bin := filepath.Join(t.TempDir(), "mpack-launcher")
	joined := `printf '{"type":"result","success":true,"data":{"args":"%s"}}\n' "$*"`
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+joined+"\n"), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}
	r := &launcher.Runner{BinPath: bin, Available: true}

	data, err := r.Install(context.Background(), "1.20.1", "/tmp/mc", "fabric", "0.16.5", "bmclapi", "/opt/java/bin/java", nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	var out struct {
		Args string `json:"args"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("result data %s: %v", data, err)
	}
	for _, want := range []string{"install", "--mc", "1.20.1", "--dir", "/tmp/mc",
		"--loader", "fabric", "--loader-version", "0.16.5", "--mirror", "bmclapi",
		"--java", "/opt/java/bin/java"} {
		if !strings.Contains(out.Args, want) {
			t.Fatalf("argv = %q, missing %q", out.Args, want)
		}
	}
	if strings.Contains(out.Args, "--version") {
		t.Fatalf("argv = %q, install 不接受 --version（Rust 侧是 --mc）", out.Args)
	}
}

// TestLaunch_ForwardsArgs: the CLI flags the Rust binary expects must reach it,
// including the optional ones.
func TestLaunch_ForwardsArgs(t *testing.T) {
	// $@ goes into the result data, so the test can read back the argv.
	joined := `printf '{"type":"result","success":true,"data":{"args":"%s"}}\n' "$*"`
	bin := filepath.Join(t.TempDir(), "mpack-launcher")
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher uses /bin/sh")
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+joined+"\n"), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}
	r := &launcher.Runner{BinPath: bin, Available: true}

	dir := t.TempDir()
	data, err := r.Launch(context.Background(), "1.20.1", dir, "Player", "/usr/bin/java", 4096, nil)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	var out struct {
		Args string `json:"args"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("result data %s: %v", data, err)
	}
	for _, want := range []string{"launch", "--version", "1.20.1", "--dir", dir,
		"--username", "Player", "--java", "/usr/bin/java", "--xmx", "4096"} {
		if !strings.Contains(out.Args, want) {
			t.Fatalf("argv = %q, missing %q", out.Args, want)
		}
	}
}

// TestInstall_OptionalArgsOmitted: loader/mirror empty must not emit empty flags.
func TestInstall_OptionalArgsOmitted(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mpack-launcher")
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher uses /bin/sh")
	}
	joined := `printf '{"type":"result","success":true,"data":{"args":"%s"}}\n' "$*"`
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+joined+"\n"), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}
	r := &launcher.Runner{BinPath: bin, Available: true}

	data, err := r.Install(context.Background(), "1.20.1", t.TempDir(), "", "", "", "", nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if strings.Contains(string(data), "--loader") || strings.Contains(string(data), "--mirror") {
		t.Fatalf("argv = %s, want no empty optional flags", data)
	}
}

// TestRun_CanceledContext: the version manifest download can hang; canceling the
// task must not leave the child process writing into a closed pipe forever.
func TestRun_CanceledContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher uses /bin/sh")
	}
	bin := filepath.Join(t.TempDir(), "mpack-launcher")
	// 后台 sleep + wait：sh 不会被 dash 的末命令 exec 优化替换，杀掉 sh 之后
	// sleep 仍持有 stdout 写端 —— 这正是启动器派生 java 后的管道状态。
	script := "#!/bin/sh\n" + "printf '%s\\n' " + shQuote(phaseLine("downloading_assets", "开始下载")) +
		"\nsleep 30 &\nwait\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}
	r := &launcher.Runner{BinPath: bin, Available: true}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := r.Install(ctx, "1.20.1", t.TempDir(), "", "", "", "", nil)
	// WaitDelay=3s：取消后必须在秒级返回；修复前的阻塞读法会等到 sleep 自己结束（30s）。
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("cancel did not stop the run: took %s", took)
	}
	if err == nil {
		t.Fatal("want an error once the binary is killed")
	}
}

// TestNewRunner: env override wins, a real file makes the runner available, and
// a missing binary must report Available=false rather than pretend to work.
func TestNewRunner(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(t.TempDir(), "mpack-launcher")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}

	t.Setenv("MPACK_LAUNCHER_BIN", bin)
	if r := launcher.NewRunner(root); !r.Available || r.BinPath != bin {
		t.Fatalf("env override: got %+v, want %s", r, bin)
	}

	// Workbench-local copy under .tools/launcher/.
	t.Setenv("MPACK_LAUNCHER_BIN", "")
	local := filepath.Join(root, ".tools", "launcher", launcherName())
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(local, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write local launcher: %v", err)
	}
	if r := launcher.NewRunner(root); !r.Available || r.BinPath != local {
		t.Fatalf("local copy: got %+v, want %s", r, local)
	}

	// Nothing anywhere: Available=false, and BinPath still names what was missing.
	os.RemoveAll(local)
	r := launcher.NewRunner(root)
	if r.Available {
		t.Fatalf("want Available=false when no binary exists, got %+v", r)
	}
	if !strings.Contains(r.BinPath, "mpack-launcher") {
		t.Fatalf("BinPath = %q, want it to name the missing binary", r.BinPath)
	}
}

func launcherName() string {
	if runtime.GOOS == "windows" {
		return "mpack-launcher.exe"
	}
	return "mpack-launcher"
}
