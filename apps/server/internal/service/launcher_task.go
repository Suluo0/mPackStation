package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mpackstation/internal/service/launcher"
	"mpackstation/internal/task"
)

// LauncherInstallPayload is the task payload for KindLauncherInstall.
type LauncherInstallPayload struct {
	Version     string `json:"version"`
	Loader      string `json:"loader,omitempty"`
	Mirror      string `json:"mirror,omitempty"`
	MinecraftDir string `json:"minecraft_dir"`
}

// LauncherLaunchPayload is the task payload for KindLauncherLaunch.
type LauncherLaunchPayload struct {
	Version      string `json:"version"`
	Username     string `json:"username"`
	MinecraftDir string `json:"minecraft_dir"`
	JavaPath     string `json:"java_path,omitempty"`
	XmxMB        int    `json:"xmx_mb,omitempty"`
}

// phaseToProgress maps a launcher phase to a rough progress percentage.
// The binary emits phase events only (no progress bar), so we map phases
// to a monotonic sequence for the task UI.
var phaseProgress = map[string]float64{
	"preparing":           5,
	"resolving_version":   15,
	"downloading_libraries": 40,
	"downloading_assets":  70,
	"installing_loader":   80,
	"verifying":           90,
	"authenticating":      30,
	"await_user":          35,
	"authenticated":       40,
	"launching":           95,
}

// HandleLauncherInstallTask installs a Minecraft version via mPackLauncher.
func (a *API) HandleLauncherInstallTask(ctx context.Context, ex *task.Execution) error {
	var p LauncherInstallPayload
	if err := json.Unmarshal(ex.Task.Payload, &p); err != nil {
		return &task.TaskError{Code: "bad_payload", Message: err.Error()}
	}
	if p.Version == "" || p.MinecraftDir == "" {
		return &task.TaskError{Code: "bad_payload", Message: "version and minecraft_dir are required"}
	}

	runner := launcher.NewRunner(a.workbenchRoot())
	_ = ex.Progress(ctx, 2, "starting mpack-launcher install")

	// Heartbeat: downloads can be silent for tens of seconds.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = ex.Progress(ctx, 50, "downloading…")
			}
		}
	}()

	_, err := runner.Install(ctx, p.Version, p.MinecraftDir, p.Loader, p.Mirror,
		func(phase, message string) {
			if pct, ok := phaseProgress[phase]; ok {
				_ = ex.Progress(ctx, pct, message)
			} else {
				_ = ex.Progress(ctx, 50, message)
			}
		})
	close(done)

	if err != nil {
		return &task.TaskError{Code: "install_failed", Message: err.Error()}
	}
	_ = ex.Progress(ctx, 100, "install complete")
	return ex.Succeed(ctx, fmt.Sprintf("version %s installed", p.Version))
}

// HandleLauncherLaunchTask launches Minecraft via mPackLauncher.
func (a *API) HandleLauncherLaunchTask(ctx context.Context, ex *task.Execution) error {
	var p LauncherLaunchPayload
	if err := json.Unmarshal(ex.Task.Payload, &p); err != nil {
		return &task.TaskError{Code: "bad_payload", Message: err.Error()}
	}
	if p.Version == "" || p.Username == "" || p.MinecraftDir == "" {
		return &task.TaskError{Code: "bad_payload", Message: "version, username and minecraft_dir are required"}
	}

	runner := launcher.NewRunner(a.workbenchRoot())
	_ = ex.Progress(ctx, 10, "starting mpack-launcher launch")

	result, err := runner.Launch(ctx, p.Version, p.MinecraftDir, p.Username, p.JavaPath, p.XmxMB,
		func(phase, message string) {
			if pct, ok := phaseProgress[phase]; ok {
				_ = ex.Progress(ctx, pct, message)
			}
		})
	if err != nil {
		return &task.TaskError{Code: "launch_failed", Message: err.Error()}
	}

	// Parse PID from result.
	var info struct {
		PID int `json:"pid"`
	}
	_ = json.Unmarshal(result, &info)
	_ = ex.Progress(ctx, 100, fmt.Sprintf("game launched (pid %d)", info.PID))
	return ex.Succeed(ctx, fmt.Sprintf("game launched (pid %d)", info.PID))
}


// SubmitLauncherInstall queues a launcher install task.
func (a *API) SubmitLauncherInstall(ctx context.Context, p LauncherInstallPayload) (*task.Task, bool, error) {
	if a.queue == nil {
		return nil, false, &DomainError{Status: 503, Code: "task_queue_unavailable", Message: "task queue not configured"}
	}
	payload, _ := json.Marshal(p)
	t, _, err := a.queue.Submit(ctx, task.SubmitRequest{
		Kind:        task.KindLauncherInstall,
		Title:       fmt.Sprintf("Install Minecraft %s", p.Version),
		Payload:     json.RawMessage(payload),
		MaxAttempts: 1,
	})
	return t, false, err
}

// SubmitLauncherLaunch queues a launcher launch task.
func (a *API) SubmitLauncherLaunch(ctx context.Context, p LauncherLaunchPayload) (*task.Task, bool, error) {
	if a.queue == nil {
		return nil, false, &DomainError{Status: 503, Code: "task_queue_unavailable", Message: "task queue not configured"}
	}
	payload, _ := json.Marshal(p)
	t, _, err := a.queue.Submit(ctx, task.SubmitRequest{
		Kind:        task.KindLauncherLaunch,
		Title:       fmt.Sprintf("Launch Minecraft %s", p.Version),
		Payload:     json.RawMessage(payload),
		MaxAttempts: 1,
	})
	return t, false, err
}
