package service

import (
	"context"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

// TestUpdateModCategoryKeepsCatalogGenerationCurrent 锁住「改分类不作废目录」。
//
// 回归来源：分类（category）是 0027 引入的纯展示字段，但 UpdatePackMod 曾经对
// 任何 PATCH 都无条件 InvalidatePackGeneration。后果是「右键把 JEI 从优化挪到
// 科技」=> packs.config_revision+1、current_generation_id=NULL => 已建好的
// 1330 项目录立刻变成 catalog_stale，索引/网格/关系态全部退化成占位页。
//
// 口径与迁移 0028 收窄后的 catalog_pack_mods_UPDATE 触发器一致：目录成员由
// status / sha1 / current_selection_id 决定，category 不在列。
func TestUpdateModCategoryKeepsCatalogGenerationCurrent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "mod-category-revision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := New(db)
	adapter, err := provider.NewCurseForgeFixture([]byte(`{
        "projects":[{"id":"p1","slug":"alpha","name":"Alpha Mod"}],
        "versions":[{"id":"v1","projectId":"p1","name":"1.0","versionNumber":"1.0","files":[{"id":"f1","name":"alpha.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]}],
        "metadata":[{"project":{"id":"p1","slug":"alpha","name":"Alpha Mod"},"version":{"id":"v1","projectId":"p1","name":"1.0"},"dependencies":[]}]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(adapter))

	ctx := context.Background()
	pack, err := app.CreatePack(ctx, CreatePackInput{Name: "分类回归", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "cat-pack")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := app.AddPackMod(ctx, pack.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1", Required: true}, "cat-add")
	if err != nil {
		t.Fatal(err)
	}

	revision := func() int64 {
		t.Helper()
		var v int64
		if err := db.QueryRow(`SELECT config_revision FROM packs WHERE id=?`, pack.ID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	generation := func() string {
		t.Helper()
		var v *string
		if err := db.QueryRow(`SELECT current_generation_id FROM packs WHERE id=?`, pack.ID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v == nil {
			return ""
		}
		return *v
	}

	// 加模组本身必须作废目录：这是「目录成员变了」的正例。
	baseline := revision()
	if baseline <= 1 {
		t.Fatalf("config_revision after add = %d, want > 1 (add must invalidate)", baseline)
	}
	generationBefore := generation()

	// 反例：只改分类，多次、含清空，都不能动代次。
	for _, category := range []string{"优化", "科技", "主线", ""} {
		c := category
		updated, err := app.UpdatePackMod(ctx, pack.ID, mod.ID, UpdateModInput{Category: &c}, "cat-"+category)
		if err != nil {
			t.Fatalf("update category=%q: %v", category, err)
		}
		if updated.Category != category {
			t.Fatalf("category after update = %q, want %q", updated.Category, category)
		}
		if got := revision(); got != baseline {
			t.Fatalf("config_revision after category=%q = %d, want %d (category must not invalidate)", category, got, baseline)
		}
		if got := generation(); got != generationBefore {
			t.Fatalf("current_generation_id after category=%q = %q, want %q", category, got, generationBefore)
		}
	}

	// 正例：status 变到 disabled 必须作废（目录成员由 status 决定）。
	disabled := "disabled"
	if _, err := app.UpdatePackMod(ctx, pack.ID, mod.ID, UpdateModInput{Status: &disabled}, "cat-disable"); err != nil {
		t.Fatal(err)
	}
	if got := revision(); got != baseline+1 {
		t.Fatalf("config_revision after status=disabled = %d, want %d", got, baseline+1)
	}
	if got := generation(); got != "" {
		t.Fatalf("current_generation_id after status=disabled = %q, want empty", got)
	}

	// 状态接口与读路径必须同口径：stale=false ⟺ GetItemCatalog 成功。
	// 回归来源：/catalog/status 曾经只看 source==built，作废代次时仍报
	// stale=false，构建面板据此显示「目录是最新的」，而索引页是空的。
	_, readErr := app.GetItemCatalog(ctx, pack.ID, "zh_cn")
	status, err := app.GetCatalogStatus(ctx, pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	if readable := readErr == nil; readable == status.Stale {
		t.Fatalf("status.stale=%v but readErr=%v: 状态灯与读路径分叉了", status.Stale, readErr)
	}
}
