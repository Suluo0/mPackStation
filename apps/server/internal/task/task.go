// Package task implements the durable task queue and its worker protocol.
//
// The package owns all writes to tasks, task_events and task_idem_keys. Domain
// handlers are registered by service packages; this package knows only about
// task lifecycle, leases and fencing.
package task

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Kind is the stable internal task kind.
type Kind string

const (
	KindResolve  Kind = "resolve"
	KindDownload Kind = "download"
	KindIndex    Kind = "index"
	KindBuild    Kind = "build"
	KindPublish  Kind = "publish"
	KindImport   Kind = "import"
	KindCacheGC  Kind = "cache_gc"
)

// Status is the durable lifecycle state of a task.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusLeased    Status = "leased"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

const (
	defaultLeaseTTL     = 30 * time.Second
	defaultTaskDeadline = 2 * time.Hour
	maxRecoveryCount    = 10
	maxBackoff          = 300 * time.Second
)

var (
	// ErrNotFound means that a task does not exist.
	ErrNotFound = errors.New("task not found")
	// ErrInvalidTransition means that the requested state transition is illegal.
	ErrInvalidTransition = errors.New("invalid task state transition")
	// ErrLeaseLost means that the worker no longer owns the task lease.
	ErrLeaseLost = errors.New("task lease lost")
	// ErrIdempotencyConflict means the key was previously used with another request.
	ErrIdempotencyConflict = errors.New("idempotency key payload conflict")
	// ErrIdempotencyConsumed means the key is retained but its task was removed.
	ErrIdempotencyConsumed = errors.New("idempotency key already consumed")
	// ErrNoTaskAvailable means no queued task is ready for this worker.
	ErrNoTaskAvailable = errors.New("no task available")
	// ErrUnknownKind means a task kind has no registered handler.
	ErrUnknownKind = errors.New("unknown task kind")
)

// Clock makes time-dependent state transitions deterministic in tests.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// IDGenerator supplies IDs for task and event rows.
type IDGenerator func() string

func randomID() string {
	// The timestamp plus a process-local counter is sufficient for the durable
	// primary key because SQLite uniqueness remains the final guard. The queue
	// never exposes this format as an API contract.
	return fmt.Sprintf("t-%d", time.Now().UnixNano())
}

// SubmitRequest describes a new durable task.
type SubmitRequest struct {
	PackID         *string
	Kind           Kind
	Title          string
	Payload        json.RawMessage
	IdempotencyKey string
	MaxAttempts    int
}

