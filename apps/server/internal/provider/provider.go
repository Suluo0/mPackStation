// Package provider contains the only boundary for external mod platforms.
// Platform-specific response shapes never leave this package.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrNotFound     = errors.New("provider resource not found")
	ErrRateLimited  = errors.New("provider rate limited")
	ErrUnauthorized = errors.New("provider unauthorized")
	ErrUnavailable  = errors.New("provider unavailable")
)

type Name string

const (
	CurseForge Name = "curseforge"
	Modrinth   Name = "modrinth"
)

type SearchRequest struct {
	Query, MCVersion, Loader, Cursor string
	Limit                            int
}
type SearchResult struct {
	Items      []Project `json:"items"`
	NextCursor string    `json:"nextCursor"`
	Total      int       `json:"total"`
}
type Project struct {
	ID        string `json:"id"`
	Slug      string `json:"slug,omitempty"`
	Name      string `json:"name"`
	Summary   string `json:"summary,omitempty"`
	IconURL   string `json:"iconUrl,omitempty"`
	Downloads int64  `json:"downloads"`
}
type Version struct {
	ID            string       `json:"id"`
	ProjectID     string       `json:"projectId,omitempty"`
	Name          string       `json:"name,omitempty"`
	VersionNumber string       `json:"versionNumber,omitempty"`
	GameVersions  []string     `json:"gameVersions,omitempty"`
	Loaders       []string     `json:"loaders,omitempty"`
	// DatePublished is RFC3339 when the provider reports it; versions are
	// returned newest-first so "the first compatible entry" is the latest.
	DatePublished string       `json:"datePublished,omitempty"`
	Files         []File       `json:"files,omitempty"`
	Dependencies  []Dependency `json:"dependencies,omitempty"`
}
type File struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DownloadURL string `json:"downloadUrl"`
	SHA1        string `json:"sha1"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Primary     bool   `json:"primary"`
}

// UnmarshalJSON normalizes the string and numeric identifiers and the
// provider-specific file length/hash representations used by CF and MR.
func (f *File) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID          json.RawMessage `json:"id"`
		Name        string          `json:"name"`
		DisplayName string          `json:"displayName"`
		FileName    string          `json:"filename"`
		DownloadURL string          `json:"downloadUrl"`
		URL         string          `json:"url"`
		SHA1        string          `json:"sha1"`
		SHA256      string          `json:"sha256"`
		Size        int64           `json:"size"`
		FileLength  int64           `json:"fileLength"`
		Primary     bool            `json:"primary"`
		Hashes      json.RawMessage `json:"hashes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	id, err := normalizeID(raw.ID)
	if err != nil {
		return err
	}
	name := raw.Name
	if name == "" {
		name = raw.DisplayName
	}
	if name == "" {
		name = raw.FileName
	}
	url := raw.DownloadURL
	if url == "" {
		url = raw.URL
	}
	size := raw.Size
	if size == 0 {
		size = raw.FileLength
	}
	sha1, sha256 := raw.SHA1, raw.SHA256
	// CurseForge: hashes is [{value, algo}]; Modrinth: {"sha1": ..., "sha512": ...}.
	var cfHashes []struct {
		Value string `json:"value"`
		Algo  int    `json:"algo"`
	}
	if len(raw.Hashes) > 0 && json.Unmarshal(raw.Hashes, &cfHashes) == nil {
		for _, h := range cfHashes {
			switch h.Algo {
			case 1:
				if sha1 == "" {
					sha1 = h.Value
				}
			case 2:
				if sha256 == "" {
					sha256 = h.Value
				}
			}
		}
	} else if len(raw.Hashes) > 0 {
		var mrHashes struct {
			SHA1   string `json:"sha1"`
			SHA256 string `json:"sha256"`
		}
		if json.Unmarshal(raw.Hashes, &mrHashes) == nil {
			if sha1 == "" {
				sha1 = mrHashes.SHA1
			}
			if sha256 == "" {
				sha256 = mrHashes.SHA256
			}
		}
	}
	f.ID, f.Name, f.DownloadURL, f.SHA1, f.SHA256, f.Size, f.Primary = id, name, url, sha1, sha256, size, raw.Primary
	return nil
}

func normalizeID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return "", fmt.Errorf("provider id: %w", err)
	}
	return n.String(), nil
}

type Metadata struct {
	Project      Project
	Version      Version
	Dependencies []Dependency
}
type Dependency struct {
	ProjectID, VersionID, Constraint, Kind, Reason string
}
type DownloadRequest struct{ ProjectID, VersionID string }
type DownloadResult struct {
	ProjectID, VersionID, FileName, DownloadURL, SHA1, SHA256 string
	Size                                                      int64
	Content                                                   []byte
}
type PublishRequest struct{ ProjectID, VersionID, FilePath string }
type PublishResult struct{ RemoteID, Status, URL string }
type RemoteStatus struct{ ID, Status, URL, Detail string }

// Adapter is the normalized provider contract used by services.
type Adapter interface {
	Name() Name
	Search(context.Context, SearchRequest) (SearchResult, error)
	Project(context.Context, string) (Project, error)
	Versions(context.Context, string) ([]Version, error)
	Metadata(context.Context, string, string) (Metadata, error)
	Download(context.Context, DownloadRequest) (DownloadResult, error)
	Publish(context.Context, PublishRequest) (PublishResult, error)
	RemoteStatus(context.Context, string) (RemoteStatus, error)
}

// Registry resolves adapters without exposing platform SDK types.
// The map is guarded because provider credentials can be added/removed at
// runtime (settings page) while searches read concurrently.
type Registry struct {
	mu       sync.RWMutex
	adapters map[Name]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{adapters: make(map[Name]Adapter, len(adapters))}
	for _, a := range adapters {
		if a != nil {
			r.adapters[a.Name()] = a
		}
	}
	return r
}
func (r *Registry) Get(name string) (Adapter, error) {
	if r == nil {
		return nil, ErrUnavailable
	}
	r.mu.RLock()
	a, ok := r.adapters[Name(name)]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}

// Set registers or replaces an adapter at runtime (e.g. after the user saves
// a CurseForge key in settings).
func (r *Registry) Set(a Adapter) {
	if r == nil || a == nil {
		return
	}
	r.mu.Lock()
	r.adapters[a.Name()] = a
	r.mu.Unlock()
}

// Remove unregisters a provider at runtime (e.g. after the user clears a key).
func (r *Registry) Remove(name Name) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.adapters, name)
	r.mu.Unlock()
}

// List returns every registered adapter. Used by fan-out search; the order is
// deterministic (sorted by provider name) so responses are stable.
func (r *Registry) List() []Adapter {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
