package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

var (
	// ErrPublishFailed is returned after a provider call failed and the
	// durable release was marked failed. Retrying is always explicit.
	ErrPublishFailed = errors.New("publish failed")
	// ErrPublishIdempotencyConflict means a key was reused for another input.
	ErrPublishIdempotencyConflict = errors.New("publish idempotency key conflict")
	// ErrProviderStatusUnavailable means a remote state probe could not be
	// completed; the existing release state is intentionally preserved.
	ErrProviderStatusUnavailable = errors.New("provider status unavailable")
)

// PublishInput identifies one explicit publication request.
type PublishInput struct {
	PackID, PackVersionID, Provider, ArtifactID, IdempotencyKey string
	ProjectID, VersionID                                        string
}

// Release is the public, credential-free release state.
type Release struct {
	ID, PackID, PackVersionID, Provider, Status, RemoteID string
	IdempotencyKey, RemoteState, ArtifactID               string
	ErrorCode, ErrorMessage                               string
	CreatedAt, UpdatedAt                                  string
}

// P7Service owns build publication orchestration while SQL remains in store
// and remote HTTP remains behind provider.Adapter.
type P7Service struct {
	repo     *store.Repository
	registry *provider.Registry
	now      func() time.Time
}

// NewP7Service creates the P7 service over a migrated database.
func NewP7Service(db *sql.DB) *P7Service {
	if db == nil {
		return &P7Service{now: time.Now}
	}
	return &P7Service{repo: store.NewRepository(db), now: time.Now}
}

// NewP7ServiceFromSource is the composition-root adapter used by httpapi. It
// accepts the same opaque source as the rest of the service layer so HTTP
// remains unaware of database types.
func NewP7ServiceFromSource(source any) *P7Service {
	if db, ok := source.(*sql.DB); ok {
		return NewP7Service(db)
	}
	return NewP7Service(nil)
}

// RegisterExportDirectory forwards the explicit destination approval through
// the same API rule set used by non-HTTP callers.
func (s *P7Service) RegisterExportDirectory(ctx context.Context, name, directory string) error {
	if err := s.ready(); err != nil {
		return err
	}
	a := &API{repo: s.repo, now: s.now}
	return a.RegisterExportDirectory(ctx, name, directory)
}

// BuildPack forwards deterministic build orchestration through the service
// boundary. It exists on P7Service so the HTTP composition root can keep P7
// routes independent from the broad dashboard API.
func (s *P7Service) BuildPack(ctx context.Context, in BuildInput) (BuildResult, error) {
	if err := s.ready(); err != nil {
		return BuildResult{}, err
	}
	a := &API{repo: s.repo, now: s.now}
	return a.BuildPack(ctx, in)
}

// SetProviderRegistry injects normalized CurseForge/Modrinth adapters.
func (s *P7Service) SetProviderRegistry(registry *provider.Registry) {
	if s != nil {
		s.registry = registry
	}
}

func (s *P7Service) ready() error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	return nil
}

// PublishPack creates a durable idempotency record and performs one explicit
// provider call. A repeated request for a succeeded, publishing, or failed
// key returns the recorded state without invoking the provider again.
func (s *P7Service) PublishPack(ctx context.Context, in PublishInput) (Release, error) {
	if err := s.ready(); err != nil {
		return Release{}, err
	}
	if err := validatePublishInput(in); err != nil {
		return Release{}, err
	}
	if _, err := s.repo.GetPackVersion(ctx, in.PackID, in.PackVersionID); err != nil {
		return Release{}, err
	}
	artifact, err := s.repo.GetArtifact(ctx, in.ArtifactID)
	if err != nil {
		return Release{}, err
	}
	if artifact.PackID != in.PackID || artifact.PackVersionID != in.PackVersionID || artifact.Status != "ready" {
		return Release{}, ErrPublishIdempotencyConflict
	}
	now := s.nowMillis()
	created := store.ReleaseRecord{ID: newID("release"), PackID: in.PackID, PackVersionID: in.PackVersionID, Provider: in.Provider, Status: "pending", IdempotencyKey: in.IdempotencyKey, RemoteState: `{}`, ArtifactID: in.ArtifactID, CreatedAt: now, UpdatedAt: now}
	rel, err := s.repo.CreateRelease(ctx, created)
	if err != nil {
		return Release{}, err
	}
	if rel.PackID != in.PackID || rel.PackVersionID != in.PackVersionID || rel.ArtifactID != in.ArtifactID {
		return Release{}, ErrPublishIdempotencyConflict
	}
	// A durable terminal/publishing row is authoritative. This guard is what
	// prevents duplicate external side effects on repeated HTTP requests.
	if rel.ID != created.ID || rel.Status != "pending" {
		return releaseDTO(rel), nil
	}
	return s.publishOnce(ctx, rel, artifact, in.ProjectID, in.VersionID)
}

