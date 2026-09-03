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
	"os/exec"
	"path/filepath"
	"sync"
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
}

// NewRunner creates a Runner that looks for the binary next to the server
// executable (under .tools/launcher/) or on PATH.
func NewRunner(workbenchRoot string) *Runner {
	candidate := filepath.Join(workbenchRoot, ".tools", "launcher", "mpack-launcher.exe")
	return &Runner{BinPath: candidate}
}

// Install runs `mpack-launcher install --version <version> --dir <dir> [--loader <loader>] [--mirror <mirror>]`.
// phaseFn receives each phase event as it arrives. Returns the final result data.
func (r *Runner) Install(ctx context.Context, version, minecraftDir string, loader, mirror string, phaseFn PhaseHandler) (json.RawMessage, error) {
	args := []string{"install", "--version", version, "--dir", minecraftDir}
	if loader != "" {
		args = append(args, "--loader", loader)
	}
	if mirror != "" {
		args = append(args, "--mirror", mirror)
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

	// Parse JSON Lines from stdout.
	var resultData json.RawMessage
	var resultErr error
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
				resultData = ev.Data
			} else {
				resultErr = fmt.Errorf("%s: %s", ev.Error, ev.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stdout: %w", err)
	}

	waitErr := cmd.Wait()
	if resultErr != nil {
		return nil, resultErr
	}
	if waitErr != nil {
		stderrMu.Lock()
		msg := string(stderrBuf)
		stderrMu.Unlock()
		return nil, fmt.Errorf("mpack-launcher exited: %w (stderr: %s)", waitErr, msg)
	}
	return resultData, nil
}
