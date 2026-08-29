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
	"mpackstation/internal/task"
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
	ID             string `json:"id"`
	PackID         string `json:"packId"`
	PackVersionID  string `json:"packVersionId"`
	Provider       string `json:"provider"`
	Status         string `json:"status"`
	RemoteID       string `json:"remoteId"`
	IdempotencyKey string `json:"idempotencyKey"`
	RemoteState    string `json:"remoteState"`
	ArtifactID     string `json:"artifactId"`
	ErrorCode      string `json:"errorCode"`
	ErrorMessage   string `json:"errorMessage"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// P7Service owns build publication orchestration while SQL remains in store
// and remote HTTP remains behind provider.Adapter.
type P7Service struct {
	repo     *store.Repository
	registry *provider.Registry
	queue    *task.Queue
	now      func() time.Time
}

// NewP7Service creates the P7 service over a migrated database.
func NewP7Service(db *sql.DB) *P7Service {
	if db == nil {
		return &P7Service{now: time.Now}
	}
	q, _ := task.NewQueue(db)
	return &P7Service{repo: store.NewRepository(db), now: time.Now, queue: q}
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

func (s *P7Service) ListDeliveryChecks(ctx context.Context, packID, versionID string) ([]DeliveryCheck, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a := &API{repo: s.repo, now: s.now}
	return a.ListDeliveryChecks(ctx, packID, versionID)
}
func (s *P7Service) RunDeliveryChecks(ctx context.Context, packID, versionID string, checks []DeliveryCheck) ([]DeliveryCheck, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a := &API{repo: s.repo, now: s.now}
	return a.RunDeliveryChecks(ctx, packID, versionID, checks)
}
func (s *P7Service) ListArtifacts(ctx context.Context, packID, versionID string) ([]Artifact, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a := &API{repo: s.repo, now: s.now}
	return a.ListArtifacts(ctx, packID, versionID)
}
func (s *P7Service) ReadArtifact(ctx context.Context, packID, artifactID string) (Artifact, []byte, error) {
	if err := s.ready(); err != nil {
		return Artifact{}, nil, err
	}
	a := &API{repo: s.repo, now: s.now}
	return a.ReadArtifact(ctx, packID, artifactID)
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
	requestState, _ := json.Marshal(map[string]any{"request": map[string]string{"projectId": in.ProjectID, "versionId": in.VersionID}})
	created := store.ReleaseRecord{ID: newID("release"), PackID: in.PackID, PackVersionID: in.PackVersionID, Provider: in.Provider, Status: "pending", IdempotencyKey: in.IdempotencyKey, RemoteState: string(requestState), ArtifactID: in.ArtifactID, CreatedAt: now, UpdatedAt: now}
	rel, err := s.repo.CreateRelease(ctx, created)
	if err != nil {
		return Release{}, err
	}
	if rel.PackID != in.PackID || rel.PackVersionID != in.PackVersionID || rel.ArtifactID != in.ArtifactID {
		return Release{}, ErrPublishIdempotencyConflict
	}
	var prior struct {
		Request struct {
			ProjectID string `json:"projectId"`
			VersionID string `json:"versionId"`
		} `json:"request"`
	}
	if json.Unmarshal([]byte(rel.RemoteState), &prior) == nil && (prior.Request.ProjectID != "" || prior.Request.VersionID != "") && (prior.Request.ProjectID != in.ProjectID || prior.Request.VersionID != in.VersionID) {
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
		state, _ := mergeRemoteState(rel.RemoteState, map[string]string{"provider": "local", "status": "succeeded"})
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
	state, _ := mergeRemoteState(rel.RemoteState, map[string]string{"status": result.Status, "url": result.URL})
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
	if prior, err := s.repo.GetRelease(ctx, id); err == nil {
		if merged, merr := mergeRemoteState(prior.RemoteState, map[string]string{"errorCode": code}); merr == nil {
			var incoming map[string]any
			_ = json.Unmarshal([]byte(state), &incoming)
			var base map[string]any
			_ = json.Unmarshal(merged, &base)
			for k, v := range incoming {
				base[k] = v
			}
			if b, merr := json.Marshal(base); merr == nil {
				state = string(b)
			}
		}
	}
	_ = s.repo.FinishRelease(ctx, id, "failed", "", state, code, "provider publication failed", s.nowMillis())
	latest, err := s.repo.GetRelease(ctx, id)
	if err != nil {
		return Release{}, err
	}
	return releaseDTO(latest), ErrPublishFailed
}

func mergeRemoteState(existing string, fields map[string]string) ([]byte, error) {
	obj := map[string]any{}
	if existing != "" {
		_ = json.Unmarshal([]byte(existing), &obj)
	}
	for k, v := range fields {
		obj[k] = v
	}
	return json.Marshal(obj)
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
	if rel.Provider == "local" || rel.Status != "publishing" {
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

func (s *P7Service) ListReleases(ctx context.Context, packID, versionID string) ([]Release, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListReleases(ctx, packID, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]Release, 0, len(rows))
	for _, r := range rows {
		out = append(out, releaseDTO(r))
	}
	return out, nil
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
