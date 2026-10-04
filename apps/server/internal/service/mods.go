package service

import (
	"unicode"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

var (
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrProviderNotFound    = errors.New("provider resource not found")
	ErrInvalidSHA1         = errors.New("invalid sha1")
)

type ModSearchInput struct {
	Provider, Query, MCVersion, Loader, Cursor string
	Limit                                      int
}
type ModSearchResult struct {
	Items      []provider.Project `json:"items"`
	NextCursor string             `json:"next_cursor"`
	Total      int                `json:"total"`
	// Fallback 装「本包加载器装不上、但同 MC 版本有别的加载器版本」的模组。
	// 主结果为空时才去查，用来回答「为什么搜不到」而不是给一个空列表。
	Fallback []provider.Project `json:"fallback,omitempty"`
}
type AddModInput struct {
	Provider, ProjectID, VersionID string
	Required                       bool
}
type LocalModInput struct {
	DisplayName string `json:"displayName"`
	FileName    string `json:"fileName"`
	SHA1        string `json:"sha1"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Required    bool   `json:"required"`
}
type UpdateModInput struct {
	VersionID *string `json:"versionId"`
	Status    *string `json:"status"`
	Required  *bool   `json:"required"`
	Category  *string `json:"category"`
}
type Mod struct {
	ID                 string  `json:"id"`
	CanonicalModID     string  `json:"canonicalModId"`
	Category           string  `json:"category"`
	CurrentSelectionID string  `json:"selectionId"`
	PackID             string  `json:"packId"`
	Source             string  `json:"source"`
	ProjectID          *string `json:"projectId"`
	VersionID          *string `json:"versionId"`
	DisplayName        string  `json:"displayName"`
	FileName           string  `json:"fileName"`
	SHA1               *string `json:"sha1"`
	Status             string  `json:"status"`
	Required           bool    `json:"required"`
	// Mirror* name the pinned counterpart on the other platform (null when the
	// mod is single-platform here). Versions are pinned at add time on both
	// sides and never auto-updated.
	MirrorSource    *string `json:"mirrorSource"`
	MirrorProjectID *string `json:"mirrorProjectId"`
	// 展示性增强，全部可为 null / 空（前端各自降级，不作为功能依赖）：
	// NameZh = 社区中文名（别名表按 slug 反查）；Description = 平台一句话描述
	// （0029 起落库，历史行缺失时服务层读取时按需补拉并回写）；
	// ContentKinds = 解析产物精确分类计数（模组树展开计数与「空模组」判定用）。
	NameZh       *string          `json:"nameZh"`
	Description  *string          `json:"description"`
	ContentKinds map[string]int64 `json:"contentKinds"`
	// Origin: manual = 手动添加; compat-fix = 兼容知识库自动加装的补丁。
	Origin    string `json:"origin"`
	AddedAt   string `json:"addedAt"`
	UpdatedAt string `json:"updatedAt"`
}
type Lock struct {
	ID             string `json:"id"`
	PackID         string `json:"packId"`
	SchemaVersion  int    `json:"schemaVersion"`
	SnapshotJSON   string `json:"snapshot"`
	SnapshotSHA256 string `json:"sha256"`
	CreatedAt      string `json:"createdAt"`
}
type Conflict struct {
	ID          string `json:"id"`
	PackID      string `json:"packId"`
	Fingerprint string `json:"fingerprint"`
	Kind        string `json:"kind"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	// detailPath / resolvedAt 不能带 omitempty：前端契约 (apps/web/src/api/mods.ts
	// conflictSchema) 声明的是「字符串可为 null」,而 omitempty 会把空串/nil 整个键
	// 省掉, zod 收到 undefined 直接判「接口数据结构不符合约定」, 整页依赖数据加载失败
	// (截图实测: /packs/:id/dependencies 只有一行红色报错, 冲突列表全空)。
	DetailPath string         `json:"detailPath"`
	Detail     map[string]any `json:"detail,omitempty"`
	// 时间字段必须显式打 tag：Go 默认导出 CreatedAt/UpdatedAt, 与全站 camelCase 不一致。
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	ResolvedAt *string `json:"resolvedAt"`
}
type PackHealth struct {
	PackID          string `json:"packId"`
	Mods            int    `json:"mods"`
	Installed       int    `json:"installed"`
	PendingErrors   int    `json:"pendingErrors"`
	PendingWarnings int    `json:"pendingWarnings"`
	Healthy         bool   `json:"healthy"`
}

var p5Registries sync.Map // *API -> *provider.Registry; composition stays out of transport.

// SetProviderRegistry injects provider adapters at the composition root (most
// commonly fixture adapters in tests, production adapters later).
func (a *API) SetProviderRegistry(r *provider.Registry) {
	if a != nil {
		p5Registries.Store(a, r)
	}
}
func (a *API) p5Registry() *provider.Registry {
	if a == nil {
		return nil
	}
	if r, ok := p5Registries.Load(a); ok {
		return r.(*provider.Registry)
	}
	return nil
}

func (a *API) ModSearch(ctx context.Context, packID string, in ModSearchInput) (ModSearchResult, error) {
	if err := a.ready(); err != nil {
		return ModSearchResult{}, err
	}
	pack, err := a.repo.GetPack(ctx, packID)
	if err != nil {
		return ModSearchResult{}, err
	}
	in = withPackSearchDefaults(in, pack)
	ad, err := a.p5Adapter(in.Provider)
	if err != nil {
		return ModSearchResult{}, err
	}
	r, err := ad.Search(ctx, provider.SearchRequest{Query: platformSearchQuery(in.Query), MCVersion: in.MCVersion, Loader: in.Loader, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return ModSearchResult{}, mapProviderError(err)
	}
	out := ModSearchResult{Items: r.Items, NextCursor: r.NextCursor, Total: r.Total}
	// 与多平台路径同一开关：主结果里没有真正相关的命中才降级（理由见 ModSearchAll）。
	terms := searchTerms(in.Query)
	if in.Loader != "" && !hasRelevantProject(terms, r.Items) {
		out.Fallback = relaxLoaderAndSearch(ctx, ad, in, terms)
	}
	return out, nil
}

// relaxLoaderAndSearch 摘掉加载器限制再搜一次，只留下「明确只支持别的加载器」的条目。
//
// 这是「降级搜索」的实现：Fabric 包里搜 Mekanism 时，带 fabric facet 查是空的，
// 但同 MC 版本下 Mekanism 确实存在（Forge/NeoForge 版）。把这些条目交出去，
// 界面就能解释清楚，而不是留一个空列表让人怀疑搜索坏了。
//
// 查不到 MC 版本这一维**不要摘**：降级是为了回答「换个加载器能不能装」，
// 如果把 MC 版本也放开，返回的会是一堆版本也对不上的噪音。
func relaxLoaderAndSearch(ctx context.Context, ad provider.Adapter, in ModSearchInput, terms []string) []provider.Project {
	relaxed := in
	relaxed.Loader = ""
	r, err := ad.Search(ctx, provider.SearchRequest{Query: platformSearchQuery(relaxed.Query), MCVersion: relaxed.MCVersion, Cursor: relaxed.Cursor, Limit: relaxed.Limit})
	if err != nil {
		return nil // 降级是增强，失败就退回「没有额外交代」而不是把主搜索也报错
	}
	return onlyOtherLoaderProjects(r.Items, in.Loader, terms)
}

// onlyOtherLoaderProjects 是 otherLoaderRelevant 在纯项目列表上的版本。
func onlyOtherLoaderProjects(items []provider.Project, want string, terms []string) []provider.Project {
	out := []provider.Project{}
	for _, p := range items {
		if len(out) >= modSearchFallbackLimit {
			break
		}
		if otherLoaderRelevant(p, want, terms) {
			out = append(out, p)
		}
	}
	return out
}

// hasRelevantProject 是 hasRelevantHit 在纯项目列表上的版本。
func hasRelevantProject(terms []string, items []provider.Project) bool {
	for _, p := range items {
		if modSearchBestScore(terms, p) >= modSearchRelevantScore {
			return true
		}
	}
	return false
}

// searchTerms 返回本次搜索的评分词表：原始查询在前，别名展开的英文词在后。
//
// 中文别名（通用机械）在英文平台元数据上永远打不到分（modSearchScore 最低档
// 10 分）——平台查询词已经换用别名词，相关度判定、降级准入也必须用别名词
// 再看一遍取最高分，否则降级区的 ≥50 分门槛会把别名正主滤掉：
// Fabric 包搜「通用机械」时 Mekanism 只有 forge/neoforge 版，主结果被 loader
// 拦掉、降级区又被打分门槛滤掉，哪儿都不出现（2026-10-04 排查确认的回归）。
func searchTerms(query string) []string {
	q := strings.TrimSpace(query)
	terms := []string{q}
	if s := expandModSearchAlias(q); s != "" && !strings.EqualFold(s, q) {
		terms = append(terms, s)
	}
	return terms
}

// modSearchBestScore 取整个词表的最高相关度分。
func modSearchBestScore(terms []string, p provider.Project) int {
	best := 0
	for _, t := range terms {
		if s := modSearchScore(t, p); s > best {
			best = s
		}
	}
	return best
}

func declaresLoader(loaders []string, want string) bool {
	for _, l := range loaders {
		if strings.EqualFold(strings.TrimSpace(l), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// withPackSearchDefaults 把调用方没显式指定的过滤维度补成当前包的口径。
//
// 商店面板原先只传 `q`，于是搜出来的是「世上所有叫这名字的模组」——包括
// 装不进这个包的。加载器与 MC 版本是两个独立维度，缺一个都会漏进不兼容项，
// 所以两个都要补。
func withPackSearchDefaults(in ModSearchInput, pack store.PackRecord) ModSearchInput {
	if strings.TrimSpace(in.MCVersion) == "" {
		in.MCVersion = strings.TrimSpace(pack.MCVersion)
	}
	if strings.TrimSpace(in.Loader) == "" {
		in.Loader = packSearchLoader(pack.Loader)
	}
	return in
}

// packSearchLoader 把包记录里的加载器转成搜索过滤值；认不出来就返回空(=不过滤)。
//
// 这里刻意**不复用 normalizeLoader**：那个函数把一切不认识的值兜底成 "forge"，
// 用来展示无害，用在搜索过滤上却会让一个 loader 字段异常的包被静默当成 Forge 包，
// 搜出一堆同样装不上的东西。搜索宁可不过滤，也不要猜错。
func packSearchLoader(v string) string {
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case "fabric", "forge", "neoforge", "quilt":
		return s
	default:
		return ""
	}
}
func (a *API) p5Adapter(name string) (provider.Adapter, error) {
	ad, err := a.p5Registry().Get(name)
	if err != nil {
		// 注册表里没有这个 provider 名字 = 请求写错了,该给 400;
		// 只有真·远端不可用才是 502。以前一律摊成 ErrProviderUnavailable。
		if errors.Is(err, provider.ErrNotFound) {
			return nil, ErrInvalidArgument
		}
		return nil, ErrProviderUnavailable
	}
	return ad, nil
}

// ModVersions lists a project's versions on a provider so the UI can offer a
// real version choice before AddPackMod. Read-only; no download happens here.
func (a *API) ModVersions(ctx context.Context, packID, providerName, projectID string) ([]provider.Version, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, ErrInvalidArgument
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	ad, err := a.p5Adapter(providerName)
	if err != nil {
		return nil, err
	}
	v, err := ad.Versions(ctx, projectID)
	if err != nil {
		return nil, mapProviderError(err)
	}
	return v, nil
}
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, provider.ErrNotFound):
		return ErrProviderNotFound
	case errors.Is(err, provider.ErrUnavailable), errors.Is(err, provider.ErrRateLimited), errors.Is(err, provider.ErrUnauthorized):
		return ErrProviderUnavailable
	default:
		return err
	}
}

// ModSearchMirror is the other-platform half of a merged catalog hit. Present
// only when the same mod was found on both platforms (identity table first,
// normalized-name pairing as fallback).
type ModSearchMirror struct {
	Provider  string `json:"provider"`
	ProjectID string `json:"projectId"`
	Slug      string `json:"slug"`
	Downloads int64  `json:"downloads"`
}

// ModSearchAllItem is one catalog hit tagged with the platform it came from.
// When Mirror is set the two entries are the same mod; Downloads is then the
// sum of both platforms so dual-platform mods rank first.
type ModSearchAllItem struct {
	Provider string `json:"provider"`
	provider.Project
	Mirror *ModSearchMirror `json:"mirror,omitempty"`
}

// ModSearchAllResult merges every platform's hits and reports per-platform
// failures independently, so one missing key or outage never blocks the rest.
type ModSearchAllResult struct {
	Items      []ModSearchAllItem `json:"items"`
	Errors     map[string]string  `json:"errors"`
	Total      int                `json:"total"`
	NextCursor *string            `json:"next_cursor"`
	// Fallback 是「降级搜索结果」：本包加载器搜不到东西时，摘掉加载器限制重搜
	// 得到的、明确只支持其他加载器的模组。每个条目的 Loaders 会让界面能说清
	// 「这个只有 NeoForge 版」，而不是让用户对着一片空白猜。
	Fallback []ModSearchAllItem `json:"fallback,omitempty"`
}

// providerErrorCode maps provider failures to stable per-platform codes.
func providerErrorCode(err error) string {
	switch {
	case errors.Is(err, provider.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, provider.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, provider.ErrNotFound):
		return "not_found"
	default:
		return "unavailable"
	}
}

// ModSearchAll 把一次模糊查询并发扇出到所有已知平台，并保证「搜出来的东西
// 装得进当前包」。两件事：
//
//  1. 调用方没给 loader / mcVersion 时，用包的现成口径补齐。商店面板只传 q，
//     少了这一步就会把 Forge 专有的模组摆在 Fabric 包的用户面前。
//  2. 补齐后仍然一个都搜不到，就摘掉加载器限制再搜一遍，结果放进 Fallback
//     —— 让界面能说清「Mekanism 只有 Forge / NeoForge 版」，而不是留一个
//     空列表让人以为搜索坏了。
func (a *API) ModSearchAll(ctx context.Context, packID string, in ModSearchInput) (ModSearchAllResult, error) {
	if err := a.ready(); err != nil {
		return ModSearchAllResult{}, err
	}
	pack, err := a.repo.GetPack(ctx, packID)
	if err != nil {
		return ModSearchAllResult{}, err
	}
	in = withPackSearchDefaults(in, pack)
	terms := searchTerms(in.Query)

	out, err := a.modSearchFanout(ctx, in)
	if err != nil {
		return ModSearchAllResult{}, err
	}
	out.Total = len(out.Items)
	// 触发降级的条件**不是**「主结果为空」，而是「主结果里没有任何一条跟查询
	// 真正相关」。平台的模糊搜索会用摘要里的边角匹配把列表塞满 —— 实测在一个
	// Fabric 包里搜 mek，主结果非空但两条都是 NoEmotecraft / KeProfiles 这种
	// 噪音，用「空」当开关等于永不降级，Mekanism 永远露不出来。
	if in.Loader != "" && !hasRelevantHit(terms, out.Items) {
		relaxed := in
		relaxed.Loader = ""
		if alt, altErr := a.modSearchFanout(ctx, relaxed); altErr == nil {
			out.Fallback = onlyOtherLoaderItems(alt.Items, in.Loader, terms)
		}
	}
	return out, nil
}

// onlyOtherLoaderItems 是 otherLoaderRelevant 在合并卡片上的版本。
func onlyOtherLoaderItems(items []ModSearchAllItem, want string, terms []string) []ModSearchAllItem {
	out := []ModSearchAllItem{}
	for _, it := range items {
		if len(out) >= modSearchFallbackLimit {
			break
		}
		if otherLoaderRelevant(it.Project, want, terms) {
			out = append(out, it)
		}
	}
	return out
}

// otherLoaderRelevant 是降级区**唯一**的准入判断，两个条件缺一不可：
//
//  1. 明确声明了加载器、且都不含本包的加载器（说不出加载器的不算，因为降级区
//     的意义就是解释「为什么不适用」）；
//  2. 跟查询真正相关（见 modSearchBestScore，评分词表含别名展开词 —— 中文
//     别名对英文元数据打不上分，不看别名词 Mekanism 就会被这道门槛滤掉）。
//
// 第 2 条不是锦上添花：降级查询摘掉了加载器限制，平台会把摘要里沾边的全都倒
// 出来，不过滤的话降级区就是一屏噪音，正好把真正的答案（Mekanism）盖掉。
func otherLoaderRelevant(p provider.Project, want string, terms []string) bool {
	if len(p.Loaders) == 0 || declaresLoader(p.Loaders, want) {
		return false
	}
	return modSearchBestScore(terms, p) >= modSearchRelevantScore
}

// modSearchRelevantScore 是「这条命中跟查询是否真的相关」的分界线。
// 取 50 正好把「slug 包含」及以上算相关，把「仅摘要提及」(20) 和平台硬塞 (10) 排除。
const modSearchRelevantScore = 50

// modSearchFallbackLimit 给降级区封顶：它只是说明性的附注，不该长过主结果。
const modSearchFallbackLimit = 10

// hasRelevantHit 判断主结果里有没有一条跟查询真正相关的命中。
// 只看有没有，不看有几条 —— 有一条就说明「本包确实有这个东西」，不需要降级。
func hasRelevantHit(terms []string, items []ModSearchAllItem) bool {
	for _, it := range items {
		if modSearchBestScore(terms, it.Project) >= modSearchRelevantScore {
			return true
		}
	}
	return false
}

// platformSearchQuery 把用户输入换成实际发给平台的查询词：查询命中别名表时
// 用别名展开的 slug（「通用机械」→ mekanism）。平台元数据没有中文，按原文
// 发出去只会得到噪音甚至空结果；slug 在两个平台的搜索接口都能命中正主。
// 相关度仍按 searchTerms 的词表（原文 + 别名词）打分，用户输入不丢。
func platformSearchQuery(query string) string {
	q := strings.TrimSpace(query)
	if s := expandModSearchAlias(q); s != "" && !strings.EqualFold(s, q) {
		return s
	}
	return q
}

// modSearchFanout 是纯扇出：并发查询 + 跨平台配对 + slug 直取 + 复合排序。
//
// 它**不含**包口径补齐与降级决策 —— 那两件事留在调用方，因为降级要靠
// 「主结果是否为空」来判断，而本函数会被调用两次（主查一次、降级查一次）。
//
// Adapters are stateless, so no locking is needed: each goroutine writes only
// its own result slot. Rate-limit safety comes from exactly one request per
// platform per search, no retries, and a per-platform timeout.
func (a *API) modSearchFanout(ctx context.Context, in ModSearchInput) (ModSearchAllResult, error) {
	known := []provider.Name{provider.CurseForge, provider.Modrinth}
	type outcome struct {
		name  provider.Name
		items []provider.Project
		err   error
	}
	slots := make([]outcome, len(known))
	var wg sync.WaitGroup
	for i, name := range known {
		slots[i].name = name
		ad, err := a.p5Registry().Get(string(name))
		if err != nil {
			slots[i].err = errNotConfigured
			continue
		}
		wg.Add(1)
		go func(i int, ad provider.Adapter) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			// 复合排序需要足够的候选池：每平台至少取 25（同一次请求，不加请求数）
			fetchLimit := in.Limit
			if fetchLimit < 25 {
				fetchLimit = 25
			}
			// 平台查询词换用别名展开词（通用机械 → mekanism）：平台元数据没有
			// 中文，原文发出去只会得到噪音。相关度仍按 searchTerms 词表打分。
			r, err := ad.Search(pctx, provider.SearchRequest{Query: platformSearchQuery(in.Query), MCVersion: in.MCVersion, Loader: in.Loader, Cursor: in.Cursor, Limit: fetchLimit})
			if err != nil {
				slots[i].err = err
				return
			}
			slots[i].items = r.Items
		}(i, ad)
	}
	wg.Wait()

	out := ModSearchAllResult{Items: []ModSearchAllItem{}}
	for _, s := range slots {
		if s.err != nil {
			if out.Errors == nil {
				out.Errors = map[string]string{}
			}
			if errors.Is(s.err, errNotConfigured) {
				out.Errors[string(s.name)] = "not_configured"
			} else {
				out.Errors[string(s.name)] = providerErrorCode(s.err)
			}
			continue
		}
		for _, p := range s.items {
			out.Items = append(out.Items, ModSearchAllItem{Provider: string(s.name), Project: p})
		}
	}
	// 跨平台合并: 身份表(本机已确认 + 内置知识库)优先, 名称规范化相同兜底;
	// 配对成功的合并成一张卡, 下载量取两边之和。
	identities, err := a.repo.ListModIdentities(ctx)
	if err != nil {
		identities = nil // 合并是增强, 身份表读取失败不阻塞搜索
	}
	identities = append(identities, baselineModIdentities()...)
	out.Items = pairSearchItems(out.Items, identities)
	// slug 直取（模组搜索修复 2026-10-03）：平台相关度会埋掉 slug-only 正主
	// （搜 ae2 出一堆附属，Applied Energistics 2 的标题不含 ae2）。
	// 别名命中或查询本身像 slug 且结果里没有精确命中时，按 slug 直接拉一次。
	if slug := expandModSearchAlias(in.Query); slug != "" {
		out.Items = a.injectSlugMatch(ctx, slug, out.Items, in.Loader)
	} else if candidate := strings.ToLower(strings.TrimSpace(in.Query)); candidate != "" && !strings.ContainsAny(candidate, " \t") {
		out.Items = a.injectSlugMatch(ctx, candidate, out.Items, in.Loader)
	}
	// 复合排序：缩写/俗名优先（slug 精确 > 名称首词 > 名称分词前缀 > 名称包含 >
	// slug 前缀 > slug 包含 > 摘要），下载量兜底，provider+name 定序。
	// 别名注入的正主（精致存储→sophisticated-storage、aer→ae2）代表用户的
	// 搜索意图本身，排在一切有机命中之前（95 分，仅次于 slug 精确 100）。
	// 评分用词表最高分：中文别名查询（通用机械）对英文元数据打不上分，
	// 不看别名词的话降级区外的一切排序都退化成噪音序。
	terms := searchTerms(in.Query)
	aliasSlug := expandModSearchAlias(in.Query)
	sort.SliceStable(out.Items, func(i, j int) bool {
		si, sj := modSearchBestScore(terms, out.Items[i].Project), modSearchBestScore(terms, out.Items[j].Project)
		if aliasSlug != "" {
			if strings.EqualFold(out.Items[i].Slug, aliasSlug) {
				si = 95
			}
			if strings.EqualFold(out.Items[j].Slug, aliasSlug) {
				sj = 95
			}
		}
		if si != sj {
			return si > sj
		}
		if out.Items[i].Downloads != out.Items[j].Downloads {
			return out.Items[i].Downloads > out.Items[j].Downloads
		}
		if out.Items[i].Provider != out.Items[j].Provider {
			return out.Items[i].Provider < out.Items[j].Provider
		}
		return out.Items[i].Name < out.Items[j].Name
	})
	return out, nil
}

