package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mpackstation/internal/store"
)

// These tests are the P3 acceptance gate. They intentionally exercise the
// public HTTP contract rather than the current implementation details. A
// missing route, a 200 response with the wrong shape, or a missing stable
// error envelope is a failing acceptance result.

func TestP3FrontendRoutesAreBackedByHandlers(t *testing.T) {
	handler, db := p3Router(t)
	defer db.Close()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/dashboard"},
		{http.MethodGet, "/api/tasks?recent=20"},
		{http.MethodGet, "/api/activities?limit=10"},
		{http.MethodGet, "/api/system/health"},
		{http.MethodGet, "/api/system/status"},
		{http.MethodGet, "/api/onboarding"},
		{http.MethodPut, "/api/onboarding"},
		{http.MethodGet, "/api/meta/mc-versions"},
		{http.MethodGet, "/api/packs"},
		{http.MethodPost, "/api/packs"},
		{http.MethodGet, "/api/packs/p3-missing"},
		{http.MethodPatch, "/api/packs/p3-missing"},
		{http.MethodPost, "/api/packs/p3-missing/duplicate"},
		{http.MethodPost, "/api/packs/p3-missing/archive"},
		{http.MethodPost, "/api/packs/p3-missing/unarchive"},
		{http.MethodDelete, "/api/packs/p3-missing"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := p3Request(t, route.method, route.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
				t.Fatalf("P3-HTTP-001 missing route: %s %s (status=%d)", route.method, route.path, res.Code)
			}
		})
	}
}

func TestP3DashboardResponseMatchesFrontendContract(t *testing.T) {
	handler, db := p3Router(t)
	defer db.Close()

	res := p3Do(t, handler, http.MethodGet, "/api/dashboard", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("P3-HTTP-002 dashboard status=%d body=%s", res.Code, res.Body.String())
	}
	var body map[string]json.RawMessage
	p3DecodeJSON(t, res, &body)
	p3RequireKeys(t, body, "packs", "lastEditedPackId", "todayResolvedCount")
	var packs []map[string]json.RawMessage
	p3Unmarshal(t, body["packs"], &packs)
	var lastEdited any
	p3Unmarshal(t, body["lastEditedPackId"], &lastEdited)
	var today int
	p3Unmarshal(t, body["todayResolvedCount"], &today)
	if today < 0 {
		t.Fatalf("P3-HTTP-002 todayResolvedCount=%d; aggregate counts cannot be negative", today)
	}
	for i, pack := range packs {
		p3RequireKeys(t, pack, "id", "name", "iconUrl", "mcVersion", "loader", "packVersion", "modCount", "conflicts", "edits", "alerts", "lastEditedAt", "createdAt")
		for _, nested := range []string{"modCount", "conflicts", "edits", "alerts"} {
			var object map[string]json.RawMessage
			p3Unmarshal(t, pack[nested], &object)
			if len(object) == 0 {
				t.Fatalf("P3-HTTP-002 packs[%d].%s must be an object", i, nested)
			}
		}
	}
	_ = lastEdited
}

func TestP3ListContractsUseStableArraysAndPaginationInputs(t *testing.T) {
	handler, db := p3Router(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO packs(id,name,mc_version,loader,loader_version,created_at,updated_at,last_edited_at) VALUES(?,?,?,?,?,?,?,?)`,
		"p3-pack", "P3 fixture", "1.20.1", "fabric", "0.15", 1, 2, 2); err != nil {
		t.Fatalf("seed pack: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks(id,pack_id,kind,title,status,progress,payload,created_at,updated_at,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		"p3-task", "p3-pack", "build", "P3 build", "succeeded", 100, `{}`, 1, 2, 1, 2); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO activities(id,pack_id,kind,action,text,created_at) VALUES(?,?,?,?,?,?)`,
		"p3-activity", "p3-pack", "mod", "add", "向「P3 fixture」添加了 JEI", 2); err != nil {
		t.Fatalf("seed activity: %v", err)
	}

	res := p3Do(t, handler, http.MethodGet, "/api/tasks?recent=20", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("P3-HTTP-003 tasks status=%d body=%s", res.Code, res.Body.String())
	}
	var tasks []map[string]json.RawMessage
	p3DecodeJSON(t, res, &tasks)
	for i, task := range tasks {
		p3RequireKeys(t, task, "id", "type", "title", "packId", "packName", "status", "progress", "error", "startedAt", "finishedAt")
		var progress float64
		p3Unmarshal(t, task["progress"], &progress)
		if progress < 0 || progress > 100 {
			t.Fatalf("P3-HTTP-003 tasks[%d].progress=%v is outside [0,100]", i, progress)
		}
	}

	res = p3Do(t, handler, http.MethodGet, "/api/activities?limit=10", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("P3-HTTP-003 activities status=%d body=%s", res.Code, res.Body.String())
	}
	var activities []map[string]json.RawMessage
	p3DecodeJSON(t, res, &activities)
	for i, activity := range activities {
		p3RequireKeys(t, activity, "id", "kind", "text", "packId", "at")
		var text string
		p3Unmarshal(t, activity["text"], &text)
		if text == "" {
			t.Fatalf("P3-HTTP-003 activities[%d].text must be non-empty", i)
		}
	}

	// A malformed cursor/filter must be a stable client error, not silently
	// return the first page with a different query semantics.
	res = p3Do(t, handler, http.MethodGet, "/api/tasks?limit=0", nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("P3-HTTP-003 invalid pagination status=%d body=%s", res.Code, res.Body.String())
	}
	p3RequireError(t, res, "invalid_argument")
}

