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
func (h *HTTPAdapter) Search(ctx context.Context, in SearchRequest) (SearchResult, error) {
	q := url.Values{"query": {in.Query}}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}
	if in.Cursor != "" {
		q.Set("offset", in.Cursor)
	}
	var raw struct {
		Hits []struct {
			ProjectID   string `json:"project_id"`
			ID          string `json:"id"`
			Slug        string `json:"slug"`
			Title       string `json:"title"`
			Name        string `json:"name"`
			Description string `json:"description"`
			IconURL     string `json:"icon_url"`
			Downloads   int64  `json:"downloads"`
		} `json:"hits"`
		Data []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Slug      string `json:"slug"`
			Summary   string `json:"summary"`
			LogoURL   string `json:"logo"`
			Downloads int64  `json:"downloads"`
		} `json:"data"`
		Pagination struct {
			Total         int `json:"total"`
			Offset, Limit int
		} `json:"pagination"`
	}
	path := "/v2/search"
	if h.name == CurseForge {
		path = "/v1/mods/search"
	}
	if e := h.call(ctx, "GET", path, q, &raw); e != nil {
		return SearchResult{}, e
	}
	out := SearchResult{}
	for _, x := range raw.Hits {
		id := x.ProjectID
		if id == "" {
			id = x.ID
		}
		name := x.Title
		if name == "" {
			name = x.Name
		}
		out.Items = append(out.Items, Project{ID: id, Slug: x.Slug, Name: name, Summary: x.Description, IconURL: x.IconURL, Downloads: x.Downloads})
	}
	for _, x := range raw.Data {
		id := x.ID
		name := x.Name
		out.Items = append(out.Items, Project{ID: id, Slug: x.Slug, Name: name, Summary: x.Summary, IconURL: x.LogoURL, Downloads: x.Downloads})
	}
	out.Total = raw.Pagination.Total
	if out.Total == 0 {
		out.Total = len(out.Items)
	}
	return out, nil
}
func (h *HTTPAdapter) Project(ctx context.Context, id string) (Project, error) {
	var x struct {
		ID          string `json:"id"`
		Slug        string `json:"slug"`
		Title       string `json:"title"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Summary     string `json:"summary"`
		IconURL     string `json:"icon_url"`
		LogoURL     string `json:"logo"`
		Downloads   int64  `json:"downloads"`
	}
	path := "/v2/project/" + url.PathEscape(id)
	if h.name == CurseForge {
		path = "/v1/mods/" + url.PathEscape(id)
	}
	if e := h.call(ctx, "GET", path, nil, &x); e != nil {
		return Project{}, e
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
		icon = x.LogoURL
	}
	return Project{ID: id, Slug: x.Slug, Name: name, Summary: summary, IconURL: icon, Downloads: x.Downloads}, nil
}
func (h *HTTPAdapter) Versions(ctx context.Context, id string) ([]Version, error) {
	var raw []struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		VersionNumber string   `json:"version_number"`
		GameVersions  []string `json:"game_versions"`
		Loaders       []string `json:"loaders"`
		Files         []File   `json:"files"`
	}
	path := "/v2/project/" + url.PathEscape(id) + "/version"
	if h.name == CurseForge {
		path = "/v1/mods/" + url.PathEscape(id) + "/files"
	}
	if e := h.call(ctx, "GET", path, nil, &raw); e != nil {
		return nil, e
	}
	out := make([]Version, 0, len(raw))
	for _, x := range raw {
		out = append(out, Version{ID: x.ID, ProjectID: id, Name: x.Name, VersionNumber: x.VersionNumber, GameVersions: x.GameVersions, Loaders: x.Loaders, Files: x.Files})
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
			return Metadata{Project: p, Version: v}, nil
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
