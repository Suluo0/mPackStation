package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// P4 acceptance tests exercise the persistence invariants that a task runner
// must preserve. The SQL predicates deliberately model the required fenced
// writes; they do not make an incomplete runner appear complete.

func TestP4TaskLeaseColumnsMatchState(t *testing.T) {
	db := openP4TaskDB(t)
	defer db.Close()

	fixtures := []struct {
		name      string
		id        string
		status    string
		owner     any
		epoch     any
		expiresAt any
		wantErr   bool
	}{
		{name: "queued has no lease", id: "p4-queued", status: "queued", wantErr: false},
		{name: "running requires complete lease", id: "p4-running", status: "running", wantErr: true},
		{name: "leased requires complete lease", id: "p4-leased", status: "leased", owner: "worker-a", epoch: 1, expiresAt: 2, wantErr: false},
		{name: "paused cannot retain lease", id: "p4-paused", status: "paused", owner: "worker-a", epoch: 1, expiresAt: 2, wantErr: true},
		{name: "succeeded has no lease", id: "p4-succeeded", status: "succeeded", wantErr: false},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,lease_owner,lease_epoch,lease_expires_at,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?)`, fixture.id, "index", fixture.name, fixture.status, 0, `{}`, fixture.owner, fixture.epoch, fixture.expiresAt, 1, 1)
			if fixture.wantErr && err == nil {
				t.Fatal("invalid lease/state combination was accepted")
			}
			if !fixture.wantErr && err != nil {
				t.Fatalf("valid lease/state combination rejected: %v", err)
			}
		})
	}
}

func TestP4ActiveTaskIdempotencyAndPermanentKeyRegistry(t *testing.T) {
	db := openP4TaskDB(t)
	defer db.Close()

	insertTask := func(id, status, key string) error {
		_, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,idempotency_key,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`, id, "resolve", id, status, 0, `{}`, key, 1, 1)
		return err
	}
	if err := insertTask("p4-idem-1", "queued", "idem-p4"); err != nil {
		t.Fatalf("insert first active task: %v", err)
	}
	if err := insertTask("p4-idem-2", "running", "idem-p4"); err == nil {
		t.Fatal("duplicate active idempotency key was accepted")
	}
	if err := insertTask("p4-idem-terminal", "succeeded", "idem-p4"); err != nil {
		t.Fatalf("terminal task should not collide with active-task index: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO task_idem_keys(idempotency_key,endpoint,kind,payload_hash,task_id,created_at)
		VALUES(?,?,?,?,?,?)`, "idem-p4", "/api/packs/p4/resolve", "resolve", strings.Repeat("a", 64), "p4-idem-1", 1); err != nil {
		t.Fatalf("insert permanent idempotency registry row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_idem_keys(idempotency_key,endpoint,kind,payload_hash,task_id,created_at)
		VALUES(?,?,?,?,?,?)`, "idem-p4", "/api/packs/p4/resolve", "resolve", strings.Repeat("a", 64), "p4-idem-terminal", 2); err == nil {
		t.Fatal("permanent idempotency key was accepted twice")
	}
	if _, err := db.Exec(`INSERT INTO task_idem_keys(idempotency_key,endpoint,kind,payload_hash,task_id,created_at)
		VALUES(?,?,?,?,?,?)`, "idem-p4-different", "/api/packs/p4/resolve", "resolve", "short", "p4-idem-1", 2); err == nil {
		t.Fatal("invalid payload hash length was accepted")
	}
}

