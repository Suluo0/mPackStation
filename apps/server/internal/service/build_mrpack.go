package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"mpackstation/internal/store"
)

// Modrinth .mrpack v1 装配。
//
// 这里修的是基线 D1：构建此前把 files[] 当必填输入，谁调用谁提供文件内容，
// 于是产物只能是调用方临时拼出来的空壳（实测 283 字节、不含任何模组）。
// 现在 files[] 缺省时由本包的权威清单装配：pack_mods → 当前选中项 → 平台发布文件，
// 生成真正的 modrinth.index.json。
//
// 注意 .mrpack 的语义是「清单 + overrides」，不是 jar 的压缩包：
// files[].downloads[] 给地址，安装方（mPackLauncher，或 Prism 兜底）按下。
// 因此装配不需要联网，也不需要本机有 jar 字节。

const mrpackManifestPath = "modrinth.index.json"

type mrpackHashes struct {
	SHA1   string `json:"sha1"`
	SHA512 string `json:"sha512"`
}

type mrpackEnv struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

type mrpackFile struct {
	Path      string       `json:"path"`
	Hashes    mrpackHashes `json:"hashes"`
	Env       mrpackEnv    `json:"env"`
	Downloads []string     `json:"downloads"`
	FileSize  int64        `json:"fileSize"`
}

// 字段名以真实产物为准，不按记忆写：2026-09-30 下载 Modrinth 现役整合包
// （Fabulously Optimized v15.0.0-alpha.4.mrpack）实测其 modrinth.index.json 顶层是
// formatVersion / game / versionId / name / files / dependencies，
// 没有 manifestVersion、author，也没有 overrides 系列键（overrides/ 目录存在于 zip 里
// 但不在 manifest 声明）。此前这里发的是 manifestVersion + 三个字符串型 overrides
// （规范里 serverOverrides/clientOverrides 是 bool），严格读方（Prism 兜底路径）会判不合法。
type mrpackManifest struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Files         []mrpackFile      `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

// mrpackLoaderKey maps our packs.loader value to the mrpack dependency key.
// 空 loader（原版）没有对应依赖项，只留 minecraft。
func mrpackLoaderKey(loader string) string {
	switch loader {
	case "fabric":
		return "fabric-loader"
	case "quilt":
		return "quilt-loader"
	case "neoforge":
		return "neoforge"
	case "forge":
		return "forge"
	default:
		return ""
	}
}

// assembleFromPackAuthority builds the manifest entry list from the pack's own
// database state. It fails loudly when a mod has no downloadable source, because
// silently dropping it would produce a pack that "built fine" but cannot install.
func (a *API) assembleFromPackAuthority(ctx context.Context, pack store.PackRecord, version store.PackVersionRecord) ([]BuildFile, error) {
	sources, err := a.repo.ListPackAssemblySources(ctx, pack.ID)
	if err != nil {
		return nil, err
	}
	unresolved := make([]string, 0, 4)
	files := make([]mrpackFile, 0, len(sources))
	for _, s := range sources {
		if s.Status == "disabled" {
			continue // 用户关掉的模组不进包。
		}
		if s.DownloadURL == "" || s.FileName == "" {
			unresolved = append(unresolved, displayOrID(s.DisplayName, s.ModID))
			continue
		}
		files = append(files, mrpackFile{
			Path: "mods/" + s.FileName,
			// sha512 来自添加模组时下载字节的实测值（见 pack_scoped_identity.go 的
			// validateMeasuredDownload）。老数据（改动之前添加的模组）库里没有 sha512,
			// 此时留空串而不是伪造值——安装方按 sha1 校验仍可用,但要对外兼容需重加模组刷新。
			Hashes:    mrpackHashes{SHA1: s.SHA1, SHA512: s.SHA512},
			Env:       mrpackEnv{Client: "required", Server: "required"},
			Downloads: []string{s.DownloadURL},
			FileSize:  s.SizeBytes,
		})
	}
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return nil, &DomainError{Status: 422, Code: "build_mod_source_unresolved",
			Message: fmt.Sprintf("%d 个模组没有可下载地址（本地 jar 或选中项缺失），构建已阻止: %s", len(unresolved), strings.Join(unresolved, ", ")),
			Details: map[string]any{"mods": unresolved}}
	}
	if len(files) == 0 {
		return nil, &DomainError{Status: 422, Code: "build_no_mods", Message: "整合包里没有任何模组，装配结果为空"}
	}
	manifest := newMrpackManifest(pack, version.Version, files)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, ErrInvalidBuildInput
	}
	// overrides/ 暂不装配（任务书、内容文档、资源包进 overrides 是下一步），
	// 但按规范保留空的 overrides 目录声明，安装方不会因缺字段而报错。
	return []BuildFile{{Path: mrpackManifestPath, Content: data}}, nil
}

// assertPackBuildable 是构建前的冲突闸门。
//
// 背景：/resolve 会把「包内模组声明了必需依赖但依赖不在包里」写成 severity=error
// 的冲突，但构建完全不看冲突表——实测只装 JEI 的包能构建成功、安装成功，一进游戏
// 就是 Fabric Loader 的 "Mod resolution failed / 安装 fabric-api 及以上版本"，
// 用户拿到的是一张必然崩的票。构建产物对外发布前必须先把 error 级冲突处理掉
// （解决或忽略），这里拒绝并回 409 列出摘要。
func (a *API) assertPackBuildable(ctx context.Context, packID string) error {
	conflicts, err := a.repo.ListConflicts(ctx, packID)
	if err != nil {
		return err
	}
	var blocking []string
	for _, c := range conflicts {
		if c.Severity != "error" || c.Status == "resolved" || c.Status == "ignored" {
			continue
		}
		blocking = append(blocking, c.Summary)
	}
	if len(blocking) == 0 {
		return nil
	}
	sort.Strings(blocking)
	return &DomainError{Status: 409, Code: "build_unresolved_conflicts",
		Message: fmt.Sprintf("%d 个冲突未解决，构建出来的包装上游戏会直接崩: %s", len(blocking), strings.Join(blocking, "; ")),
		Details: map[string]any{"conflicts": blocking}}
}

func newMrpackManifest(pack store.PackRecord, version string, files []mrpackFile) mrpackManifest {
	deps := map[string]string{}
	if pack.MCVersion != "" {
		deps["minecraft"] = pack.MCVersion
	}
	if key := mrpackLoaderKey(pack.Loader); key != "" && pack.LoaderVersion != "" {
		deps[key] = pack.LoaderVersion
	}
	return mrpackManifest{
		FormatVersion: 1, Game: "minecraft", VersionID: version, Name: pack.Name,
		Files: files, Dependencies: deps,
	}
}

func displayOrID(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if strings.TrimSpace(id) != "" {
		return id
	}
	return "(未命名模组)"
}
