// 应用级密钥文件（2026-10-05）：与默认配置文件同目录（config.DefaultPath 的
// OS 惯例位置），独立成 secrets.json。为什么不能只放数据库：开发实例的数据目录
// 是 /tmp/mpack-data（重启即清），CurseForge key 这种要跨重启的凭据放里面
// 会「每次启动都显示未配置」——用户反馈点名的正是这个。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SecretsPath 返回密钥文件路径；无用户配置目录的极端环境返回空串（调用方退化为
// 只用数据库存储，行为与旧版一致）。
func SecretsPath() string {
	p := DefaultPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "secrets.json")
}

// LoadSecretsFile 读取密钥文件；文件不存在 = 空表（不报错），解析失败才报错。
func LoadSecretsFile(path string) (map[string]string, error) {
	out := map[string]string{}
	if path == "" {
		return out, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("read secrets file: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("parse secrets file: %w", err)
	}
	return out, nil
}

// SaveSecretsFile 原子落盘（tmp + rename），目录不存在自动创建，权限 0600。
func SaveSecretsFile(path string, secrets map[string]string) error {
	if path == "" {
		return fmt.Errorf("secrets path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}
	raw, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("encode secrets: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write secrets file: %w", err)
	}
	return os.Rename(tmp, path)
}
