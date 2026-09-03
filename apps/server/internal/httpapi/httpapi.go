// Package httpapi exposes the HTTP contract. Handlers only decode requests,
// invoke service use-cases, and serialize responses; SQL and file operations
// remain outside this package.
package httpapi

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"mpackstation/internal/provider"
	"mpackstation/internal/service"
	"mpackstation/internal/task"
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
		// A typed nil map (e.g. DomainError.Details unset) is not nil as `any`;
		// guard explicitly so the envelope always carries an object, not null.
		if value, ok := details[0].(map[string]any); ok && value != nil {
			d = value
		}
	}
	WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": RequestID(r.Context()), "details": d}})
}

// NewRouter assembles the local API over an explicit database handle.
// token is the required write-token (see auth.md); an empty token rejects all
// write requests with 503 auth_not_configured.
func NewRouter(db *sql.DB, version, token string) http.Handler {
	return newRouter(service.New(db), service.NewTaskAPI(db), service.NewP7Service(db), service.NewImportService(db), version, token)
}

// NewRouterWithService is useful to tests and future composition roots.
func NewRouterWithService(app *service.API, version, token string) http.Handler {
	return newRouter(app, nil, nil, nil, version, token)
}

// NewRouterWithProviders wires real provider adapters (Modrinth/CurseForge)
// into both the catalog service and the publish pipeline. A non-nil queue
// additionally enables task-based tool installation.
func NewRouterWithProviders(db *sql.DB, version, token string, reg *provider.Registry, q *task.Queue) http.Handler {
	app := service.New(db)
	app.SetProviderRegistry(reg)
	if q != nil {
		app.SetTaskQueue(q)
		_ = q.RegisterHandler(task.KindToolInstall, task.HandlerFunc(app.HandleToolInstallTask))
		_ = q.RegisterHandler(task.KindLauncherInstall, task.HandlerFunc(app.HandleLauncherInstallTask))
		_ = q.RegisterHandler(task.KindLauncherLaunch, task.HandlerFunc(app.HandleLauncherLaunchTask))
	}
	p7 := service.NewP7Service(db)
	p7.SetProviderRegistry(reg)
	return newRouter(app, service.NewTaskAPI(db), p7, service.NewImportService(db), version, token)
}

func newRouter(app *service.API, taskAPI *service.TaskAPI, p7 *service.P7Service, importer *service.ImportService, version, token string) http.Handler {
	mux := http.NewServeMux()
	registerSystemRoutes(mux, app, version)
	registerDashboardRoutes(mux, app)
	registerTaskRoutes(mux, app, taskAPI)
	registerPackRoutes(mux, app)
	registerModRoutes(mux, app)
	registerContentRoutes(mux, app)
	registerPublishRoutes(mux, app, taskAPI, p7, version)
	registerImportRoutes(mux, importer)
	return requestIDMiddleware(accessLogMiddleware(recoverMiddleware(maxBodyMiddleware(securityMiddleware(token, fallbackEnvelopeMiddleware(mux))))))
}

// checkMCVersionCandidate enforces the closed candidate list for the pack
// endpoints (contract §3.2 → 422 pack_unsupported_mc_version). The import path
// bypasses this on purpose: imported packs keep whatever MC version the archive
// declares.
func checkMCVersionCandidate(w http.ResponseWriter, r *http.Request, app *service.API, mcVersion string) bool {
	if strings.TrimSpace(mcVersion) == "" {
		return true // required-field check stays in the service layer
	}
	versions, err := app.MCVersions(r.Context())
	if err != nil {
		writeError(w, r, err)
		return false
	}
	if !slices.Contains(versions, mcVersion) {
		apiError(w, r, http.StatusUnprocessableEntity, "pack_unsupported_mc_version", "mcVersion is not a supported candidate", map[string]any{"candidates": versions})
		return false
	}
	return true
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
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apiError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the 8MB limit")
			return false
		}
		apiError(w, r, http.StatusBadRequest, "invalid_argument", "request body is not valid JSON", map[string]any{"error": err.Error()})
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
	if err != nil || v < 1 || v > 100 {
		// Contract: out-of-range pagination params are a 400, not a silent clamp.
		return -1
	}
	return v
}

func parseIfMatch(r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" {
		return 0, false
	}
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		raw = strings.Trim(raw, "\"")
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value >= 0
}

