package task

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpackstation/internal/store"
)

// p4TestClock lets lifecycle, lease and recovery behavior be asserted without
// sleeping. The production queue must obtain all time decisions through Clock.
type p4TestClock struct{ now time.Time }

func (c *p4TestClock) Now() time.Time { return c.now }

func (c *p4TestClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestP4RunnerHappyPathIsDurableAndAuditable(t *testing.T) {
	db := p4OpenQueueDB(t)
	defer db.Close()
	clock := &p4TestClock{now: time.UnixMilli(1_000)}
	var sequence atomic.Int64
	q, err := NewQueue(db, WithClock(clock), WithIDGenerator(func() string {
		return "p4-id-" + string(rune('a'+sequence.Add(1)))
	}))
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	ctx := context.Background()
	taskRow, duplicate, err := q.Submit(ctx, SubmitRequest{Kind: KindIndex, Title: "index fixture", Payload: []byte(`{"mod":"jei"}`), MaxAttempts: 1})
	if err != nil || duplicate {
		t.Fatalf("submit task: task=%v duplicate=%v err=%v", taskRow, duplicate, err)
	}
	leased, err := q.Lease(ctx, "worker-a")
	if err != nil {
		t.Fatalf("lease task: %v", err)
	}
	if leased.Status != StatusLeased || leased.LeaseEpoch == 0 {
		t.Fatalf("leased task = status %q epoch %d, want leased with non-zero epoch", leased.Status, leased.LeaseEpoch)
	}
	if err := q.Begin(ctx, leased.ID, "worker-a", leased.LeaseEpoch); err != nil {
		t.Fatalf("begin task: %v", err)
	}
	if err := q.Progress(ctx, leased.ID, "worker-a", leased.LeaseEpoch, 42, "indexed 1/2"); err != nil {
		t.Fatalf("progress task: %v", err)
	}
	if err := q.Succeed(ctx, leased.ID, "worker-a", leased.LeaseEpoch, "succeeded"); err != nil {
		t.Fatalf("succeed task: %v", err)
	}
	final, err := q.Get(ctx, leased.ID)
	if err != nil {
		t.Fatalf("read final task: %v", err)
	}
	if final.Status != StatusSucceeded || final.Progress != 100 || final.LeaseOwner != "" || final.LeaseExpiresAt != nil {
		t.Fatalf("final task = status=%q progress=%v owner=%q expiry=%v; terminal task must clear lease", final.Status, final.Progress, final.LeaseOwner, final.LeaseExpiresAt)
	}
	events, err := q.Events(ctx, leased.ID)
	if err != nil {
		t.Fatalf("read task events: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("event count=%d, want queued,leased,running,progress,succeeded lifecycle evidence", len(events))
	}
	for index, event := range events {
		if event.Sequence != index+1 {
			t.Errorf("event[%d] sequence=%d, want %d", index, event.Sequence, index+1)
		}
	}
}

func TestP4FencingRejectsStaleWorkerAndWrongOwner(t *testing.T) {
	db := p4OpenQueueDB(t)
	defer db.Close()
	clock := &p4TestClock{now: time.UnixMilli(1_000)}
	q, err := NewQueue(db, WithClock(clock), WithIDGenerator(p4IDs()))
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	ctx := context.Background()
	row, _, err := q.Submit(ctx, SubmitRequest{Kind: KindDownload, Title: "fenced fixture"})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	leased, err := q.Lease(ctx, "worker-a")
	if err != nil {
		t.Fatalf("lease task: %v", err)
	}
	staleCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err = q.Begin(staleCtx, row.ID, "worker-a", leased.LeaseEpoch-1)
	cancel()
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale begin error=%v, want ErrLeaseLost", err)
	}
	if err := q.Begin(ctx, row.ID, "worker-a", leased.LeaseEpoch); err != nil {
		t.Fatalf("valid begin: %v", err)
	}
	if err := q.Heartbeat(ctx, row.ID, "worker-b", leased.LeaseEpoch); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong-owner heartbeat error=%v, want ErrLeaseLost", err)
	}
	if err := q.Progress(ctx, row.ID, "worker-a", leased.LeaseEpoch-1, 90, "stale"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale progress error=%v, want ErrLeaseLost", err)
	}
	if err := q.Progress(ctx, row.ID, "worker-a", leased.LeaseEpoch, 90, "valid"); err != nil {
		t.Fatalf("valid progress: %v", err)
	}
}

