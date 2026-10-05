// 应用设置（与密钥文件同目录，OS 惯例位置）。目前只有「项目目录」一项——
// 导入的整合包按每个项目一个目录落在这里面（用户 2026-10-05 定稿的导入模型）。
// 放 OS 用户目录而不是 /tmp 开发库：项目根目录的指定要跨重启生效。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appSettingsName = "appsettings.json"

// AppSettings 是用户可改的应用级设置。
type AppSettings struct {
	// ProjectRoot 是项目根目录：每个导入/新建的整合包在这里有自己的目录。
	ProjectRoot string `json:"projectRoot"`
}

// DefaultProjectRoot 返回项目根目录的默认值：用户目录下的 Documents/mPackStation
// Projects（对齐 docs/active/design/pack-project-dir.md 的调研结论——用户主动创建的
// 工程放 Documents，应用配置放 AppData，两者不混）。
func DefaultProjectRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Documents", "mPackStation Projects")
}

// AppSettingsPath 返回应用设置文件路径；无用户配置目录时返回空串。
func AppSettingsPath() string {
	p := DefaultPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), appSettingsName)
}

// LoadAppSettings 读取应用设置；文件不存在 = 默认值。
func LoadAppSettings() (AppSettings, error) {
	out := AppSettings{}
	path := AppSettingsPath()
	if path == "" {
		return out, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("read app settings: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("parse app settings: %w", err)
	}
	return out, nil
}

// SaveAppSettings 原子落盘（tmp + rename），目录不存在自动创建。
func SaveAppSettings(s AppSettings) error {
	path := AppSettingsPath()
	if path == "" {
		return fmt.Errorf("app settings path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create app settings dir: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode app settings: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write app settings: %w", err)
	}
	return os.Rename(tmp, path)
}

// ResolveProjectRoot 返回当前生效的项目根目录：用户在设置里指定的优先，
// 未指定时用默认值。返回空串表示环境异常（无家目录），调用方应报错而不是继续。
func ResolveProjectRoot() (string, error) {
	s, err := LoadAppSettings()
	if err != nil {
		return "", err
	}
	if p := s.ProjectRoot; p != "" {
		return p, nil
	}
	p := DefaultProjectRoot()
	if p == "" {
		return "", fmt.Errorf("cannot determine project root: no user home directory")
	}
	return p, nil
}

// SanitizeDirName 把包名转成安全的目录名段：跨平台保留字与路径分隔符替换为下划线。
func SanitizeDirName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			out = append(out, '_')
		case r < 0x20:
			// 控制字符丢弃
		default:
			out = append(out, r)
		}
	}
	s := string(out)
	if s == "" {
		s = "pack"
	}
	// Windows 保留名兜底
	if runtime.GOOS == "windows" {
		for _, reserved := range []string{"CON", "PRN", "AUX", "NUL"} {
			if s == reserved {
				s = "_" + s
			}
		}
	}
	return s
}
