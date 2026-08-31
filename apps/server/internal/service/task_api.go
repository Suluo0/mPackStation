package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"mpackstation/internal/task"
)

// TaskAPI is the application-facing facade for task inspection and user
// controls. HTTP handlers depend on this service boundary rather than on the
// task queue implementation.
type TaskAPI struct {
	adapter *task.HTTPAdapter
}

// NewTaskAPI assembles the task facade over an explicit database handle.
// A nil db yields an unavailable facade (ready() reports ErrUnavailable).
func NewTaskAPI(db *sql.DB) *TaskAPI {
	if db != nil {
		return &TaskAPI{adapter: task.NewHTTPAdapter(db)}
	}
	return &TaskAPI{}
}

func (a *TaskAPI) ready() error {
	if a == nil || a.adapter == nil {
		return ErrUnavailable
	}
	return nil
}

// TaskView is the single contract Task DTO (docs/api/dto.md): list, detail and
// control endpoints all return this exact shape. Payloads, filesystem paths
// and lease ownership are intentionally not included; extra runtime detail
// (message/attempt/log) is available via the log endpoint. The DTO and its
// mapping live here in the service layer (B4); the task package only owns
// queue state and raw records.
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

// taskView maps a raw queue record to the contract Task DTO. PackName is left
// nil here; callers with a pack-name join (ListTasks) fill it.
func taskView(item *task.Task) TaskView {
	var errMsg *string
	if item.ErrorMessage != "" {
		v := item.ErrorMessage
		errMsg = &v
	} else if item.ErrorCode != "" {
		v := item.ErrorCode
		errMsg = &v
	}
	return TaskView{
		ID: item.ID, PackID: item.PackID, Type: publicKind(item.Kind),
		Title: item.Title, Status: publicStatus(item.Status), Progress: progressPercent(item.Progress),
		Error: errMsg, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt,
	}
}

func eventViews(events []task.Event) []EventView {
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
	return result
}

// progressPercent normalizes the internal float progress to the contract's
// 0-100 integer.
func progressPercent(p float64) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return int(p + 0.5)
}

// publicKind maps internal task kinds to the contract's open-string type.
// Unknown kinds pass through unchanged, never as an empty string.
func publicKind(kind task.Kind) string {
	switch kind {
	case task.KindIndex:
		return "index-mod"
	case task.KindBuild:
		return "build-pack"
	case task.KindImport:
		return "import-pack"
	case task.KindResolve:
		return "update-preflight"
	case task.KindDownload:
		return "download-mod"
	case task.KindPublish:
		return "publish-pack"
	case task.KindToolInstall:
		return "tool-install"
	case task.KindCacheGC:
		return "cache-cleanup"
	default:
		return string(kind)
	}
}

// publicStatus maps internal states to the contract enum:
// queued/running/paused/success/failed/cancelled.
func publicStatus(status task.Status) string {
	switch status {
	case task.StatusQueued:
		return "queued"
	case task.StatusLeased, task.StatusRunning:
		return "running"
	case task.StatusSucceeded:
		return "success"
	case task.StatusCanceled:
		return "cancelled"
	default:
		return string(status)
	}
}

func (a *TaskAPI) Get(ctx context.Context, id string) (TaskView, error) {
	if err := a.ready(); err != nil {
		return TaskView{}, err
	}
	item, err := a.adapter.Get(ctx, id)
	if err != nil {
		return TaskView{}, err
	}
	return taskView(item), nil
}

func (a *TaskAPI) Events(ctx context.Context, id string) ([]EventView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	events, err := a.adapter.Events(ctx, id)
	if err != nil {
		return nil, err
	}
	return eventViews(events), nil
}

func (a *TaskAPI) Cancel(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.adapter.Cancel(ctx, id)
}

func (a *TaskAPI) Pause(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.adapter.Pause(ctx, id)
}

func (a *TaskAPI) Resume(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.adapter.Resume(ctx, id)
}

func (a *TaskAPI) Retry(ctx context.Context, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.adapter.Retry(ctx, id)
}

// Task error predicates keep transport error mapping independent from the
// queue implementation package.
func IsTaskNotFound(err error) bool          { return errors.Is(err, task.ErrNotFound) }
func IsTaskInvalidTransition(err error) bool { return errors.Is(err, task.ErrInvalidTransition) }
func IsTaskLeaseLost(err error) bool         { return errors.Is(err, task.ErrLeaseLost) }
func IsTaskUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable) || task.IsAdapterUnavailable(err)
}
func IsTaskNotAvailable(err error) bool { return errors.Is(err, task.ErrNoTaskAvailable) }
func IsTaskIdempotencyConflict(err error) bool {
	return errors.Is(err, task.ErrIdempotencyConflict)
}
func IsTaskIdempotencyConsumed(err error) bool { return errors.Is(err, task.ErrIdempotencyConsumed) }
func IsTaskUnknownKind(err error) bool         { return errors.Is(err, task.ErrUnknownKind) }
