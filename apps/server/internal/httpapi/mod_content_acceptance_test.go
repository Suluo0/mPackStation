package httpapi

import (
	"archive/zip"
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
	"mpackstation/internal/task"
)

func modContentHTTPFixture(t *testing.T) (*service.API, *task.Queue, string, string, http.Handler) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "mc-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := service.New(db)
	pack, err := app.CreatePack(context.Background(), service.CreatePackInput{
		Name: "MC HTTP", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15",
	}, "mc-http-pack")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(&fakeHTTPProvider{jar: buildMinimalJarHTTP(t)}))
	mod, err := app.AddPackMod(context.Background(), pack.ID, service.AddModInput{
		Provider: "modrinth", ProjectID: "proj", VersionID: "ver",
	}, "mc-http-mod")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	q, err := task.NewQueue(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	app.SetTaskQueue(q)
	t.Cleanup(func() { _ = db.Close() })
	return app, q, pack.ID, mod.ID, NewRouterWithService(app, "test", "test-token")
}

type fakeHTTPProvider struct {
	provider.Adapter
	jar []byte
}

func (f *fakeHTTPProvider) Name() provider.Name { return "modrinth" }
func (f *fakeHTTPProvider) Metadata(_ context.Context, _, _ string) (provider.Metadata, error) {
	return provider.Metadata{
		Project: provider.Project{ID: "proj", Name: "Test Mod"},
		Version: provider.Version{ID: "ver", VersionNumber: "1.0.0"},
	}, nil
}
func (f *fakeHTTPProvider) Download(_ context.Context, _ provider.DownloadRequest) (provider.DownloadResult, error) {
	return provider.DownloadResult{Content: f.jar, FileName: "test.jar"}, nil
}