// RetryPublish is the only way to retry a failed non-idempotent publication.
// It must be invoked intentionally by an operator or a future task handler.
func (s *P7Service) RetryPublish(ctx context.Context, releaseID, projectID, versionID string) (Release, error) {
	if err := s.ready(); err != nil {
		return Release{}, err
	}
	rel, err := s.repo.GetRelease(ctx, releaseID)
	if err != nil {
		return Release{}, err
	}
	if rel.Status != "failed" {
		return releaseDTO(rel), nil
	}
	artifact, err := s.repo.GetArtifact(ctx, rel.ArtifactID)
	if err != nil {
		return Release{}, err
	}
	return s.publishOnce(ctx, rel, artifact, projectID, versionID)
}

func (s *P7Service) publishOnce(ctx context.Context, rel store.ReleaseRecord, artifact store.ArtifactRecord, projectID, versionID string) (Release, error) {
	if err := verifyArtifactFile(artifact.Path, artifact.SHA256, artifact.SizeBytes); err != nil {
		_ = s.repo.SetReleasePublishing(ctx, rel.ID, s.nowMillis())
		_ = s.repo.FinishRelease(ctx, rel.ID, "failed", "", `{}`, "artifact_missing", "artifact is unavailable", s.nowMillis())
		latest, _ := s.repo.GetRelease(ctx, rel.ID)
		return releaseDTO(latest), ErrPublishFailed
	}
	if err := s.repo.SetReleasePublishing(ctx, rel.ID, s.nowMillis()); err != nil {
		latest, getErr := s.repo.GetRelease(ctx, rel.ID)
		if getErr == nil {
			return releaseDTO(latest), nil
		}
		return Release{}, err
	}
	if rel.Provider == "local" {
		state, _ := json.Marshal(map[string]string{"provider": "local", "status": "succeeded"})
		if err := s.repo.FinishRelease(ctx, rel.ID, "succeeded", artifact.ID, string(state), "", "", s.nowMillis()); err != nil {
			return Release{}, err
		}
		latest, err := s.repo.GetRelease(ctx, rel.ID)
		return releaseDTO(latest), err
	}
	if s.registry == nil {
		return s.finishPublishFailure(ctx, rel.ID, "provider_unavailable")
	}
	adapter, err := s.registry.Get(rel.Provider)
	if err != nil {
		return s.finishPublishFailure(ctx, rel.ID, "provider_unavailable")
	}
	result, err := adapter.Publish(ctx, provider.PublishRequest{ProjectID: projectID, VersionID: versionID, FilePath: artifact.Path})
	if err != nil {
		return s.finishPublishFailure(ctx, rel.ID, "publish_failed")
	}
	state, _ := json.Marshal(map[string]string{"status": result.Status, "url": result.URL})
	status := normalizeRemoteStatus(result.Status)
	if status == "failed" {
		return s.finishPublishFailureWithState(ctx, rel.ID, string(state), "publish_failed")
	}
	if err := s.repo.FinishRelease(ctx, rel.ID, status, result.RemoteID, string(state), "", "", s.nowMillis()); err != nil {
		return Release{}, err
	}
	latest, err := s.repo.GetRelease(ctx, rel.ID)
	return releaseDTO(latest), err
}