// Task is the task row and its current lease metadata.
type Task struct {
	ID             string
	PackID         *string
	Kind           Kind
	Title          string
	Status         Status
	Progress       float64
	Message        string
	Payload        json.RawMessage
	PayloadPath    string
	ErrorCode      string
	ErrorMessage   string
	Attempt        int
	MaxAttempts    int
	RecoverCount   int
	LeaseOwner     string
	LeaseEpoch     int64
	LeaseExpiresAt *time.Time
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

// Event is an append-only, human-readable task lifecycle record.
type Event struct {
	ID        string
	TaskID    string
	Sequence  int
	Status    Status
	Message   string
	Detail    json.RawMessage
	CreatedAt time.Time
}

// TaskError allows a handler to return stable, user-facing failure metadata.
type TaskError struct {
	Code      string
	Message   string
	Detail    json.RawMessage
	Retryable bool
}

func (e *TaskError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

// Handler is implemented by a service package. It must not write task tables
// directly; Execution methods are the only lifecycle write boundary.
type Handler interface {
	Handle(context.Context, *Execution) error
}

// HandlerFunc adapts a function into a Handler.
type HandlerFunc func(context.Context, *Execution) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, e *Execution) error { return f(ctx, e) }

// Registry maps task kinds to domain handlers. It intentionally has no domain
// imports so service packages can register handlers without a dependency cycle.
type Registry struct {
	mu       sync.RWMutex
	handlers map[Kind]Handler
}

// NewRegistry creates an empty handler registry.
func NewRegistry() *Registry { return &Registry{handlers: make(map[Kind]Handler)} }

// Register associates kind with handler, replacing any previous handler.
func (r *Registry) Register(kind Kind, handler Handler) error {
	if !validKind(kind) || handler == nil {
		return fmt.Errorf("register handler: %w", ErrUnknownKind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = handler
	return nil
}

func (r *Registry) get(kind Kind) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[kind]
	return h, ok
}

// Queue is the durable task state machine.
type Queue struct {
	db       *sql.DB
	clock    Clock
	id       IDGenerator
	leaseTTL time.Duration
	deadline time.Duration
	registry *Registry

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// Option customizes a Queue. Options are primarily useful for deterministic tests.
type Option func(*Queue)

// WithClock sets the queue clock.
func WithClock(clock Clock) Option {
	return func(q *Queue) {
		if clock != nil {
			q.clock = clock
		}
	}
}

// WithIDGenerator sets the ID source.
func WithIDGenerator(generator IDGenerator) Option {
	return func(q *Queue) {
		if generator != nil {
			q.id = generator
		}
	}
}

// WithLeaseTTL sets the worker lease duration.
func WithLeaseTTL(ttl time.Duration) Option {
	return func(q *Queue) {
		if ttl > 0 {
			q.leaseTTL = ttl
		}
	}
}

// WithTaskDeadline sets the maximum running duration used by recovery.
func WithTaskDeadline(deadline time.Duration) Option {
	return func(q *Queue) {
		if deadline > 0 {
			q.deadline = deadline
		}
	}
}

// WithRegistry connects a worker runner to a handler registry.
func WithRegistry(registry *Registry) Option { return func(q *Queue) { q.registry = registry } }

// NewQueue creates a queue over the canonical SQLite schema.
func NewQueue(db *sql.DB, options ...Option) (*Queue, error) {
	if db == nil {
		return nil, errors.New("task queue database is nil")
	}
	q := &Queue{
		db: db, clock: systemClock{}, id: randomID,
		leaseTTL: defaultLeaseTTL, deadline: defaultTaskDeadline,
		registry: NewRegistry(), running: make(map[string]context.CancelFunc),
	}
	for _, option := range options {
		if option != nil {
			option(q)
		}
	}
	return q, nil
}

// Submit persists a queued task and its first event. Idempotency keys are
// retained forever and bind an endpoint/kind to the canonical payload hash.
func (q *Queue) Submit(ctx context.Context, request SubmitRequest) (*Task, bool, error) {
	if err := validateSubmit(request); err != nil {
		return nil, false, err
	}
	now := q.clock.Now().UnixMilli()
	payload, hash, err := canonicalPayload(request.Payload)
	if err != nil {
		return nil, false, err
	}
	maxAttempts := request.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin submit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if request.IdempotencyKey != "" {
		var existingID sql.NullString
		var existingHash string
		err = tx.QueryRowContext(ctx, `SELECT task_id, payload_hash FROM task_idem_keys WHERE idempotency_key = ?`, request.IdempotencyKey).Scan(&existingID, &existingHash)
		if err == nil {
			if existingHash != hash {
				return nil, false, ErrIdempotencyConflict
			}
			if !existingID.Valid {
				return nil, false, ErrIdempotencyConsumed
			}
			existing, loadErr := q.loadTask(ctx, tx, existingID.String)
			if loadErr != nil {
				return nil, false, loadErr
			}
			return existing, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, fmt.Errorf("read idempotency key: %w", err)
		}
	}
	id := q.id()
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks
        (id, pack_id, kind, title, status, progress, message, payload, attempt, max_attempts, created_at, updated_at, idempotency_key)
        VALUES (?, ?, ?, ?, 'queued', 0, '', ?, 0, ?, ?, ?, ?)`,
		id, request.PackID, string(request.Kind), request.Title, string(payload), maxAttempts, now, now, nullString(request.IdempotencyKey)); err != nil {
		return nil, false, fmt.Errorf("insert task: %w", err)
	}
	if request.IdempotencyKey != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_idem_keys(idempotency_key, endpoint, kind, payload_hash, task_id, created_at) VALUES (?, 'task.submit', ?, ?, ?, ?)`, request.IdempotencyKey, request.Kind, hash, id, now); err != nil {
			return nil, false, fmt.Errorf("insert idempotency key: %w", err)
		}
	}
	if err := q.appendEvent(ctx, tx, id, StatusQueued, "queued", []byte(`{}`), now); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit submit: %w", err)
	}
	result, err := q.Get(ctx, id)
	return result, false, err
}

// Get returns one task by ID.
func (q *Queue) Get(ctx context.Context, id string) (*Task, error) {
	return q.loadTask(ctx, q.db, id)
}

