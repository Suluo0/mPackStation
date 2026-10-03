package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"mpackstation/internal/service/launcher"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

// LauncherInstallPayload is the task payload for KindLauncherInstall.
type LauncherInstallPayload struct {
	Version string `json:"version"`
	// PackID 只是发起上下文（哪个包要这个版本），安装本身按目录进行。
	PackID string `json:"pack_id,omitempty"`
	Loader string `json:"loader,omitempty"`
	// ArtifactID 是构建产物（kind='mrpack'）的 id：给了它就按整合包导入安装，
	// mc/loader 版本与模组清单来自包内 manifest，p.Version 可省略。
	// 只接受 id 而不接受路径，是为了让「装哪个包」可追溯到库里的记录，
	// 而不是让调用方拿任意文件系统路径当输入。
	ArtifactID    string `json:"artifact_id,omitempty"`
	LoaderVersion string `json:"loader_version,omitempty"`
	Mirror        string `json:"mirror,omitempty"`
	MinecraftDir  string `json:"minecraft_dir"`
	JavaPath      string `json:"java_path,omitempty"`
}

// LauncherLaunchPayload is the task payload for KindLauncherLaunch.
type LauncherLaunchPayload struct {
	// Version 可以留空：留空时 SubmitLauncherLaunch 从 launcher_installs 解析该目录
	// 实际装出来的版本目录 ID（见 migrations/0022）。前端不再靠猜 mc 版本号。
	Version      string `json:"version,omitempty"`
	PackID       string `json:"pack_id,omitempty"`
	Username     string `json:"username"`
	MinecraftDir string `json:"minecraft_dir"`
	JavaPath     string `json:"java_path,omitempty"`
	XmxMB        int    `json:"xmx_mb,omitempty"`
}

// phaseToProgress maps a launcher phase to a rough progress percentage.
// The binary emits phase events only (no progress bar), so we map phases
// to a monotonic sequence for the task UI.
var phaseProgress = map[string]float64{
	"preparing":             5,
	"resolving_version":     15,
	"downloading_libraries": 40,
	"downloading_assets":    70,
	"installing_loader":     80,
	"verifying":             90,
	"authenticating":        30,
	"await_user":            35,
	"authenticated":         40,
	"launching":             95,
}

// HandleLauncherInstallTask installs a Minecraft version via mPackLauncher.
func (a *API) HandleLauncherInstallTask(ctx context.Context, ex *task.Execution) error {
	var p LauncherInstallPayload
	if err := json.Unmarshal(ex.Task.Payload, &p); err != nil {
		return &task.TaskError{Code: "bad_payload", Message: err.Error()}
	}
	if (p.Version == "" && p.ArtifactID == "") || p.MinecraftDir == "" {
		return &task.TaskError{Code: "bad_payload", Message: "version (or artifact_id) and minecraft_dir are required"}
	}

	runner := launcher.NewRunner(a.workbenchRoot())
	if !runner.Available {
		return &task.TaskError{Code: "launcher_binary_missing", Message: "mpack-launcher 未安装,找不到二进制: " + runner.BinPath}
	}
	// 按包安装：制品路径在任务开始时再查一次库，避免「入队后产物被删」时空转。
	var mrpackPath string
	if p.ArtifactID != "" {
		art, aerr := a.artifactForInstall(ctx, p)
		if aerr != nil {
			return &task.TaskError{Code: "install_artifact_unavailable", Message: aerr.Error()}
		}
		mrpackPath = art.Path
	}
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

	onPhase := func(phase, message string) {
		if pct, ok := phaseProgress[phase]; ok {
			_ = ex.Progress(ctx, pct, message)
		} else {
			_ = ex.Progress(ctx, 50, message)
		}
	}
	var (
		installed json.RawMessage
		err       error
	)
	if mrpackPath != "" {
		installed, err = runner.InstallFromMrpack(ctx, mrpackPath, p.MinecraftDir, p.Mirror, p.JavaPath, onPhase)
	} else {
		installed, err = runner.Install(ctx, p.Version, p.MinecraftDir, p.Loader, p.LoaderVersion, p.Mirror, p.JavaPath, onPhase)
	}
	close(done)

	if err != nil {
		return &task.TaskError{Code: "install_failed", Message: err.Error()}
	}
	// 把内核返回的版本目录 ID 落到库里：启动要用它，而不是包的 mc_version。
	// 之前这里是 `_, err :=`，结果整包丢弃，「装完就能启动」在真实二进制上必然失败。
	versionID := installedVersionID(installed, p.Version)
	if _, rerr := a.recordLauncherInstall(ctx, p, versionID, ex.Task.ID); rerr != nil {
		return &task.TaskError{Code: "install_record_failed", Message: rerr.Error()}
	}
	_ = ex.Progress(ctx, 100, "install complete")
	return ex.Succeed(ctx, fmt.Sprintf("version %s installed", versionID))
}