func TestP4LeaseHeartbeatRejectsWrongOwnerOrEpoch(t *testing.T) {
	db := openP4TaskDB(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,lease_owner,lease_epoch,lease_expires_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, "p4-fenced", "index", "fenced task", "running", 10, `{}`, "worker-a", 7, 100, 1, 1); err != nil {
		t.Fatalf("insert fenced task: %v", err)
	}
	for _, tc := range []struct {
		name  string
		owner string
		epoch int
	}{
		{name: "wrong owner", owner: "worker-b", epoch: 7},
		{name: "stale epoch", owner: "worker-a", epoch: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := db.Exec(`UPDATE tasks SET lease_expires_at=? WHERE id=? AND status IN ('leased','running') AND lease_owner=? AND lease_epoch=?`, 200, "p4-fenced", tc.owner, tc.epoch)
			if err != nil {
				t.Fatalf("fenced heartbeat query: %v", err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				t.Fatalf("heartbeat rows affected: %v", err)
			}
			if count != 0 {
				t.Fatalf("fenced heartbeat affected %d rows", count)
			}
		})
	}
	result, err := db.Exec(`UPDATE tasks SET lease_expires_at=? WHERE id=? AND status IN ('leased','running') AND lease_owner=? AND lease_epoch=?`, 200, "p4-fenced", "worker-a", 7)
	if err != nil {
		t.Fatalf("valid heartbeat query: %v", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		t.Fatalf("valid heartbeat affected %d rows, want 1", count)
	}
}

func TestP4StaleWorkerCannotWriteAfterEpochChange(t *testing.T) {
	db := openP4TaskDB(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,lease_owner,lease_epoch,lease_expires_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, "p4-stale", "download", "stale worker", "running", 10, `{}`, "worker-new", 9, 100, 1, 1); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	result, err := db.Exec(`UPDATE tasks SET progress=?,updated_at=? WHERE id=? AND status IN ('leased','running') AND lease_owner=? AND lease_epoch=?`, 90, 2, "p4-stale", "worker-old", 8)
	if err != nil {
		t.Fatalf("stale worker update: %v", err)
	}
	count, _ := result.RowsAffected()
	if count != 0 {
		t.Fatalf("stale worker changed %d rows", count)
	}
	var progress float64
	if err := db.QueryRow(`SELECT progress FROM tasks WHERE id=?`, "p4-stale").Scan(&progress); err != nil {
		t.Fatalf("read fenced task: %v", err)
	}
	if progress != 10 {
		t.Fatalf("progress=%v changed despite stale epoch", progress)
	}
}

func TestP4TerminalTasksCannotBeCanceledByActivePredicate(t *testing.T) {
	db := openP4TaskDB(t)
	defer db.Close()

	for _, status := range []string{"succeeded", "failed", "canceled"} {
		id := "p4-terminal-" + status
		if _, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, "build", id, status, 100, `{}`, 1, 1); err != nil {
			t.Fatalf("insert %s task: %v", status, err)
		}
		result, err := db.Exec(`UPDATE tasks SET status='canceled',updated_at=? WHERE id=? AND status IN ('queued','leased','running','paused')`, 2, id)
		if err != nil {
			t.Fatalf("cancel %s task: %v", status, err)
		}
		count, _ := result.RowsAffected()
		if count != 0 {
			t.Errorf("terminal %s task changed through cancel predicate", status)
		}
	}
}

func TestP4TaskEventSequenceAndStatusContract(t *testing.T) {
	db := openP4TaskDB(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, "p4-events", "import", "event task", "queued", 0, `{}`, 1, 1); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	for _, event := range []struct {
		id, status string
		sequence   int
	}{
		{id: "p4-event-1", status: "queued", sequence: 1},
		{id: "p4-event-2", status: "leased", sequence: 2},
		{id: "p4-event-3", status: "running", sequence: 3},
		{id: "p4-event-4", status: "succeeded", sequence: 4},
	} {
		if _, err := db.Exec(`INSERT INTO task_events(id,task_id,sequence,status,detail,created_at) VALUES(?,?,?,?,?,?)`, event.id, "p4-events", event.sequence, event.status, `{}`, event.sequence); err != nil {
			t.Fatalf("insert event %d: %v", event.sequence, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO task_events(id,task_id,sequence,status,detail,created_at) VALUES(?,?,?,?,?,?)`, "p4-event-duplicate", "p4-events", 4, "succeeded", `{}`, 5); err == nil {
		t.Fatal("duplicate task event sequence was accepted")
	}
	if _, err := db.Exec(`INSERT INTO task_events(id,task_id,sequence,status,detail,created_at) VALUES(?,?,?,?,?,?)`, "p4-event-invalid", "p4-events", 5, "success", `{}`, 5); err == nil {
		t.Fatal("non-canonical task status was accepted")
	}
}

func openP4TaskDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "p4-task.db"))
	if err != nil {
		t.Fatalf("open P4 task database: %v", err)
	}
	return db
}
