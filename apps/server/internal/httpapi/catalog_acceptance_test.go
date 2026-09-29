package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"mpackstation/internal/service"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

func TestCatalogStatusAndRebuildAuthorization(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "catalog-http.db"))
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
	pack, err := app.CreatePack(context.Background(), service.CreatePackInput{Name: "Catalog HTTP", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1"}, "create")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithService(app, "test", "token")
	request := httptest.NewRequest(http.MethodGet, "/api/packs/"+pack.ID+"/mods?includeBuiltin=true", nil)
	request.Host = "localhost"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"canonicalModId":"minecraft"`) || !strings.Contains(response.Body.String(), `"origin":"builtin"`) {
		t.Fatalf("builtin content source = %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/packs/"+pack.ID+"/catalog/status", nil)
	request.Host = "localhost"
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"pending"`) {
		t.Fatalf("status response %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/packs/"+pack.ID+"/catalog/rebuild", strings.NewReader(`{}`))
	request.Host = "localhost"
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized rebuild = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/packs/"+pack.ID+"/catalog/rebuild", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-MPack-Token", "token")
	request.Header.Set("Origin", "http://localhost")
	request.Host = "localhost"
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"taskId"`) {
		t.Fatalf("authorized rebuild = %d %s", response.Code, response.Body.String())
	}
}
