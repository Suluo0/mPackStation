package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mpackstation/internal/service"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

// TestHealthIdentityExposesDataDir pins the endpoint that startup scripts use to
// tell "the instance I just started" from "a different server already on this
// port". See issue-chain-test-port-collision: chain-test-run.sh once assumed any
// 200 on /api/health meant its own backend, so a collision routed 173 write
// cases into the development database.
func TestHealthIdentityExposesDataDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "identity.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	queue, err := task.NewQueue(db)
	if err != nil {
		t.Fatal(err)
	}
	app := service.New(db)
	app.SetTaskQueue(queue)
	router := NewRouterWithService(app, "test")

	request := httptest.NewRequest(http.MethodGet, "/api/health/identity", nil)
	request.Host = "localhost"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("identity = %d, want 200: %s", response.Code, response.Body.String())
	}
	var got struct {
		DataDir string `json:"dataDir"`
		PID     int    `json:"pid"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("identity body is not the agreed shape: %v (%s)", err, response.Body.String())
	}
	if got.DataDir == "" {
		t.Fatal("dataDir must be non-empty: scripts compare it to detect a foreign instance")
	}
	if got.PID == 0 {
		t.Fatal("pid must be non-zero so a stale-port diagnosis is possible")
	}
}

// The identity endpoint is read-only and leaks no secret (the data directory is
// already implied by the port the caller reached), so it must work without a
// token — the pre-flight check runs before any credential is in play.
func TestHealthIdentityNeedsNoToken(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "identity-notoken.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	queue, err := task.NewQueue(db)
	if err != nil {
		t.Fatal(err)
	}
	app := service.New(db)
	app.SetTaskQueue(queue)
	router := NewRouterWithService(app, "test")

	request := httptest.NewRequest(http.MethodGet, "/api/health/identity", nil)
	request.Host = "localhost"
	// Deliberately no X-MPack-Token header.
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("identity without token = %d, want 200", response.Code)
	}
}