func buildMinimalJarHTTP(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"fabric.mod.json":                                `{"id":"testmod","name":"Test Mod","version":"1.0.0"}`,
		"data/testmod/recipe/simple.json":                `{"type":"minecraft:crafting_shapeless","ingredients":[{"item":"minecraft:dirt"}],"result":{"id":"testmod:magic_dirt","count":1}}`,
		"data/testmod/recipe/special/wildcard.json":      `{"type":"testmod:wildcard"}`,
		"assets/testmod/models/item/magic_dirt.json":     `{}`,
		"data/testmod/worldgen/structure/test_ruin.json": `{"type":"minecraft:jigsaw"}`,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func runParseTask(t *testing.T, app *service.API, _ *task.Queue, packID, modID string) {
	t.Helper()
	if _, err := app.ParseModContentSync(context.Background(), packID, modID); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

func mcDo(t *testing.T, handler http.Handler, method, path string, write bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Host = "localhost"
	req.Header.Set("Origin", "http://localhost")
	req.Header.Set("X-Request-ID", "test-req")
	if write {
		req.Header.Set("X-MPack-Token", "test-token")
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestPOST_ParseModContent_202(t *testing.T) {
	_, _, packID, modID, handler := modContentHTTPFixture(t)
	w := mcDo(t, handler, "POST", "/api/packs/"+packID+"/mods/"+modID+"/content/parse", true)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TaskID == "" {
		t.Error("taskId is empty")
	}
}

func TestGET_ModContent_404_NotParsed(t *testing.T) {
	_, _, packID, modID, handler := modContentHTTPFixture(t)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content", false)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Error.Code != "content_not_parsed" {
		t.Errorf("code = %q, want content_not_parsed", env.Error.Code)
	}
}

func TestGET_ModContentRun_404(t *testing.T) {
	_, _, packID, modID, handler := modContentHTTPFixture(t)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content/run", false)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestGET_ModContent_AfterParse_200(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []struct {
			Kind      string `json:"kind"`
			IsDynamic bool   `json:"isDynamic"`
		} `json:"items"`
		Total int `json:"total"`
		Run   struct {
			Status string `json:"status"`
		} `json:"run"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("total = %d, want 5", resp.Total)
	}
	if resp.Run.Status != "succeeded" {
		t.Errorf("run status = %q, want succeeded", resp.Run.Status)
	}
}

func TestGET_ModContent_KindFilter(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content?kind=recipe", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Total int `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("recipe total = %d, want 2", resp.Total)
	}
}

func TestGET_ModContent_InvalidKind_400(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content?kind=bogus", false)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestGET_ModContentRun_AfterParse_200(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content/run", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Run struct {
			Status       string `json:"status"`
			ParsedCount  int    `json:"parsedCount"`
			DynamicCount int    `json:"dynamicCount"`
		} `json:"run"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Run.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", resp.Run.Status)
	}
	if resp.Run.ParsedCount != 5 {
		t.Errorf("parsedCount = %d, want 5", resp.Run.ParsedCount)
	}
	if resp.Run.DynamicCount != 1 {
		t.Errorf("dynamicCount = %d, want 1", resp.Run.DynamicCount)
	}
}

func TestPOST_ParseModContent_404_ModNotFound(t *testing.T) {
	_, _, packID, _, handler := modContentHTTPFixture(t)
	w := mcDo(t, handler, "POST", "/api/packs/"+packID+"/mods/nonexistent-mod/content/parse", true)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestPOST_ParseModContent_409_Idempotent(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	// Parse first via sync.
	runParseTask(t, app, q, packID, modID)
	// POST should return 409 because same sha1 is already parsed.
	w := mcDo(t, handler, "POST", "/api/packs/"+packID+"/mods/"+modID+"/content/parse", true)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Error.Code != "parse_already_current" {
		t.Errorf("code = %q, want parse_already_current", env.Error.Code)
	}
}

func TestGET_ModContentById_404(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content/nonexistent-content-id", false)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestGET_ModContentById_200(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	// First list to get a valid content id.
	wList := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content?limit=1", false)
	if wList.Code != http.StatusOK {
		t.Fatalf("list status = %d", wList.Code)
	}
	var listResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(wList.Body.Bytes(), &listResp)
	if len(listResp.Items) == 0 {
		t.Fatal("no items in list")
	}
	contentID := listResp.Items[0].ID
	w := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content/"+contentID, false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Item struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Item.ID != contentID {
		t.Errorf("item id = %q, want %q", resp.Item.ID, contentID)
	}
}

func TestGET_ModContent_PaginationCursor(t *testing.T) {
	app, q, packID, modID, handler := modContentHTTPFixture(t)
	runParseTask(t, app, q, packID, modID)
	// limit=1 should give a next_cursor.
	w1 := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content?limit=1", false)
	if w1.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w1.Code)
	}
	var resp1 struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	if len(resp1.Items) != 1 {
		t.Fatalf("page1 items = %d, want 1", len(resp1.Items))
	}
	if resp1.NextCursor == "" {
		t.Fatal("next_cursor is empty, want non-empty")
	}
	// Use cursor to get next page.
	w2 := mcDo(t, handler, "GET", "/api/packs/"+packID+"/mods/"+modID+"/content?limit=1&cursor="+resp1.NextCursor, false)
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w2.Code)
	}
	var resp2 struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	if len(resp2.Items) != 1 {
		t.Fatalf("page2 items = %d, want 1", len(resp2.Items))
	}
	if resp2.Items[0].ID == resp1.Items[0].ID {
		t.Error("page2 returned same item as page1")
	}
}
func TestModContentIconsHTTPPackBoundary(t *testing.T) {
	_, _, packID, modID, h := modContentHTTPFixture(t)
	for _, tc := range []struct {
		pack, token string
		want        int
	}{{packID, "", 401}, {"other-pack", "test-token", 404}} {
		req := httptest.NewRequest("POST", "/api/packs/"+tc.pack+"/mods/"+modID+"/content/icons/resolve", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if tc.token != "" {
			req.Header.Set("X-MPack-Token", tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("status %d want %d: %s", w.Code, tc.want, w.Body.String())
		}
	}
}
