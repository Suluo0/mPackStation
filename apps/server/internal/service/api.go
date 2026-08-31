// Package service contains the application use-cases exposed by HTTP. It is
// deliberately independent of transport details; all SQL is delegated to
// store.Repository.
package service

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"database/sql"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

var ErrUnavailable = errors.New("service unavailable")

// ErrInvalidArgument is returned when a request cannot satisfy a domain
// invariant. The HTTP layer maps it to the stable invalid_argument code.
var ErrInvalidArgument = errors.New("invalid argument")

// IsNotFound and IsConflict keep transport mapping independent from the store
// package while preserving stable domain error semantics.
func IsNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
func IsConflict(err error) bool { return errors.Is(err, store.ErrConflict) }

// API is the stable application boundary consumed by httpapi.
type API struct {
	repo    *store.Repository
	now     func() time.Time
	dataDir string
	queue   *task.Queue
}

// SetTaskQueue injects the process queue used for tool-install submissions.
func (a *API) SetTaskQueue(q *task.Queue) {
	if a != nil {
		a.queue = q
	}
}

// New creates the local single-instance service.
func New(db *sql.DB) *API {
	api := &API{now: time.Now}
	if db == nil {
		return api
	}
	api.repo = store.NewRepository(db)
	if dir, err := api.repo.DatabaseDir(context.Background()); err == nil {
		api.dataDir = dir
	}
	return api
}

func (a *API) ready() error {
	if a == nil || a.repo == nil {
		return ErrUnavailable
	}
	return nil
}

type Dashboard struct {
	Packs              []DashboardPack `json:"packs"`
	LastEditedPackID   *string         `json:"lastEditedPackId"`
	TodayResolvedCount int             `json:"todayResolvedCount"`
}
type DashboardPack struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	IconURL     *string `json:"iconUrl"`
	MCVersion   string  `json:"mcVersion"`
	Loader      string  `json:"loader"`
	PackVersion string  `json:"packVersion"`
	ModCount    struct {
		Total     int `json:"total"`
		Installed int `json:"installed"`
		Selected  int `json:"selected"`
	} `json:"modCount"`
	Conflicts struct {
		Resolved int `json:"resolved"`
		Pending  int `json:"pending"`
	} `json:"conflicts"`
	Edits struct {
		Recipes    int `json:"recipes"`
		Structures int `json:"structures"`
		Ores       int `json:"ores"`
		Quests     int `json:"quests"`
	} `json:"edits"`
	Alerts struct {
		Crashes   int `json:"crashes"`
		Updatable int `json:"updatable"`
	} `json:"alerts"`
	LastEditedAt string `json:"lastEditedAt"`
	CreatedAt    string `json:"createdAt"`
}

