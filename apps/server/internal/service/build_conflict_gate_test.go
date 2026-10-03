package service

// O20/O21 的回归：构建产物必须是「装上就能跑」的包，冲突列表必须是活的。
//
// 真机链路（scripts/verify-terminal-chain.sh --launch）实测：只加 JEI 的包
// /resolve 会检出「缺少依赖模组：JEI 需要 fabric-api」的 error 级冲突，
// 但构建一路绿灯，安装也成功，进程一起来就被 Fabric Loader 判 Mod resolution
// failed 弹错误界面——流水线走完却启动不了游戏。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"mpackstation/internal/store"
)

func seedPendingConflict(t *testing.T, db *sql.DB, packID, fp, severity, status, summary string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO conflicts(id,pack_id,fingerprint,kind,severity,status,summary,created_at,updated_at)
		VALUES(?,?,?,'dependency',?,?,?,?,?)`,
		"conf-"+fp, packID, fp, severity, status, summary, time.Now().UnixMilli(), time.Now().UnixMilli())
}

func conflictStatus(t *testing.T, db *sql.DB, packID, fp string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM conflicts WHERE pack_id=? AND fingerprint=?`, packID, fp).Scan(&status); err != nil {
		t.Fatalf("read conflict %s: %v", fp, err)
	}
	return status
}

// TestBuildBlockedByUnresolvedFatalConflict 是 O20：致命冲突没处理完就不许构建。
func TestBuildBlockedByUnresolvedFatalConflict(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "gate", export); err != nil {
		t.Fatal(err)
	}
	seedPendingConflict(t, db, packID, "jei:need-fabric-api", "error", "pending",
		"缺少依赖模组：Just Enough Items (JEI) 需要 fabric-api")

	_, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "gate",
	})
	var de *DomainError
	if !errors.As(err, &de) {
		t.Fatalf("构建应被冲突闸门拒绝，实得 err=%v", err)
	}
	if de.Status != 409 || de.Code != "build_unresolved_conflicts" {
		t.Fatalf("状态/错误码 = %d/%s，想要 409/build_unresolved_conflicts", de.Status, de.Code)
	}
	if !strings.Contains(de.Message, "fabric-api") {
		t.Fatalf("消息要把冲突摘要带出来，实得 %q", de.Message)
	}

	// 用户按提示补了依赖（或人工判定不影响），冲突结案后构建放行。
	mustExec(t, db, `UPDATE conflicts SET status='resolved',resolved_at=? WHERE pack_id=?`,
		time.Now().UnixMilli(), packID)
	result, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "gate",
	})
	if err != nil {
		t.Fatalf("冲突结案后仍被拒: %v", err)
	}
	if result.Artifact.Kind != "mrpack" {
		t.Fatalf("产物类型 = %q，想要 mrpack", result.Artifact.Kind)
	}
}

// TestBuildIgnoresWarningAndIgnoredConflicts 保证闸门不把非致命冲突当阻塞条件，
// 否则「警告」和「用户已忽略」都会变成构建不出去的死锁。
func TestBuildIgnoresWarningAndIgnoredConflicts(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "gate", export); err != nil {
		t.Fatal(err)
	}
	seedPendingConflict(t, db, packID, "warn-dup", "warning", "pending", "重复模组，建议只留一个")
	seedPendingConflict(t, db, packID, "user-ignored", "error", "ignored", "已忽略的冲突")

	if _, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "gate",
	}); err != nil {
		t.Fatalf("warning/ignored 冲突不该拦构建: %v", err)
	}
}

// TestReResolveAutoResolvesStaleConflicts 是 O21：冲突表只增不失效，
// 补上依赖重新 resolve 后旧红条还挂在 pending。
func TestReResolveAutoResolvesStaleConflicts(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	now := time.Now().UnixMilli()
	snapshot := `{"schema":2,"mods":[]}`

	first := store.LockRecord{ID: "lock-1", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: snapshot, CreatedAt: now}
	missing := store.ConflictRecord{ID: "c-1", PackID: packID, Fingerprint: "jei:need-fabric-api",
		Kind: "dependency", Severity: "error", Status: "pending",
		Summary: "缺少依赖模组：JEI 需要 fabric-api", CreatedAt: now, UpdatedAt: now}
	if err := app.repo.CreateLock(context.Background(), first, nil, []store.ConflictRecord{missing}, "req-1"); err != nil {
		t.Fatal(err)
	}
	if got := conflictStatus(t, db, packID, "jei:need-fabric-api"); got != "pending" {
		t.Fatalf("首轮检出后 = %q，想要 pending", got)
	}

	// 第二轮：依赖补齐，本轮没再检出这条冲突。
	second := store.LockRecord{ID: "lock-2", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: snapshot, CreatedAt: now + 1}
	if err := app.repo.CreateLock(context.Background(), second, nil, nil, "req-2"); err != nil {
		t.Fatal(err)
	}
	if got := conflictStatus(t, db, packID, "jei:need-fabric-api"); got != "resolved" {
		t.Fatalf("依赖补齐后旧冲突仍是 %q：冲突只增不删，用户看不到包已修好，构建闸门也会被过期记录一直拦死", got)
	}
	var resolvedAt sql.NullInt64
	if err := db.QueryRow(`SELECT resolved_at FROM conflicts WHERE pack_id=? AND fingerprint=?`,
		packID, "jei:need-fabric-api").Scan(&resolvedAt); err != nil {
		t.Fatal(err)
	}
	if !resolvedAt.Valid {
		t.Fatal("自动结案要写下 resolved_at，否则审计上分不清人工与自动")
	}
}

