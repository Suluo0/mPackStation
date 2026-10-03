package service

// O25 的回归：平台不可用是「网络态」，不是「这个包有缺陷」。
//
// 链路测试重跑（chain-run14）实测：主包在 11-本地模组 里有一个 source='local' 的本机 jar
// 模组，resolve 时 p5Adapter 拿不到适配器，conflict() 给它写了 kind='provider_unavailable'，
// 而 conflict() 的白名单又把未知 kind 悄悄改写成 'dependency'、severity 恒为 error。
// 于是库里躺着一条码着「Provider unavailable」的 error 级 pending 冲突，构建闸门（O20）
// 把整个包的构建锁死：用户看到的是「1 个冲突未解决：Provider unavailable」——
// 既不知道这是 Modrinth 此刻打不开，也没有任何办法在界面上「解决」它。

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mpackstation/internal/store"
)

func TestTransientProviderConflictIsWarningAndDoesNotBlockBuild(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)

	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "gate", export); err != nil {
		t.Fatal(err)
	}

	// 测试用的 API 没有注入 provider registry：任何平台调用都在这里失败，
	// 等价于「Modrinth 此刻打不开」，也等价于本机 jar 模组没有平台来源
	// （chain-test 的 11-本地模组 走的就是这同一条分支）。
	if _, err := app.ResolvePack(context.Background(), packID, "req-o25"); err != nil {
		t.Fatalf("resolve 失败: %v", err)
	}

	rows, err := db.Query(`SELECT kind,severity,status,summary FROM conflicts WHERE pack_id=? ORDER BY fingerprint`, packID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	total := 0
	for rows.Next() {
		var kind, severity, status, summary string
		if err := rows.Scan(&kind, &severity, &status, &summary); err != nil {
			t.Fatal(err)
		}
		total++
		if kind != "provider_unavailable" {
			got = append(got, kind)
		}
		if severity != "warning" {
			got = append(got, kind+":"+severity)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("resolve 没写出任何冲突，本例前提不成立（provider 不可用必须留痕）")
	}
	if len(got) > 0 {
		t.Fatalf("网络态冲突被写成了 %v：kind 必须保持 provider_unavailable（别伪装成依赖问题）、severity 必须是 warning（别拦构建）", got)
	}

	// 版本已被上面的 resolve 绑上锁快照，构建必须原样回传（O13/O22 的口径），
	// 本例要证的是：网络态冲突不在这之后再把构建拦死。
	var snapshot string
	if err := db.QueryRow(`SELECT snapshot_json FROM pack_locks WHERE pack_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, packID).Scan(&snapshot); err != nil {
		t.Fatalf("读锁快照: %v", err)
	}
	if _, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "gate",
		LockSnapshot: json.RawMessage(snapshot),
	}); err != nil {
		t.Fatalf("平台不可用把构建锁死了，用户既看不懂也修不了: %v", err)
	}
}

// TestConflictWhitelistKeepsKnownKinds 保证 conflict() 不再把合法 kind 改写掉。
// 白名单漏掉任何一个枚举里的 kind，写库时就会伪装成 'dependency'（0024 之前甚至会撞
// CHECK 约束回滚整个 resolve 事务）。
func TestConflictWhitelistKeepsKnownKinds(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	now := time.Now().UnixMilli()
	kinds := []string{"dependency", "version", "loader", "duplicate", "crash", "known_issue", "provider_unavailable"}
	m := store.PackModRecord{ID: "pm-1", PackID: packID, DisplayName: "One"}
	var conflicts []store.ConflictRecord
	for _, k := range kinds {
		c := conflict(m, k, "S-"+k, "test")
		if c.Kind != k {
			t.Fatalf("conflict() 把 kind=%q 改写成了 %q：库里的分类从此不可信", k, c.Kind)
		}
		conflicts = append(conflicts, c)
	}
	lock := store.LockRecord{ID: "lock-kinds", PackID: packID, SchemaVersion: 2,
		SnapshotJSON: `{"schemaVersion":2,"mods":[]}`, CreatedAt: now}
	if err := app.repo.CreateLock(context.Background(), lock, nil, conflicts, "req-kinds"); err != nil {
		t.Fatalf("枚举内的 kind 必须都能写库（CHECK 没覆盖就是整事务回滚）: %v", err)
	}
	rows, err := db.Query(`SELECT kind FROM conflicts WHERE pack_id=?`, packID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		seen[k] = true
	}
	for _, k := range kinds {
		if !seen[k] {
			t.Fatalf("kind=%q 被改写或写丢了，实际落库 %v", k, seen)
		}
	}
}