// List returns recent tasks in stable creation order. A non-positive limit uses
// the bounded default of 100 to prevent an unbounded API read.
func (q *Queue) List(ctx context.Context, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := q.db.QueryContext(ctx, `SELECT id FROM tasks ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan task id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("list task rows: %w", err)
	}
	_ = rows.Close()
	var result []*Task
	for _, id := range ids {
		item, err := q.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// Events returns append-only lifecycle events for one task.
func (q *Queue) Events(ctx context.Context, id string) ([]Event, error) {
	if _, err := q.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := q.db.QueryContext(ctx, `SELECT id, task_id, sequence, status, message, detail, created_at FROM task_events WHERE task_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()
	var result []Event
	for rows.Next() {
		var event Event
		var status string
		var detail string
		var created int64
		if err := rows.Scan(&event.ID, &event.TaskID, &event.Sequence, &status, &event.Message, &detail, &created); err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		event.Status = Status(status)
		event.Detail = json.RawMessage(detail)
		event.CreatedAt = time.UnixMilli(created)
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read task events: %w", err)
	}
	return result, nil
}

// Lease atomically claims one ready queued task for workerID and increments its epoch.
func (q *Queue) Lease(ctx context.Context, workerID string) (*Task, error) {
	if workerID == "" {
		return nil, errors.New("worker id is empty")
	}
	if _, err := q.Recover(ctx); err != nil {
		return nil, err
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin lease: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := q.clock.Now().UnixMilli()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE status = 'queued' AND updated_at <= ? ORDER BY created_at, id LIMIT 1`, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoTaskAvailable
	}
	if err != nil {
		return nil, fmt.Errorf("select queued task: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='leased', lease_owner=?, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=?, attempt=attempt+1, updated_at=? WHERE id=? AND status='queued'`, workerID, now+q.leaseTTL.Milliseconds(), now, id)
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return nil, ErrLeaseLost
	}
	if err := q.appendEvent(ctx, tx, id, StatusLeased, "leased", jsonDetail("worker", workerID), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit lease: %w", err)
	}
	return q.Get(ctx, id)
}

// Begin transitions a leased task to running, fenced by owner and epoch.
func (q *Queue) Begin(ctx context.Context, id, workerID string, epoch int64) error {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='running', started_at=COALESCE(started_at,?), updated_at=? WHERE id=? AND status='leased' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`, now, now, id, workerID, epoch, now)
	if err != nil {
		return fmt.Errorf("start task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.fencedResult(ctx, id, workerID, epoch)
	}
	if err := q.appendEvent(ctx, tx, id, StatusRunning, "running", []byte(`{}`), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit begin: %w", err)
	}
	return nil
}

// Heartbeat extends a live lease. It accepts leased and running states.
func (q *Queue) Heartbeat(ctx context.Context, id, workerID string, epoch int64) error {
	now := q.clock.Now().UnixMilli()
	result, err := q.db.ExecContext(ctx, `UPDATE tasks SET lease_expires_at=?, updated_at=? WHERE id=? AND status IN ('leased','running') AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`, now+q.leaseTTL.Milliseconds(), now, id, workerID, epoch, now)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.fencedResult(ctx, id, workerID, epoch)
	}
	return nil
}

// Progress updates a running task and appends a human-readable event.
func (q *Queue) Progress(ctx context.Context, id, workerID string, epoch int64, progress float64, message string) error {
	if progress < 0 || progress > 100 {
		return errors.New("task progress must be between 0 and 100")
	}
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin progress: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET progress=?, message=?, updated_at=? WHERE id=? AND status='running' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`, progress, message, now, id, workerID, epoch, now)
	if err != nil {
		return fmt.Errorf("update progress: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.fencedResultTx(ctx, tx, id, workerID, epoch)
	}
	if err := q.appendEvent(ctx, tx, id, StatusRunning, message, jsonDetail("progress", progress), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit progress: %w", err)
	}
	return nil
}

// Succeed completes a running task, fenced by owner and epoch.
func (q *Queue) Succeed(ctx context.Context, id, workerID string, epoch int64, message string) error {
	return q.finish(ctx, id, workerID, epoch, StatusSucceeded, "", "", message)
}

