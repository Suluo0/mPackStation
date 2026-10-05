package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"mpackstation/internal/config"
	"mpackstation/internal/provider"
)

// CurseForge 凭据的运行时管理(设置页)。key 持久化在本地 secrets 表,
// 保存前先用一次真实平台调用验证有效性,保存/清除立即生效、无需重启。
// 启动时由装配根(main)从库里读出并注册适配器;环境变量 CURSEFORGE_API_KEY
// 优先级高于库里的 key。

const curseforgeSecretKey = "curseforge_api_key"

// SetCurseForgeKey validates the key against the live CurseForge API, persists
// it, and hot-registers the provider adapter. A 400 means the platform
// rejected the key; a 502 means the platform could not be reached at all.
func (a *API) SetCurseForgeKey(ctx context.Context, key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return &DomainError{Status: 400, Code: "invalid_argument", Message: "key must not be empty"}
	}
	ad, err := provider.NewHTTPAdapter(provider.CurseForge, "https://api.curseforge.com", key, nil)
	if err != nil {
		return &DomainError{Status: 400, Code: "invalid_argument", Message: "invalid CurseForge key", Wrapped: err}
	}
	// Probe with the cheapest real call: a 1-result search. CF answers 403 for
	// invalid keys and the call doubles as a reachability probe.
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := ad.Search(pctx, provider.SearchRequest{Query: "a", Limit: 1}); err != nil {
		_ = a.repo.SetSetting(ctx, "provider.curseforge.reachable", "false", a.now().UnixMilli())
		if errors.Is(err, provider.ErrUnauthorized) {
			return &DomainError{Status: 400, Code: "invalid_argument", Message: "CurseForge key invalid or expired", Wrapped: err}
		}
		return mapProviderError(err)
	}
	if err := a.repo.PutSecret(ctx, curseforgeSecretKey, key, a.now().UnixMilli()); err != nil {
		return err
	}
	// 双写密钥文件（OS 用户配置目录，2026-10-05）：/tmp 里的开发库重启即清，
	// 只写库的话每次启动都要重新配 key（用户反馈点名）。文件为跨重启权威，
	// 失败只记日志 —— 库里那份仍可用，不阻塞保存。
	if err := mergeSecretsFile(curseforgeSecretKey, key); err != nil {
		log.Printf("persist curseforge key to secrets file: %v", err)
	}
	if reg := a.p5Registry(); reg != nil {
		reg.Set(ad)
	}
	if err := a.repo.SetSetting(ctx, "provider.curseforge.reachable", "true", a.now().UnixMilli()); err != nil {
		return err
	}
	return nil
}

// ClearCurseForgeKey removes the stored key and unregisters the adapter.
// A key supplied via the CURSEFORGE_API_KEY environment variable is not
// affected here and will re-register on the next start.
func (a *API) ClearCurseForgeKey(ctx context.Context) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.repo.DeleteSecret(ctx, curseforgeSecretKey); err != nil {
		return err
	}
	if err := mergeSecretsFile(curseforgeSecretKey, ""); err != nil {
		log.Printf("remove curseforge key from secrets file: %v", err)
	}
	if reg := a.p5Registry(); reg != nil {
		reg.Remove(provider.CurseForge)
	}
	return a.repo.SetSetting(ctx, "provider.curseforge.reachable", "unknown", a.now().UnixMilli())
}

// ProbeProviderStatus performs a lightweight live call against every
// registered provider and records reachability in settings, so the status
// cards reflect reality instead of a permanent "unknown". Best-effort: probe
// failures are recorded, never fatal.
func (a *API) ProbeProviderStatus(ctx context.Context) {
	if a.ready() != nil {
		return
	}
	reg := a.p5Registry()
	if reg == nil {
		return
	}
	for _, ad := range reg.List() {
		reachable := "true"
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if _, err := ad.Search(pctx, provider.SearchRequest{Query: "a", Limit: 1}); err != nil {
			reachable = "false"
		}
		cancel()
		_ = a.repo.SetSetting(ctx, "provider."+string(ad.Name())+".reachable", reachable, a.now().UnixMilli())
	}
}

// mergeSecretsFile 把一个密钥写进/移出用户配置目录的 secrets.json（value 为空串
// = 移除）。文件与库双写：库里那份让启动装配与既有关联查询继续工作，文件那份
// 保证凭据在 /tmp 数据目录被清后依然存在。
func mergeSecretsFile(key, value string) error {
	path := config.SecretsPath()
	if path == "" {
		return nil // 无用户配置目录的极端环境：退化为只写库
	}
	secrets, err := config.LoadSecretsFile(path)
	if err != nil {
		return err
	}
	if value == "" {
		delete(secrets, key)
	} else {
		secrets[key] = value
	}
	return config.SaveSecretsFile(path, secrets)
}

// fileHasSecret 查密钥文件里是否有某个凭据（2026-10-05）：状态与引导清单原本只看
// 数据库 secrets 表，而开发库在 /tmp 会被清，文件里的跨重启凭据得一并算数。
func fileHasSecret(key string) bool {
	path := config.SecretsPath()
	if path == "" {
		return false
	}
	secrets, err := config.LoadSecretsFile(path)
	if err != nil {
		return false
	}
	return secrets[key] != ""
}
