package service

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"mpackstation/internal/store"
)

// 兼容知识库: 随二进制分发(go:embed), 只读。与身份表同哲学——起步只放人工
// 核实的条目, 没核实的不准入库。knownIssues 由维护者整理后填入, 每条必须
// 带 source 说明核实出处。
//
//go:embed compat_knowledge_baseline.json
var compatKnowledgeJSON []byte

// compatEndpoint 用身份标识一个模组: MR/CF 项目 ID 任一命中即算同一个
// (pack_mods 行有主源 + 镜像两侧 ID)。
type compatEndpoint struct {
	MR string `json:"mr"`
	CF string `json:"cf"`
}

type compatFix struct {
	Type string         `json:"type"` // "install_mod"
	Mod  compatEndpoint `json:"mod"`
	Note string         `json:"note"`
}

type compatIssue struct {
	A          compatEndpoint `json:"a"`
	B          compatEndpoint `json:"b"`
	MCVersions []string       `json:"mcVersions"` // 空 = 所有 MC 版本
	Loaders    []string       `json:"loaders"`    // 空 = 所有 loader
	Severity   string         `json:"severity"`   // fatal / warning
	Summary    string         `json:"summary"`
	Fix        *compatFix     `json:"fix"`
	Source     string         `json:"source"`
}

type compatRecommendation struct {
	MR         string   `json:"mr"`
	CF         string   `json:"cf"`
	Name       string   `json:"name"`
	Reason     string   `json:"reason"`
	MCVersions []string `json:"mcVersions"`
	Loaders    []string `json:"loaders"`
}

type compatKnowledge struct {
	KnownIssues     []compatIssue          `json:"knownIssues"`
	Recommendations []compatRecommendation `json:"recommendations"`
}

// baselineCompatKnowledge parses the embedded knowledge pack; malformed asset
// yields empty knowledge (runtime stays defensive).
func baselineCompatKnowledge() compatKnowledge {
	var k compatKnowledge
	if err := json.Unmarshal(compatKnowledgeJSON, &k); err != nil {
		return compatKnowledge{}
	}
	return k
}

// CompatRecommendation is one curated compat-mod suggestion for a pack,
// scoped by MC version/loader, skipping mods already installed.
type CompatRecommendation struct {
	Name      string `json:"name"`
	Reason    string `json:"reason"`
	Provider  string `json:"provider"`
	ProjectID string `json:"projectId"`
}

// ListCompatRecommendations returns the curated compat mods applicable to this
// pack. Provider picks Modrinth first (faster), CurseForge when only its
// adapter is configured; entries with no usable adapter are omitted.
func (a *API) ListCompatRecommendations(ctx context.Context, packID string) ([]CompatRecommendation, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	pack, err := a.repo.GetPack(ctx, packID)
	if err != nil {
		return nil, err
	}
	mods, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return nil, err
	}
	out := []CompatRecommendation{}
	for _, rec := range baselineCompatKnowledge().Recommendations {
		if !scopeOK(pack.MCVersion, pack.Loader, rec.MCVersions, rec.Loaders) {
			continue
		}
		if fixAlreadyPresent(mods, &compatFix{Mod: compatEndpoint{MR: rec.MR, CF: rec.CF}}) {
			continue
		}
		if rec.MR != "" {
			if _, err := a.p5Adapter("modrinth"); err == nil {
				out = append(out, CompatRecommendation{Name: rec.Name, Reason: rec.Reason, Provider: "modrinth", ProjectID: rec.MR})
				continue
			}
		}
		if rec.CF != "" {
			if _, err := a.p5Adapter("curseforge"); err == nil {
				out = append(out, CompatRecommendation{Name: rec.Name, Reason: rec.Reason, Provider: "curseforge", ProjectID: rec.CF})
			}
		}
	}
	return out, nil
}

// endpointMatch: 模组行的主源或镜像任一 ID 等于端点的 MR/CF ID 即命中。
func endpointMatch(m store.PackModRecord, e compatEndpoint) bool {
	for _, id := range []string{m.ProjectID, m.MirrorProjectID} {
		if id != "" && (id == e.MR || id == e.CF) {
			return true
		}
	}
	return false
}

// scopeOK: 条目声明了 MC 版本/loader 范围时必须匹配当前包; 空范围普遍适用。
func scopeOK(mcVersion, loader string, mcVersions, loaders []string) bool {
	if len(mcVersions) > 0 {
		found := false
		for _, v := range mcVersions {
			if v == mcVersion {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(loaders) > 0 {
		found := false
		for _, l := range loaders {
			if strings.EqualFold(l, loader) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// compatHit 是一对同时存在于包内、且被知识库判定有已知问题的模组。
type compatHit struct {
	Issue compatIssue
	ModA  store.PackModRecord
	ModB  store.PackModRecord
}

// scanCompatKnowledge pairs every active mod against known issues. Pure
// function: no I/O, fully unit-testable.
func scanCompatKnowledge(mcVersion, loader string, mods []store.PackModRecord, k compatKnowledge) []compatHit {
	active := make([]store.PackModRecord, 0, len(mods))
	for _, m := range mods {
		if m.Status != "removed" && m.Status != "disabled" {
			active = append(active, m)
		}
	}
	hits := []compatHit{}
	for _, issue := range k.KnownIssues {
		if !scopeOK(mcVersion, loader, issue.MCVersions, issue.Loaders) {
			continue
		}
		var ma, mb *store.PackModRecord
		for i := range active {
			if ma == nil && endpointMatch(active[i], issue.A) {
				ma = &active[i]
			}
			if mb == nil && endpointMatch(active[i], issue.B) {
				mb = &active[i]
			}
		}
		if ma != nil && mb != nil && ma.ID != mb.ID {
			hits = append(hits, compatHit{Issue: issue, ModA: *ma, ModB: *mb})
		}
	}
	return hits
}

// fixAlreadyPresent: 修复模组已在包内(主源或镜像命中)时不再重复加装。
func fixAlreadyPresent(mods []store.PackModRecord, fix *compatFix) bool {
	if fix == nil {
		return false
	}
	for _, m := range mods {
		if m.Status == "removed" {
			continue
		}
		if endpointMatch(m, fix.Mod) {
			return true
		}
	}
	return false
}