/* modSearchScore：查询与一个平台项目的相关度分（大者优先）。 */
func modSearchScore(query string, p provider.Project) int {
	q := strings.ToLower(strings.TrimSpace(query))
	slug := strings.ToLower(p.Slug)
	name := strings.ToLower(p.Name)
	switch {
	case q == "":
		return 50
	case slug == q:
		return 100
	// slug 模糊容错（别名注入的正主常差一个字符）：aer → ae2
	case len([]rune(q)) >= 3 && levenshtein(slug, q) <= 1:
		return 85
	case strings.HasPrefix(name, q):
		return 90
	}
	for _, tok := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if strings.HasPrefix(tok, q) {
			return 80
		}
	}
	switch {
	case strings.HasPrefix(slug, q):
		return 70
	case strings.Contains(name, q):
		return 60
	case strings.Contains(slug, q):
		return 50
	case p.Summary != "" && strings.Contains(strings.ToLower(p.Summary), q):
		return 20
	}
	return 10
}

/* injectSlugMatch：结果里没有 slug 精确命中时，按 slug 直取 Modrinth 项目并置顶。
   网络失败静默放弃（搜索本身已经给出可用结果），错误不外泄。 */
func (a *API) injectSlugMatch(ctx context.Context, slug string, items []ModSearchAllItem, loader string) []ModSearchAllItem {
	if slug == "" {
		return items
	}
	for _, it := range items {
		if strings.EqualFold(it.Slug, slug) {
			return items
		}
	}
	ad, err := a.p5Registry().Get(string(provider.Modrinth))
	if err != nil {
		return items
	}
	pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	p, err := ad.Project(pctx, slug)
	if err != nil {
		return items
	}
	// 这条直取路径绕开了平台侧的 facets，所以必须自己补一次加载器检查：
	// 否则搜 "mekanism" 时，slug 精确命中会把 Mekanism 直接塞进 Fabric 包的
	// 主结果里 —— 既让用户看到装不上的东西，又让「主结果为空才降级」永远
	// 不成立。降级查询时 loader 是空串，于是这里不拦，正好能把它捞回来。
	if loader != "" && len(p.Loaders) > 0 && !declaresLoader(p.Loaders, loader) {
		return items
	}
	return append([]ModSearchAllItem{{Provider: string(provider.Modrinth), Project: p}}, items...)
}

