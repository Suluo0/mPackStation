package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"mpackstation/internal/provider"
)

type ModContentIcons struct {
	TagIcons       map[string]string `json:"tagIcons"`
	Items          []ModContentItem  `json:"items"`
	Missing        []string          `json:"missing"`
	MissingReasons map[string]string `json:"missingReasons,omitempty"`
	Warnings       []string          `json:"warnings"`
	MCVersion      string            `json:"mcVersion"`
}

var assetVersionPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// ResolveModContentIcons rebuilds icons from source models, never the obsolete
// stored item_icon rows. Vanilla resources are shared on disk per MC version.
func (a *API) ResolveModContentIcons(ctx context.Context, packID, modID string) (ModContentIcons, error) {
	result := ModContentIcons{Items: []ModContentItem{}, Missing: []string{}, Warnings: []string{}, TagIcons: map[string]string{}}
	if err := a.ready(); err != nil {
		return result, err
	}
	if _, err := a.repo.GetPackModInPack(ctx, packID, modID); err != nil {
		return result, &DomainError{Status: 404, Code: "mod_not_found", Message: "mod not found in this pack"}
	}
	pack, err := a.repo.GetPack(ctx, packID)
	if err != nil {
		return result, err
	}
	result.MCVersion = pack.MCVersion
	if !assetVersionPattern.MatchString(pack.MCVersion) {
		return result, ErrInvalidArgument
	}
	r := newIconResources()
	// Keep the request within the HTTP server's write deadline. A failed vanilla
	// fetch does not discard usable mod icons and is explicitly reported to UI.
	fetchCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := a.loadVanillaIcons(fetchCtx, pack.MCVersion, r); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Warnings = append(result.Warnings, "原版资源尚未就绪，部分基础素材和继承原版模型的图标暂不可用；请重试加载图标。")
	}
	ext := &ExtractedContent{}
	r.addCommonTagRepresentatives(pack.MCVersion)
	for _, kind := range []string{"item_model", "texture", "tag"} {
		cursor := ""
		for {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			rows, next, _, err := a.repo.ListModContent(ctx, packID, modID, kind, 500, cursor)
			if err != nil {
				return result, err
			}
			for _, row := range rows {
				item := ContentItem{Kind: row.Kind, Key: row.Key, Path: row.Path, Payload: []byte(row.Payload)}
				if kind == "item_model" {
					ext.Items = append(ext.Items, item)
				} else if kind == "texture" {
					ext.Textures = append(ext.Textures, item)
				} else {
					ext.Tags = append(ext.Tags, item)
				}
			}
			if next == "" {
				break
			}
			cursor = next
		}
	}
	r.addContent(ext)
	icons, missingReasons := r.icons()
	result.TagIcons = r.tagIcons(icons)
	for id := range missingReasons {
		result.Missing = append(result.Missing, id)
	}
	sort.Strings(result.Missing)
	result.MissingReasons = missingReasons
	for _, icon := range icons {
		result.Items = append(result.Items, ModContentItem{ID: icon.Key, Kind: icon.Kind, Key: icon.Key, Path: icon.Path, Payload: icon.Payload})
	}
	return result, nil
}

func (a *API) loadVanillaIcons(ctx context.Context, version string, r *iconResources) error {
	raw, err := a.loadMinecraftClientArchive(ctx, version)
	if err != nil {
		return err
	}
	tmp := newIconResources()
	if err = tmp.addClientJar(raw); err != nil {
		return err
	}
	*r = *tmp
	return nil
}

func (a *API) loadMinecraftClientArchive(ctx context.Context, version string) ([]byte, error) {
	dir := filepath.Join(a.dataDir, "cache", "minecraft-assets")
	target := filepath.Join(dir, version+".jar")
	if info, err := os.Stat(target); err == nil && info.Size() <= modContentMaxJarBytes {
		raw, err := os.ReadFile(target)
		if err == nil {
			return raw, nil
		}
	}
	raw, err := provider.FetchMinecraftClient(ctx, version)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "client-*.tmp")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(name, target); err != nil {
		return nil, fmt.Errorf("cache vanilla assets: %w", err)
	}
	return raw, nil
}

func (a *API) loadVanillaLanguage(ctx context.Context, version, locale string) (json.RawMessage, error) {
	dir := filepath.Join(a.dataDir, "cache", "minecraft-assets", version, "lang")
	target := filepath.Join(dir, locale+".json")
	if raw, err := os.ReadFile(target); err == nil && len(raw) <= 8<<20 && json.Valid(raw) {
		return raw, nil
	}
	raw, err := provider.FetchMinecraftLanguage(ctx, version, locale)
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid minecraft language JSON")
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "lang-*.tmp")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if err = os.Rename(name, target); err != nil {
		return nil, err
	}
	return raw, nil
}
