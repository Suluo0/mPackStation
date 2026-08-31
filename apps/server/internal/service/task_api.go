package service

import (
	"context"
	"database/sql"
	"errors"

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

// TaskView and EventView are transport-neutral service DTOs. Aliases preserve
// one canonical mapping while keeping the task package out of httpapi.
type TaskView = task.TaskView
type EventView = task.EventView

func (a *TaskAPI) Get(ctx context.Context, id string) (TaskView, error) {
	if err := a.ready(); err != nil {
		return TaskView{}, err
	}
	return a.adapter.Get(ctx, id)
}

func (a *TaskAPI) Events(ctx context.Context, id string) ([]EventView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.adapter.Events(ctx, id)
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
