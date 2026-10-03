package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// 装配路径的三个"必须说得清"的失败口径。它们的共同要求是：不许塌成
// 裸 400 invalid_argument / 裸 409 conflict，也不许静默产出一个缺模组的包。

// seedLocalModWithoutSelection 复刻 POST /mods/local 的真实形状：pack_mods 有行、
// status='installed'、但 current_selection_id 为 NULL（本机 jar 没有平台选中项）。
func seedLocalModWithoutSelection(t *testing.T, db *sql.DB, packID string) {
	t.Helper()
	now := time.Now().UnixMilli()
	mustExec(t, db, `INSERT INTO pack_mods(id,pack_id,source,project_id,version_id,display_name,file_name,sha1,status,required,added_at,updated_at,origin,mod_id,current_selection_id)
		VALUES('pm-local',?,'local',NULL,NULL,'Chain Local Mod','chain-local.jar',NULL,'installed',0,?,?,'manual',NULL,NULL)`, packID, now, now)
}

func TestBuildDoesNotSilentlyDropModWithoutSelection(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	seedLocalModWithoutSelection(t, db, packID)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "mrpack", export); err != nil {
		t.Fatal(err)
	}

	_, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "mrpack",
	})
	var de *DomainError
	if !errors.As(err, &de) || de.Status != 422 || de.Code != "build_mod_source_unresolved" {
		t.Fatalf("无选中项的本地模组应阻止构建, 实得 %v", err)
	}
	if !strings.Contains(de.Message, "Chain Local Mod") {
		t.Fatalf("消息必须点名缺来源的模组: %q", de.Message)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE pack_id=?`, packID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("被阻止的构建仍然登记了 %d 个产物", count)
	}
}

func TestBuildInputConflictHasExplicitCode(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "inputs", export); err != nil {
		t.Fatal(err)
	}
	first, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "inputs",
		Files:        []BuildFile{{Path: "a.txt", Content: []byte("a")}},
		LockSnapshot: json.RawMessage(`{"mods":[{"id":"jei"}]}`),
	})
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	if first.Artifact.Kind != "zip" {
		t.Fatalf("带 files 的构建应保持 zip, 实得 %q", first.Artifact.Kind)
	}

	// 同一版本换一份锁快照再构建：输入不可静默变更，但错误码要能指出去哪。
	_, err = app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "inputs",
		Files:        []BuildFile{{Path: "a.txt", Content: []byte("a")}},
		LockSnapshot: json.RawMessage(`{"mods":[{"id":"other"}]}`),
	})
	var de *DomainError
	if !errors.As(err, &de) || de.Status != 409 || de.Code != "build_input_conflict" {
		t.Fatalf("锁快照变更应 409 build_input_conflict, 实得 %v", err)
	}
	if !strings.Contains(de.Message, "版本") {
		t.Fatalf("消息要说明是版本输入冲突: %q", de.Message)
	}
}

func TestBuildLockMismatchHasExplicitCode(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "locked", export); err != nil {
		t.Fatal(err)
	}

	// 给版本绑一把真实锁：此后构建必须带上与它一致的 lockSnapshot。
	snapshot := `{"mods":[{"id":"jei"}]}`
	sha, err := canonicalLockSHA(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO pack_locks(id,pack_id,snapshot_schema_version,snapshot_json,snapshot_sha256,created_at)
		VALUES('lock-1',?,1,?,?,?)`, packID, snapshot, sha, time.Now().UnixMilli())
	mustExec(t, db, `UPDATE pack_versions SET lock_id='lock-1' WHERE id=?`, versionID)

	_, err = app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "locked",
		Files: []BuildFile{{Path: "a.txt", Content: []byte("a")}},
	})
	var de *DomainError
	if !errors.As(err, &de) || de.Status != 422 || de.Code != "build_lock_mismatch" {
		t.Fatalf("缺一致锁快照应 422 build_lock_mismatch, 实得 %v", err)
	}
	if de.Details["lockId"] != "lock-1" {
		t.Fatalf("details 应带回 lockId 供前端取回快照, 实得 %#v", de.Details)
	}
}

func canonicalLockSHA(snapshot string) (string, error) {
	var v any
	if err := json.Unmarshal([]byte(snapshot), &v); err != nil {
		return "", err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return hashJSON(data), nil
}
