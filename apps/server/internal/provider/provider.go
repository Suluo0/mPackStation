// Package provider contains the only boundary for external mod platforms.
// Platform-specific response shapes never leave this package.
package provider

import (
	"context"
	"errors"
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
