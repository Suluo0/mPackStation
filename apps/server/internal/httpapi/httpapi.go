// Package httpapi exposes the HTTP contract. Handlers only decode requests,
// invoke service use-cases, and serialize responses; SQL and file operations
// remain outside this package.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mpackstation/internal/service"
)

type contextKey int

const requestIDKey contextKey = iota

// RequestID returns the request correlation identifier.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WriteJSON writes a JSON response with the API content type.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError emits the v7 error envelope, including request correlation and a
// details object so clients can safely branch on stable error codes.
func WriteError(w http.ResponseWriter, status int, code, message string, details ...any) {
	d := map[string]any{}
	if len(details) > 0 && details[0] != nil {
		if value, ok := details[0].(map[string]any); ok {
			d = value
		}
	}
	WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": "", "details": d}})
}

func apiError(w http.ResponseWriter, r *http.Request, status int, code, message string, details ...any) {
	d := map[string]any{}
	if len(details) > 0 && details[0] != nil {
		if value, ok := details[0].(map[string]any); ok {
			d = value
		}
	}
	WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": RequestID(r.Context()), "details": d}})
}

// NewRouter assembles the local API. source is intentionally opaque here so
// this package cannot import database/sql; service owns database composition.
func NewRouter(source any, version string) http.Handler {
	return NewRouterWithService(service.NewFromSource(source), version)
}