func TestP3CreatePackSuccessAndValidationError(t *testing.T) {
	handler, db := p3Router(t)
	defer db.Close()

	body := []byte(`{"name":"P3 created","mcVersion":"1.20.1","loader":"fabric","description":"contract fixture"}`)
	req := p3Request(t, http.MethodPost, "/api/packs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "p3-create-success")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("P3-HTTP-004 create status=%d body=%s", res.Code, res.Body.String())
	}
	var created map[string]json.RawMessage
	p3DecodeJSON(t, res, &created)
	p3RequireKeys(t, created, "id", "name")

	invalid := []byte(`{"name":"","mcVersion":"","loader":"unknown"}`)
	res = p3Do(t, handler, http.MethodPost, "/api/packs", bytes.NewReader(invalid))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("P3-HTTP-004 invalid create status=%d body=%s", res.Code, res.Body.String())
	}
	p3RequireError(t, res, "invalid_argument")
}

func TestP3MissingResourceAndSecurityErrorsAreStable(t *testing.T) {
	handler, db := p3Router(t)
	defer db.Close()

	res := p3Do(t, handler, http.MethodGet, "/api/packs/does-not-exist", nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("P3-HTTP-005 missing pack status=%d body=%s", res.Code, res.Body.String())
	}
	p3RequireError(t, res, "pack_not_found")

	// Every mutating endpoint must reject requests without the local bootstrap
	// token. This also guards against treating a desktop-only deployment as a
	// reason to omit its browser security boundary.
	req := httptest.NewRequest(http.MethodPost, "/api/packs", bytes.NewReader([]byte(`{}`)))
	req.Host = "localhost"
	req.Header.Set("Origin", "http://localhost")
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("P3-HTTP-005 missing token status=%d body=%s", res.Code, res.Body.String())
	}
	p3RequireError(t, res, "unauthorized")
}

func TestP3SystemAndOnboardingContracts(t *testing.T) {
	handler, db := p3Router(t)
	defer db.Close()

	for _, path := range []string{"/api/system/health", "/api/system/status"} {
		res := p3Do(t, handler, http.MethodGet, path, nil)
		if res.Code != http.StatusOK {
			t.Fatalf("P3-HTTP-006 %s status=%d body=%s", path, res.Code, res.Body.String())
		}
		var object map[string]json.RawMessage
		p3DecodeJSON(t, res, &object)
		if path == "/api/system/health" {
			p3RequireKeys(t, object, "curseforgeKeyConfigured", "modrinthReachable", "curseforgeReachable", "storageWritable", "storageFreeBytes")
		} else {
			p3RequireKeys(t, object, "modrinthReachable", "curseforgeReachable", "cacheSizeBytes", "storageFreeBytes")
		}
	}

	res := p3Do(t, handler, http.MethodGet, "/api/onboarding", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("P3-HTTP-006 onboarding status=%d body=%s", res.Code, res.Body.String())
	}
	var onboarding map[string]json.RawMessage
	p3DecodeJSON(t, res, &onboarding)
	p3RequireKeys(t, onboarding, "steps")
	var steps map[string]json.RawMessage
	p3Unmarshal(t, onboarding["steps"], &steps)
	p3RequireKeys(t, steps, "curseforgeKey", "firstPack", "firstMod")
}

func p3Router(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "p3-contract.db"))
	if err != nil {
		t.Fatalf("open P3 database: %v", err)
	}
	return NewRouter(db, "test"), db
}

func p3Request(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.Host = "localhost"
	req.Header.Set("X-MPack-Token", "test")
	req.Header.Set("Origin", "http://localhost")
	return req
}

func p3Do(t *testing.T, handler http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := p3Request(t, method, path, body)
	if body != nil && method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func p3DecodeJSON(t *testing.T, res *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		t.Fatalf("P3 contract response is not valid JSON: %v (body=%s)", err, res.Body.String())
	}
}

func p3Unmarshal(t *testing.T, raw json.RawMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("P3 contract field is invalid: %v (json=%s)", err, raw)
	}
}

func p3RequireKeys(t *testing.T, object map[string]json.RawMessage, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			t.Errorf("P3 contract missing required JSON key %q", key)
		}
	}
}

func p3RequireError(t *testing.T, res *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			RequestID string         `json:"request_id"`
			Details   map[string]any `json:"details"`
		} `json:"error"`
	}
	p3DecodeJSON(t, res, &envelope)
	if envelope.Error.Code != wantCode {
		t.Errorf("P3 error code=%q, want %q", envelope.Error.Code, wantCode)
	}
	if envelope.Error.Message == "" {
		t.Error("P3 error message must be non-empty")
	}
	if envelope.Error.RequestID == "" || res.Header().Get("X-Request-ID") == "" {
		t.Error("P3 error must be traceable by request_id in body and response header")
	}
}