// Fail completes or schedules retry for a running task. Retryable errors use
// 1/2/4/8 second exponential backoff, capped at five minutes.
func (q *Queue) Fail(ctx context.Context, id, workerID string, epoch int64, taskErr *TaskError) error {
	if taskErr == nil {
		taskErr = &TaskError{Code: "task_failed", Message: "task failed"}
	}
	if taskErr.Code == "" {
		taskErr.Code = "task_failed"
	}
	if taskErr.Message == "" {
		taskErr.Message = "task failed"
	}
	return q.failWithRetry(ctx, id, workerID, epoch, taskErr)
}

// Cancel cooperatively cancels a queued, leased, running or paused task and
// increments the fencing epoch so an old worker can no longer write.
func (q *Queue) Cancel(ctx context.Context, id string) error {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cancel: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='canceled', lease_owner=NULL, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=NULL, finished_at=?, updated_at=? WHERE id=? AND status IN ('queued','leased','running','paused')`, now, now, id)
	if err != nil {
		return fmt.Errorf("cancel task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.taskTransitionStatus(ctx, id)
	}
	if err := q.appendEvent(ctx, tx, id, StatusCanceled, "canceled", []byte(`{}`), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cancel: %w", err)
	}
	q.stopRunning(id)
	return nil
}

// Pause pauses only task kinds that can safely resume from a queued boundary.
func (q *Queue) Pause(ctx context.Context, id, workerID string, epoch int64) error {
	task, err := q.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Kind == KindPublish {
		return fmt.Errorf("%w: publish cannot be paused", ErrInvalidTransition)
	}
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pause: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='paused', lease_owner=NULL, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=NULL, updated_at=? WHERE id=? AND status='running' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`, now, id, workerID, epoch, now)
	if err != nil {
		return fmt.Errorf("pause task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.fencedResultTx(ctx, tx, id, workerID, epoch)
	}
	if err := q.appendEvent(ctx, tx, id, StatusPaused, "paused", []byte(`{}`), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pause: %w", err)
	}
	q.stopRunning(id)
	return nil
}

// Resume moves a paused task back to the ready queue.
func (q *Queue) Resume(ctx context.Context, id string) error {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin resume: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='queued', message='queued', error_code='', error_message='', updated_at=? WHERE id=? AND status='paused'`, now, id)
	if err != nil {
		return fmt.Errorf("resume task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.taskTransitionStatusTx(ctx, tx, id)
	}
	if err := q.appendEvent(ctx, tx, id, StatusQueued, "queued", jsonDetail("resumed", true), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit resume: %w", err)
	}
	return nil
}

// Retry requeues a failed task as a fresh logical attempt while retaining its history.
func (q *Queue) Retry(ctx context.Context, id string) error {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin retry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='queued', progress=0, message='queued', error_code='', error_message='', attempt=0, recover_count=0, lease_owner=NULL, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=NULL, started_at=NULL, finished_at=NULL, updated_at=? WHERE id=? AND status='failed'`, now, id)
	if err != nil {
		return fmt.Errorf("retry task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.taskTransitionStatusTx(ctx, tx, id)
	}
	if err := q.appendEvent(ctx, tx, id, StatusQueued, "queued", jsonDetail("retried", true), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retry: %w", err)
	}
	return nil
}

