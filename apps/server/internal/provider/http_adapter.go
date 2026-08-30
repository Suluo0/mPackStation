package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPAdapter is a small standard-library adapter for the public CF/MR APIs.
// It is opt-in: callers must provide a base URL (tests commonly use httptest).
type HTTPAdapter struct {
	name   Name
	base   *url.URL
	client *http.Client
	token  string
}

func NewHTTPAdapter(name Name, baseURL, token string, client *http.Client) (*HTTPAdapter, error) {
	u, e := url.Parse(strings.TrimRight(baseURL, "/"))
	if e != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid provider base url")
	}
	if name != CurseForge && name != Modrinth {
		return nil, fmt.Errorf("unsupported provider %q", name)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPAdapter{name: name, base: u, client: client, token: token}, nil
}
func (h *HTTPAdapter) Name() Name { return h.name }
func (h *HTTPAdapter) call(ctx context.Context, method, path string, q url.Values, out any) error {
	u := *h.base
	u.Path = strings.TrimRight(h.base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if e != nil {
		return e
	}
	if h.token != "" {
		if h.name == CurseForge {
			req.Header.Set("x-api-key", h.token)
		} else {
			req.Header.Set("Authorization", h.token)
		}
	}
	resp, e := h.client.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return ErrNotFound
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return ErrUnauthorized
	}
	if resp.StatusCode == 429 {
		return ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	if out == nil {
		return nil
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if e != nil {
		return ErrUnavailable
	}
	if e = json.Unmarshal(b, out); e != nil {
		return fmt.Errorf("decode provider response: %w", e)
	}
	return nil
}

// cfLoaderType maps a loader name to CurseForge's ModLoaderType enum
// (0=Any 1=Forge 2=Cauldron 3=LiteLoader 4=Fabric 5=Quilt 6=NeoForge).
// Unknown loaders return "" so the filter is simply omitted.
func cfLoaderType(loader string) string {
	switch strings.ToLower(strings.TrimSpace(loader)) {
	case "forge":
		return "1"
	case "fabric":
		return "4"
	case "quilt":
		return "5"
	case "neoforge":
		return "6"
	default:
		return ""
	}
}

func (h *HTTPAdapter) Search(ctx context.Context, in SearchRequest) (SearchResult, error) {
	// Each platform has its own query vocabulary; build params per provider.
	q := url.Values{}
	path := "/v2/search"
	if h.name == CurseForge {
		// CurseForge: gameId=432 (Minecraft) is mandatory or the API 400s.
		path = "/v1/mods/search"
		q.Set("gameId", "432")
		q.Set("classId", "6") // mods
		q.Set("searchFilter", in.Query)
		if in.Limit > 0 {
			q.Set("pageSize", strconv.Itoa(in.Limit))
		}
		if in.Cursor != "" {
			q.Set("index", in.Cursor)
		}
		if in.MCVersion != "" {
			q.Set("gameVersion", in.MCVersion)
		}
		if lt := cfLoaderType(in.Loader); lt != "" {
			q.Set("modLoaderType", lt)
		}
		q.Set("sortField", "6") // total downloads
		q.Set("sortOrder", "desc")
	} else {
		// Modrinth: query/limit/offset, filters go into JSON facets.
		q.Set("query", in.Query)
		if in.Limit > 0 {
			q.Set("limit", strconv.Itoa(in.Limit))
		}
		if in.Cursor != "" {
			q.Set("offset", in.Cursor)
		}
		facets := [][]string{{"project_type:mod"}}
		if in.MCVersion != "" {
			facets = append(facets, []string{"versions:" + in.MCVersion})
		}
		if in.Loader != "" {
			facets = append(facets, []string{"categories:" + strings.ToLower(in.Loader)})
		}
		if b, e := json.Marshal(facets); e == nil {
			q.Set("facets", string(b))
		}
	}
	var raw struct {
		Hits []struct {
			ProjectID   json.RawMessage `json:"project_id"`
			ID          json.RawMessage `json:"id"`
			Slug        string          `json:"slug"`
			Title       string          `json:"title"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			IconURL     string          `json:"icon_url"`
			Downloads   int64           `json:"downloads"`
		} `json:"hits"`
		Data []struct {
			ID      json.RawMessage `json:"id"`
			Name    string          `json:"name"`
			Slug    string          `json:"slug"`
			Summary string          `json:"summary"`
			Logo    struct {
				ThumbnailURL string `json:"thumbnailUrl"`
				URL          string `json:"url"`
			} `json:"logo"`
			Downloads     int64 `json:"downloads"`
			DownloadCount int64 `json:"downloadCount"`
		} `json:"data"`
		Pagination struct {
			Total         int `json:"total"`
			TotalCount    int `json:"totalCount"`
			Offset, Limit int
		} `json:"pagination"`
	}
	if e := h.call(ctx, "GET", path, q, &raw); e != nil {
		return SearchResult{}, e
	}
	out := SearchResult{}
	for _, x := range raw.Hits {
		id, e := normalizeID(x.ProjectID)
		if e != nil || id == "" {
			id, e = normalizeID(x.ID)
		}
		if e != nil {
			return SearchResult{}, fmt.Errorf("decode provider project id: %w", e)
		}
		name := x.Title
		if name == "" {
			name = x.Name
		}
		out.Items = append(out.Items, Project{ID: id, Slug: x.Slug, Name: name, Summary: x.Description, IconURL: x.IconURL, Downloads: x.Downloads})
	}
	for _, x := range raw.Data {
		id, e := normalizeID(x.ID)
		if e != nil {
			return SearchResult{}, fmt.Errorf("decode provider project id: %w", e)
		}
		name := x.Name
		downloads := x.Downloads
		if downloads == 0 {
			downloads = x.DownloadCount
		}
		icon := x.Logo.ThumbnailURL
		if icon == "" {
			icon = x.Logo.URL
		}
		out.Items = append(out.Items, Project{ID: id, Slug: x.Slug, Name: name, Summary: x.Summary, IconURL: icon, Downloads: downloads})
	}
	out.Total = raw.Pagination.Total
	if out.Total == 0 {
		out.Total = raw.Pagination.TotalCount
	}
	if out.Total == 0 {
		out.Total = len(out.Items)
	}
	return out, nil
}
func (h *HTTPAdapter) Project(ctx context.Context, id string) (Project, error) {
	path := "/v2/project/" + url.PathEscape(id)
	if h.name == CurseForge {
		path = "/v1/mods/" + url.PathEscape(id)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	var raw json.RawMessage
	if e := h.call(ctx, "GET", path, nil, &raw); e != nil {
		return Project{}, e
	}
	if e := json.Unmarshal(raw, &envelope); e == nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		raw = envelope.Data
	}
	var x struct {
		ID          json.RawMessage `json:"id"`
		Slug        string          `json:"slug"`
		Title       string          `json:"title"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Summary     string          `json:"summary"`
		IconURL     string          `json:"icon_url"`
		Logo        struct {
			ThumbnailURL string `json:"thumbnailUrl"`
			URL          string `json:"url"`
		} `json:"logo"`
		Downloads     int64 `json:"downloads"`
		DownloadCount int64 `json:"downloadCount"`
	}
	if e := json.Unmarshal(raw, &x); e != nil {
		return Project{}, fmt.Errorf("decode project: %w", e)
	}
	name := x.Title
	if name == "" {
		name = x.Name
	}
	summary := x.Description
	if summary == "" {
		summary = x.Summary
	}
	icon := x.IconURL
	if icon == "" {
		icon = x.Logo.ThumbnailURL
	}
	if icon == "" {
		icon = x.Logo.URL
	}
	downloads := x.Downloads
	if downloads == 0 {
		downloads = x.DownloadCount
	}
	return Project{ID: id, Slug: x.Slug, Name: name, Summary: summary, IconURL: icon, Downloads: downloads}, nil
}
func (h *HTTPAdapter) Versions(ctx context.Context, id string) ([]Version, error) {
	var payload json.RawMessage
	path := "/v2/project/" + url.PathEscape(id) + "/version"
	if h.name == CurseForge {
		path = "/v1/mods/" + url.PathEscape(id) + "/files"
	}
	if e := h.call(ctx, "GET", path, nil, &payload); e != nil {
		return nil, e
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if e := json.Unmarshal(payload, &envelope); e == nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		payload = envelope.Data
	}
	var raw []struct {
		ID             json.RawMessage `json:"id"`
		Name           string          `json:"name"`
		DisplayName    string          `json:"displayName"`
		VersionNumber  string          `json:"version_number"`
		GameVersions   []string        `json:"game_versions"`
		GameVersionsCF []string        `json:"gameVersions"`
		Loaders        []string        `json:"loaders"`
		Files          []File          `json:"files"`
		DownloadURL    string          `json:"downloadUrl"`
		FileName       string          `json:"fileName"`
		FileLength     int64           `json:"fileLength"`
		Hashes         []struct {
			Value string `json:"value"`
			Algo  int    `json:"algo"`
		} `json:"hashes"`
		Dependencies []struct {
			ProjectID      string `json:"project_id"`
			ProjectIDCamel string `json:"projectId"`
			VersionID      string `json:"version_id"`
			VersionIDCamel string `json:"versionId"`
			Kind           string `json:"dependency_type"`
			Relation       string `json:"relationType"`
			Constraint     string `json:"version_range"`
		} `json:"dependencies"`
	}
	if e := json.Unmarshal(payload, &raw); e != nil {
		return nil, fmt.Errorf("decode versions: %w", e)
	}
	out := make([]Version, 0, len(raw))
	for _, x := range raw {
		versionID, e := normalizeID(x.ID)
		if e != nil {
			return nil, fmt.Errorf("decode provider version id: %w", e)
		}
		name := x.Name
		if name == "" {
			name = x.DisplayName
		}
		gameVersions := x.GameVersions
		loaders := x.Loaders
		if len(gameVersions) == 0 && len(x.GameVersionsCF) > 0 {
			// CurseForge mixes game versions, loader names and side markers
			// ("Client"/"Server") into one array; split them out here.
			for _, gv := range x.GameVersionsCF {
				switch strings.ToLower(gv) {
				case "client", "server":
					// side marker, not a game version
				case "forge", "neoforge", "fabric", "quilt", "liteloader", "cauldron":
					loaders = append(loaders, gv)
				default:
					gameVersions = append(gameVersions, gv)
				}
			}
		}
		files := x.Files
		if len(files) == 0 && (x.DownloadURL != "" || x.FileName != "") {
			files = []File{{Name: x.FileName, DownloadURL: x.DownloadURL, Size: x.FileLength, Primary: true}}
			for _, hash := range x.Hashes {
				if hash.Algo == 1 {
					files[0].SHA1 = hash.Value
				}
				if hash.Algo == 2 {
					files[0].SHA256 = hash.Value
				}
			}
		}
		deps := make([]Dependency, 0, len(x.Dependencies))
		for _, d := range x.Dependencies {
			pid := d.ProjectID
			if pid == "" {
				pid = d.ProjectIDCamel
			}
			vid := d.VersionID
			if vid == "" {
				vid = d.VersionIDCamel
			}
			kind := d.Kind
			if kind == "" {
				kind = d.Relation
			}
			deps = append(deps, Dependency{ProjectID: pid, VersionID: vid, Constraint: d.Constraint, Kind: kind, Reason: "provider dependency"})
		}
		out = append(out, Version{ID: versionID, ProjectID: id, Name: name, VersionNumber: x.VersionNumber, GameVersions: gameVersions, Loaders: loaders, Files: files, Dependencies: deps})
	}
	return out, nil
}
func (h *HTTPAdapter) Metadata(ctx context.Context, pid, vid string) (Metadata, error) {
	p, e := h.Project(ctx, pid)
	if e != nil {
		return Metadata{}, e
	}
	vs, e := h.Versions(ctx, pid)
	if e != nil {
		return Metadata{}, e
	}
	for _, v := range vs {
		if vid == "" || v.ID == vid {
			return Metadata{Project: p, Version: v, Dependencies: v.Dependencies}, nil
		}
	}
	return Metadata{}, ErrNotFound
}
func (h *HTTPAdapter) Download(ctx context.Context, q DownloadRequest) (DownloadResult, error) {
	m, e := h.Metadata(ctx, q.ProjectID, q.VersionID)
	if e != nil {
		return DownloadResult{}, e
	}
	for _, f := range m.Version.Files {
		if f.Primary || len(m.Version.Files) == 1 {
			return DownloadResult{ProjectID: q.ProjectID, VersionID: m.Version.ID, FileName: f.Name, DownloadURL: f.DownloadURL, SHA1: f.SHA1, SHA256: f.SHA256, Size: f.Size}, nil
		}
	}
	return DownloadResult{}, ErrNotFound
}
func (h *HTTPAdapter) Publish(context.Context, PublishRequest) (PublishResult, error) {
	return PublishResult{}, ErrUnavailable
}
func (h *HTTPAdapter) RemoteStatus(context.Context, string) (RemoteStatus, error) {
	return RemoteStatus{Status: "unknown"}, nil
}