type Pack struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	IconURL       *string `json:"iconUrl"`
	MCVersion     string  `json:"mcVersion"`
	Loader        string  `json:"loader"`
	LoaderVersion *string `json:"loaderVersion"`
	Description   *string `json:"description"`
	Status        string  `json:"status"`
	PackVersion   string  `json:"packVersion"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

type CreatePackInput struct {
	Name          string `json:"name"`
	MCVersion     string `json:"mcVersion"`
	Loader        string `json:"loader"`
	LoaderVersion string `json:"loaderVersion"`
	Description   string `json:"description"`
}
type UpdatePackInput struct {
	Name          *string `json:"name"`
	MCVersion     *string `json:"mcVersion"`
	Loader        *string `json:"loader"`
	LoaderVersion *string `json:"loaderVersion"`
	Description   *string `json:"description"`
}

type Activity struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Text   string  `json:"text"`
	PackID *string `json:"packId"`
	At     string  `json:"at"`
}
type SystemHealth struct {
	CurseForgeKeyConfigured bool   `json:"curseforgeKeyConfigured"`
	ModrinthReachable       bool   `json:"modrinthReachable"`
	CurseForgeReachable     bool   `json:"curseforgeReachable"`
	ModrinthStatus          string `json:"modrinthStatus"`
	CurseForgeStatus        string `json:"curseforgeStatus"`
	StorageWritable         bool   `json:"storageWritable"`
	StorageFreeBytes        int64  `json:"storageFreeBytes"`
}
type SystemStatus struct {
	ModrinthReachable   bool   `json:"modrinthReachable"`
	CurseForgeReachable bool   `json:"curseforgeReachable"`
	ModrinthStatus      string `json:"modrinthStatus"`
	CurseForgeStatus    string `json:"curseforgeStatus"`
	CacheSizeBytes      int64  `json:"cacheSizeBytes"`
	StorageFreeBytes    int64  `json:"storageFreeBytes"`
}
type Onboarding struct {
	Steps struct {
		CurseForgeKey bool `json:"curseforgeKey"`
		FirstPack     bool `json:"firstPack"`
		FirstMod      bool `json:"firstMod"`
		PrismAccount  bool `json:"prismAccount"`
	} `json:"steps"`
}

func (a *API) Dashboard(ctx context.Context) (Dashboard, error) {
	if err := a.ready(); err != nil {
		return Dashboard{}, err
	}
	rows, err := a.repo.DashboardPacks(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	out := Dashboard{Packs: make([]DashboardPack, 0, len(rows))}
	var max int64
	for _, p := range rows {
		d := DashboardPack{ID: p.ID, Name: p.Name, MCVersion: p.MCVersion, Loader: normalizeLoader(p.Loader), PackVersion: p.Version, LastEditedAt: iso(p.LastEditedAt), CreatedAt: iso(p.CreatedAt)}
		if p.IconPath != "" {
			v := p.IconPath
			d.IconURL = &v
		}
		d.ModCount.Total = p.ModTotal
		d.ModCount.Installed = p.ModInstalled
		d.ModCount.Selected = p.ModSelected
		d.Conflicts.Resolved = p.ConflictsResolved
		d.Conflicts.Pending = p.ConflictsPending
		d.Edits.Recipes = p.Recipes
		d.Edits.Structures = p.Structures
		d.Edits.Ores = p.Ores
		d.Edits.Quests = p.Quests
		d.Alerts.Crashes = p.Crashes
		d.Alerts.Updatable = p.Updatable
		out.Packs = append(out.Packs, d)
		if p.LastEditedAt > max {
			max = p.LastEditedAt
			id := p.ID
			out.LastEditedPackID = &id
		}
	}
	start := a.now().In(time.Local)
	y, m, day := start.Date()
	midnight := time.Date(y, m, day, 0, 0, 0, 0, time.Local)
	out.TodayResolvedCount, err = a.repo.TodayResolvedCount(ctx, midnight.UnixMilli())
	if err != nil {
		return Dashboard{}, err
	}
	return out, nil
}

func (a *API) ListPacks(ctx context.Context) ([]Pack, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListPacks(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]Pack, 0, len(rows))
	for _, p := range rows {
		out = append(out, a.pack(p, "0.1.0"))
	}
	return out, nil
}
func (a *API) GetPack(ctx context.Context, id string) (Pack, error) {
	if err := a.ready(); err != nil {
		return Pack{}, err
	}
	p, err := a.repo.GetPack(ctx, id)
	if err != nil {
		return Pack{}, err
	}
	return a.pack(p, "0.1.0"), nil
}

func (a *API) CreatePack(ctx context.Context, input CreatePackInput, requestID string) (Pack, error) {
	if err := a.ready(); err != nil {
		return Pack{}, err
	}
	if err := validateCreate(input); err != nil {
		return Pack{}, err
	}
	now := a.now().UnixMilli()
	id := newID("pack")
	verID := newID("packver")
	p := store.PackRecord{ID: id, Name: strings.TrimSpace(input.Name), MCVersion: strings.TrimSpace(input.MCVersion), Loader: input.Loader, LoaderVersion: strings.TrimSpace(input.LoaderVersion), Description: input.Description, Status: "active", CreatedAt: now, UpdatedAt: now, LastEditedAt: now}
	v := store.PackVersionRecord{ID: verID, PackID: id, Version: "0.1.0", Channel: "draft", Source: "manual", CreatedAt: now, UpdatedAt: now}
	err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.CreatePack(ctx, p, v); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: id, Kind: "pack", Action: "create", Text: fmt.Sprintf("创建了整合包「%s」", p.Name), At: now}, map[string]any{"name": p.Name}, requestID); err != nil {
			return err
		}
		if err := tx.AddOutbox(ctx, newID("event"), id, "pack", id, "pack.created", map[string]any{"packId": id}, now); err != nil {
			return err
		}
		return tx.AddAudit(ctx, newID("audit"), id, "pack.create", requestID, map[string]any{"name": p.Name}, now)
	})
	if err != nil {
		if IsConflict(err) {
			return Pack{}, &DomainError{Status: 422, Code: "pack_name_duplicate", Message: "a pack with this name already exists", Wrapped: err}
		}
		return Pack{}, err
	}
	return a.pack(p, v.Version), nil
}

func (a *API) UpdatePack(ctx context.Context, id string, input UpdatePackInput, requestID string) (Pack, error) {
	if err := a.ready(); err != nil {
		return Pack{}, err
	}
	p, err := a.repo.GetPack(ctx, id)
	if err != nil {
		return Pack{}, err
	}
	if input.Name != nil {
		p.Name = strings.TrimSpace(*input.Name)
	}
	if input.MCVersion != nil {
		p.MCVersion = strings.TrimSpace(*input.MCVersion)
	}
	if input.Loader != nil {
		p.Loader = *input.Loader
	}
	if input.LoaderVersion != nil {
		p.LoaderVersion = strings.TrimSpace(*input.LoaderVersion)
	}
	if input.Description != nil {
		p.Description = *input.Description
	}
	if err := validatePack(p); err != nil {
		return Pack{}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	p.UpdatedAt = a.now().UnixMilli()
	p.LastEditedAt = p.UpdatedAt
	err = a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpdatePack(ctx, p); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: id, Kind: "pack", Action: "edit", Text: fmt.Sprintf("编辑了整合包「%s」", p.Name), At: p.UpdatedAt}, nil, requestID); err != nil {
			return err
		}
		if err := tx.AddOutbox(ctx, newID("event"), id, "pack", id, "pack.updated", map[string]any{"packId": id}, p.UpdatedAt); err != nil {
			return err
		}
		return tx.AddAudit(ctx, newID("audit"), id, "pack.update", requestID, nil, p.UpdatedAt)
	})
	if err != nil {
		if IsConflict(err) {
			return Pack{}, &DomainError{Status: 422, Code: "pack_name_duplicate", Message: "a pack with this name already exists", Wrapped: err}
		}
		return Pack{}, err
	}
	return a.pack(p, "0.1.0"), nil
}

func (a *API) ArchivePack(ctx context.Context, id, requestID string) (Pack, error) {
	if err := a.ready(); err != nil {
		return Pack{}, err
	}
	p, err := a.repo.GetPack(ctx, id)
	if err != nil {
		return Pack{}, err
	}
	at := a.now().UnixMilli()
	err = a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.SetPackStatus(ctx, id, "archived", at); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: id, Kind: "pack", Action: "archive", Text: fmt.Sprintf("归档了整合包「%s」", p.Name), At: at}, nil, requestID); err != nil {
			return err
		}
		if err := tx.AddOutbox(ctx, newID("event"), id, "pack", id, "pack.archived", map[string]any{"packId": id}, at); err != nil {
			return err
		}
		return tx.AddAudit(ctx, newID("audit"), id, "pack.archive", requestID, nil, at)
	})
	if err != nil {
		return Pack{}, err
	}
	p.Status = "archived"
	p.UpdatedAt = at
	p.LastEditedAt = at
	return a.pack(p, "0.1.0"), nil
}

// UnarchivePack restores an archived pack to the active dashboard scope.
func (a *API) UnarchivePack(ctx context.Context, id, requestID string) (Pack, error) {
	if err := a.ready(); err != nil {
		return Pack{}, err
	}
	p, err := a.repo.GetPack(ctx, id)
	if err != nil {
		return Pack{}, err
	}
	at := a.now().UnixMilli()
	err = a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.SetPackStatus(ctx, id, "active", at); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: id, Kind: "pack", Action: "edit", Text: fmt.Sprintf("恢复了整合包「%s」", p.Name), At: at}, nil, requestID); err != nil {
			return err
		}
		if err := tx.AddOutbox(ctx, newID("event"), id, "pack", id, "pack.unarchived", map[string]any{"packId": id}, at); err != nil {
			return err
		}
		return tx.AddAudit(ctx, newID("audit"), id, "pack.unarchive", requestID, nil, at)
	})
	if err != nil {
		return Pack{}, err
	}
	p.Status = "active"
	p.UpdatedAt = at
	p.LastEditedAt = at
	return a.pack(p, "0.1.0"), nil
}
func (a *API) DeletePack(ctx context.Context, id, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	p, err := a.repo.GetPack(ctx, id)
	if err != nil {
		return err
	}
	at := a.now().UnixMilli()
	return a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.DeletePack(ctx, id); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), Kind: "pack", Action: "delete", Text: fmt.Sprintf("删除了整合包「%s」", p.Name), At: at}, nil, requestID); err != nil {
			return err
		}
		if err := tx.AddOutbox(ctx, newID("event"), "", "pack", id, "pack.deleted", map[string]any{"packId": id}, at); err != nil {
			return err
		}
		return tx.AddAudit(ctx, newID("audit"), "", "pack.delete", requestID, nil, at)
	})
}

func (a *API) ListTasks(ctx context.Context, limit int) ([]TaskView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := a.repo.ListTasks(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TaskView, 0, len(rows))
	for _, t := range rows {
		var packID, packName *string
		if t.PackID != "" {
			v := t.PackID
			packID = &v
		}
		if t.PackName != "" {
			v := t.PackName
			packName = &v
		}
		var msg *string
		if t.ErrorMessage != "" {
			v := t.ErrorMessage
			msg = &v
		}
		var started, finished *time.Time
		if t.StartedAt.Valid {
			v := time.UnixMilli(t.StartedAt.Int64).UTC()
			started = &v
		}
		if t.FinishedAt.Valid {
			v := time.UnixMilli(t.FinishedAt.Int64).UTC()
			finished = &v
		}
		out = append(out, TaskView{ID: t.ID, Type: publicKind(task.Kind(t.Kind)), Title: t.Title, PackID: packID, PackName: packName, Status: publicStatus(task.Status(t.Status)), Progress: progressPercent(t.Progress), Error: msg, StartedAt: started, FinishedAt: finished})
	}
	return out, nil
}

func (a *API) ListActivities(ctx context.Context, limit int) ([]Activity, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := a.repo.ListActivities(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(rows))
	for _, r := range rows {
		kind := activityKind(r.Kind, r.Action)
		var pid *string
		if r.PackID != "" {
			v := r.PackID
			pid = &v
		}
		out = append(out, Activity{ID: r.ID, Kind: kind, Text: r.Text, PackID: pid, At: iso(r.At)})
	}
	return out, nil
}

func activityKind(kind, action string) string {
	switch {
	case kind == "mod" && action == "add":
		return "add-mod"
	case kind == "conflict" || action == "resolve":
		return "resolve"
	case kind == "build" || action == "build":
		return "build"
	case kind == "content" || kind == "quest" || kind == "pack":
		return "edit"
	case kind == "task" && action == "import", kind == "import":
		return "import"
	default:
		return "alert"
	}
}

func (a *API) SystemHealth(ctx context.Context) (SystemHealth, error) {
	if err := a.ready(); err != nil {
		return SystemHealth{}, err
	}
	s, err := a.repo.System(ctx)
	if err != nil {
		return SystemHealth{}, err
	}
	w, f := storageInfo(a.dataDir)
	return SystemHealth{CurseForgeKeyConfigured: s.CurseForgeKeyConfigured, ModrinthReachable: s.ModrinthReachable, CurseForgeReachable: s.CurseForgeReachable, ModrinthStatus: s.ModrinthStatus, CurseForgeStatus: s.CurseForgeStatus, StorageWritable: w, StorageFreeBytes: f}, nil
}
func (a *API) SystemStatus(ctx context.Context) (SystemStatus, error) {
	if err := a.ready(); err != nil {
		return SystemStatus{}, err
	}
	s, err := a.repo.System(ctx)
	if err != nil {
		return SystemStatus{}, err
	}
	_, f := storageInfo(a.dataDir)
	return SystemStatus{ModrinthReachable: s.ModrinthReachable, CurseForgeReachable: s.CurseForgeReachable, ModrinthStatus: s.ModrinthStatus, CurseForgeStatus: s.CurseForgeStatus, CacheSizeBytes: s.CacheSizeBytes, StorageFreeBytes: f}, nil
}
func (a *API) Onboarding(ctx context.Context) (Onboarding, error) {
	if err := a.ready(); err != nil {
		return Onboarding{}, err
	}
	o, err := a.repo.Onboarding(ctx)
	if err != nil {
		return Onboarding{}, err
	}
	var out Onboarding
	out.Steps.CurseForgeKey = o.CurseForgeKey
	out.Steps.FirstPack = o.FirstPack
	out.Steps.FirstMod = o.FirstMod
	out.Steps.PrismAccount = a.prismAccountReady()
	return out, nil
}

// workbenchRoot anchors every tool path: data dir lives at <root>/data, so
// the root is its parent. All tool paths stay relative to this root, which
// keeps the whole workbench portable after distribution.
func (a *API) workbenchRoot() string {
	if a.dataDir == "" {
		return ""
	}
	return filepath.Dir(a.dataDir)
}

func (a *API) prismExe() string {
	root := a.workbenchRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".tools", "prism", "prismlauncher.exe")
}

// prismDataDir is the portable Prism root passed via -d on every launch, so
// instances and the logged-in account travel with the workbench directory.
func (a *API) prismDataDir() string {
	root := a.workbenchRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".tools", "prism-data")
}

// prismAccountReady is a live file check: Prism stores the Microsoft account
// in accounts.json under its -d root after the user logs in once.
func (a *API) prismAccountReady() bool {
	dir := a.prismDataDir()
	if dir == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Join(dir, "accounts.json"))
	if err != nil {
		return false
	}
	var acc struct {
		Accounts []json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(b, &acc); err != nil {
		return false
	}
	return len(acc.Accounts) > 0
}

// SubmitPrismInstall enqueues the installer as a first-class task; the task
// log is the source of truth for success. A currently-active install is
// rejected with 409 task_invalid_transition per contract; a finished one does
// not block reinstall after deletion.
func (a *API) SubmitPrismInstall(ctx context.Context) (*task.Task, bool, error) {
	if err := a.ready(); err != nil {
		return nil, false, err
	}
	if a.queue == nil {
		return nil, false, ErrUnavailable
	}
	if a.prismExeExists() {
		return nil, false, nil
	}
	tasks, err := a.queue.List(ctx, 50)
	if err != nil {
		return nil, false, err
	}
	for _, t := range tasks {
		if t.Kind != task.KindToolInstall {
			continue
		}
		switch t.Status {
		case task.StatusQueued, task.StatusLeased, task.StatusRunning, task.StatusPaused:
			return nil, false, &DomainError{Status: 409, Code: "task_invalid_transition", Message: "a prism install task is already active", Details: map[string]any{"taskId": t.ID}}
		}
	}
	t, _, err := a.queue.Submit(ctx, task.SubmitRequest{
		Kind: task.KindToolInstall, Title: "Install Prism Launcher (portable)",
		Payload: json.RawMessage(`{"tool":"prism"}`), MaxAttempts: 1,
	})
	return t, false, err
}

func (a *API) prismExeExists() bool {
	exe := a.prismExe()
	if exe == "" {
		return false
	}
	st, err := os.Stat(exe)
	return err == nil && !st.IsDir()
}

// HandleToolInstallTask runs scripts/prism-install.bat, streaming installer
// output into the task event log, and succeeds only if the exe exists after.
// The task lease is 30s while downloads are silent, so a heartbeat keeps
// Progress flowing and an early-exit makes a recovered rerun harmless.
func (a *API) HandleToolInstallTask(ctx context.Context, ex *task.Execution) error {
	if a.prismExeExists() {
		return ex.Succeed(ctx, "prismlauncher.exe already present under .tools/prism")
	}
	root := a.workbenchRoot()
	if root == "" {
		return &task.TaskError{Code: "no_root", Message: "workbench root unknown"}
	}
	script := filepath.Join(root, "scripts", "prism-install.bat")
	if _, err := os.Stat(script); err != nil {
		return &task.TaskError{Code: "installer_missing", Message: "scripts/prism-install.bat not found"}
	}
	_ = ex.Progress(ctx, 5, "starting prism-install.bat")
	cmd := exec.CommandContext(ctx, "cmd.exe", "/c", script)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &task.TaskError{Code: "pipe", Message: err.Error()}
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return &task.TaskError{Code: "start_failed", Message: err.Error()}
	}
	// Heartbeat: downloads are silent for tens of seconds; the lease (30s)
	// dies without a Progress call and the task would be recovered mid-run.
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
				_ = ex.Progress(ctx, 50, "installer running…")
			}
		}
	}()
	scanner := bufio.NewScanner(stdout)
	last := time.Now()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Throttle: one output event per second keeps the log readable.
		if time.Since(last) >= time.Second {
			_ = ex.Progress(ctx, 50, line)
			last = time.Now()
		}
	}
	close(done)
	if err := cmd.Wait(); err != nil {
		return &task.TaskError{Code: "install_failed", Message: fmt.Sprintf("installer exited: %v", err)}
	}
	if !a.prismExeExists() {
		return &task.TaskError{Code: "exe_missing", Message: "installer finished but prismlauncher.exe not found"}
	}
	return ex.Succeed(ctx, "prismlauncher.exe installed under .tools/prism")
}

// LaunchPrismLogin opens the Prism GUI against the portable data dir so the
// user can add their Microsoft account; completion is auto-detected from
// accounts.json by the onboarding step.
func (a *API) LaunchPrismLogin(ctx context.Context) error {
	if err := a.ready(); err != nil {
		return err
	}
	exe := a.prismExe()
	if !a.prismExeExists() {
		return fmt.Errorf("%w: prism launcher not installed", store.ErrNotFound)
	}
	dir := a.prismDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create prism data dir: %w", err)
	}
	cmd := exec.Command(exe, "-d", dir)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch prism: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (a *API) AcknowledgeOnboarding(ctx context.Context, steps map[string]bool, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	for step := range steps {
		switch step {
		case "curseforgeKey", "firstPack", "firstMod":
			// client-acknowledgeable steps
		case "prismAccount":
			// Backend-owned step: set automatically when the portable Prism
			// accounts.json appears. Writing it is rejected per contract.
			return &DomainError{Status: 422, Code: "onboarding_step_readonly", Message: "prismAccount is set by the backend and cannot be written"}
		default:
			return &DomainError{Status: 422, Code: "onboarding_unknown_step", Message: "unknown onboarding step: " + step}
		}
	}
	for _, step := range []string{"curseforgeKey", "firstPack", "firstMod"} {
		if !steps[step] {
			continue
		}
		at := a.now().UnixMilli()
		if err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
			if err := tx.AcknowledgeOnboarding(ctx, step, at); err != nil {
				return err
			}
			return tx.AddAudit(ctx, newID("audit"), "", "onboarding.ack", requestID, map[string]any{"step": step}, at)
		}); err != nil {
			return err
		}
	}
	return nil
}
func (a *API) MCVersions(ctx context.Context) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return []string{"1.21.8", "1.21.1", "1.20.6", "1.20.1", "1.19.2", "1.18.2"}, nil
}

func (a *API) pack(p store.PackRecord, version string) Pack {
	var icon, loaderVersion, description *string
	if p.IconPath != "" {
		icon = &p.IconPath
	}
	if p.LoaderVersion != "" {
		v := p.LoaderVersion
		loaderVersion = &v
	}
	if p.Description != "" {
		v := p.Description
		description = &v
	}
	return Pack{ID: p.ID, Name: p.Name, IconURL: icon, MCVersion: p.MCVersion, Loader: normalizeLoader(p.Loader), LoaderVersion: loaderVersion, Description: description, Status: p.Status, PackVersion: version, CreatedAt: iso(p.CreatedAt), UpdatedAt: iso(p.UpdatedAt)}
}
func validateCreate(i CreatePackInput) error {
	p := store.PackRecord{Name: strings.TrimSpace(i.Name), MCVersion: strings.TrimSpace(i.MCVersion), Loader: i.Loader, LoaderVersion: i.LoaderVersion}
	if err := validatePack(p); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	return nil
}
func validatePack(p store.PackRecord) error {
	if p.Name == "" {
		return errors.New("name is required")
	}
	if len([]rune(p.Name)) > 128 {
		return errors.New("name is too long")
	}
	if p.MCVersion == "" {
		return errors.New("mcVersion is required")
	}
	switch p.Loader {
	case "forge", "neoforge", "fabric", "quilt":
	default:
		return errors.New("loader must be forge, neoforge, fabric, or quilt")
	}
	return nil
}
func normalizeLoader(v string) string {
	switch v {
	case "forge", "neoforge", "fabric", "quilt":
		return v
	default:
		return "forge"
	}
}
func iso(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano) }
func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
