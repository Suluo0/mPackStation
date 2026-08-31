package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// HTTPAdapter is the narrow transport-facing facade for task inspection and
// user controls. It keeps HTTP handlers from reaching into Queue internals;
// the queue remains the only owner of task state transitions. It returns raw
// domain records only — presentation mapping (contract Task DTO, JSON event
// decoding) lives in the service layer (B4).
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

func (a *HTTPAdapter) ready() error {
	if a == nil || a.queue == nil {
		return errHTTPAdapterUnavailable
	}
	return nil
}

// Get returns one raw task record.
func (a *HTTPAdapter) Get(ctx context.Context, id string) (*Task, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.queue.Get(ctx, id)
}

// Events returns the append-only raw event history for a task.
func (a *HTTPAdapter) Events(ctx context.Context, id string) ([]Event, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.queue.Events(ctx, id)
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
