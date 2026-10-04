// Package config defines the process configuration contract.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config contains the stable, single-instance server settings.
type Config struct {
	ListenAddr            string
	AllowLAN              bool
	DataDir               string
	FrontendOrigin        string
	ReadOnlySideEffectQPS int
	DownloadConcurrency   int
	TaskRecoverInterval   time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	StartupToken          string
}

// Default returns safe loopback defaults.
func Default() Config {
	return Config{ListenAddr: "127.0.0.1:18871", DataDir: "data", FrontendOrigin: "http://127.0.0.1:5273", ReadOnlySideEffectQPS: 4, DownloadConcurrency: 4, TaskRecoverInterval: 30 * time.Second, HTTPReadHeaderTimeout: 5 * time.Second, HTTPReadTimeout: 30 * time.Second, HTTPWriteTimeout: 60 * time.Second}
}

// DefaultPath returns the conventional per-user config file location, aligned
// with each OS's application-settings convention (调研结论见
// docs/active/user_check/排查确认记录.md #4：应用默认配置放 Roaming，不放 Documents）：
//
//	Windows: %APPDATA%\mPackStation\config.toml   (AppData\Roaming)
//	macOS:   ~/Library/Application Support/mPackStation/config.toml
//	Linux:   ~/.config/mpackstation/config.toml   (XDG_CONFIG_HOME)
//
// The file may not exist at all — that is normal. The path is only where Load
// looks by default; a missing file means "use built-in defaults".
func DefaultPath() string {
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return "" // 无用户配置目录的极端环境：退化为「没有默认配置文件」
	}
	dir := "mPackStation"
	if runtime.GOOS == "linux" {
		dir = "mpackstation" // XDG 惯例用小写
	}
	return filepath.Join(base, dir, "config.toml")
}

// Load applies a minimal TOML-like key/value file followed by MPACK_* overrides.
// Unknown keys are ignored to preserve forward compatibility of config files.
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		if err := applyFile(&c, path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}
	applyEnv(&c, os.Environ())
	absDataDir, err := filepath.Abs(c.DataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data dir: %w", err)
	}
	c.DataDir = filepath.Clean(absDataDir)
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func applyFile(c *Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if line == "" || strings.HasPrefix(line, "[") || !strings.Contains(line, "=") {
			continue
		}
		p := strings.SplitN(line, "=", 2)
		apply(c, strings.TrimSpace(p[0]), strings.Trim(strings.TrimSpace(p[1]), "\"'"))
	}
	return s.Err()
}

func applyEnv(c *Config, env []string) {
	for _, item := range env {
		p := strings.SplitN(item, "=", 2)
		if len(p) == 2 && strings.HasPrefix(p[0], "MPACK_") {
			apply(c, strings.ToLower(strings.TrimPrefix(p[0], "MPACK_")), p[1])
		}
	}
}

func apply(c *Config, key, value string) {
	switch strings.ToLower(key) {
	case "listen_addr":
		c.ListenAddr = value
	case "data_dir", "data":
		c.DataDir = value
	case "frontend_origin":
		c.FrontendOrigin = value
	case "allow_lan":
		if v, err := strconv.ParseBool(value); err == nil {
			c.AllowLAN = v
		}
	case "readonly_side_effect_qps":
		if v, err := strconv.Atoi(value); err == nil {
			c.ReadOnlySideEffectQPS = v
		}
	case "download_concurrency":
		if v, err := strconv.Atoi(value); err == nil {
			c.DownloadConcurrency = v
		}
	case "startup_token":
		c.StartupToken = value
	}
}

// Validate rejects unsafe or unusable values before startup.
func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return errors.New("listen_addr is required")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return errors.New("data_dir is required")
	}
	if c.ReadOnlySideEffectQPS < 1 {
		return errors.New("readonly_side_effect_qps must be >= 1")
	}
	if c.DownloadConcurrency < 1 || c.DownloadConcurrency > 16 {
		return errors.New("download_concurrency must be between 1 and 16")
	}
	if c.TaskRecoverInterval <= 0 || c.HTTPReadHeaderTimeout <= 0 || c.HTTPReadTimeout <= 0 || c.HTTPWriteTimeout <= 0 {
		return errors.New("timeouts must be positive")
	}
	if c.AllowLAN && len(strings.TrimSpace(c.StartupToken)) < 16 {
		return errors.New("startup_token must be at least 16 characters when allow_lan is enabled")
	}
	return nil
}