func TestP4IdempotencyCanonicalizesPayloadAndRejectsDifferentPayload(t *testing.T) {
	db := p4OpenQueueDB(t)
	defer db.Close()
	q, err := NewQueue(db, WithClock(&p4TestClock{now: time.UnixMilli(1_000)}), WithIDGenerator(p4IDs()))
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	ctx := context.Background()
	first, duplicate, err := q.Submit(ctx, SubmitRequest{Kind: KindResolve, Title: "resolve", Payload: []byte(`{"a":1,"b":[true]}`), IdempotencyKey: "p4-idem"})
	if err != nil || duplicate {
		t.Fatalf("first submit: duplicate=%v err=%v", duplicate, err)
	}
	second, duplicate, err := q.Submit(ctx, SubmitRequest{Kind: KindResolve, Title: "resolve", Payload: []byte(` { "b" : [ true ], "a": 1 } `), IdempotencyKey: "p4-idem"})
	if err != nil || !duplicate || second.ID != first.ID {
		t.Fatalf("canonical duplicate: task=%v duplicate=%v err=%v; want same task and duplicate=true", second, duplicate, err)
	}
	if _, _, err := q.Submit(ctx, SubmitRequest{Kind: KindResolve, Title: "resolve", Payload: []byte(`{"a":2}`), IdempotencyKey: "p4-idem"}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different payload error=%v, want ErrIdempotencyConflict", err)
	}
}

func TestP4ConcurrentSameKeyCreatesOneLogicalTask(t *testing.T) {
	db := p4OpenQueueDB(t)
	defer db.Close()
	var ids atomic.Int64
	q, err := NewQueue(db, WithClock(&p4TestClock{now: time.UnixMilli(1_000)}), WithIDGenerator(func() string { return "concurrent-" + string(rune('a'+ids.Add(1))) }))
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan struct {
		id  string
		dup bool
		err error
	}, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, duplicate, submitErr := q.Submit(ctx, SubmitRequest{Kind: KindIndex, Title: "concurrent", Payload: []byte(`{"same":true}`), IdempotencyKey: "p4-concurrent"})
			if row == nil {
				results <- struct {
					id  string
					dup bool
					err error
				}{dup: duplicate, err: submitErr}
				return
			}
			results <- struct {
				id  string
				dup bool
				err error
			}{id: row.ID, dup: duplicate, err: submitErr}
		}()
	}
	wg.Wait()
	close(results)
	var idsSeen []string
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent submit error: %v", result.err)
		}
		idsSeen = append(idsSeen, result.id)
	}
	if len(idsSeen) != 2 || idsSeen[0] == "" || idsSeen[0] != idsSeen[1] {
		t.Fatalf("concurrent submit task IDs=%v; want one logical task", idsSeen)
	}
	rows, err := q.List(ctx, 100)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("durable task count=%d, want 1", len(rows))
	}
}

