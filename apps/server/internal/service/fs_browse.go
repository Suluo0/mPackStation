package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DirEntry is one browsable directory returned by BrowseDirectories.
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FileEntry is one regular file in the browsed directory (导入文件选择用：
// 前端按扩展名过滤，路径直接回传后端，不走内容上传——2026-10-05 用户定稿)。
type FileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// FsBrowseResult lists one level of directories plus common Minecraft roots.
type FsBrowseResult struct {
	Path        string     `json:"path"`
	Parent      string     `json:"parent"`
	Directories []DirEntry `json:"directories"`
	Files       []FileEntry `json:"files"`
	Suggested   []DirEntry `json:"suggested"`
}

// SuggestedMinecraftDirs returns common local Minecraft instance roots for
// the current OS so the launcher can offer a picker starting point.
func SuggestedMinecraftDirs() []DirEntry {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(home, "Library", "Application Support", "minecraft"),
			filepath.Join(home, "Library", "Application Support", "PrismLauncher", "instances"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		candidates = []string{
			filepath.Join(appData, ".minecraft"),
			filepath.Join(appData, "PrismLauncher", "instances"),
		}
	default:
		candidates = []string{
			filepath.Join(home, ".minecraft"),
			filepath.Join(home, ".local", "share", "PrismLauncher", "instances"),
		}
	}
	out := make([]DirEntry, 0, len(candidates))
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, DirEntry{Name: filepath.Base(p), Path: p})
		}
	}
	if len(out) == 0 {
		out = append(out, DirEntry{Name: "home", Path: home})
	}
	return out
}

// BrowseDirectories lists subdirectories under path (or home when empty).
// It never returns file contents — only names for the local path picker.
func (a *API) BrowseDirectories(ctx context.Context, path string) (FsBrowseResult, error) {
	if err := a.ready(); err != nil {
		return FsBrowseResult{}, err
	}
	_ = ctx
	target := strings.TrimSpace(path)
	if target == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return FsBrowseResult{}, ErrInvalidArgument
		}
		target = home
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return FsBrowseResult{}, ErrInvalidArgument
	}
	// Normalize without requiring the path to already exist; missing dirs still
	// return suggested roots so the UI can recover.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return FsBrowseResult{
			Path:        abs,
			Parent:      filepath.Dir(abs),
			Directories: []DirEntry{},
			Files:       []FileEntry{},
			Suggested:   SuggestedMinecraftDirs(),
		}, nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return FsBrowseResult{}, ErrInvalidArgument
	}
	dirs := make([]DirEntry, 0, len(entries))
	files := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && name != ".minecraft" {
			continue
		}
		if !e.IsDir() {
			// 常规文件也列出（导入文件选择：路径直接回传后端读盘，不走内容上传）
			if fi, err := e.Info(); err == nil && fi.Mode().IsRegular() {
				files = append(files, FileEntry{Name: name, Path: filepath.Join(abs, name), Size: fi.Size()})
			}
			continue
		}
		dirs = append(dirs, DirEntry{Name: name, Path: filepath.Join(abs, name)})
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	return FsBrowseResult{
		Path:        abs,
		Parent:      parent,
		Directories: dirs,
		Files:       files,
		Suggested:   SuggestedMinecraftDirs(),
	}, nil
}