// TestReResolveKeepsIgnoredConflict 确认失效逻辑不覆盖用户显式「忽略」。
func TestReResolveKeepsIgnoredConflict(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	now := time.Now().UnixMilli()
	snapshot := `{"schema":2,"mods":[]}`
	c := store.ConflictRecord{ID: "c-1", PackID: packID, Fingerprint: "dup-mods",
		Kind: "duplicate", Severity: "error", Status: "pending",
		Summary: "重复模组", CreatedAt: now, UpdatedAt: now}
	lock := store.LockRecord{ID: "lock-1", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: snapshot, CreatedAt: now}
	if err := app.repo.CreateLock(context.Background(), lock, nil, []store.ConflictRecord{c}, "req-1"); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `UPDATE conflicts SET status='ignored' WHERE pack_id=? AND fingerprint='dup-mods'`, packID)

	again := store.LockRecord{ID: "lock-2", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: snapshot, CreatedAt: now + 1}
	if err := app.repo.CreateLock(context.Background(), again, nil, []store.ConflictRecord{c}, "req-2"); err != nil {
		t.Fatal(err)
	}
	if got := conflictStatus(t, db, packID, "dup-mods"); got != "ignored" {
		t.Fatalf("重新检出把用户忽略冲掉了 = %q，想要 ignored", got)
	}
}

// TestBuildAcceptsSnapshotResentVerbatim 是 O22：锁快照的不一致判定只看字节哈希，
// 而 resolve 存的是结构体声明顺序、客户端原样回传会被重排成键序——
// 「先 GET /locks 再原样 POST」这条路本来永远走不通，绑了锁的版本构建不出去。
func TestBuildAcceptsSnapshotResentVerbatim(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "gate", export); err != nil {
		t.Fatal(err)
	}
	// 与 resolve 写库的形状一致：schemaVersion 在前，不是字典序。
	snapshot := `{"schemaVersion":2,"packId":"` + packID + `","mods":[]}`
	lock := store.LockRecord{ID: "lock-1", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: snapshot, CreatedAt: time.Now().UnixMilli()}
	if err := app.repo.CreateLock(context.Background(), lock, nil, nil, "req-1"); err != nil {
		t.Fatal(err)
	}
	var bound string
	if err := db.QueryRow(`SELECT lock_id FROM pack_versions WHERE id=?`, versionID).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if bound != "lock-1" {
		t.Fatalf("CreateLock 没把锁绑到当前版本，实得 %q（本例前提不成立）", bound)
	}

	result, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "gate",
		LockSnapshot: json.RawMessage(snapshot),
	})
	if err != nil {
		var de *DomainError
		if errors.As(err, &de) && de.Code == "build_lock_mismatch" {
			t.Fatalf("原样回传锁快照仍被判不一致，版本一旦绑锁就再也构建不出去: %s", de.Message)
		}
		t.Fatal(err)
	}
	if result.Artifact.Kind != "mrpack" {
		t.Fatalf("产物类型 = %q", result.Artifact.Kind)
	}

	// 真正的错配（内容不同）仍然要拒。
	if _, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "gate",
		LockSnapshot: json.RawMessage(`{"schemaVersion":2,"packId":"other","mods":[]}`),
	}); err == nil {
		t.Fatal("换了 packId 的锁快照必须构建失败")
	}
}

// TestResolvePersistsKnownIssueConflict 是 O23：兼容知识库命中写出的冲突
// kind='known_issue'，而 0002 的 CHECK 没这个值——知识库一旦有条目命中，
// resolve 就在 INSERT 上撞约束、事务回滚、接口 500。0023 扩了 CHECK。
func TestResolvePersistsKnownIssueConflict(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	now := time.Now().UnixMilli()
	c := store.ConflictRecord{ID: "c-ki", PackID: packID, Fingerprint: "pm-a:pm-b:known_issue",
		Kind: "known_issue", Severity: "error", Status: "pending",
		Summary: "两个模组合成配方撞车（解法: 装 Polymorph）",
		Detail:  map[string]any{"reason": "compat_knowledge"}, CreatedAt: now, UpdatedAt: now}
	lock := store.LockRecord{ID: "lock-ki", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: `{"schemaVersion":2,"mods":[]}`, CreatedAt: now}
	if err := app.repo.CreateLock(context.Background(), lock, nil, []store.ConflictRecord{c}, "req-ki"); err != nil {
		t.Fatalf("known_issue 冲突写不入库，resolve 会整事务回滚: %v", err)
	}
	if got := conflictStatus(t, db, packID, "pm-a:pm-b:known_issue"); got != "pending" {
		t.Fatalf("落库后 status = %q", got)
	}
}
