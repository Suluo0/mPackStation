package provider

import (
	"context"
	"encoding/json"
	"fmt"
)

type fixtureFault struct {
	Code string `json:"code"`
}
type fixtureCatalog struct {
	Projects []Project               `json:"projects"`
	Versions []Version               `json:"versions"`
	Metadata []Metadata              `json:"metadata"`
	Faults   map[string]fixtureFault `json:"faults"`
}

// FixtureAdapter is deterministic and offline, suitable for contract tests
// and local development. Its DTOs are identical to the production contract.
type FixtureAdapter struct {
	name    Name
	catalog fixtureCatalog
}

func NewFixture(name Name, data []byte) (*FixtureAdapter, error) {
	if name != CurseForge && name != Modrinth {
		return nil, fmt.Errorf("unsupported provider %q", name)
	}
	var c fixtureCatalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("decode %s fixture: %w", name, err)
	}
	return &FixtureAdapter{name: name, catalog: c}, nil
}
func NewCurseForgeFixture(data []byte) (*FixtureAdapter, error) { return NewFixture(CurseForge, data) }
func NewModrinthFixture(data []byte) (*FixtureAdapter, error)   { return NewFixture(Modrinth, data) }
func (f *FixtureAdapter) Name() Name                            { return f.name }
func (f *FixtureAdapter) fault(op, id string) error {
	if x, ok := f.catalog.Faults[op+":"+id]; ok {
		return faultError(x.Code)
	}
	if x, ok := f.catalog.Faults[op]; ok {
		return faultError(x.Code)
	}
	return nil
}
func faultError(code string) error {
	switch code {
	case "404":
		return ErrNotFound
	case "401", "403":
		return ErrUnauthorized
	case "429":
		return ErrRateLimited
	default:
		return ErrUnavailable
	}
}
func (f *FixtureAdapter) Search(ctx context.Context, q SearchRequest) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if err := f.fault("search", q.Query); err != nil {
		return SearchResult{}, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	out := make([]Project, 0, limit)
	for _, p := range f.catalog.Projects {
		if q.Query == "" || contains(p.Name, q.Query) || contains(p.Slug, q.Query) {
			out = append(out, p)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return SearchResult{Items: out, Total: len(out)}, nil
}
func contains(a, b string) bool { return len(b) == 0 || len(a) >= len(b) && indexFold(a, b) >= 0 }
func indexFold(a, b string) int {
	for i := 0; i+len(b) <= len(a); i++ {
		match := true
		for j := range b {
			ca, cb := a[i+j], b[j]
			if ca >= 'A' && ca <= 'Z' {
				ca += 32
			}
			if cb >= 'A' && cb <= 'Z' {
				cb += 32
			}
			if ca != cb {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
func (f *FixtureAdapter) Project(ctx context.Context, id string) (Project, error) {
	if err := f.fault("project", id); err != nil {
		return Project{}, err
	}
	for _, p := range f.catalog.Projects {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, ErrNotFound
}
func (f *FixtureAdapter) Versions(ctx context.Context, id string) ([]Version, error) {
	if err := f.fault("versions", id); err != nil {
		return nil, err
	}
	out := []Version{}
	for _, v := range f.catalog.Versions {
		if v.ProjectID == id {
			out = append(out, v)
		}
	}
	return out, nil
}
func (f *FixtureAdapter) Metadata(ctx context.Context, projectID, versionID string) (Metadata, error) {
	if err := f.fault("metadata", projectID+":"+versionID); err != nil {
		return Metadata{}, err
	}
	for _, m := range f.catalog.Metadata {
		if m.Project.ID == projectID && (versionID == "" || m.Version.ID == versionID || m.Version.ID == "") {
			return m, nil
		}
	}
	return Metadata{}, ErrNotFound
}
func (f *FixtureAdapter) Download(ctx context.Context, q DownloadRequest) (DownloadResult, error) {
	if err := f.fault("download", q.ProjectID+":"+q.VersionID); err != nil {
		return DownloadResult{}, err
	}
	for _, v := range f.catalog.Versions {
		if v.ProjectID == q.ProjectID && v.ID == q.VersionID {
			for _, x := range v.Files {
				if x.Primary || len(v.Files) == 1 {
					return DownloadResult{ProjectID: q.ProjectID, VersionID: q.VersionID, FileName: x.Name, DownloadURL: x.DownloadURL, SHA1: x.SHA1, SHA256: x.SHA256, Size: x.Size}, nil
				}
			}
		}
	}
	return DownloadResult{}, ErrNotFound
}
func (f *FixtureAdapter) Publish(context.Context, PublishRequest) (PublishResult, error) {
	return PublishResult{Status: "accepted"}, nil
}
func (f *FixtureAdapter) RemoteStatus(context.Context, string) (RemoteStatus, error) {
	return RemoteStatus{Status: "unknown"}, nil
}
