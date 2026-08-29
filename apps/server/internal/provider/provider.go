// Package provider contains the only boundary for external mod platforms.
// Platform-specific response shapes never leave this package.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Items      []Project
	NextCursor string
	Total      int
}
type Project struct {
	ID, Slug, Name, Summary, IconURL string
	Downloads                        int64
}
type Version struct {
	ID, ProjectID, Name, VersionNumber string
	GameVersions, Loaders              []string
	Files                              []File
	Dependencies                       []Dependency
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
		DownloadURL string          `json:"downloadUrl"`
		URL         string          `json:"url"`
		SHA1        string          `json:"sha1"`
		SHA256      string          `json:"sha256"`
		Size        int64           `json:"size"`
		FileLength  int64           `json:"fileLength"`
		Primary     bool            `json:"primary"`
		Hashes      []struct {
			Value string `json:"value"`
			Algo  int    `json:"algo"`
		} `json:"hashes"`
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
	url := raw.DownloadURL
	if url == "" {
		url = raw.URL
	}
	size := raw.Size
	if size == 0 {
		size = raw.FileLength
	}
	sha1, sha256 := raw.SHA1, raw.SHA256
	for _, h := range raw.Hashes {
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
type Registry struct{ adapters map[Name]Adapter }

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
	a, ok := r.adapters[Name(name)]
	if !ok {
		return nil, ErrUnavailable
	}
	return a, nil
}
