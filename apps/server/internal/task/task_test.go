package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestTaskStateMachineAndFencing is a human-readable acceptance case:
// submit -> lease -> begin -> progress -> cancel, followed by a stale-worker
// write. The stale write must be rejected and the terminal state preserved.
func TestTaskStateMachineAndFencing(t *testing.T) {
	db := openTaskTestDB(t)
	clock := &fakeClock{now: time.UnixMilli(1000)}
	ids := sequenceIDs()
	queue, err := NewQueue(db, WithClock(clock.NowClock()), WithIDGenerator(ids), WithLeaseTTL(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := queue.Submit(context.Background(), SubmitRequest{Kind: KindDownload, Title: "下载测试", Payload: []byte(`{"b":2,"a":1}`), IdempotencyKey: "case-1"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	leased, err := queue.Lease(context.Background(), "worker-a")
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased.ID != task.ID || leased.Status != StatusLeased || leased.LeaseEpoch != 1 || leased.Attempt != 1 {
		t.Fatalf("lease result = %#v", leased)
	}
	if err := queue.Begin(context.Background(), task.ID, "worker-a", leased.LeaseEpoch); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := queue.Progress(context.Background(), task.ID, "worker-a", leased.LeaseEpoch, 35, "已下载 35%%"); err != nil {
		t.Fatalf("progress: %v", err)
	}
	if err := queue.Heartbeat(context.Background(), task.ID, "worker-b", leased.LeaseEpoch); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong worker heartbeat = %v, want ErrLeaseLost", err)
	}
	if err := queue.Cancel(context.Background(), task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := queue.Succeed(context.Background(), task.ID, "worker-a", leased.LeaseEpoch, "late success"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker completion = %v, want ErrLeaseLost", err)
	}
	current, err := queue.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != StatusCanceled || current.LeaseOwner != "" || current.LeaseEpoch != 2 {
		t.Fatalf("canceled task = %#v", current)
	}
	events, err := queue.Events(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[0].Sequence != 1 || events[len(events)-1].Status != StatusCanceled {
		t.Fatalf("task events = %#v", events)
	}
}

// TestTaskIdempotencyCanonicalPayload demonstrates that equivalent JSON binds
// to one task, while reusing a key for a different payload is rejected.
func TestTaskIdempotencyCanonicalPayload(t *testing.T) {
	db := openTaskTestDB(t)
	queue, err := NewQueue(db, WithIDGenerator(sequenceIDs()))
	if err != nil {
		t.Fatal(err)
	}
	first, reused, err := queue.Submit(context.Background(), SubmitRequest{Kind: KindResolve, Title: "解析", Payload: []byte(" { \"b\": 2, \"a\": 1 } "), IdempotencyKey: "same-key"})
	if err != nil || reused {
		t.Fatalf("first submit = task %v reused %v err %v", first, reused, err)
	}
	second, reused, err := queue.Submit(context.Background(), SubmitRequest{Kind: KindResolve, Title: "解析", Payload: []byte(`{"a":1,"b":2}`), IdempotencyKey: "same-key"})
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("equivalent submit = task %v reused %v err %v", second, reused, err)
	}
	_, _, err = queue.Submit(context.Background(), SubmitRequest{Kind: KindResolve, Title: "解析", Payload: []byte(`{"a":2}`), IdempotencyKey: "same-key"})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different payload error = %v, want ErrIdempotencyConflict", err)
	}
}

// TestTaskRetryBackoffAndRecovery documents retry delay and crash recovery:
// retryable failure is requeued with backoff; an expired lease is requeued and
// fenced; after ten recovery failures the task is terminally failed.
func TestTaskRetryBackoffAndRecovery(t *testing.T) {
	db := openTaskTestDB(t)
	clock := &fakeClock{now: time.UnixMilli(10_000)}
	queue, err := NewQueue(db, WithClock(clock.NowClock()), WithIDGenerator(sequenceIDs()), WithLeaseTTL(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := queue.Submit(context.Background(), SubmitRequest{Kind: KindIndex, Title: "索引", MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := queue.Lease(context.Background(), "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Begin(context.Background(), item.ID, "worker-a", lease.LeaseEpoch); err != nil {
		t.Fatal(err)
	}
	if err := queue.Fail(context.Background(), item.ID, "worker-a", lease.LeaseEpoch, &TaskError{Code: "upstream_timeout", Message: "上游超时", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	current, _ := queue.Get(context.Background(), item.ID)
	if current.Status != StatusQueued || current.Attempt != 1 || current.ErrorCode != "upstream_timeout" {
		t.Fatalf("retry state = %#v", current)
	}
	if _, err := queue.Lease(context.Background(), "worker-b"); !errors.Is(err, ErrNoTaskAvailable) {
		t.Fatalf("lease before backoff = %v, want ErrNoTaskAvailable", err)
	}
	clock.Advance(time.Second)
	lease, err = queue.Lease(context.Background(), "worker-b")
	if err != nil {
		t.Fatalf("lease after backoff: %v", err)
	}
	if lease.LeaseEpoch != 2 {
		t.Fatalf("second epoch = %d, want 2", lease.LeaseEpoch)
	}
	if err := queue.Begin(context.Background(), item.ID, "worker-b", lease.LeaseEpoch); err != nil {
		t.Fatal(err)
	}
	if err := queue.Fail(context.Background(), item.ID, "worker-b", lease.LeaseEpoch, &TaskError{Code: "permanent", Message: "最终失败"}); err != nil {
		t.Fatal(err)
	}
	current, _ = queue.Get(context.Background(), item.ID)
	if current.Status != StatusFailed || current.FinishedAt == nil {
		t.Fatalf("terminal retry state = %#v", current)
	}
	if err := queue.Retry(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
	lease, err = queue.Lease(context.Background(), "worker-c")
	if err != nil {
		t.Fatalf("manual retry lease: %v", err)
	}
	if err := queue.Begin(context.Background(), item.ID, "worker-c", lease.LeaseEpoch); err != nil {
		t.Fatal(err)
	}
	oldEpoch := lease.LeaseEpoch
	clock.Advance(31 * time.Second)
	if recovered, err := queue.Recover(context.Background()); err != nil || recovered != 1 {
		t.Fatalf("recover count=%d err=%v", recovered, err)
	}
	if err := queue.Heartbeat(context.Background(), item.ID, "worker-c", oldEpoch); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("recovered stale heartbeat = %v, want ErrLeaseLost", err)
	}
	current, _ = queue.Get(context.Background(), item.ID)
	if current.Status != StatusQueued || current.RecoverCount != 1 {
		t.Fatalf("recovered state = %#v", current)
	}
}

// TestTaskRegistryRunner proves the queue invokes only a registered handler
// and persists the terminal success event without domain-specific code.
func TestTaskRegistryRunner(t *testing.T) {
	db := openTaskTestDB(t)
	registry := NewRegistry()
	called := false
	if err := registry.Register(KindBuild, HandlerFunc(func(ctx context.Context, execution *Execution) error {
		called = true
		return execution.Progress(ctx, 50, "构建中")
	})); err != nil {
		t.Fatal(err)
	}
	queue, _ := NewQueue(db, WithRegistry(registry), WithIDGenerator(sequenceIDs()))
	item, _, err := queue.Submit(context.Background(), SubmitRequest{Kind: KindBuild, Title: "构建"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.RunOnce(context.Background(), "worker-a"); err != nil {
		t.Fatalf("run once: %v", err)
	}
	current, _ := queue.Get(context.Background(), item.ID)
	if !called || current.Status != StatusSucceeded || current.Progress != 100 {
		t.Fatalf("runner result called=%v task=%#v", called, current)
	}
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) NowClock() Clock { return c }
func (c *fakeClock) Advance(duration time.Duration) { c.now = c.now.Add(duration) }

func sequenceIDs() IDGenerator {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("id-%03d", n)
	}
}

func openTaskTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "task.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.Exec(`
CREATE TABLE tasks (
 id TEXT PRIMARY KEY, pack_id TEXT, kind TEXT NOT NULL,
 title TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'queued',
 progress REAL NOT NULL DEFAULT 0 CHECK(progress >= 0 AND progress <= 100),
 message TEXT NOT NULL DEFAULT '', payload TEXT NOT NULL DEFAULT '{}',
 payload_path TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 error_message TEXT NOT NULL DEFAULT '', attempt INTEGER NOT NULL DEFAULT 0,
 max_attempts INTEGER NOT NULL DEFAULT 3, recover_count INTEGER NOT NULL DEFAULT 0,
 lease_owner TEXT, lease_epoch INTEGER, lease_expires_at INTEGER,
 idempotency_key TEXT, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 started_at INTEGER, finished_at INTEGER
);
CREATE TABLE task_events (
 id TEXT PRIMARY KEY, task_id TEXT, sequence INTEGER NOT NULL,
 status TEXT NOT NULL, message TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '{}',
 created_at INTEGER NOT NULL, UNIQUE(task_id, sequence)
);
CREATE TABLE task_idem_keys (
 idempotency_key TEXT PRIMARY KEY, endpoint TEXT NOT NULL, kind TEXT NOT NULL,
 payload_hash TEXT NOT NULL, task_id TEXT, created_at INTEGER NOT NULL
);`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