// writeError is the single error translator at the HTTP boundary (B3 merges
// the four previous per-family translators). Whatever the interior layers
// return, the response envelope is always the contract shape. Order: typed
// DomainError > ValidationError > domain sentinels > generic fallbacks.
// Contract: same idempotency key with different input is 422
// idempotency_conflict on every endpoint (contract.md).
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var de *service.DomainError
	if errors.As(err, &de) {
		apiError(w, r, de.Status, de.Code, de.Message, de.Details)
		return
	}
	var ve *service.ValidationError
	if errors.As(err, &ve) {
		apiError(w, r, http.StatusUnprocessableEntity, validationErrorCode(ve), "resource validation failed", map[string]any{"issues": ve.Issues})
		return
	}
	switch {
	// import preview lifecycle
	case errors.Is(err, service.ErrImportInvalidSource):
		apiError(w, r, http.StatusUnprocessableEntity, "import_invalid_source", "import source is invalid")
	case errors.Is(err, service.ErrImportUnsafeArchive):
		apiError(w, r, http.StatusUnprocessableEntity, "unsafe_archive", "archive failed safety checks")
	case errors.Is(err, service.ErrImportConsumed):
		apiError(w, r, http.StatusConflict, "import_preview_consumed", "import preview was already consumed")
	case errors.Is(err, service.ErrImportExpired):
		apiError(w, r, http.StatusGone, "import_preview_expired", "import preview is expired")
	// idempotency: the task queue is the enforcement point for import/publish
	// submit paths; the contract fixes 422 idempotency_conflict for key reuse
	// with different input on every endpoint.
	case errors.Is(err, task.ErrIdempotencyConflict), errors.Is(err, task.ErrIdempotencyConsumed):
		apiError(w, r, http.StatusUnprocessableEntity, "idempotency_conflict", "idempotency key was used with a different request")
	// task control surface
	case service.IsTaskNotFound(err):
		apiError(w, r, http.StatusNotFound, "task_not_found", "task not found")
	case service.IsTaskInvalidTransition(err):
		apiError(w, r, http.StatusConflict, "task_invalid_transition", "task state transition is not allowed")
	case service.IsTaskLeaseLost(err):
		apiError(w, r, http.StatusConflict, "task_lease_lost", "task lease is no longer valid")
	case service.IsTaskNotAvailable(err):
		apiError(w, r, http.StatusConflict, "task_not_available", "task is not available")
	case service.IsTaskUnknownKind(err):
		apiError(w, r, http.StatusInternalServerError, "task_unknown_kind", "task kind is not registered")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		apiError(w, r, http.StatusRequestTimeout, "request_canceled", "task request was canceled")
	// build / publish (P7)
	case errors.Is(err, service.ErrInvalidBuildInput):
		apiError(w, r, http.StatusBadRequest, "invalid_argument", "build or publish input is invalid")
	case errors.Is(err, service.ErrExportDirNotAllowed):
		apiError(w, r, http.StatusForbidden, "export_dir_not_allowed", "export directory is not approved")
	case errors.Is(err, service.ErrDeliveryBlocked):
		apiError(w, r, http.StatusUnprocessableEntity, "build_blocked", "delivery checks are blocked")
	case errors.Is(err, service.ErrPublishFailed):
		apiError(w, r, http.StatusBadGateway, "provider_unavailable", "publication failed; retry is explicit")
	case errors.Is(err, service.ErrPublishIdempotencyConflict):
		apiError(w, r, http.StatusUnprocessableEntity, "idempotency_conflict", "publication key or artifact conflicts")
	case errors.Is(err, service.ErrProviderStatusUnavailable):
		apiError(w, r, http.StatusBadGateway, "provider_unavailable", "remote status is unavailable")
	case errors.Is(err, service.ErrArtifactMissing):
		apiError(w, r, http.StatusGone, "artifact_expired", "artifact is no longer available")
	// provider / revision / generic service sentinels
	case errors.Is(err, service.ErrProviderNotFound):
		apiError(w, r, http.StatusNotFound, "provider_not_found", "provider resource not found")
	case errors.Is(err, service.ErrProviderUnavailable):
		apiError(w, r, http.StatusBadGateway, "provider_unavailable", "provider is unavailable")
	case errors.Is(err, service.ErrInvalidSHA1):
		apiError(w, r, http.StatusBadRequest, "invalid_sha1", "provider returned an invalid SHA-1")
	case errors.Is(err, service.ErrRevisionConflict):
		apiError(w, r, http.StatusPreconditionFailed, "revision_conflict", "resource revision is stale")
	case service.IsNotFound(err):
		apiError(w, r, http.StatusNotFound, "pack_not_found", "pack not found")
	case service.IsConflict(err):
		apiError(w, r, http.StatusConflict, "conflict", "resource conflict")
	case errors.Is(err, service.ErrInvalidArgument):
		apiError(w, r, http.StatusBadRequest, "invalid_argument", "request argument is invalid")
	case service.IsTaskUnavailable(err), errors.Is(err, service.ErrUnavailable):
		apiError(w, r, http.StatusServiceUnavailable, "not_ready", "service is not ready")
	default:
		apiError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// validationErrorCode maps per-issue codes to the fine-grained domain code the
// contract assigns (standards.md D-5). issues are always included in details.
func validationErrorCode(ve *service.ValidationError) string {
	if ve.Domain != "quest" {
		return "content_invalid"
	}
	for _, i := range ve.Issues {
		switch i.Code {
		case "cycle":
			return "quest_cycle"
		case "cross_pack_reference", "missing_mod_reference":
			return "quest_invalid_reference"
		}
	}
	for _, i := range ve.Issues {
		if i.Code == "orphan_node" {
			return "quest_orphan_node"
		}
	}
	return "quest_invalid"
}

func p7Ready(p7 *service.P7Service) error {
	if p7 == nil {
		return service.ErrUnavailable
	}
	return nil
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
func securityMiddleware(token string, next http.Handler) http.Handler {
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
		// The token is resolved once at process bootstrap (MPACK_TOKEN env, or a
		// generated random value persisted to <data>/runtime-token). Hardcoded
		// fallbacks are forbidden by auth.md / decision D-8.
		if token == "" {
			apiError(w, r, http.StatusServiceUnavailable, "auth_not_configured", "write authentication is not configured")
			return
		}
		provided := r.Header.Get("X-MPack-Token")
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
