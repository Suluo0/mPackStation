package task

import (
	"context"
	"errors"
	"time"
)

// Worker drives a Queue from a single background goroutine. It deliberately
// contains no domain logic; handlers remain registered on the queue.
type Worker struct {
	queue   *Queue
	ID      string
	IdleMin time.Duration
	IdleMax time.Duration
}

// NewWorker creates a safe, cancellable worker loop.
func NewWorker(queue *Queue, id string) *Worker {
	if id == "" {
		id = "worker-1"
	}
	return &Worker{queue: queue, ID: id, IdleMin: 100 * time.Millisecond, IdleMax: 5 * time.Second}
}

// Run executes tasks until ctx is cancelled. Idle polling uses bounded
// exponential backoff and cancellation always interrupts the wait.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.queue == nil {
		return errors.New("worker queue is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := w.queue.Recover(ctx); err != nil {
		return err
	}
	min, max := w.IdleMin, w.IdleMax
	if min <= 0 {
		min = 100 * time.Millisecond
	}
	if max < min {
		max = min
	}
	delay := min
	for {
		_, err := w.queue.RunOnce(ctx, w.ID)
		if err == nil {
			delay = min
			continue
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrNoTaskAvailable) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			if delay < max {
				delay *= 2
				if delay > max {
					delay = max
				}
			}
			continue
		}
		// Transient queue/handler failures should not spin hot.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}
