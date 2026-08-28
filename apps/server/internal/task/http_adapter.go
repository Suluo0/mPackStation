package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// HTTPAdapter is the narrow transport-facing facade for task inspection and
// user controls. It keeps HTTP handlers from reaching into Queue internals;
// the queue remains the only owner of task state transitions.
type HTTPAdapter struct {
	queue *Queue
}

// NewHTTPAdapter creates a task facade from the process database source. The
// source is intentionally opaque to callers outside the task boundary.
func NewHTTPAdapter(source any) *HTTPAdapter {
	db, ok := source.(*sql.DB)
	if !ok || db == nil {
		return &HTTPAdapter{}
	}
	queue, err := NewQueue(db)
	if err != nil {
		return &HTTPAdapter{}
	}
	return &HTTPAdapter{queue: queue}
}

var errHTTPAdapterUnavailable = errors.New("task adapter unavailable")

// IsAdapterUnavailable reports whether the process composition did not supply
// a usable task queue. It is exported for higher-level error mapping without
// exposing the sentinel itself.
func IsAdapterUnavailable(err error) bool { return errors.Is(err, errHTTPAdapterUnavailable) }

// TaskView is the stable, non-sensitive task detail representation exposed to
// HTTP. Payloads, filesystem paths and lease ownership are intentionally not
// included in this view.
type TaskView struct {
	ID           string     `json:"id"`
	PackID       *string    `json:"packId,omitempty"`
	Type         string     `json:"type"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Progress     float64    `json:"progress"`
	Message      string     `json:"message"`
	ErrorCode    string     `json:"errorCode,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	Attempt      int        `json:"attempt"`
	MaxAttempts  int        `json:"maxAttempts"`
	RecoverCount int        `json:"recoverCount"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

// EventView is the safe task event representation used by detail and log
// endpoints. Event detail is already canonical JSON and never contains the
// task payload or lease credentials.
type EventView struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"taskId"`
	Sequence  int       `json:"sequence"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Detail    any       `json:"detail"`
	CreatedAt time.Time `json:"createdAt"`
}

func (a *HTTPAdapter) ready() error {
	if a == nil || a.queue == nil {
		return errHTTPAdapterUnavailable
	}
	return nil
}

// Get returns one safe task detail view.
func (a *HTTPAdapter) Get(ctx context.Context, id string) (TaskView, error) {
	if err := a.ready(); err != nil {
		return TaskView{}, err
	}
	item, err := a.queue.Get(ctx, id)
	if err != nil {
		return TaskView{}, err
	}
	return view(item), nil
}

// Events returns the append-only event history for a task.
func (a *HTTPAdapter) Events(ctx context.Context, id string) ([]EventView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	events, err := a.queue.Events(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]EventView, 0, len(events))
	for _, event := range events {
		var detail any
		if len(event.Detail) != 0 {
			if err := json.Unmarshal(event.Detail, &detail); err != nil {
				detail = map[string]any{}
			}
		}
		result = append(result, EventView{ID: event.ID, TaskID: event.TaskID, Sequence: event.Sequence, Status: string(event.Status), Message: event.Message, Detail: detail, CreatedAt: event.CreatedAt})
	}
	return result, nil
}

// Cancel applies the user cancellation transition.
func (a *HTTPAdapter) Cancel(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.queue.Cancel(ctx, id)
}

// Pause applies a user pause using the currently persisted lease owner and
// epoch. The queue rechecks both values in its fenced UPDATE, so a concurrent
// recovery or worker transition cannot be paused by a stale observation.
func (a *HTTPAdapter) Pause(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	item, err := a.queue.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Status != StatusRunning || item.LeaseOwner == "" || item.LeaseEpoch == 0 {
		return fmt.Errorf("%w: task is not running", ErrInvalidTransition)
	}
	return a.queue.Pause(ctx, id, item.LeaseOwner, item.LeaseEpoch)
}

// Resume moves a paused task back to queued.
func (a *HTTPAdapter) Resume(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.queue.Resume(ctx, id)
}

// Retry explicitly resets a failed or canceled task to queued.
func (a *HTTPAdapter) Retry(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.queue.Retry(ctx, id)
}

func view(item *Task) TaskView {
	result := TaskView{
		ID: item.ID, PackID: item.PackID, Type: publicKind(item.Kind),
		Title: item.Title, Status: publicStatus(item.Status), Progress: item.Progress,
		Message: item.Message, ErrorCode: item.ErrorCode, ErrorMessage: item.ErrorMessage,
		Attempt: item.Attempt, MaxAttempts: item.MaxAttempts, RecoverCount: item.RecoverCount,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
	result.StartedAt = item.StartedAt
	result.FinishedAt = item.FinishedAt
	return result
}

func publicKind(kind Kind) string {
	switch kind {
	case KindIndex:
		return "index-mod"
	case KindBuild:
		return "build-pack"
	case KindImport:
		return "import-pack"
	case KindResolve:
		return "update-preflight"
	case KindDownload:
		return "download-mod"
	case KindPublish:
		return "publish-pack"
	case KindCacheGC:
		return "cache-cleanup"
	default:
		return string(kind)
	}
}

func publicStatus(status Status) string {
	switch status {
	case StatusQueued, StatusLeased:
		return "running"
	case StatusSucceeded:
		return "success"
	case StatusCanceled:
		return "cancelled"
	default:
		return string(status)
	}
}
