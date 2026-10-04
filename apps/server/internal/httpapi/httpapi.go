// Package httpapi exposes the HTTP contract. Handlers only decode requests,
// invoke service use-cases, and serialize responses; SQL and file operations
// remain outside this package.
package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
func NewRouter(db *sql.DB, version string) http.Handler {
	return newRouter(service.New(db), service.NewTaskAPI(db), service.NewP7Service(db), service.NewImportService(db), version)
}

// NewRouterWithService is useful to tests and future composition roots.
func NewRouterWithService(app *service.API, version string) http.Handler {
	return newRouter(app, nil, nil, nil, version)
}

// NewRouterWithProviders wires real provider adapters (Modrinth/CurseForge)
// into both the catalog service and the publish pipeline. A non-nil queue
// additionally enables task-based tool installation.
func NewRouterWithProviders(db *sql.DB, version string, reg *provider.Registry, q *task.Queue) http.Handler {
	app := service.New(db)
	app.SetProviderRegistry(reg)
	if q != nil {
		app.SetTaskQueue(q)
		_ = q.RegisterHandler(task.KindToolInstall, task.HandlerFunc(app.HandleToolInstallTask))
		_ = q.RegisterHandler(task.KindLauncherInstall, task.HandlerFunc(app.HandleLauncherInstallTask))
		_ = q.RegisterHandler(task.KindLauncherLaunch, task.HandlerFunc(app.HandleLauncherLaunchTask))
		_ = q.RegisterHandler(task.KindParseModContent, task.HandlerFunc(app.HandleParseModContentTask))
		_ = q.RegisterHandler(task.KindCatalogInit, task.HandlerFunc(app.HandleCatalogInitTask))
	}
	p7 := service.NewP7Service(db)
	p7.SetProviderRegistry(reg)
	return newRouter(app, service.NewTaskAPI(db), p7, service.NewImportService(db), version)
}

func newRouter(app *service.API, taskAPI *service.TaskAPI, p7 *service.P7Service, importer *service.ImportService, version string) http.Handler {
	mux := http.NewServeMux()
	registerSystemRoutes(mux, app, version)
	registerDashboardRoutes(mux, app)
	registerTaskRoutes(mux, app, taskAPI)
	registerPackRoutes(mux, app)
	registerModRoutes(mux, app)
	registerContentRoutes(mux, app)
	registerModContentRoutes(mux, app)
	registerCatalogRoutes(mux, app)
	registerPublishRoutes(mux, app, taskAPI, p7, version)
	registerImportRoutes(mux, importer)
	registerFSRoutes(mux, app)
	return requestIDMiddleware(accessLogMiddleware(recoverMiddleware(maxBodyMiddleware(securityMiddleware(fallbackEnvelopeMiddleware(mux))))))
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
	case errors.Is(err, service.ErrExportDirConflict):
		apiError(w, r, http.StatusConflict, "export_dir_conflict", "this directory is already approved under another name")
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
	case service.IsPackHasActiveTasks(err):
		apiError(w, r, http.StatusConflict, "pack_has_active_tasks", "pack still has queued or running tasks; wait for them to finish before deleting")
	case service.IsConflict(err):
		apiError(w, r, http.StatusConflict, "conflict", "resource conflict")
	case errors.Is(err, service.ErrInvalidArgument):
		apiError(w, r, http.StatusBadRequest, "invalid_argument", "request argument is invalid")
	// mod content extraction
	case errors.Is(err, service.ErrModContentNotParsed):
		apiError(w, r, http.StatusNotFound, "content_not_parsed", "mod content has not been parsed yet")
	case errors.Is(err, service.ErrModContentAlreadyCurrent):
		apiError(w, r, http.StatusConflict, "parse_already_current", "content already parsed for this jar version")
	case errors.Is(err, service.ErrJarBytesUnavailable):
		apiError(w, r, http.StatusUnprocessableEntity, "jar_bytes_unavailable", "mod jar bytes are not available for parsing")
	case errors.Is(err, service.ErrInvalidContentKind):
		apiError(w, r, http.StatusBadRequest, "invalid_kind", "content kind is not valid")
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

// securityMiddleware guards the loopback boundary, not user identity.
//
// Why no write token (user ruling 2026-10-03): this is a single-user local IDE.
// The server binds 127.0.0.1 only, so the LAN cannot reach it at all, and the
// frontend is served from the same loopback origin. A token added no real
// protection — worse, the "how does the frontend obtain the token" chain was
// itself the source of a 401 bug (vite once read a stale runtime-token belonging
// to a different instance: reads worked, every write returned 401).
//
// What is kept, and why it still matters even on loopback:
//   - Host/Origin checks. A malicious web page in the user's own browser can
//     issue requests to 127.0.0.1 (that's what DNS rebinding is). Same-origin
//     policy does not stop a page from *sending* requests, only from reading
//     the responses. The Origin check rejects those cross-site writes.
//   - If the bind address is ever widened past loopback, cross-site request
//     forgery becomes real and there is no token left to stop it. Treat widening
//     the bind address as requiring a security review.
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validHost(r.Host) {
			apiError(w, r, http.StatusBadRequest, "invalid_host", "request host is not allowed")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !validOrigin(origin, r.Host) {
			apiError(w, r, http.StatusForbidden, "invalid_origin", "request origin is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Host 校验防的是 DNS rebinding:攻击者域名重绑到本机后,浏览器送来的 Host 是
// 域名而不是地址。这层不能因为「只监听回环」而删 —— 浏览器里任意网页都能向
// 127.0.0.1 发请求,同源策略挡不住「发出去」,只挡得住「读到响应」。
func validHost(host string) bool {
	h := hostName(host)
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	return inList(h, csvEnv("MPACK_ALLOWED_HOSTS"))
}

// Origin 校验防的是跨站请求:同源即合法(含从另一台设备用局域网地址打开),
// 回环别名 localhost/127.0.0.1/::1 视为同一台机器,其余靠 MPACK_FRONTEND_ORIGIN 显式放行。
func validOrigin(raw, requestHost string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	oh, rh := hostName(u.Host), hostName(requestHost)
	if oh != "" && strings.EqualFold(oh, rh) {
		return true
	}
	if isLoopbackName(oh) && isLoopbackName(rh) {
		return true
	}
	for _, allowed := range csvEnv("MPACK_FRONTEND_ORIGIN") {
		if strings.EqualFold(raw, allowed) {
			return true
		}
	}
	return false
}

func hostName(host string) string {
	h := host
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i+1:], "]") {
		h = h[:i]
	}
	return strings.Trim(h, "[]")
}

func isLoopbackName(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func csvEnv(key string) []string {
	var out []string
	for _, item := range strings.Split(os.Getenv(key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func inList(h string, list []string) bool {
	return slices.ContainsFunc(list, func(item string) bool {
		return strings.EqualFold(h, item)
	})
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