func TestP4CancelPauseResumeRetryAndRecoveryAreExplicitTransitions(t *testing.T) {
	db := p4OpenQueueDB(t)
	defer db.Close()
	clock := &p4TestClock{now: time.UnixMilli(1_000)}
	q, err := NewQueue(db, WithClock(clock), WithIDGenerator(p4IDs()), WithLeaseTTL(time.Second), WithTaskDeadline(time.Hour))
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	ctx := context.Background()
	row, _, err := q.Submit(ctx, SubmitRequest{Kind: KindBuild, Title: "pause fixture", MaxAttempts: 1})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	leased, err := q.Lease(ctx, "worker-a")
	if err != nil {
		t.Fatalf("lease task: %v", err)
	}
	if err := q.Begin(ctx, row.ID, "worker-a", leased.LeaseEpoch); err != nil {
		t.Fatalf("begin task: %v", err)
	}
	if err := q.Pause(ctx, row.ID, "worker-a", leased.LeaseEpoch); err != nil {
		t.Fatalf("pause task: %v", err)
	}
	paused, err := q.Get(ctx, row.ID)
	if err != nil || paused.Status != StatusPaused {
		t.Fatalf("paused task=%v err=%v, want paused", paused, err)
	}
	if err := q.Resume(ctx, row.ID); err != nil {
		t.Fatalf("resume task: %v", err)
	}
	if err := q.Cancel(ctx, row.ID); err != nil {
		t.Fatalf("cancel queued task: %v", err)
	}
	canceled, err := q.Get(ctx, row.ID)
	if err != nil || canceled.Status != StatusCanceled {
		t.Fatalf("canceled task=%v err=%v, want canceled", canceled, err)
	}
	if err := q.Retry(ctx, row.ID); err != nil {
		t.Fatalf("explicit retry canceled task: %v", err)
	}
	retried, err := q.Get(ctx, row.ID)
	if err != nil || retried.Status != StatusQueued {
		t.Fatalf("retried task=%v err=%v, want queued", retried, err)
	}

	recoveryRow, _, err := q.Submit(ctx, SubmitRequest{Kind: KindImport, Title: "recovery fixture"})
	if err != nil {
		t.Fatalf("submit recovery task: %v", err)
	}
	recoveryLease, err := q.Lease(ctx, "worker-recovery")
	if err != nil {
		t.Fatalf("lease recovery task: %v", err)
	}
	clock.Advance(2 * time.Second)
	recovered, err := q.Recover(ctx)
	if err != nil {
		t.Fatalf("recover task: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered count=%d, want 1", recovered)
	}
	requeued, err := q.Get(ctx, recoveryRow.ID)
	if err != nil || requeued.Status != StatusQueued || requeued.RecoverCount != 1 {
		t.Fatalf("requeued task=%v err=%v; want queued recover_count=1", requeued, err)
	}
	if err := q.Progress(ctx, recoveryRow.ID, "worker-recovery", recoveryLease.LeaseEpoch, 80, "stale"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old worker progress error=%v, want ErrLeaseLost", err)
	}
}

func TestP4RunOnceUsesRegisteredHandlerAndLeavesTerminalState(t *testing.T) {
	db := p4OpenQueueDB(t)
	defer db.Close()
	clock := &p4TestClock{now: time.UnixMilli(1_000)}
	registry := NewRegistry()
	if err := registry.Register(KindIndex, HandlerFunc(func(ctx context.Context, execution *Execution) error {
		if err := execution.Progress(ctx, 50, "halfway"); err != nil {
			return err
		}
		return nil
	})); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	q, err := NewQueue(db, WithClock(clock), WithRegistry(registry), WithIDGenerator(p4IDs()))
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	if _, _, err := q.Submit(context.Background(), SubmitRequest{Kind: KindIndex, Title: "run once"}); err != nil {
		t.Fatalf("submit task: %v", err)
	}
	row, err := q.RunOnce(context.Background(), "worker-run")
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	final, err := q.Get(context.Background(), row.ID)
	if err != nil || final.Status != StatusSucceeded || final.Progress != 100 {
		t.Fatalf("run once final=%v err=%v; want succeeded at 100%%", final, err)
	}
}

func p4OpenQueueDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "p4-runner.db"))
	if err != nil {
		t.Fatalf("open task database: %v", err)
	}
	return db
}

func p4IDs() IDGenerator {
	var sequence atomic.Int64
	return func() string { return "p4-id-" + string(rune('a'+sequence.Add(1))) }
}