// normalizeModName folds a mod name for cross-platform comparison: case,
// spaces and punctuation are ignored ("Just Enough Items (JEI)" matches
// "just enough items jei"). Conservative on purpose: only exact normalized
// equality pairs two entries — a missed pair shows two cards, a wrong pair
// would weld two different mods together.
func normalizeModName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// pairSearchItems merges cross-platform duplicates of the same mod. The
// higher-download entry stays primary; the other platform becomes its Mirror.
// Same-provider name collisions are never merged.
func pairSearchItems(items []ModSearchAllItem, identities []store.ModIdentityRecord) []ModSearchAllItem {
	byID := make(map[string]string, 2*len(identities))
	for _, id := range identities {
		k := "pair:" + id.MRProjectID + "|" + id.CFProjectID
		byID["modrinth:"+id.MRProjectID] = k
		byID["curseforge:"+id.CFProjectID] = k
	}
	out := make([]ModSearchAllItem, 0, len(items))
	pos := map[string]int{}
	providerAt := map[string]string{}
	for _, it := range items {
		k := "name:" + normalizeModName(it.Name)
		if pk, ok := byID[it.Provider+":"+it.ID]; ok {
			k = pk
		}
		idx, seen := pos[k]
		if !seen || providerAt[k] == it.Provider {
			if !seen {
				pos[k] = len(out)
				providerAt[k] = it.Provider
			}
			out = append(out, it)
			continue
		}
		if it.Downloads > out[idx].Downloads {
			mirror := &ModSearchMirror{Provider: out[idx].Provider, ProjectID: out[idx].ID, Slug: out[idx].Slug, Downloads: out[idx].Downloads}
			it.Downloads += out[idx].Downloads
			it.Mirror = mirror
			out[idx] = it
			providerAt[k] = it.Provider
		} else {
			out[idx].Mirror = &ModSearchMirror{Provider: it.Provider, ProjectID: it.ID, Slug: it.Slug, Downloads: it.Downloads}
			out[idx].Downloads += it.Downloads
		}
	}
	return out
}