// Recover requeues expired leases and fails tasks that exceed the recovery budget.
// A running task older than the configured deadline is failed rather than requeued.
func (q *Queue) Recover(ctx context.Context) (int, error) {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id, status, recover_count, started_at FROM tasks WHERE status IN ('leased','running') AND lease_expires_at <= ? ORDER BY updated_at, id`, now)
	if err != nil {
		return 0, fmt.Errorf("scan expired tasks: %w", err)
	}
	defer rows.Close()
	type expired struct {
		id string
		status Status
		recoverCount int
		startedAt sql.NullInt64
	}
	var expiredTasks []expired
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.status, &item.recoverCount, &item.startedAt); err != nil {
			return 0, fmt.Errorf("read expired task: %w", err)
		}
		expiredTasks = append(expiredTasks, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read expired tasks: %w", err)
	}
	count := 0
	for _, item := range expiredTasks {
		newCount := item.recoverCount + 1
		deadlineExceeded := item.startedAt.Valid && now-item.startedAt.Int64 >= q.deadline.Milliseconds()
		if newCount >= maxRecoveryCount || deadlineExceeded {
			code, message := "recovery_budget_exhausted", "task recovery budget exhausted"
			if deadlineExceeded {
				code, message = "task_deadline_exceeded", "task deadline exceeded"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status='failed', recover_count=?, lease_owner=NULL, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=NULL, error_code=?, error_message=?, finished_at=?, updated_at=? WHERE id=? AND status IN ('leased','running') AND lease_expires_at<=?`, newCount, code, message, now, now, item.id, now); err != nil {
				return 0, fmt.Errorf("fail recovered task %s: %w", item.id, err)
			}
			if err := q.appendEvent(ctx, tx, item.id, StatusFailed, message, jsonDetail("recover_count", newCount), now); err != nil {
				return 0, err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status='queued', recover_count=?, lease_owner=NULL, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=NULL, message='recovered', updated_at=? WHERE id=? AND status IN ('leased','running') AND lease_expires_at<=?`, newCount, now, item.id, now); err != nil {
				return 0, fmt.Errorf("requeue recovered task %s: %w", item.id, err)
			}
			if err := q.appendEvent(ctx, tx, item.id, StatusQueued, "recovered", jsonDetail("recover_count", newCount), now); err != nil {
				return 0, err
			}
		}
		q.stopRunning(item.id)
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit recovery: %w", err)
	}
	return count, nil
}

// Execution is the fenced capability passed to a domain handler.
type Execution struct {
	queue    *Queue
	Task     *Task
	WorkerID string
	Epoch    int64
}

// Progress records handler progress.
func (e *Execution) Progress(ctx context.Context, progress float64, message string) error {
	return e.queue.Progress(ctx, e.Task.ID, e.WorkerID, e.Epoch, progress, message)
}

// Succeed records successful completion.
func (e *Execution) Succeed(ctx context.Context, message string) error {
	return e.queue.Succeed(ctx, e.Task.ID, e.WorkerID, e.Epoch, message)
}

// Fail records a handler failure and applies retry policy.
func (e *Execution) Fail(ctx context.Context, taskErr *TaskError) error {
	return e.queue.Fail(ctx, e.Task.ID, e.WorkerID, e.Epoch, taskErr)
}

// RunOnce leases and executes one task. It returns ErrNoTaskAvailable when idle.
func (q *Queue) RunOnce(ctx context.Context, workerID string) (*Task, error) {
	task, err := q.Lease(ctx, workerID)
	if err != nil {
		return nil, err
	}
	if err := q.Begin(ctx, task.ID, workerID, task.LeaseEpoch); err != nil {
		return task, err
	}
	handler, ok := q.registry.get(task.Kind)
	if !ok {
		_ = q.finish(ctx, task.ID, workerID, task.LeaseEpoch, StatusFailed, "unknown_task_kind", "no handler registered for task kind", "failed")
		return task, ErrUnknownKind
	}
	workCtx, cancel := context.WithCancel(ctx)
	q.trackRunning(task.ID, cancel)
	defer func() { q.stopRunning(task.ID) }()
	execution := &Execution{queue: q, Task: task, WorkerID: workerID, Epoch: task.LeaseEpoch}
	err = handler.Handle(workCtx, execution)
	if err == nil {
		err = q.Succeed(ctx, task.ID, workerID, task.LeaseEpoch, "succeeded")
		return task, err
	}
	var taskErr *TaskError
	if errors.As(err, &taskErr) {
		return task, q.Fail(ctx, task.ID, workerID, task.LeaseEpoch, taskErr)
	}
	if errors.Is(err, context.Canceled) || errors.Is(workCtx.Err(), context.Canceled) {
		// A concurrent user cancellation already fenced and transitioned the
		// row. Treat that cooperative exit as success from the runner's point
		// of view; the durable task state is already canceled.
		current, readErr := q.Get(ctx, task.ID)
		if readErr == nil && current.Status == StatusCanceled {
			return task, nil
		}
		return task, q.Cancel(ctx, task.ID)
	}
	return task, q.Fail(ctx, task.ID, workerID, task.LeaseEpoch, &TaskError{Code: "task_failed", Message: err.Error(), Retryable: true})
}

func (q *Queue) trackRunning(id string, cancel context.CancelFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.running[id] = cancel
}

func (q *Queue) stopRunning(id string) {
	q.mu.Lock()
	cancel := q.running[id]
	delete(q.running, id)
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (q *Queue) finish(ctx context.Context, id, workerID string, epoch int64, status Status, code, errMessage, message string) error {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status=?, progress=?, message=?, error_code=?, error_message=?, lease_owner=NULL, lease_expires_at=NULL, finished_at=?, updated_at=? WHERE id=? AND status='running' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`, status, map[bool]float64{true: 100, false: 0}[status == StatusSucceeded], message, code, errMessage, now, now, id, workerID, epoch, now)
	if err != nil {
		return fmt.Errorf("finish task: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.fencedResultTx(ctx, tx, id, workerID, epoch)
	}
	if err := q.appendEvent(ctx, tx, id, status, message, jsonDetail("error_code", code), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit finish: %w", err)
	}
	return nil
}

