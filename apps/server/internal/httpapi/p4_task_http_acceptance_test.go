package httpapi

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	"mpackstation/internal/store"
)

// P4's HTTP surface is kept separate from P3's dashboard gate: task control
// routes are part of the durable task state machine, not a dashboard-only
// concern.
func TestP4TaskControlRoutesAreBackedByHandlers(t *testing.T) {
	handler, db := p4HTTPRouter(t)
	defer db.Close()

	for _, route := range []struct {
		method       string
		path         string
		notFoundCode string
	}{
		{http.MethodGet, "/api/tasks/p4-missing", "task_not_found"},
		{http.MethodPost, "/api/tasks/p4-missing/pause", "task_not_found"},
		{http.MethodPost, "/api/tasks/p4-missing/resume", "task_not_found"},
		{http.MethodPost, "/api/tasks/p4-missing/cancel", "task_not_found"},
		{http.MethodPost, "/api/tasks/p4-missing/retry", "task_not_found"},
		{http.MethodGet, "/api/tasks/p4-missing/log", "task_not_found"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			res := p3Do(t, handler, route.method, route.path, nil)
			if res.Code == http.StatusMethodNotAllowed {
				t.Fatalf("P4-HTTP-001 missing task route: %s %s (status=%d)", route.method, route.path, res.Code)
			}
			if res.Code == http.StatusNotFound {
				p3RequireError(t, res, route.notFoundCode)
			}
		})
	}
}

func TestP4TerminalTaskControlReturnsStableTransitionError(t *testing.T) {
	handler, db := p4HTTPRouter(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO tasks(id,kind,title,status,progress,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		"p4-terminal-http", "build", "terminal P4 fixture", "succeeded", 100, `{}`, 1, 1); err != nil {
		t.Fatalf("seed terminal task: %v", err)
	}
	res := p3Do(t, handler, http.MethodPost, "/api/tasks/p4-terminal-http/cancel", nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("P4-HTTP-002 terminal cancel status=%d body=%s", res.Code, res.Body.String())
	}
	p3RequireError(t, res, "task_invalid_transition")
}

func p4HTTPRouter(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "p4-http.db"))
	if err != nil {
		t.Fatalf("open P4 HTTP database: %v", err)
	}
	return NewRouter(db, "test"), db
}