var errNotConfigured = errors.New("provider not configured")

func (a *API) ListPackMods(ctx context.Context, packID string) ([]Mod, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return nil, err
	}
	return a.enrichModDTOs(ctx, rows), nil
}

// ListPackContentSources returns every mod-shaped content source in a pack,
// including the required builtin Minecraft instance.
func (a *API) ListPackContentSources(ctx context.Context, packID string) ([]Mod, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListPackMembers(ctx, packID)
	if err != nil {
		return nil, err
	}
	return a.enrichModDTOs(ctx, rows), nil
}

// enrichModDTOs 给模组清单补展示字段，全部 best-effort（任何一步失败都只是
// 字段留空，清单本身照常返回）：
//   - ContentKinds：解析产物精确分类计数（mod_content GROUP BY，空模组判定与
//     展开计数都用它，前端不再对 1000 条取样自己数）；
//   - NameZh：社区中文名，按平台 slug 从别名表反查（modsearch_alias.go）；
//   - Description：平台一句话描述，读 platform_projects.description（0029），
//     历史行缺失时对前几个模组后台补拉一次并回写，下次清单就有了。
func (a *API) enrichModDTOs(ctx context.Context, rows []store.PackModRecord) []Mod {
	packID := ""
	externalIDs := make([]string, 0, len(rows))
	for _, m := range rows {
		if packID == "" {
			packID = m.PackID
		}
		if isPlatformSource(m.Source) && m.ProjectID != "" {
			externalIDs = append(externalIDs, m.ProjectID)
		}
	}
	briefs := map[string]store.PlatformProjectBrief{}
	if len(externalIDs) > 0 {
		if b, err := a.repo.PlatformProjectBriefs(ctx, externalIDs); err == nil {
			briefs = b
		}
	}
	kinds := map[string]map[string]int64{}
	if packID != "" {
		if k, err := a.repo.ModContentKindCounts(ctx, packID); err == nil {
			kinds = k
		}
	}

	out := make([]Mod, 0, len(rows))
	var missing [][2]string // {platform, projectID}
	for _, m := range rows {
		dto := modDTO(m)
		if k := kinds[m.ID]; k != nil {
			dto.ContentKinds = k
		} else {
			dto.ContentKinds = map[string]int64{}
		}
		switch {
		case m.ModID == "minecraft":
			// 内置原版行：没有平台元数据，描述与中文名直接给定。
			desc := "Minecraft 本体（原版物品、配方、进度、结构、群系、语言与纹理）。"
			zh := "我的世界"
			dto.Description, dto.NameZh = &desc, &zh
		case isPlatformSource(m.Source) && m.ProjectID != "":
			if b, ok := briefs[m.Source+"|"+m.ProjectID]; ok {
				if zh := zhNameForSlug(b.Slug); zh != "" {
					dto.NameZh = &zh
				}
				if b.Description != "" {
					dto.Description = &b.Description
				} else {
					missing = append(missing, [2]string{m.Source, m.ProjectID})
				}
			} else {
				missing = append(missing, [2]string{m.Source, m.ProjectID})
			}
		}
		out = append(out, dto)
	}
	if len(missing) > 0 {
		go a.backfillDescriptions(missing)
	}
	return out
}

// backfillDescriptions 对缺描述的平台模组补拉一次（≤5 个、共享 10s 预算）并
// 回写 platform_projects。fire-and-forget：描述是展示增强，失败静默，
// 不重试 —— 用户下次进面板时清单还会再触发一轮。
func (a *API) backfillDescriptions(missing [][2]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if len(missing) > 5 {
		missing = missing[:5]
	}
	var wg sync.WaitGroup
	for _, m := range missing {
		platform, projectID := m[0], m[1]
		ad, err := a.p5Adapter(platform)
		if err != nil {
			continue // 平台未配置/不可用：跳过，下次再说
		}
		wg.Add(1)
		go func(ad provider.Adapter, platform, projectID string) {
			defer wg.Done()
			p, err := ad.Project(ctx, projectID)
			if err != nil || p.Summary == "" {
				return
			}
			_ = a.repo.SetPlatformProjectDescription(ctx, platform, projectID, p.Summary)
		}(ad, platform, projectID)
	}
	wg.Wait()
}

