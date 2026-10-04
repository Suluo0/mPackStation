package service

import (
	"context"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

// modSearchFixture 复刻「MEK 现象」：一个只有 NeoForge 版的模组 + 一个正经
// Fabric 模组。loaders 字段是判定能否装进当前包的唯一依据。
const modSearchFixture = `{
  "projects":[
    {"id":"MEK","slug":"mekanism","name":"Mekanism","summary":"tech","loaders":["neoforge"],"downloads":3752120},
    {"id":"FAB","slug":"fabric-thing","name":"Fabric Thing","summary":"utility","loaders":["fabric"],"downloads":100}
  ]
}`

func modSearchFixtureApp(t *testing.T, loader, mcVersion string) *API {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "mod-search.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := New(db)
	adapter, err := provider.NewModrinthFixture([]byte(modSearchFixture))
	if err != nil {
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(adapter))
	if _, err := app.CreatePack(context.Background(), CreatePackInput{
		Name: "搜索-" + loader, MCVersion: mcVersion, Loader: loader, LoaderVersion: "0.15",
	}, "search-"+loader); err != nil {
		t.Fatal(err)
	}
	return app
}

func packIDOf(t *testing.T, app *API) string {
	t.Helper()
	packs, err := app.ListPacks(context.Background())
	if err != nil || len(packs) == 0 {
		t.Fatalf("list packs = %#v, %v", packs, err)
	}
	return packs[0].ID
}

func slugs(items []ModSearchAllItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Slug)
	}
	return out
}

