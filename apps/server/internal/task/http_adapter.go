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

// NewHTTPAdapter creates a task facade over an explicit database handle.
// A nil db or a queue construction failure yields an unavailable adapter.
func NewHTTPAdapter(db *sql.DB) *HTTPAdapter {
	if db == nil {
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

// TaskView is the single contract Task DTO (docs/api/dto.md) exposed to HTTP:
// list, detail and control endpoints all return this exact shape. Payloads,
// filesystem paths and lease ownership are intentionally not included; extra
// runtime detail (message/attempt/log) is available via the log endpoint.
type TaskView struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	PackID     *string    `json:"packId"`
	PackName   *string    `json:"packName"`
	Status     string     `json:"status"`
	Progress   int        `json:"progress"`
	Error      *string    `json:"error"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
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

// PublicView maps a task to the contract Task DTO. PackName is left nil here;
// callers with a pack-name join (e.g. service.ListTasks) may fill it.
func PublicView(item *Task) TaskView {
	var errMsg *string
	if item.ErrorMessage != "" {
		v := item.ErrorMessage
		errMsg = &v
	} else if item.ErrorCode != "" {
		v := item.ErrorCode
		errMsg = &v
	}
	return TaskView{
		ID: item.ID, PackID: item.PackID, Type: PublicKind(item.Kind),
		Title: item.Title, Status: PublicStatus(item.Status), Progress: ProgressPercent(item.Progress),
		Error: errMsg, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt,
	}
}

// ProgressPercent normalizes the internal float progress to the contract's
// 0-100 integer.
func ProgressPercent(p float64) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return int(p + 0.5)
}

func view(item *Task) TaskView { return PublicView(item) }

// PublicKind maps internal task kinds to the contract's open-string type.
// Unknown kinds pass through unchanged, never as an empty string.
func PublicKind(kind Kind) string {
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
	case KindToolInstall:
		return "tool-install"
	case KindCacheGC:
		return "cache-cleanup"
	default:
		return string(kind)
	}
}

// PublicStatus maps internal states to the contract enum:
// queued/running/paused/success/failed/cancelled.
func PublicStatus(status Status) string {
	switch status {
	case StatusQueued:
		return "queued"
	case StatusLeased, StatusRunning:
		return "running"
	case StatusSucceeded:
		return "success"
	case StatusCanceled:
		return "cancelled"
	default:
		return string(status)
	}
}