// isPlatformSource 判断来源是不是平台模组（本地 zip 上传的没有平台元数据）。
func isPlatformSource(source string) bool {
	return source == "modrinth" || source == "curseforge"
}

// zhNameForSlug 从别名表反查社区中文名。别名表的 key 一半是英文缩写
// （ae2/jei），只有含汉字的别名才算「中文名」，反查时跳过英文键。
// 表里没有的模组返回空串，前端降级成只显示英文名。
func zhNameForSlug(slug string) string {
	if slug == "" {
		return ""
	}
	for alias, s := range modSearchAliases {
		if s == slug && containsHan(alias) {
			return alias
		}
	}
	return ""
}

// containsHan 判断字符串是否含汉字。
func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func modDTO(m store.PackModRecord) Mod {
	strPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	origin := m.Origin
	if origin == "" {
		origin = "manual"
	}
	dto := Mod{ID: m.ID, Category: m.Category, CanonicalModID: m.ModID, CurrentSelectionID: m.CurrentSelectionID, PackID: m.PackID, Source: m.Source, ProjectID: strPtr(m.ProjectID), VersionID: strPtr(m.VersionID), DisplayName: m.DisplayName, FileName: m.FileName, SHA1: strPtr(m.SHA1), Status: m.Status, Required: m.Required, MirrorSource: strPtr(m.MirrorSource), MirrorProjectID: strPtr(m.MirrorProjectID), Origin: origin, AddedAt: iso(m.AddedAt), UpdatedAt: iso(m.UpdatedAt)}
	// 契约要求 contentKinds 永远是对象（前端 zod record 不收 null）；清单路径的
	// 精确计数由 enrichModDTOs 填，单模组响应（添加/更新等）给空对象即可。
	dto.ContentKinds = map[string]int64{}
	return dto
}

// otherProviderOf names the opposite catalog platform, or "" for local mods.
func otherProviderOf(source string) string {
	switch source {
	case "modrinth":
		return "curseforge"
	case "curseforge":
		return "modrinth"
	}
	return ""
}

// resolveMirror best-effort pins the counterpart project+version on the other
// platform at add time. Every failure path is silent by design (用户拍板:
// 镜像查不到照常添加, 标"仅单平台"), and once pinned the mirror never follows
// newer releases — rebuilding the pack reproduces exactly what was debugged.
func (a *API) resolveMirror(ctx context.Context, m *store.PackModRecord, pack store.PackRecord, primaryVersionNumber string) {
	otherName := otherProviderOf(m.Source)
	if otherName == "" || m.ProjectID == "" {
		return
	}
	ad, err := a.p5Adapter(otherName)
	if err != nil {
		return // 对方平台未配置/不可用: 仅单平台
	}
	// 1. 定位对方平台项目: 已知镜像 > 身份表 > 名称精确搜索
	otherProject := m.MirrorProjectID
	if otherProject == "" {
		if ids, err := a.repo.ListModIdentities(ctx); err == nil {
			ids = append(ids, baselineModIdentities()...)
			for _, id := range ids {
				if m.Source == "modrinth" && id.MRProjectID == m.ProjectID {
					otherProject = id.CFProjectID
				} else if m.Source == "curseforge" && id.CFProjectID == m.ProjectID {
					otherProject = id.MRProjectID
				}
				if otherProject != "" {
					break
				}
			}
		}
	}
	if otherProject == "" {
		r, err := ad.Search(ctx, provider.SearchRequest{Query: m.DisplayName, Limit: 10})
		if err != nil {
			return
		}
		for _, p := range r.Items {
			if normalizeModName(p.Name) == normalizeModName(m.DisplayName) {
				otherProject = p.ID
				break
			}
		}
	}
	if otherProject == "" {
		return
	}
	m.MirrorSource, m.MirrorProjectID = otherName, otherProject
	// 2. 立即钉版本: 兼容当前包(MC 版本+loader)优先, 同版本号优先, 否则最新兼容
	if vs, err := ad.Versions(ctx, otherProject); err == nil {
		if best := pickMirrorVersion(vs, pack.MCVersion, pack.Loader, primaryVersionNumber); best != "" {
			m.MirrorVersionID = best
		}
	}
	// 3. 项目级配对永久复用(即使版本没钉到)
	mr, cf := otherProject, m.ProjectID
	if m.Source == "modrinth" {
		mr, cf = m.ProjectID, otherProject
	}
	_ = a.repo.UpsertModIdentity(ctx, store.ModIdentityRecord{MRProjectID: mr, CFProjectID: cf, DisplayName: m.DisplayName, ConfirmedAt: time.Now().UnixMilli()})
}

// pickMirrorVersion chooses the counterpart file: same version number wins,
// otherwise the newest file compatible with the pack (versions arrive
// newest-first from the provider layer). "" means no compatible file exists.
func pickMirrorVersion(vs []provider.Version, mcVersion, loader, wantVersionNumber string) string {
	loader = strings.ToLower(loader)
	firstCompatible := ""
	for _, v := range vs {
		mcOK, loaderOK := false, loader == ""
		for _, g := range v.GameVersions {
			if g == mcVersion {
				mcOK = true
				break
			}
		}
		for _, l := range v.Loaders {
			if strings.ToLower(l) == loader {
				loaderOK = true
				break
			}
		}
		if !mcOK || !loaderOK {
			continue
		}
		if firstCompatible == "" {
			firstCompatible = v.ID
		}
		if wantVersionNumber != "" && v.VersionNumber == wantVersionNumber {
			return v.ID
		}
	}
	return firstCompatible
}

func (a *API) AddPackMod(ctx context.Context, packID string, in AddModInput, requestID string) (Mod, error) {
	return a.addPackMod(ctx, packID, in, requestID, "manual", 0)
}

// addPackMod is the add flow with origin tagging and auto-fix recursion depth.
// origin "manual" = 用户手动添加; "compat-fix" = 兼容知识库自动加装的补丁。
func (a *API) addPackMod(ctx context.Context, packID string, in AddModInput, requestID, origin string, depth int) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	if strings.TrimSpace(in.Provider) == "" || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.VersionID) == "" {
		return Mod{}, ErrInvalidArgument
	}
	packRec, err := a.repo.GetPack(ctx, packID)
	if err != nil {
		return Mod{}, err
	}
	ad, err := a.p5Adapter(in.Provider)
	if err != nil {
		return Mod{}, err
	}
	meta, err := ad.Metadata(ctx, in.ProjectID, in.VersionID)
	if err != nil {
		return Mod{}, mapProviderError(err)
	}
	dl, err := ad.Download(ctx, provider.DownloadRequest{ProjectID: in.ProjectID, VersionID: in.VersionID})
	if err != nil {
		return Mod{}, mapProviderError(err)
	}
	var extracted *ExtractedContent
	dlSHA512 := ""
	if len(dl.Content) > 0 {
		actualSHA1, actualSHA256, actualSHA512, verifyErr := validateMeasuredDownload(dl.SHA1, dl.SHA256, dl.Content)
		if verifyErr != nil {
			return Mod{}, ErrInvalidSHA1
		}
		dlSHA512 = actualSHA512
		dl.SHA1, dl.SHA256, dl.Size = actualSHA1, actualSHA256, int64(len(dl.Content))
		extracted, err = ExtractModContent(bytes.NewReader(dl.Content))
		if err != nil {
			return Mod{}, ErrInvalidArgument
		}
	}
	if !validHash(dl.SHA1, 40) {
		return Mod{}, ErrInvalidSHA1
	}
	now := time.Now().UnixMilli()
	id := newID("mod")
	canonicalModID := ""
	if extracted != nil {
		canonicalModID = normalizeDeclaredModID(extracted.ModID)
		if canonicalModID == "" {
			return Mod{}, ErrInvalidArgument
		}
	}
	m := store.PackModRecord{ID: id, PackID: packID, ModID: canonicalModID, Source: string(ad.Name()), ProjectID: in.ProjectID, VersionID: in.VersionID, DisplayName: meta.Project.Name, FileName: dl.FileName, SHA1: strings.ToLower(dl.SHA1), Status: "installed", Required: in.Required, AddedAt: now, UpdatedAt: now, Origin: origin}
	// 添加时立即钉死另一平台的对应版本(查不到不阻塞: 照常添加, 仅单平台)。
	a.resolveMirror(ctx, &m, packRec, meta.Version.VersionNumber)
	activityText := "Added " + m.DisplayName
	if origin == "compat-fix" {
		activityText = "Auto-added compat fix " + m.DisplayName + "(兼容知识库命中, 自动加装)"
	}
	err = a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: m.SHA1, SHA256: dl.SHA256, FilePath: "jar://" + m.SHA1, SizeBytes: dl.Size, ModIDs: []string{id}, ParsedAt: now}); err != nil {
			return err
		}
		if err := tx.AddPackMod(ctx, &m); err != nil {
			return err
		}
		if extracted != nil {
			selection := verifiedPackSelection(m, extracted.Version, dl.SHA256, dlSHA512, dl.DownloadURL, dl.Size, now)
			selection.ProjectSlug = meta.Project.Slug
			selection.ReleaseName = meta.Version.VersionNumber
			if err := tx.EnsurePackSelection(ctx, selection); err != nil {
				return err
			}
		}
		if err := tx.InvalidatePackGeneration(ctx, packID); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "add-mod", Text: activityText, At: now}, map[string]any{"mod_id": m.ID}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", m.ID, "mod.added", map[string]any{"mod_id": m.ID}, now)
	})
	if err != nil {
		return Mod{}, err
	}
	// 兼容知识库: 新模组入场后检查已知冲突, 有官方解法且解法模组不在包里就
	// 自动加装(只加兼容补丁不带内容; 深度限制防链式失控)。失败不影响本次添加。
	if depth < 2 {
		a.autoFixCompat(ctx, packRec, requestID, depth)
	}
	return modDTO(m), nil
}