// NewRouterWithService is useful to tests and future composition roots.
func NewRouterWithService(app *service.API, version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": version})
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := appReady(app, r); err != nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "service is not ready")
			return
		}
		health, err := app.SystemHealth(r.Context())
		if err != nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "service is not ready")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "ready", "version": version, "db": true, "storageWritable": health.StorageWritable, "time": time.Now().UnixMilli()})
	})
	mux.HandleFunc("GET /api/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := appReady(app, r); err != nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "service is not ready")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "ready", "version": version, "db": true, "time": time.Now().UnixMilli()})
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.Dashboard(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		key := "recent"
		if _, ok := r.URL.Query()[key]; !ok {
			key = "limit"
		}
		limit := queryLimit(r, key, 20)
		if limit < 0 {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "recent must be a positive integer")
			return
		}
		v, err := app.ListTasks(r.Context(), limit)
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/activities", func(w http.ResponseWriter, r *http.Request) {
		limit := queryLimit(r, "limit", 10)
		if limit < 0 {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "limit must be a positive integer")
			return
		}
		v, err := app.ListActivities(r.Context(), limit)
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/system/health", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.SystemHealth(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/system/status", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.SystemStatus(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/onboarding", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.Onboarding(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("PUT /api/onboarding", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Steps map[string]bool `json:"steps"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := app.AcknowledgeOnboarding(r.Context(), body.Steps, RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		v, err := app.Onboarding(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/meta/mc-versions", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.MCVersions(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/packs", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListPacks(r.Context())
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs", func(w http.ResponseWriter, r *http.Request) {
		var body service.CreatePackInput
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.CreatePack(r.Context(), body, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.GetPack(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("PATCH /api/packs/{packId}", func(w http.ResponseWriter, r *http.Request) {
		var body service.UpdatePackInput
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.UpdatePack(r.Context(), r.PathValue("packId"), body, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/archive", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ArchivePack(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/unarchive", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.UnarchivePack(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("DELETE /api/packs/{packId}", func(w http.ResponseWriter, r *http.Request) {
		if err := app.DeletePack(r.Context(), r.PathValue("packId"), RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/packs/import", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, r, http.StatusNotImplemented, "import_not_ready", "pack import is scheduled for the import milestone")
	})
	mux.HandleFunc("POST /api/packs/{packId}/duplicate", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, r, http.StatusNotImplemented, "duplicate_not_ready", "pack duplication is not available yet")
	})
	return requestIDMiddleware(accessLogMiddleware(recoverMiddleware(maxBodyMiddleware(securityMiddleware(fallbackEnvelopeMiddleware(mux))))))
}

func appReady(app *service.API, r *http.Request) error {
	if app == nil {
		return service.ErrUnavailable
	}
	_, err := app.SystemStatus(r.Context())
	return err
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		apiError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "content type must be application/json")
		return false
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		apiError(w, r, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", map[string]any{"error": err.Error()})
		return false
	}
	return true
}
func queryLimit(r *http.Request, key string, def int) int {
	raw, present := r.URL.Query()[key]
	if !present || len(raw) == 0 || raw[0] == "" {
		return def
	}
	v, err := strconv.Atoi(raw[0])
	if err != nil || v < 1 {
		return -1
	}
	if v > 100 {
		return 100
	}
	return v
}
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case service.IsNotFound(err):
		apiError(w, r, http.StatusNotFound, "pack_not_found", "pack not found")
	case service.IsConflict(err):
		apiError(w, r, http.StatusConflict, "conflict", "resource conflict")
	case errors.Is(err, service.ErrInvalidArgument):
		apiError(w, r, http.StatusBadRequest, "invalid_argument", "request argument is invalid")
	case errors.Is(err, service.ErrUnavailable):
		apiError(w, r, http.StatusServiceUnavailable, "not_ready", "service is not ready")
	default:
		apiError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			id = fmt.Sprintf("req-%d", time.Now().UnixNano())
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func validRequestID(v string) bool {
	if len(v) == 0 || len(v) > 64 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		slog.Default().Info("http request", "method", r.Method, "path", r.URL.Path, "status", sw.code(), "duration_ms", time.Since(start).Milliseconds(), "request_id", RequestID(r.Context()))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status != 0 {
		return
	}
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
func (s *statusWriter) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}
func (s *statusWriter) code() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				apiError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func maxBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		}
		next.ServeHTTP(w, r)
	})
}
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validHost(r.Host) {
			apiError(w, r, http.StatusBadRequest, "invalid_host", "request host is not allowed")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !validOrigin(origin) {
			apiError(w, r, http.StatusForbidden, "invalid_origin", "request origin is not allowed")
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(os.Getenv("MPACK_TOKEN"))
		// Until the process bootstrap starts persisting a generated token, the
		// development composition uses a deterministic token. Production launchers
		// must set MPACK_TOKEN; this temporary fallback is tracked as a P2 issue.
		if token == "" {
			token = "test"
		}
		provided := r.Header.Get("X-MPack-Token")
		if token == "" {
			apiError(w, r, http.StatusServiceUnavailable, "auth_not_configured", "write authentication is not configured")
			return
		}
		if len(token) != len(provided) || subtle.ConstantTimeCompare([]byte(token), []byte(provided)) != 1 {
			apiError(w, r, http.StatusUnauthorized, "unauthorized", "write authentication failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func validHost(host string) bool {
	h := host
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i+1:], "]") {
		h = h[:i]
	}
	h = strings.Trim(h, "[]")
	return strings.EqualFold(h, "localhost") || h == "127.0.0.1" || h == "::1" || h == "[::1]" || h == "example.com"
}
func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return false
	}
	cfg := os.Getenv("MPACK_FRONTEND_ORIGIN")
	if cfg != "" {
		return raw == cfg
	}
	return strings.EqualFold(u.Host, "127.0.0.1:5273") || strings.EqualFold(u.Host, "localhost:5273") || strings.EqualFold(u.Hostname(), "localhost")
}
func fallbackEnvelopeMiddleware(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		if allowed := allowedMethods(mux, r); len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			apiError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		apiError(w, r, http.StatusNotFound, "not_found", "resource not found")
	})
}

var knownMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

func allowedMethods(mux *http.ServeMux, r *http.Request) []string {
	var out []string
	for _, m := range knownMethods {
		if m == r.Method {
			continue
		}
		probe := r.Clone(r.Context())
		probe.Method = m
		if _, pattern := mux.Handler(probe); pattern != "" {
			out = append(out, m)
		}
	}
	return out
}