// TestModSearchAllFallsBackWhenPackLoaderHasNoMatch 是「Fabric 包里搜 MEK」
// 的直接回归。带 fabric 过滤查是空的，必须降级捞回同 MC 版本的 NeoForge 版，
// 并带上 loaders 让界面能说清原因 —— 而不是丢一个空列表让用户以为搜索坏了。
func TestModSearchAllFallsBackWhenPackLoaderHasNoMatch(t *testing.T) {
	app := modSearchFixtureApp(t, "fabric", "1.21.1")
	res, err := app.ModSearchAll(context.Background(), packIDOf(t, app), ModSearchInput{Query: "mek"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("主结果 = %v, want 空 —— Fabric 包不该把只有 NeoForge 版的模组列出来", slugs(res.Items))
	}
	if len(res.Fallback) != 1 || res.Fallback[0].Slug != "mekanism" {
		t.Fatalf("降级结果 = %v, want [mekanism]", slugs(res.Fallback))
	}
	if got := res.Fallback[0].Loaders; len(got) != 1 || got[0] != "neoforge" {
		t.Fatalf("降级条目的 loaders = %v, want [neoforge] —— 界面靠它拼「仅支持 NeoForge」", got)
	}
}

// TestModSearchAllKeepsMainResultsWhenLoaderMatches 是反例：加载器对得上时
// 必须走正常主结果，且**不该**去发那次降级查询。
func TestModSearchAllKeepsMainResultsWhenLoaderMatches(t *testing.T) {
	app := modSearchFixtureApp(t, "fabric", "1.21.1")
	res, err := app.ModSearchAll(context.Background(), packIDOf(t, app), ModSearchInput{Query: "fabric"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Slug != "fabric-thing" {
		t.Fatalf("主结果 = %v, want [fabric-thing]", slugs(res.Items))
	}
	if len(res.Fallback) != 0 {
		t.Fatalf("降级结果 = %v, want 空 —— 主结果非空时不该降级", slugs(res.Fallback))
	}
}

// TestModSearchAllAppliesPackLoaderWhenCallerOmitsIt 锁住「后端自己补包口径」。
//
// 商店面板只传 q（不传 loader/mcVersion），所以补默认值是这条链路唯一的过滤点。
// 用同一个查询、只换包的加载器做差分 —— 结果必须从降级区翻到主区，否则说明
// 补进去的 loader 根本没生效（这正是「搜 MEK 搜出一堆装不上的东西」的成因）。
func TestModSearchAllAppliesPackLoaderWhenCallerOmitsIt(t *testing.T) {
	ctx := context.Background()
	fabricApp := modSearchFixtureApp(t, "fabric", "1.21.1")
	neoforgeApp := modSearchFixtureApp(t, "neoforge", "1.21.1")

	fabricRes, err := fabricApp.ModSearchAll(ctx, packIDOf(t, fabricApp), ModSearchInput{Query: "mek"})
	if err != nil {
		t.Fatal(err)
	}
	neoforgeRes, err := neoforgeApp.ModSearchAll(ctx, packIDOf(t, neoforgeApp), ModSearchInput{Query: "mek"})
	if err != nil {
		t.Fatal(err)
	}

	if len(fabricRes.Items) != 0 || len(fabricRes.Fallback) != 1 {
		t.Fatalf("Fabric 包: 主=%v 降级=%v, want 主空/降级 1 条", slugs(fabricRes.Items), slugs(fabricRes.Fallback))
	}
	if len(neoforgeRes.Items) != 1 || neoforgeRes.Items[0].Slug != "mekanism" || len(neoforgeRes.Fallback) != 0 {
		t.Fatalf("NeoForge 包: 主=%v 降级=%v, want 主 [mekanism]/降级空 —— 同一查询换个加载器结果必须翻转",
			slugs(neoforgeRes.Items), slugs(neoforgeRes.Fallback))
	}
}

// TestPackSearchLoaderRefusesToGuess 守住一个具体的雷。
//
// normalizeLoader 把一切不认识的值兜底成 "forge"，用来展示无害；但如果搜索
// 过滤复用它，一个 loader 字段异常的包会被静默当成 Forge 包，照样搜出装不上
// 的东西 —— 那就等于没修。搜索宁可不过滤，也不能猜错。
func TestPackSearchLoaderRefusesToGuess(t *testing.T) {
	for _, in := range []string{"", "   ", "weird", "fabric-loader", "1.21.1"} {
		if got := packSearchLoader(in); got != "" {
			t.Fatalf("packSearchLoader(%q) = %q, want 空（未知就该不过滤，不能兜底成 forge）", in, got)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"fabric", "fabric"}, {"FabRIC", "fabric"}, {" neoForge ", "neoforge"}, {"QUILT", "quilt"},
	} {
		if got := packSearchLoader(tc.in); got != tc.want {
			t.Fatalf("packSearchLoader(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestOnlyOtherLoaderProjectsDropsUnknownLoaders 钉住降级区的准入规则：
// 只有「明确声明了加载器、且都不含本包加载器、而且跟查询真正相关」的条目才配
// 进降级区。声明为空的按未知丢弃 —— 说不出原因的条目放进去，用户只会更迷惑。
func TestOnlyOtherLoaderProjectsDropsUnknownLoaders(t *testing.T) {
	items := []provider.Project{
		{Slug: "other-only", Name: "Other Only", Loaders: []string{"neoforge"}},
		{Slug: "unknown", Name: "Unknown", Loaders: nil},
		{Slug: "has-fabric", Name: "Has Fabric", Loaders: []string{"forge", "fabric"}},
		{Slug: "unknown-empty-slice", Name: "Empty Slice", Loaders: []string{}},
	}
	got := onlyOtherLoaderProjects(items, "fabric", "other")
	if len(got) != 1 || got[0].Slug != "other-only" {
		names := make([]string, 0, len(got))
		for _, p := range got {
			names = append(names, p.Slug)
		}
		t.Fatalf("降级区 = %v, want [other-only]", names)
	}
}

// TestOtherLoaderRelevantRejectsPlatformNoise 复刻实测踩到的那一幕。
//
// 在一个 Fabric 包里搜 "mek"，Modrinth 返回的主结果不是空的，而是
// NoEmotecraft / KeProfiles 这种「摘要里沾边」的噪音 —— 于是「主结果为空才
// 降级」的开关永远打不开，Mekanism 永远露不出来。所以降级区必须同时过滤
// 相关性：只沾摘要（20 分）的不要，名称/slug 真命中的才留。
func TestOtherLoaderRelevantRejectsPlatformNoise(t *testing.T) {
	noise := provider.Project{Slug: "noemotecraft", Name: "NoEmotecraft", Summary: "Mek-inspired sound mod"}
	if otherLoaderRelevant(noise, "fabric", "mek") {
		t.Fatal("只在摘要里沾到查询的条目不该进降级区 —— 它会把真正的答案挤掉")
	}
	real := provider.Project{Slug: "mekanism", Name: "Mekanism", Loaders: []string{"neoforge"}}
	if !otherLoaderRelevant(real, "fabric", "mek") {
		t.Fatal("名称以查询开头、且不支持本包加载器的条目必须进降级区")
	}
	compatible := provider.Project{Slug: "mek-tools", Name: "Mek Tools", Loaders: []string{"fabric"}}
	if otherLoaderRelevant(compatible, "fabric", "mek") {
		t.Fatal("支持本包加载器的条目属于主结果，不该出现在降级区")
	}
}

// TestHasRelevantHitDistinguishesNoiseFromAnswer 钉住降级的触发条件：
// 看的是「主结果里有没有真正相关的命中」，不是「列表空不空」。
func TestHasRelevantHitDistinguishesNoiseFromAnswer(t *testing.T) {
	noise := []ModSearchAllItem{
		{Provider: "modrinth", Project: provider.Project{Slug: "noemotecraft", Name: "NoEmotecraft", Summary: "Mek-inspired sound mod"}},
		{Provider: "modrinth", Project: provider.Project{Slug: "keprofiles", Name: "KeProfiles", Summary: "profile helper"}},
	}
	if hasRelevantHit("mek", noise) {
		t.Fatal("整屏都是模糊噪音时不该算「搜到了」—— 那会让降级永不触发")
	}
	answer := []ModSearchAllItem{
		{Provider: "modrinth", Project: provider.Project{Slug: "mek-tools", Name: "Mek Tools for Fabric", Loaders: []string{"fabric"}}},
	}
	if !hasRelevantHit("mek", answer) {
		t.Fatal("名称以查询开头必须算「搜到了」")
	}
	if hasRelevantHit("mek", nil) {
		t.Fatal("空列表当然没有相关命中")
	}
}