// autoFixCompat scans the pack against the embedded compat knowledge and
// installs fix mods for known issues. Best-effort: any failure is silent —
// unresolved known issues still surface as conflicts on the next resolve.
func (a *API) autoFixCompat(ctx context.Context, pack store.PackRecord, requestID string, depth int) {
	mods, err := a.repo.ListPackMods(ctx, pack.ID)
	if err != nil {
		return
	}
	for _, hit := range scanCompatKnowledge(pack.MCVersion, pack.Loader, mods, baselineCompatKnowledge()) {
		fix := hit.Issue.Fix
		if fix == nil || fix.Type != "install_mod" || fixAlreadyPresent(mods, fix) {
			continue
		}
		// 修复模组优先走 Modrinth(更快), 未配置再用 CurseForge
		providerName, projectID := "", ""
		if fix.Mod.MR != "" {
			if _, err := a.p5Adapter("modrinth"); err == nil {
				providerName, projectID = "modrinth", fix.Mod.MR
			}
		}
		if providerName == "" && fix.Mod.CF != "" {
			if _, err := a.p5Adapter("curseforge"); err == nil {
				providerName, projectID = "curseforge", fix.Mod.CF
			}
		}
		if providerName == "" {
			continue
		}
		ad, _ := a.p5Adapter(providerName)
		vs, err := ad.Versions(ctx, projectID)
		if err != nil {
			continue
		}
		versionID := pickMirrorVersion(vs, pack.MCVersion, pack.Loader, "")
		if versionID == "" {
			continue // 没有兼容当前包的版本, 留给冲突列表提示
		}
		_, _ = a.addPackMod(ctx, pack.ID, AddModInput{Provider: providerName, ProjectID: projectID, VersionID: versionID, Required: false}, requestID, "compat-fix", depth+1)
	}
}

// AddLocalPackMod registers a pre-indexed local JAR by content hash. The
// client never supplies a server filesystem path; blobstore owns that mapping.
func (a *API) AddLocalPackMod(ctx context.Context, packID string, in LocalModInput, requestID string) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return Mod{}, err
	}
	if !validHash(strings.ToLower(in.SHA1), 40) || strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.FileName) == "" || in.Size < 0 {
		return Mod{}, ErrInvalidArgument
	}
	now := time.Now().UnixMilli()
	id := newID("mod")
	m := store.PackModRecord{ID: id, PackID: packID, Source: "local", DisplayName: strings.TrimSpace(in.DisplayName), FileName: strings.TrimSpace(in.FileName), SHA1: strings.ToLower(in.SHA1), Status: "installed", Required: in.Required, AddedAt: now, UpdatedAt: now}
	err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: m.SHA1, SHA256: in.SHA256, FilePath: "jar://" + m.SHA1, SizeBytes: in.Size, ModIDs: []string{id}, ParsedAt: now}); err != nil {
			return err
		}
		if err := tx.AddPackMod(ctx, &m); err != nil {
			return err
		}
		if err := tx.InvalidatePackGeneration(ctx, packID); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "add-mod", Text: "Added local " + m.DisplayName, At: now}, map[string]any{"mod_id": m.ID}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", m.ID, "mod.added", map[string]any{"mod_id": m.ID}, now)
	})
	if err != nil {
		return Mod{}, err
	}
	return modDTO(m), nil
}
func validHash(v string, n int) bool {
	if len(v) != n {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func (a *API) UpdatePackMod(ctx context.Context, packID, modID string, in UpdateModInput, requestID string) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	rows, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return Mod{}, err
	}
	var found store.PackModRecord
	ok := false
	for _, m := range rows {
		if m.ID == modID {
			found = m
			ok = true
			break
		}
	}
	if !ok {
		return Mod{}, store.ErrNotFound
	}
	// 改之前的快照：末尾据此判断这次 PATCH 是否真的动了目录。
	before := found
	if in.Category != nil {
		found.Category = strings.TrimSpace(*in.Category)
	}
	if in.VersionID != nil {
		if strings.TrimSpace(*in.VersionID) == "" {
			return Mod{}, ErrInvalidArgument
		}
		if *in.VersionID != found.VersionID {
			ad, e := a.p5Adapter(found.Source)
			if e != nil {
				return Mod{}, e
			}
			meta, e := ad.Metadata(ctx, found.ProjectID, *in.VersionID)
			if e != nil {
				return Mod{}, mapProviderError(e)
			}
			dl, e := ad.Download(ctx, provider.DownloadRequest{ProjectID: found.ProjectID, VersionID: *in.VersionID})
			if e != nil {
				return Mod{}, mapProviderError(e)
			}
			var selection *store.PackScopedSelection
			if len(dl.Content) > 0 {
				actualSHA1, actualSHA256, actualSHA512, verifyErr := validateMeasuredDownload(dl.SHA1, dl.SHA256, dl.Content)
				if verifyErr != nil {
					return Mod{}, ErrInvalidSHA1
				}
				extracted, extractErr := ExtractModContent(bytes.NewReader(dl.Content))
				if extractErr != nil || normalizeDeclaredModID(extracted.ModID) != found.ModID {
					return Mod{}, ErrInvalidArgument
				}
				dl.SHA1, dl.SHA256, dl.Size = actualSHA1, actualSHA256, int64(len(dl.Content))
				found.VersionID, found.SHA1, found.FileName, found.DisplayName, found.Status = *in.VersionID, actualSHA1, dl.FileName, meta.Project.Name, "installed"
				prepared := verifiedPackSelection(found, extracted.Version, actualSHA256, actualSHA512, dl.DownloadURL, dl.Size, time.Now().UnixMilli())
				prepared.ProjectSlug, prepared.ReleaseName = meta.Project.Slug, meta.Version.VersionNumber
				selection = &prepared
			} else {
				if !validHash(dl.SHA1, 40) {
					return Mod{}, ErrInvalidSHA1
				}
				found.VersionID, found.SHA1, found.FileName, found.DisplayName, found.Status = *in.VersionID, strings.ToLower(dl.SHA1), dl.FileName, meta.Project.Name, "pending"
			}
			// 主版本换了, 镜像版本跟着重钉(镜像项目沿用已配对的, 不重新找)。
			if packRec, e := a.repo.GetPack(ctx, packID); e == nil {
				a.resolveMirror(ctx, &found, packRec, meta.Version.VersionNumber)
			}
			now := time.Now().UnixMilli()
			found.UpdatedAt = now
			if err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
				if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: found.SHA1, SHA256: dl.SHA256, FilePath: "jar://" + found.SHA1, SizeBytes: dl.Size, ParsedAt: now}); err != nil {
					return err
				}
				if err := tx.InvalidatePackGeneration(ctx, packID); err != nil {
					return err
				}
				if err := tx.UpdatePackMod(ctx, found); err != nil {
					return err
				}
				if selection != nil {
					return tx.EnsurePackSelection(ctx, *selection)
				}
				return nil
			}); err != nil {
				return Mod{}, err
			}
			return modDTO(found), nil
		}
		found.VersionID = *in.VersionID
	}
	if in.Status != nil {
		switch *in.Status {
		case "pending", "installed", "disabled":
			if *in.Status == "installed" && found.CurrentSelectionID == "" {
				return Mod{}, ErrInvalidArgument
			}
			found.Status = *in.Status
		default:
			return Mod{}, ErrInvalidArgument
		}
	}
	if in.Required != nil {
		found.Required = *in.Required
	}
	found.UpdatedAt = time.Now().UnixMilli()
	if err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		// 只有真正影响目录成员/内容的字段变了才作废目录。分类(category)是纯展示
		// 字段，改它不该让 1330 项目录全部变成 pending —— 这条链路上曾经
		// 「挪一次分类 => 索引页变 409 占位」，根因就在这里。
		if catalogRelevantModChange(before, found) {
			if err := tx.InvalidatePackGeneration(ctx, packID); err != nil {
				return err
			}
		}
		return tx.UpdatePackMod(ctx, found)
	}); err != nil {
		return Mod{}, err
	}
	return modDTO(found), nil
}

