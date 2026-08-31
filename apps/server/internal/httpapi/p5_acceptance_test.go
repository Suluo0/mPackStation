package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/service"
	"mpackstation/internal/store"
)

// TestP5HTTPModChainAndStableProviderErrors checks the frontend-facing P5
// contract through the real router and service boundary. It intentionally
// checks both successful writes and provider failure envelopes; a route that
// merely returns JSON is not sufficient evidence for this milestone.
func TestP5HTTPModChainAndStableProviderErrors(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "p5-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := service.New(db)
	adapter, err := provider.NewCurseForgeFixture([]byte(`{
        "projects":[{"id":"p1","slug":"alpha","name":"Alpha Mod"}],
        "versions":[{"id":"v1","projectId":"p1","name":"1.0","versionNumber":"1.0","files":[{"id":"f1","name":"alpha.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]}],
        "metadata":[{"project":{"id":"p1","slug":"alpha","name":"Alpha Mod"},"version":{"id":"v1","projectId":"p1","name":"1.0"},"dependencies":[]}],
        "faults":{"search:missing":{"code":"404"},"search:throttled":{"code":"429"},"search:down":{"code":"503"}}
    }`))
	if err != nil {
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(adapter))
	handler := NewRouterWithService(app, "test", "test")
	pack, err := app.CreatePack(context.Background(), service.CreatePackInput{Name: "P5 HTTP", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p5-http-pack")
	if err != nil {
		t.Fatal(err)
	}

	res := p5HTTPDo(t, handler, http.MethodGet, "/api/packs/"+pack.ID+"/mod-search?provider=curseforge&query=alpha", nil, false)
	if res.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", res.Code, res.Body.String())
	}
	var search map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&search); err != nil {
		t.Fatalf("search JSON: %v", err)
	}
	for _, key := range []string{"items", "next_cursor", "total"} {
		if _, ok := search[key]; !ok {
			t.Fatalf("search response missing %q: %s", key, res.Body.String())
		}
	}

	res = p5HTTPDo(t, handler, http.MethodPost, "/api/packs/"+pack.ID+"/mods", bytes.NewBufferString(`{"provider":"curseforge","projectId":"p1","versionId":"v1","required":true}`), true)
	if res.Code != http.StatusCreated {
		t.Fatalf("add status=%d body=%s", res.Code, res.Body.String())
	}
	var mod service.Mod
	if err := json.NewDecoder(res.Body).Decode(&mod); err != nil {
		t.Fatalf("add JSON: %v", err)
	}
	if mod.ID == "" || mod.Status != "installed" || mod.SHA1 == nil {
		t.Fatalf("add response=%#v", mod)
	}

	res = p5HTTPDo(t, handler, http.MethodGet, "/api/packs/"+pack.ID+"/mods", nil, false)
	if res.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", res.Code, res.Body.String())
	}
	var list struct {
		Items []service.Mod `json:"items"`
		Total int           `json:"total"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("list JSON: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("list = %#v", list)
	}

	res = p5HTTPDo(t, handler, http.MethodPatch, "/api/packs/"+pack.ID+"/mods/"+mod.ID, bytes.NewBufferString(`{"status":"disabled"}`), true)
	if res.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", res.Code, res.Body.String())
	}
	var disabled service.Mod
	if err := json.NewDecoder(res.Body).Decode(&disabled); err != nil {
		t.Fatalf("disable JSON: %v", err)
	}
	if disabled.Status != "disabled" {
		t.Fatalf("disable response=%#v", disabled)
	}

	res = p5HTTPDo(t, handler, http.MethodPost, "/api/packs/"+pack.ID+"/resolve", nil, true)
	if res.Code != http.StatusAccepted {
		t.Fatalf("resolve status=%d body=%s", res.Code, res.Body.String())
	}
	var resolved map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&resolved); err != nil {
		t.Fatalf("resolve JSON: %v", err)
	}
	if _, ok := resolved["lock"]; !ok {
		t.Fatalf("resolve response missing lock: %s", res.Body.String())
	}
	res = p5HTTPDo(t, handler, http.MethodGet, "/api/packs/"+pack.ID+"/locks", nil, false)
	if res.Code != http.StatusOK {
		t.Fatalf("locks status=%d body=%s", res.Code, res.Body.String())
	}
	res = p5HTTPDo(t, handler, http.MethodGet, "/api/packs/"+pack.ID+"/health", nil, false)
	if res.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", res.Code, res.Body.String())
	}
	res = p5HTTPDo(t, handler, http.MethodDelete, "/api/packs/"+pack.ID+"/mods/"+mod.ID, nil, true)
	if res.Code != http.StatusNoContent {
		t.Fatalf("remove status=%d body=%s", res.Code, res.Body.String())
	}

	tests := []struct {
		name       string
		query      string
		status     int
		code       string
		retryAfter bool
	}{
		{name: "404", query: "missing", status: http.StatusNotFound, code: "provider_not_found"},
		{name: "429", query: "throttled", status: http.StatusBadGateway, code: "provider_unavailable", retryAfter: false},
		{name: "503", query: "down", status: http.StatusBadGateway, code: "provider_unavailable"},
	}
	for _, tc := range tests {
		t.Run("provider-"+tc.name, func(t *testing.T) {
			res := p5HTTPDo(t, handler, http.MethodGet, "/api/packs/"+pack.ID+"/mod-search?provider=curseforge&query="+tc.query, nil, false)
			if res.Code != tc.status {
				t.Fatalf("status=%d body=%s, want %d", res.Code, res.Body.String(), tc.status)
			}
			var body struct {
				Error struct {
					Code      string `json:"code"`
					RequestID string `json:"request_id"`
				} `json:"error"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatalf("error JSON: %v", err)
			}
			if body.Error.Code != tc.code || body.Error.RequestID == "" {
				t.Fatalf("error envelope=%#v, want code=%q and request id", body.Error, tc.code)
			}
			if tc.retryAfter && res.Header().Get("Retry-After") == "" {
				t.Fatalf("429 response missing Retry-After")
			}
		})
	}
}

func p5HTTPDo(t *testing.T, handler http.Handler, method, path string, body *bytes.Buffer, write bool) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Buffer
	if body == nil {
		reader = bytes.NewBuffer(nil)
	} else {
		reader = body
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "localhost"
	req.Header.Set("Origin", "http://localhost")
	if write {
		req.Header.Set("X-MPack-Token", "test")
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