func (q *Queue) failWithRetry(ctx context.Context, id, workerID string, epoch int64, taskErr *TaskError) error {
	now := q.clock.Now().UnixMilli()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin failure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var attempt, maxAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT attempt, max_attempts FROM tasks WHERE id=? AND status='running' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`, id, workerID, epoch, now).Scan(&attempt, &maxAttempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return q.fencedResultTx(ctx, tx, id, workerID, epoch)
		}
		return fmt.Errorf("read failure attempt: %w", err)
	}
	retry := taskErr.Retryable && attempt < maxAttempts
	status := StatusFailed
	message := taskErr.Message
	nextTime := now
	if retry {
		status = StatusQueued
		delay := time.Second * time.Duration(1<<(attempt-1))
		if delay > maxBackoff {
			delay = maxBackoff
		}
		nextTime += delay.Milliseconds()
		message = fmt.Sprintf("retry scheduled in %s: %s", delay, taskErr.Message)
	}
	var query string
	if retry {
		query = `UPDATE tasks SET status='queued', message=?, error_code=?, error_message=?, lease_owner=NULL, lease_epoch=COALESCE(lease_epoch,0)+1, lease_expires_at=NULL, updated_at=? WHERE id=? AND status='running' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`
	} else {
		query = `UPDATE tasks SET status='failed', message=?, error_code=?, error_message=?, lease_owner=NULL, lease_expires_at=NULL, finished_at=?, updated_at=? WHERE id=? AND status='running' AND lease_owner=? AND lease_epoch=? AND lease_expires_at>?`
	}
	var result sql.Result
	if retry {
		result, err = tx.ExecContext(ctx, query, message, taskErr.Code, taskErr.Message, nextTime, id, workerID, epoch, now)
	} else {
		result, err = tx.ExecContext(ctx, query, message, taskErr.Code, taskErr.Message, now, now, id, workerID, epoch, now)
	}
	if err != nil {
		return fmt.Errorf("write task failure: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return q.fencedResultTx(ctx, tx, id, workerID, epoch)
	}
	detail := taskErr.Detail
	if len(detail) == 0 {
		detail = []byte(`{}`)
	}
	if err := q.appendEvent(ctx, tx, id, status, message, detail, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task failure: %w", err)
	}
	return nil
}

func (q *Queue) appendEvent(ctx context.Context, tx *sql.Tx, taskID string, status Status, message string, detail []byte, now int64) error {
	if !json.Valid(detail) {
		detail = []byte(`{"raw_detail":true}`)
	}
	var sequence int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM task_events WHERE task_id=?`, taskID).Scan(&sequence); err != nil {
		return fmt.Errorf("next task event sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_events(id, task_id, sequence, status, message, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, q.id(), taskID, sequence, status, message, string(detail), now); err != nil {
		return fmt.Errorf("insert task event: %w", err)
	}
	return nil
}

