package service

import (
	"context"
	"strings"
	"testing"

	"mpackstation/internal/store"
)

// O8 守护用例：缺依赖的冲突摘要必须是模组名，不是裸外部 ID。
func TestExplainMissingDependenciesUsesDisplayNames(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	ctx := context.Background()

	mustExec(t, db, `INSERT INTO platform_projects(id,platform,external_project_id,display_name) VALUES('pp-jei','modrinth','lhGA9TYQ','Just Enough Items')`)
	mustExec(t, db, `INSERT INTO platform_projects(id,platform,external_project_id,display_name) VALUES('pp-bare','modrinth','zzz999','')`)

	from := store.PackModRecord{ID: "pm-create", PackID: packID, DisplayName: "Create", ProjectID: "P7dR8mSH", Source: "modrinth"}
	packMods := []store.PackModRecord{from}
	pending := []missingDependency{
		{from: from, dep: store.ModDependencyRecord{ToProjectID: "lhGA9TYQ", Reason: "required"}},
		{from: from, dep: store.ModDependencyRecord{ToProjectID: "zzz999", Reason: "required"}},
		{from: from, dep: store.ModDependencyRecord{ToProjectID: "never-seen", Reason: "required"}},
	}

	got := app.explainMissingDependencies(ctx, packMods, pending)
	if len(got) != 3 {
		t.Fatalf("got %d conflicts, want 3", len(got))
	}
	if !strings.Contains(got[0].Summary, "Just Enough Items") || strings.Contains(got[0].Summary, "lhGA9TYQ") {
		t.Errorf("known id should print its name, got %q", got[0].Summary)
	}
	if !strings.Contains(got[0].Summary, "Create") {
		t.Errorf("summary should name the declaring mod, got %q", got[0].Summary)
	}
	// 本地见过这个项目但没存名字 / 完全没见过：保留 ID，但要说清那是外部 ID。
	for i, id := range []string{"zzz999", "never-seen"} {
		if !strings.Contains(got[i+1].Summary, "外部 ID "+id) {
			t.Errorf("unresolved summary must label the raw id, got %q", got[i+1].Summary)
		}
		if got[i+1].Detail["missingProjectID"] != id || got[i+1].Detail["missingResolved"] != false {
			t.Errorf("detail should carry the raw id for search, got %#v", got[i+1].Detail)
		}
	}
	if got[0].Detail["missingResolved"] != true {
		t.Errorf("resolved case should mark missingResolved=true, got %#v", got[0].Detail)
	}
}

// 没有待解释依赖时不应发查询、也不应造出冲突。
func TestExplainMissingDependenciesEmpty(t *testing.T) {
	db, app, _, _ := newP7Fixture(t)
	defer db.Close()
	if got := app.explainMissingDependencies(context.Background(), nil, nil); got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

// O24：依赖类型决定冲突的语义与严重级。
//
// 以前所有类型都是「缺少依赖模组」+ error，JEI 的 mezz_config（embedded）被判成
// 缺失，构建闸门因此永远过不去；不兼容项反向判断更是从没做过。
func TestDependencyIsMissingByType(t *testing.T) {
	db, _, _, _ := newP7Fixture(t)
	defer db.Close()
	cases := []struct {
		depType string
		present bool
		want    bool
		why     string
	}{
		{"required", false, true, "必需依赖不在包里就是缺"},
		{"required", true, false, "已在包里"},
		{"", false, true, "类型缺失按 required 处理"},
		{"embedded", false, false, "已打进宿主 jar，不该要求单独安装"},
		{"embedded", true, false, "embedded 永远不算缺失"},
		{"optional", false, true, "可选依赖缺了要提示（warning），但不该拦构建"},
		{"optional", true, false, "已在包里，无话可说"},
		{"incompatible", true, true, "互斥项在包里才是冲突"},
		{"incompatible", false, false, "互斥项不在包里是好事"},
	}
	for _, c := range cases {
		if got := dependencyIsMissing(c.depType, c.present); got != c.want {
			t.Errorf("dependencyIsMissing(%q, present=%v) = %v, want %v（%s）",
				c.depType, c.present, got, c.want, c.why)
		}
	}
}

func TestExplainMissingDependenciesSeverityByType(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	defer db.Close()
	from := store.PackModRecord{ID: "pm-jei", PackID: packID, DisplayName: "JEI", ProjectID: "u6dRKJwZ", Source: "modrinth"}
	pending := []missingDependency{
		{from: from, dep: store.ModDependencyRecord{ToProjectID: "fab", Type: "required"}},
		{from: from, dep: store.ModDependencyRecord{ToProjectID: "opt", Type: "optional"}},
		{from: from, dep: store.ModDependencyRecord{ToProjectID: "bad", Type: "incompatible"}},
	}
	got := app.explainMissingDependencies(context.Background(), []store.PackModRecord{from}, pending)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	if got[0].Severity != "error" || !strings.Contains(got[0].Summary, "缺少依赖模组") {
		t.Errorf("required 该是 error 级缺依赖，实得 %s/%q", got[0].Severity, got[0].Summary)
	}
	// 闸门只看 error：可选依赖不该把构建堵死（见 build_conflict_gate_test）。
	if got[1].Severity != "warning" || !strings.Contains(got[1].Summary, "可选依赖") {
		t.Errorf("optional 该降级为 warning 提示，实得 %s/%q", got[1].Severity, got[1].Summary)
	}
	if got[2].Severity != "error" || !strings.Contains(got[2].Summary, "互斥") {
		t.Errorf("incompatible 该写成互斥且 error，实得 %s/%q", got[2].Severity, got[2].Summary)
	}
	for i, want := range []string{"required", "optional", "incompatible"} {
		if got[i].Detail["dependencyType"] != want {
			t.Errorf("detail 要带上依赖类型，实得 %#v", got[i].Detail)
		}
	}
}
