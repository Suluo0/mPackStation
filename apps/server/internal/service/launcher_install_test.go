package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"mpackstation/internal/store"
)

func TestInstalledVersionID(t *testing.T) {
	cases := []struct {
		name     string
		result   string
		fallback string
		want     string
	}{
		{"kernel version_id", `{"version_id":"fabric-loader-0.16.14-1.21.1","dir":"/x"}`, "1.21.1", "fabric-loader-0.16.14-1.21.1"},
		{"missing field falls back", `{"dir":"/x"}`, "1.21.1", "1.21.1"},
		{"empty result falls back", ``, "1.21.1", "1.21.1"},
	}
	for _, c := range cases {
		got := installedVersionID(json.RawMessage(c.result), c.fallback)
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestResolveLaunchVersionUsesInstalledDirectoryVersion(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	ctx := context.Background()

	// 同一个目录里两次安装：一条属于本包，一条无归属。带包归属的必须优先。
	seedLauncherInstall(t, db, "li-1", "/tmp/mc-shared", "1.20.1", "", 1_000)
	seedLauncherInstall(t, db, "li-2", "/tmp/mc-shared", "fabric-loader-0.16.14-1.21.1", packID, 2_000)

	p := LauncherLaunchPayload{MinecraftDir: "/tmp/mc-shared", PackID: packID, Username: "dev"}
	if err := app.resolveLaunchVersion(ctx, &p); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.Version != "fabric-loader-0.16.14-1.21.1" {
		t.Fatalf("resolved version = %q, want the loader version id", p.Version)
	}

	// 显式给定版本时不得覆盖：调用方可以启动任意已存在版本目录。
	given := LauncherLaunchPayload{Version: "1.19.4", MinecraftDir: "/tmp/mc-shared", PackID: packID}
	if err := app.resolveLaunchVersion(ctx, &given); err != nil || given.Version != "1.19.4" {
		t.Fatalf("explicit version changed: %q err=%v", given.Version, err)
	}
}

func TestResolveLaunchVersionRejectsEmptyDirectory(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()

	p := LauncherLaunchPayload{MinecraftDir: "/tmp/mc-empty", PackID: packID, Username: "dev"}
	err := app.resolveLaunchVersion(context.Background(), &p)
	var de *DomainError
	if !errors.As(err, &de) || de.Status != 409 || de.Code != "launcher_not_installed" {
		t.Fatalf("empty dir resolve error = %v, want 409 launcher_not_installed", err)
	}
}

func TestRecordLauncherInstallIsIdempotentPerDirectory(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	ctx := context.Background()

	payload := LauncherInstallPayload{PackID: packID, Version: "1.21.1", Loader: "fabric", MinecraftDir: "/tmp/mc-once"}
	first, err := app.recordLauncherInstall(ctx, payload, "fabric-loader-0.16.14-1.21.1", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.recordLauncherInstall(ctx, payload, "fabric-loader-0.16.14-1.21.1", "task-2")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.TaskID != "task-2" {
		t.Fatalf("reinstall created a second row or kept the old task: %#v %#v", first, second)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM launcher_installs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("launcher_installs rows = %d, want 1", count)
	}

	list, err := app.ListLauncherInstalls(ctx, packID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].VersionID != "fabric-loader-0.16.14-1.21.1" || list[0].PackID != packID {
		t.Fatalf("list = %#v", list)
	}
}

func seedLauncherInstall(t *testing.T, db *sql.DB, id, dir, versionID, packID string, at int64) {
	t.Helper()
	rec := store.LauncherInstallRecord{ID: id, MinecraftDir: dir, VersionID: versionID, InstalledAt: at}
	if packID != "" {
		rec.PackID = sql.NullString{String: packID, Valid: true}
	}
	if _, err := db.Exec(`INSERT INTO launcher_installs(id,minecraft_dir,version_id,loader,mc_version,pack_id,task_id,installed_at) VALUES(?,?,?,?,?,?,?,?)`,
		rec.ID, rec.MinecraftDir, rec.VersionID, rec.Loader, rec.MCVersion, rec.PackID, rec.TaskID, rec.InstalledAt); err != nil {
		t.Fatalf("seed install: %v", err)
	}
}