// artifactForInstall resolves the build artifact an install task must consume and
// checks it is actually an .mrpack of the requesting pack. 校验放在同步侧（提交时）
// 与异步侧各一次：提交时能拒掉「拿别人包的产物/拿 zip 老产物装」的请求。
func (a *API) artifactForInstall(ctx context.Context, p LauncherInstallPayload) (store.ArtifactRecord, error) {
	if a.repo == nil {
		return store.ArtifactRecord{}, &DomainError{Status: 503, Code: "database_unavailable", Message: "repository not configured"}
	}
	art, err := a.repo.GetArtifact(ctx, p.ArtifactID)
	if err != nil {
		return store.ArtifactRecord{}, &DomainError{Status: 404, Code: "artifact_not_found",
			Message: fmt.Sprintf("构建产物不存在: %s", p.ArtifactID), Wrapped: err}
	}
	if art.Kind != "mrpack" {
		return store.ArtifactRecord{}, &DomainError{Status: 422, Code: "install_artifact_not_mrpack",
			Message: fmt.Sprintf("该产物不是 .mrpack（kind=%s），不能按整合包导入安装", art.Kind)}
	}
	if art.Status != "ready" {
		return store.ArtifactRecord{}, &DomainError{Status: 409, Code: "install_artifact_not_ready",
			Message: "构建产物状态不是 ready: " + art.Status}
	}
	if p.PackID != "" && art.PackID != p.PackID {
		return store.ArtifactRecord{}, &DomainError{Status: 409, Code: "install_artifact_pack_mismatch",
			Message: fmt.Sprintf("产物属于包 %s，不属于请求里的包 %s", art.PackID, p.PackID)}
	}
	if strings.TrimSpace(art.Path) == "" {
		return store.ArtifactRecord{}, &DomainError{Status: 422, Code: "install_artifact_path_missing",
			Message: "构建产物没有落盘路径记录"}
	}
	if _, statErr := os.Stat(art.Path); statErr != nil {
		return store.ArtifactRecord{}, &DomainError{Status: 409, Code: "install_artifact_file_missing",
			Message: "产物文件不在磁盘上: " + art.Path, Wrapped: statErr}
	}
	return art, nil
}

// installedVersionID reads the version_id field of the kernel result, falling
// back to the requested MC version when a kernel (or protocol stub) omits it.
func installedVersionID(result json.RawMessage, fallback string) string {
	var data struct {
		VersionID string `json:"version_id"`
	}
	if len(result) > 0 {
		_ = json.Unmarshal(result, &data)
	}
	if strings.TrimSpace(data.VersionID) != "" {
		return data.VersionID
	}
	return fallback
}