// catalogRelevantModChange 回答「这次模组记录的改动会不会让已建好的物品目录失效」。
//
// 口径必须与迁移 0028 收窄后的 catalog_pack_mods_UPDATE 触发器一致：目录成员
// 由 status / sha1 / current_selection_id 决定，目录输入还牵涉 version_id 与
// 镜像钉版；category（用户分类）、required（构建开关）、display_name 等纯展示
// 或纯构建期字段都不在列。两边一旦分叉，就会出现「SQL 说没变、Go 说变了」。
func catalogRelevantModChange(before, after store.PackModRecord) bool {
	return before.Status != after.Status ||
		before.SHA1 != after.SHA1 ||
		before.CurrentSelectionID != after.CurrentSelectionID ||
		before.VersionID != after.VersionID ||
		before.MirrorVersionID != after.MirrorVersionID ||
		before.MirrorSource != after.MirrorSource ||
		before.MirrorProjectID != after.MirrorProjectID ||
		before.FileName != after.FileName ||
		before.ProjectID != after.ProjectID ||
		before.ModID != after.ModID ||
		before.Origin != after.Origin
}
func (a *API) RemovePackMod(ctx context.Context, packID, modID, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	at := time.Now().UnixMilli()
	err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.RemovePackMod(ctx, packID, modID, at); err != nil {
			return err
		}
		if err := tx.InvalidatePackGeneration(ctx, packID); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "remove-mod", Text: "Removed mod", At: at}, map[string]any{"mod_id": modID}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", modID, "mod.removed", map[string]any{"mod_id": modID}, at)
	})
	return err
}

func (a *API) ResolvePack(ctx context.Context, packID, requestID string) (Lock, error) {
	if err := a.ready(); err != nil {
		return Lock{}, err
	}
	// Fail as 404 pack_not_found: without this the empty member list flows on and
	// the lock insert trips the packs FK, surfacing as 500 internal_error.
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return Lock{}, err
	}
	mods, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return Lock{}, err
	}
	members, err := a.repo.ListPackMembers(ctx, packID)
	if err != nil {
		return Lock{}, err
	}
	// 缺依赖的冲突要等人话名字：循环里先收集，结束后统一解析显示名再生成（见 explainMissingDependencies）。
	var pendingMissing []missingDependency
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	lockID := newID("lock")
	deps := []store.ModDependencyRecord{}
	confs := []store.ConflictRecord{}
	snap := struct {
		Schema       int                         `json:"schemaVersion"`
		PackID       string                      `json:"packId"`
		Mods         []Mod                       `json:"mods"`
		Dependencies []store.ModDependencyRecord `json:"dependencies"`
		Conflicts    []Conflict                  `json:"conflicts"`
	}{Schema: 2, PackID: packID, Mods: []Mod{}, Dependencies: nil, Conflicts: nil}
	for _, m := range members {
		snap.Mods = append(snap.Mods, modDTO(m))
	}
	for _, m := range mods {
		ad, e := a.p5Adapter(m.Source)
		if e != nil {
			// 平台不可达 / 这个模组没有平台来源，是**网络与配置态**，不是包本身的缺陷。
			// 写成 error 会让构建闸门（O20）把「Modrinth 此刻打不开」变成「这个包不许
			// 构建」，用户在界面上既看不懂也修不了，只能反复 resolve 碰运气。因此固定
			// warning：列表里照样留痕（本轮没校验过它的依赖），但不拦构建。
			c := conflict(m, "provider_unavailable",
				fmt.Sprintf("平台暂不可用，本轮未校验依赖：%s", displayOrID(m.DisplayName, m.ID)), e.Error())
			c.Severity = "warning"
			confs = append(confs, c)
			continue
		}
		meta, e := ad.Metadata(ctx, m.ProjectID, m.VersionID)
		if e != nil {
			c := conflict(m, "provider_unavailable",
				fmt.Sprintf("模组元数据拉取失败，本轮未校验依赖：%s", displayOrID(m.DisplayName, m.ID)), e.Error())
			c.Severity = "warning"
			confs = append(confs, c)
			continue
		}
		for _, d := range meta.Dependencies {
			dID := fmt.Sprintf("dep-%s-%d", lockID, len(deps))
			drec := store.ModDependencyRecord{ID: dID, PackID: packID, FromPackModID: m.ID, ToProjectID: d.ProjectID, ToVersionID: d.VersionID, Type: normalizeDepType(d.Kind), Constraint: d.Constraint, Reason: d.Reason, CreatedAt: time.Now().UnixMilli()}
			deps = append(deps, drec)
			snap.Dependencies = append(snap.Dependencies, drec)
			present := hasProject(mods, d.ProjectID)
			if dependencyIsMissing(drec.Type, present) {
				pendingMissing = append(pendingMissing, missingDependency{from: m, dep: drec})
			}
		}
	}
	for _, c := range a.explainMissingDependencies(ctx, mods, pendingMissing) {
		confs = append(confs, c)
		snap.Conflicts = append(snap.Conflicts, conflictDTO(c))
	}
	// 兼容知识库: 已知问题未被自动修复的(无解法或解法装不上)进冲突列表。
	if packRec, e := a.repo.GetPack(ctx, packID); e == nil {
		for _, hit := range scanCompatKnowledge(packRec.MCVersion, packRec.Loader, mods, baselineCompatKnowledge()) {
			if hit.Issue.Fix != nil && fixAlreadyPresent(mods, hit.Issue.Fix) {
				continue // 补丁已在包里, 视为已处理
			}
			sev := "warning"
			if hit.Issue.Severity == "fatal" {
				sev = "error"
			}
			summary := hit.Issue.Summary
			detail := map[string]any{"reason": hit.Issue.Source, "modA": hit.ModA.DisplayName, "modB": hit.ModB.DisplayName}
			if hit.Issue.Fix != nil {
				summary += "(解法: " + hit.Issue.Fix.Note + ")"
				detail["fixNote"] = hit.Issue.Fix.Note
			}
			now := time.Now().UnixMilli()
			c := store.ConflictRecord{ID: newID("conflict"), PackID: packID, Fingerprint: hit.ModA.ID + ":" + hit.ModB.ID + ":known_issue", Kind: "known_issue", Severity: sev, Summary: summary, Detail: detail, CreatedAt: now, UpdatedAt: now}
			confs = append(confs, c)
			snap.Conflicts = append(snap.Conflicts, conflictDTO(c))
		}
	}
	raw, _ := json.Marshal(snap)
	sum := sha256.Sum256(raw)
	lock := store.LockRecord{ID: lockID, PackID: packID, SchemaVersion: 1, SnapshotJSON: string(raw), SnapshotSHA256: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UnixMilli()}
	if err := a.repo.CreateLock(ctx, lock, deps, confs, requestID); err != nil {
		return Lock{}, err
	}
	_ = requestID
	return Lock{ID: lock.ID, PackID: packID, SchemaVersion: 1, SnapshotJSON: lock.SnapshotJSON, SnapshotSHA256: lock.SnapshotSHA256, CreatedAt: iso(lock.CreatedAt)}, nil
}