func (s *P7Service) finishPublishFailure(ctx context.Context, id, code string) (Release, error) {
	return s.finishPublishFailureWithState(ctx, id, `{}`, code)
}

func (s *P7Service) finishPublishFailureWithState(ctx context.Context, id, state, code string) (Release, error) {
	_ = s.repo.FinishRelease(ctx, id, "failed", "", state, code, "provider publication failed", s.nowMillis())
	latest, err := s.repo.GetRelease(ctx, id)
	if err != nil {
		return Release{}, err
	}
	return releaseDTO(latest), ErrPublishFailed
}

// PollRelease asks the provider for remote state. A transport failure leaves
// the durable state untouched so an explicit later poll/recovery is safe.
func (s *P7Service) PollRelease(ctx context.Context, releaseID string) (Release, error) {
	if err := s.ready(); err != nil {
		return Release{}, err
	}
	rel, err := s.repo.GetRelease(ctx, releaseID)
	if err != nil {
		return Release{}, err
	}
	if rel.Provider == "local" || rel.Status != "publishing" || rel.RemoteID == "" {
		return releaseDTO(rel), nil
	}
	if s.registry == nil {
		return releaseDTO(rel), ErrProviderStatusUnavailable
	}
	adapter, err := s.registry.Get(rel.Provider)
	if err != nil {
		return releaseDTO(rel), ErrProviderStatusUnavailable
	}
	remote, err := adapter.RemoteStatus(ctx, rel.RemoteID)
	if err != nil {
		return releaseDTO(rel), ErrProviderStatusUnavailable
	}
	state, _ := json.Marshal(map[string]string{"status": remote.Status, "url": remote.URL, "detail": remote.Detail})
	status := normalizeRemoteStatus(remote.Status)
	if err := s.repo.FinishRelease(ctx, rel.ID, status, remote.ID, string(state), "", "", s.nowMillis()); err != nil {
		return Release{}, err
	}
	latest, err := s.repo.GetRelease(ctx, releaseID)
	return releaseDTO(latest), err
}

// GetRelease returns the credential-free durable release state.
func (s *P7Service) GetRelease(ctx context.Context, releaseID string) (Release, error) {
	if err := s.ready(); err != nil {
		return Release{}, err
	}
	rel, err := s.repo.GetRelease(ctx, releaseID)
	if err != nil {
		return Release{}, err
	}
	return releaseDTO(rel), nil
}

func validatePublishInput(in PublishInput) error {
	if strings.TrimSpace(in.PackID) == "" || strings.TrimSpace(in.PackVersionID) == "" || strings.TrimSpace(in.Provider) == "" || strings.TrimSpace(in.ArtifactID) == "" || strings.TrimSpace(in.IdempotencyKey) == "" {
		return ErrInvalidBuildInput
	}
	if in.Provider != "local" && in.Provider != "curseforge" && in.Provider != "modrinth" {
		return ErrInvalidBuildInput
	}
	return nil
}

func normalizeRemoteStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "published", "complete", "completed":
		return "succeeded"
	case "failed", "error", "rejected":
		return "failed"
	default:
		return "publishing"
	}
}

func releaseDTO(r store.ReleaseRecord) Release {
	return Release{ID: r.ID, PackID: r.PackID, PackVersionID: r.PackVersionID, Provider: r.Provider, Status: r.Status, RemoteID: r.RemoteID, IdempotencyKey: r.IdempotencyKey, RemoteState: r.RemoteState, ArtifactID: r.ArtifactID, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, CreatedAt: iso(r.CreatedAt), UpdatedAt: iso(r.UpdatedAt)}
}

func (s *P7Service) nowMillis() int64 {
	if s != nil && s.now != nil {
		return s.now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
