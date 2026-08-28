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
		method string
		path   string
	}{
		{http.MethodGet, "/api/tasks/p4-missing"},
		{http.MethodPost, "/api/tasks/p4-missing/pause"},
		{http.MethodPost, "/api/tasks/p4-missing/resume"},
		{http.MethodPost, "/api/tasks/p4-missing/cancel"},
		{http.MethodPost, "/api/tasks/p4-missing/retry"},
		{http.MethodGet, "/api/tasks/p4-missing/log"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			res := p3Do(t, handler, route.method, route.path, nil)
			if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
				t.Fatalf("P4-HTTP-001 missing task route: %s %s (status=%d)", route.method, route.path, res.Code)
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
