package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type minecraftDownload struct {
	URL, SHA1 string
	Size      int64
}
type minecraftVersionMetadata struct {
	Downloads  struct{ Client minecraftDownload }
	AssetIndex minecraftDownload `json:"assetIndex"`
}

var minecraftLocale = regexp.MustCompile(`^[a-z]{2,3}_[a-z0-9]{2,8}$`)

func minecraftHTTPClient() *http.Client {
	return &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !minecraftAssetURL(req.URL) {
			return fmt.Errorf("invalid asset redirect")
		}
		return nil
	}}
}

func minecraftRead(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !minecraftAssetURL(u) {
		return nil, fmt.Errorf("invalid asset URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("minecraft asset HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("minecraft asset too large")
	}
	return b, nil
}

func fetchMinecraftVersionMetadata(ctx context.Context, client *http.Client, version string) (minecraftVersionMetadata, error) {
	var result minecraftVersionMetadata
	raw, err := minecraftRead(ctx, client, "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", 8<<20)
	if err != nil {
		return result, err
	}
	var manifest struct {
		Versions []struct{ ID, URL, SHA1 string }
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return result, fmt.Errorf("invalid minecraft manifest")
	}
	for _, entry := range manifest.Versions {
		if entry.ID != version {
			continue
		}
		meta, err := minecraftRead(ctx, client, entry.URL, 8<<20)
		if err != nil {
			return result, err
		}
		if !minecraftSHA(meta, entry.SHA1) {
			return result, fmt.Errorf("version metadata checksum mismatch")
		}
		if json.Unmarshal(meta, &result) != nil {
			return result, fmt.Errorf("invalid version metadata")
		}
		return result, nil
	}
	return result, fmt.Errorf("minecraft version unavailable")
}

// FetchMinecraftClient fetches the exact, checksummed client/data archive for a game version.
func FetchMinecraftClient(ctx context.Context, version string) ([]byte, error) {
	client := minecraftHTTPClient()
	meta, err := fetchMinecraftVersionMetadata(ctx, client, version)
	if err != nil {
		return nil, err
	}
	d := meta.Downloads.Client
	if d.Size <= 0 || d.Size > 100<<20 {
		return nil, fmt.Errorf("invalid client metadata")
	}
	b, err := minecraftRead(ctx, client, d.URL, d.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != d.Size || !minecraftSHA(b, d.SHA1) {
		return nil, fmt.Errorf("minecraft client checksum mismatch")
	}
	return b, nil
}

// FetchMinecraftLanguage resolves a locale through the version's checksummed asset index.
func FetchMinecraftLanguage(ctx context.Context, version, locale string) ([]byte, error) {
	if !minecraftLocale.MatchString(locale) {
		return nil, fmt.Errorf("invalid minecraft locale")
	}
	client := minecraftHTTPClient()
	meta, err := fetchMinecraftVersionMetadata(ctx, client, version)
	if err != nil {
		return nil, err
	}
	idx := meta.AssetIndex
	if idx.Size <= 0 || idx.Size > 32<<20 {
		return nil, fmt.Errorf("invalid asset index metadata")
	}
	raw, err := minecraftRead(ctx, client, idx.URL, idx.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != idx.Size || !minecraftSHA(raw, idx.SHA1) {
		return nil, fmt.Errorf("asset index checksum mismatch")
	}
	var index struct {
		Objects map[string]struct {
			Hash string
			Size int64
		}
	}
	if json.Unmarshal(raw, &index) != nil {
		return nil, fmt.Errorf("invalid asset index")
	}
	obj, ok := index.Objects["minecraft/lang/"+locale+".json"]
	if !ok || len(obj.Hash) != 40 || obj.Size <= 0 || obj.Size > 8<<20 {
		return nil, fmt.Errorf("minecraft language unavailable")
	}
	assetURL := "https://resources.download.minecraft.net/" + obj.Hash[:2] + "/" + obj.Hash
	b, err := minecraftRead(ctx, client, assetURL, obj.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != obj.Size || !minecraftSHA(b, obj.Hash) {
		return nil, fmt.Errorf("minecraft language checksum mismatch")
	}
	return b, nil
}

func minecraftAssetURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "piston-meta.mojang.com", "piston-data.mojang.com", "launchermeta.mojang.com", "launcher.mojang.com", "resources.download.minecraft.net":
		return true
	}
	return false
}
func minecraftSHA(b []byte, want string) bool {
	sum := sha1.Sum(b)
	return len(want) == 40 && hex.EncodeToString(sum[:]) == want
}