// missingDependency 是一条「包内模组声明了依赖，但依赖目标不在包里」的待解释记录。
type missingDependency struct {
	from store.PackModRecord
	dep  store.ModDependencyRecord
}

// explainMissingDependencies 把缺依赖写成用户能看懂的冲突摘要。
//
// 旧实现直接印 "Missing dependency lhGA9TYQ"：那是 Modrinth/CurseForge 的外部项目 ID，
// 用户在界面上既看不懂也搜不到（链路测试缺陷 O8）。这里优先用 platform_projects 里
// 见过的显示名；查不到名字时才保留 ID，并明确写清那是外部 ID，方便用户拿去搜索。
// 名字解析只是给文案加分，失败不该阻塞锁定流程，因此降级为空表。
func (a *API) explainMissingDependencies(ctx context.Context, packMods []store.PackModRecord, pending []missingDependency) []store.ConflictRecord {
	if len(pending) == 0 {
		return nil
	}
	ids := make([]string, 0, len(pending))
	for _, pm := range pending {
		ids = append(ids, pm.dep.ToProjectID)
	}
	names, err := a.repo.DisplayNameForProjects(ctx, ids)
	if err != nil {
		names = map[string]string{}
	}
	out := make([]store.ConflictRecord, 0, len(pending))
	for _, pm := range pending {
		target := names[pm.dep.ToProjectID]
		resolved := target != ""
		if !resolved {
			target = "外部 ID " + pm.dep.ToProjectID + "（本地未收录，可按该 ID 搜索）"
		}
		from := displayOrID(pm.from.DisplayName, pm.from.ID)
		var c store.ConflictRecord
		switch pm.dep.Type {
		case "incompatible":
			c = conflict(pm.from, "dependency", fmt.Sprintf("模组互斥：%s 与 %s 不能同时安装", from, target), pm.dep.Reason)
		case "optional":
			c = conflict(pm.from, "dependency", fmt.Sprintf("可选依赖未安装：%s 可配合 %s 使用", from, target), pm.dep.Reason)
			c.Severity = "warning"
		default:
			c = conflict(pm.from, "dependency", fmt.Sprintf("缺少依赖模组：%s 需要 %s", from, target), pm.dep.Reason)
		}
		if c.Detail == nil {
			c.Detail = map[string]any{}
		}
		c.Detail["missingProjectID"] = pm.dep.ToProjectID
		c.Detail["missingResolved"] = resolved
		c.Detail["dependencyType"] = pm.dep.Type
		out = append(out, c)
	}
	return out
}

// dependencyIsMissing 决定一条依赖算不算「包里缺东西」（O24）。
//
//   - embedded：已经打进宿主 jar，单独再装反而是重复文件，不算缺失；
//   - incompatible：语义相反——它在包里才是冲突，不在包里是好事；
//   - optional / required（含空值）：不在包里才算缺失，但 optional 只出 warning，
//     不该像 required 那样把构建闸门堵死。
//
// 以前四类一律「缺少依赖模组」+ severity=error：JEI 的 mezz_config 是 embedded，
// 被当成缺失，构建闸门（assertPackBuildable）因此永远拦死，用户照着提示也补不出
// 一个能构建的包。
func dependencyIsMissing(depType string, presentInPack bool) bool {
	switch depType {
	case "embedded":
		return false
	case "incompatible":
		return presentInPack
	default:
		return !presentInPack
	}
}

func normalizeDepType(v string) string {
	switch v {
	case "optional", "incompatible", "embedded":
		return v
	default:
		return "required"
	}
}
func hasProject(mods []store.PackModRecord, p string) bool {
	for _, m := range mods {
		if m.ProjectID == p && m.Status != "removed" {
			return true
		}
	}
	return false
}
func conflict(m store.PackModRecord, kind, summary, reason string) store.ConflictRecord {
	// 白名单必须和 conflicts.kind 的 CHECK 枚举（迁移 0024）逐字对齐。
	// 以前这里少写了 provider_unavailable/known_issue：未知 kind 被悄悄改写成
	// 'dependency' 才落进库，于是「Modrinth 此刻打不开」在库里长得像「这个包有依赖
	// 问题」，界面上分不清、构建闸门（O20）也照着 error 级依赖冲突一路拦死。
	// 不在枚举里的 kind 仍然兜底成 'dependency'，但调用方不该再依赖这个兜底。
	switch kind {
	case "dependency", "version", "loader", "duplicate", "crash", "known_issue", "provider_unavailable":
	default:
		kind = "dependency"
	}
	return store.ConflictRecord{ID: newID("conflict"), PackID: m.PackID, Fingerprint: m.ID + ":" + kind + ":" + summary, Kind: kind, Severity: "error", Summary: summary, Detail: map[string]any{"reason": reason}, CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli()}
}
func conflictDTO(c store.ConflictRecord) Conflict {
	return Conflict{ID: c.ID, PackID: c.PackID, Fingerprint: c.Fingerprint, Kind: c.Kind, Severity: c.Severity, Status: c.Status, Summary: c.Summary, DetailPath: c.DetailPath, Detail: c.Detail, CreatedAt: iso(c.CreatedAt), UpdatedAt: iso(c.UpdatedAt)}
}
func (a *API) ListLocks(ctx context.Context, packID string) ([]Lock, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	x, e := a.repo.ListLocks(ctx, packID)
	if e != nil {
		return nil, e
	}
	o := make([]Lock, 0, len(x))
	for _, v := range x {
		o = append(o, Lock{ID: v.ID, PackID: v.PackID, SchemaVersion: v.SchemaVersion, SnapshotJSON: v.SnapshotJSON, SnapshotSHA256: v.SnapshotSHA256, CreatedAt: iso(v.CreatedAt)})
	}
	return o, nil
}
func (a *API) ListConflicts(ctx context.Context, packID string) ([]Conflict, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	x, e := a.repo.ListConflicts(ctx, packID)
	if e != nil {
		return nil, e
	}
	o := make([]Conflict, 0, len(x))
	for _, v := range x {
		o = append(o, conflictDTO(v))
	}
	return o, nil
}
func (a *API) ResolveConflict(ctx context.Context, packID, id, status, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return err
	}
	at := time.Now().UnixMilli()
	return a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.ResolveConflict(ctx, packID, id, status, at); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "conflict", Action: status, Text: "Conflict " + status, At: at}, map[string]any{"conflict_id": id}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "conflict", id, "conflict."+status, map[string]any{"conflict_id": id}, at)
	})
}
func (a *API) PackHealth(ctx context.Context, packID string) (PackHealth, error) {
	e := a.ready()
	if e != nil {
		return PackHealth{}, e
	}
	if _, e := a.repo.GetPack(ctx, packID); e != nil {
		return PackHealth{}, e
	}
	p, w, m, i, e := a.repo.PackHealth(ctx, packID)
	if e != nil {
		return PackHealth{}, e
	}
	return PackHealth{PackID: packID, Mods: m, Installed: i, PendingErrors: p, PendingWarnings: w, Healthy: p == 0}, nil
}