func (a *API) recordLauncherInstall(ctx context.Context, p LauncherInstallPayload, versionID, taskID string) (store.LauncherInstallRecord, error) {
	rec := store.LauncherInstallRecord{
		ID: newID("linstall"), MinecraftDir: p.MinecraftDir, VersionID: versionID,
		Loader: p.Loader, MCVersion: p.Version, TaskID: taskID, InstalledAt: a.nowMillis(),
	}
	if p.PackID != "" {
		rec.PackID = sql.NullString{String: p.PackID, Valid: true}
	}
	if a.repo == nil {
		return rec, nil
	}
	return a.repo.RecordLauncherInstall(ctx, rec)
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
	if !runner.Available {
		return &task.TaskError{Code: "launcher_binary_missing", Message: "mpack-launcher 未安装,找不到二进制: " + runner.BinPath}
	}
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

// launcherPreflight is the synchronous gate in front of both launcher submits.
// The worker validated the payload long after the API had already answered 202,
// so a mistyped request looked accepted and only surfaced later as a failed
// task; and when no mpack-launcher binary exists every task died on
// "fork/exec …: no such file or directory". Refuse those up front instead of
// queueing work that cannot succeed.
func (a *API) launcherPreflight(minecraftDir string) error {
	if strings.TrimSpace(minecraftDir) == "" {
		return &DomainError{Status: 400, Code: "invalid_argument", Message: "minecraft_dir is required"}
	}
	if !launcher.NewRunner(a.workbenchRoot()).Available {
		return &DomainError{Status: 503, Code: "launcher_binary_missing",
			Message: "mpack-launcher 未安装:设置 MPACK_LAUNCHER_BIN 或把二进制放到 <workbench>/.tools/launcher/"}
	}
	return nil
}

// SubmitLauncherInstall queues a launcher install task.
func (a *API) SubmitLauncherInstall(ctx context.Context, p LauncherInstallPayload) (*task.Task, bool, error) {
	if a.queue == nil {
		return nil, false, &DomainError{Status: 503, Code: "task_queue_unavailable", Message: "task queue not configured"}
	}
	if err := a.launcherPreflight(p.MinecraftDir); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(p.Version) == "" && strings.TrimSpace(p.ArtifactID) == "" {
		return nil, false, &DomainError{Status: 400, Code: "invalid_argument",
			Message: "version 与 artifact_id 至少要有一个（后者走整合包导入）"}
	}
	if strings.TrimSpace(p.ArtifactID) != "" {
		if _, err := a.artifactForInstall(ctx, p); err != nil {
			return nil, false, err
		}
		// 按包安装时 mc 版本以 manifest 为准；这里补上包的版本只为让任务标题可读。
		if strings.TrimSpace(p.Version) == "" && strings.TrimSpace(p.PackID) != "" && a.repo != nil {
			if rec, err := a.repo.GetPack(ctx, p.PackID); err == nil {
				p.Version = rec.MCVersion
			}
		}
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
	if err := a.launcherPreflight(p.MinecraftDir); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(p.Username) == "" {
		return nil, false, &DomainError{Status: 400, Code: "invalid_argument", Message: "username is required to launch"}
	}
	if err := a.resolveLaunchVersion(ctx, &p); err != nil {
		return nil, false, err
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

// resolveLaunchVersion turns an omitted launch version into the version the
// kernel actually installed into that directory. 入队前就定下来，任务载荷里
// 永远是具体版本目录 ID —— 任务是可复现的。
func (a *API) resolveLaunchVersion(ctx context.Context, p *LauncherLaunchPayload) error {
	if strings.TrimSpace(p.Version) != "" {
		return nil
	}
	if a.repo == nil {
		return &DomainError{Status: 503, Code: "database_unavailable", Message: "repository not configured"}
	}
	rec, err := a.repo.LatestLauncherInstall(ctx, p.PackID, p.MinecraftDir)
	if err != nil {
		return &DomainError{Status: 409, Code: "launcher_not_installed",
			Message: fmt.Sprintf("该目录里还没有装好的 Minecraft 版本，请先执行安装: %s", p.MinecraftDir), Wrapped: err}
	}
	p.Version = rec.VersionID
	return nil
}