func (q *Queue) loadTask(ctx context.Context, queryer interface{ QueryRowContext(context.Context, string, ...any) *sql.Row }, id string) (*Task, error) {
	var task Task
	var packID, payloadPath, errorCode, errorMessage, idem sql.NullString
	var leaseOwner sql.NullString
	var leaseEpoch sql.NullInt64
	var leaseExpires, started, finished sql.NullInt64
	var payload string
	var created, updated int64
	err := queryer.QueryRowContext(ctx, `SELECT id, pack_id, kind, title, status, progress, message, payload, payload_path, error_code, error_message, attempt, max_attempts, recover_count, lease_owner, lease_epoch, lease_expires_at, idempotency_key, created_at, updated_at, started_at, finished_at FROM tasks WHERE id=?`, id).Scan(&task.ID, &packID, &task.Kind, &task.Title, &task.Status, &task.Progress, &task.Message, &payload, &payloadPath, &errorCode, &errorMessage, &task.Attempt, &task.MaxAttempts, &task.RecoverCount, &leaseOwner, &leaseEpoch, &leaseExpires, &idem, &created, &updated, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read task: %w", err)
	}
	if packID.Valid {
		value := packID.String
		task.PackID = &value
	}
	task.Payload = json.RawMessage(payload)
	if payloadPath.Valid { task.PayloadPath = payloadPath.String }
	if errorCode.Valid { task.ErrorCode = errorCode.String }
	if errorMessage.Valid { task.ErrorMessage = errorMessage.String }
	if idem.Valid { task.IdempotencyKey = idem.String }
	if leaseOwner.Valid { task.LeaseOwner = leaseOwner.String }
	if leaseEpoch.Valid { task.LeaseEpoch = leaseEpoch.Int64 }
	if leaseExpires.Valid { t := time.UnixMilli(leaseExpires.Int64); task.LeaseExpiresAt = &t }
	task.CreatedAt, task.UpdatedAt = time.UnixMilli(created), time.UnixMilli(updated)
	if started.Valid { t := time.UnixMilli(started.Int64); task.StartedAt = &t }
	if finished.Valid { t := time.UnixMilli(finished.Int64); task.FinishedAt = &t }
	return &task, nil
}

func (q *Queue) fencedResult(ctx context.Context, id, workerID string, epoch int64) error {
	task, err := q.Get(ctx, id)
	if errors.Is(err, ErrNotFound) { return ErrNotFound }
	if err != nil { return err }
	if task.LeaseOwner != workerID || task.LeaseEpoch != epoch { return ErrLeaseLost }
	return ErrInvalidTransition
}

func (q *Queue) fencedResultTx(ctx context.Context, tx *sql.Tx, id, workerID string, epoch int64) error {
	var owner sql.NullString
	var currentEpoch sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT lease_owner, lease_epoch FROM tasks WHERE id=?`, id).Scan(&owner, &currentEpoch); errors.Is(err, sql.ErrNoRows) { return ErrNotFound } else if err != nil { return err }
	if !owner.Valid || owner.String != workerID || !currentEpoch.Valid || currentEpoch.Int64 != epoch { return ErrLeaseLost }
	return ErrInvalidTransition
}

func (q *Queue) taskTransitionStatus(ctx context.Context, id string) error {
	return q.taskTransitionStatusQuery(ctx, q.db, id)
}

func (q *Queue) taskTransitionStatusTx(ctx context.Context, tx *sql.Tx, id string) error {
	return q.taskTransitionStatusQuery(ctx, tx, id)
}

func (q *Queue) taskTransitionStatusQuery(ctx context.Context, queryer interface{ QueryRowContext(context.Context, string, ...any) *sql.Row }, id string) error {
	var status Status
	if err := queryer.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id=?`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) { return ErrNotFound } else if err != nil { return err }
	if status == StatusSucceeded || status == StatusFailed || status == StatusCanceled { return ErrInvalidTransition }
	return ErrInvalidTransition
}

func validateSubmit(request SubmitRequest) error {
	if !validKind(request.Kind) { return fmt.Errorf("invalid task kind %q", request.Kind) }
	if request.Title == "" { return errors.New("task title is empty") }
	if len(request.IdempotencyKey) > 256 { return errors.New("idempotency key exceeds 256 bytes") }
	if request.MaxAttempts < 0 || request.MaxAttempts > 16 { return errors.New("max attempts must be between 1 and 16") }
	if len(request.Payload) == 0 { request.Payload = []byte(`{}`) }
	if !json.Valid(request.Payload) { return errors.New("task payload is not valid JSON") }
	return nil
}

func validKind(kind Kind) bool {
	switch kind { case KindResolve, KindDownload, KindIndex, KindBuild, KindPublish, KindImport, KindCacheGC: return true }
	return false
}

func canonicalPayload(payload []byte) ([]byte, string, error) {
	if len(payload) == 0 { payload = []byte(`{}`) }
	var value any
	if err := json.Unmarshal(payload, &value); err != nil { return nil, "", fmt.Errorf("decode task payload: %w", err) }
	canonical, err := json.Marshal(value)
	if err != nil { return nil, "", fmt.Errorf("canonicalize task payload: %w", err) }
	hash := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(hash[:]), nil
}

func nullString(value string) any { if value == "" { return nil }; return value }

func jsonDetail(key string, value any) []byte {
	data, err := json.Marshal(map[string]any{key: value})
	if err != nil { return []byte(`{}`) }
	return data
}
