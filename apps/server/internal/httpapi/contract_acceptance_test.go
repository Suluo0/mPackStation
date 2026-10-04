package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mpackstation/internal/store"
)

// TestFrontendContractRoutesExist is intentionally strict and currently
// exposes the known v1 HTTP gap. It must become green only when the route is
// backed by the service layer and has its own success/error contract tests.
func TestFrontendContractRoutesExist(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "http-contract.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()

	handler := NewRouter(db, "test")
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/dashboard"},
		{http.MethodGet, "/api/tasks"},
		{http.MethodGet, "/api/activities"},
		{http.MethodGet, "/api/system/health"},
		{http.MethodGet, "/api/system/status"},
		{http.MethodGet, "/api/onboarding"},
		{http.MethodPut, "/api/onboarding"},
		{http.MethodGet, "/api/meta/mc-versions"},
		{http.MethodGet, "/api/packs"},
		{http.MethodPost, "/api/packs"},
		{http.MethodPost, "/api/packs/import"},
	}
	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("V7-HTTP-001 missing route %s %s (status %d)", route.method, route.path, rec.Code)
		}
	}
}
