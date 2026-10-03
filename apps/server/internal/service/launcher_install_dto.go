package service

import (
	"context"
	"strings"

	"mpackstation/internal/store"
)

// LauncherInstall is the API DTO for a version directory the kernel produced.
//
// 前端启动时需要的就是这个 versionId：带加载器时它是 fabric-loader-<ver>-<mc>
// 这类字符串，不等于包的 mc_version（见 migrations/0022 的说明）。
// 有了这个列表，LauncherPage 才能「装了哪个版本就启动哪个版本」，
// 而不是永远拿 mcVersion 去撞。
type LauncherInstall struct {
	ID           string `json:"id"`
	MinecraftDir string `json:"minecraftDir"`
	VersionID    string `json:"versionId"`
	Loader       string `json:"loader"`
	MCVersion    string `json:"mcVersion"`
	PackID       string `json:"packId"`
	TaskID       string `json:"taskId"`
	InstalledAt  string `json:"installedAt"`
}

// ListLauncherInstalls reads installed versions, optionally filtered by pack or
// instance directory. 两个过滤条件都为空时返回全部（上限 100，最新在前）。
func (a *API) ListLauncherInstalls(ctx context.Context, packID, minecraftDir string) ([]LauncherInstall, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListLauncherInstalls(ctx, strings.TrimSpace(packID), strings.TrimSpace(minecraftDir))
	if err != nil {
		return nil, err
	}
	out := make([]LauncherInstall, 0, len(rows))
	for _, r := range rows {
		out = append(out, launcherInstallDTO(r))
	}
	return out, nil
}

func launcherInstallDTO(r store.LauncherInstallRecord) LauncherInstall {
	return LauncherInstall{
		ID: r.ID, MinecraftDir: r.MinecraftDir, VersionID: r.VersionID,
		Loader: r.Loader, MCVersion: r.MCVersion, PackID: r.PackID.String,
		TaskID: r.TaskID, InstalledAt: iso(r.InstalledAt),
	}
}
