package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"mpackstation/internal/task"
)

// BuildTaskPayload is the canonical durable representation for asynchronous
// builds. File bytes are base64 encoded so payloads are self-contained JSON.
type BuildTaskPayload struct {
	PackID          string          `json:"packId"`
	PackVersionID   string          `json:"packVersionId"`
	ExportDirName   string          `json:"exportDirName"`
	Files           []BuildTaskFile `json:"files"`
	LockSnapshot    json.RawMessage `json:"lockSnapshot"`
	ContentSnapshot json.RawMessage `json:"contentSnapshot"`
	QuestSnapshot   json.RawMessage `json:"questSnapshot"`
	BuildConfig     json.RawMessage `json:"buildConfig"`
	Checks          []DeliveryCheck `json:"checks"`
}

type BuildTaskFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type PublishTaskPayload struct {
	PackID        string `json:"packId"`
	PackVersionID string `json:"packVersionId"`
	Provider      string `json:"provider"`
	ArtifactID    string `json:"artifactId"`
	ProjectID     string `json:"projectId"`
	VersionID     string `json:"versionId"`
}

// SetTaskQueue injects the process queue used for asynchronous submissions.
func (s *P7Service) SetTaskQueue(q *task.Queue) {
	if s != nil {
		s.queue = q
	}
}

// RegisterTaskHandlers installs build and publish domain handlers on a queue
// registry. The task package remains unaware of domain semantics.
func (s *P7Service) RegisterTaskHandlers(reg *task.Registry) error {
	if s == nil || reg == nil {
		return errors.New("task registry is nil")
	}
	if err := reg.Register(task.KindBuild, task.HandlerFunc(s.handleBuildTask)); err != nil {
		return err
	}
	return reg.Register(task.KindPublish, task.HandlerFunc(s.handlePublishTask))
}

// RegisterTaskHandlersOnQueue is a composition-root convenience wrapper.
func (s *P7Service) RegisterTaskHandlersOnQueue(q *task.Queue) error {
	if q == nil {
		return errors.New("task queue is nil")
	}
	if err := q.RegisterHandler(task.KindBuild, task.HandlerFunc(s.handleBuildTask)); err != nil {
		return err
	}
	return q.RegisterHandler(task.KindPublish, task.HandlerFunc(s.handlePublishTask))
}

// SubmitBuildTask persists a canonical asynchronous build request.
func (s *P7Service) SubmitBuildTask(ctx context.Context, in BuildInput, idem string) (*task.Task, bool, error) {
	if s == nil || s.queue == nil {
		return nil, false, ErrUnavailable
	}
	p := BuildTaskPayload{PackID: in.PackID, PackVersionID: in.PackVersionID, ExportDirName: in.ExportDirName, LockSnapshot: in.LockSnapshot, ContentSnapshot: in.ContentSnapshot, QuestSnapshot: in.QuestSnapshot, BuildConfig: in.BuildConfig, Checks: in.Checks}
	p.Files = make([]BuildTaskFile, 0, len(in.Files))
	for _, f := range in.Files {
		p.Files = append(p.Files, BuildTaskFile{Path: f.Path, Content: base64.StdEncoding.EncodeToString(f.Content)})
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, false, err
	}
	return s.queue.Submit(ctx, task.SubmitRequest{PackID: strPtr(in.PackID), Kind: task.KindBuild, Title: "Build pack", Payload: b, IdempotencyKey: idem})
}

// SubmitPublishTask persists a canonical asynchronous publication request.
func (s *P7Service) SubmitPublishTask(ctx context.Context, in PublishInput) (*task.Task, bool, error) {
	if s == nil || s.queue == nil {
		return nil, false, ErrUnavailable
	}
	// Contract: unknown pack is a 404 pack_not_found, not an opaque task failure.
	if _, err := s.repo.GetPack(ctx, in.PackID); err != nil {
		if IsNotFound(err) {
			return nil, false, NotFoundError("pack_not_found", "pack not found")
		}
		return nil, false, err
	}
	p := PublishTaskPayload{PackID: in.PackID, PackVersionID: in.PackVersionID, Provider: in.Provider, ArtifactID: in.ArtifactID, ProjectID: in.ProjectID, VersionID: in.VersionID}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, false, err
	}
	return s.queue.Submit(ctx, task.SubmitRequest{PackID: strPtr(in.PackID), Kind: task.KindPublish, Title: "Publish pack", Payload: b, IdempotencyKey: in.IdempotencyKey, MaxAttempts: 1})
}

func (s *P7Service) handleBuildTask(ctx context.Context, ex *task.Execution) error {
	var p BuildTaskPayload
	if err := json.Unmarshal(ex.Task.Payload, &p); err != nil {
		return &task.TaskError{Code: "invalid_payload", Message: "invalid build task payload"}
	}
	if p.PackID == "" {
		p.PackID = valueOrEmpty(ex.Task.PackID)
	}
	in := BuildInput{PackID: p.PackID, PackVersionID: p.PackVersionID, ExportDirName: p.ExportDirName, LockSnapshot: p.LockSnapshot, ContentSnapshot: p.ContentSnapshot, QuestSnapshot: p.QuestSnapshot, BuildConfig: p.BuildConfig, Checks: p.Checks, TaskID: ex.Task.ID}
	in.Files = make([]BuildFile, 0, len(p.Files))
	for _, f := range p.Files {
		b, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return &task.TaskError{Code: "invalid_payload", Message: "invalid build file encoding"}
		}
		in.Files = append(in.Files, BuildFile{Path: f.Path, Content: b})
	}
	if err := ex.Progress(ctx, 5, "building"); err != nil {
		return err
	}
	result, err := s.BuildPack(ctx, in)
	if err != nil {
		return err
	}
	_ = result
	return nil
}

func (s *P7Service) handlePublishTask(ctx context.Context, ex *task.Execution) error {
	var p PublishTaskPayload
	if err := json.Unmarshal(ex.Task.Payload, &p); err != nil {
		return &task.TaskError{Code: "invalid_payload", Message: "invalid publish task payload"}
	}
	if p.PackID == "" {
		p.PackID = valueOrEmpty(ex.Task.PackID)
	}
	_, err := s.PublishPack(ctx, PublishInput{PackID: p.PackID, PackVersionID: p.PackVersionID, Provider: p.Provider, ArtifactID: p.ArtifactID, IdempotencyKey: ex.Task.IdempotencyKey, ProjectID: p.ProjectID, VersionID: p.VersionID})
	if err != nil {
		if errors.Is(err, ErrPublishFailed) {
			return &task.TaskError{Code: "publish_failed", Message: "provider publication failed", Retryable: false}
		}
		return err
	}
	return nil
}

func strPtr(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
