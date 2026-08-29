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
	return newRouter(service.NewFromSource(source), service.NewTaskAPI(source), version)
}

// NewRouterWithService is useful to tests and future composition roots.
func NewRouterWithService(app *service.API, version string) http.Handler {
	return newRouter(app, nil, version)
}

func newRouter(app *service.API, taskAPI *service.TaskAPI, version string) http.Handler {
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
	mux.HandleFunc("GET /api/tasks/{taskId}", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeTaskError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/tasks/{taskId}/pause", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		if err := taskAPI.Pause(r.Context(), r.PathValue("taskId")); err != nil {
			writeTaskError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeTaskError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/tasks/{taskId}/resume", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		if err := taskAPI.Resume(r.Context(), r.PathValue("taskId")); err != nil {
			writeTaskError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeTaskError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/tasks/{taskId}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		if err := taskAPI.Cancel(r.Context(), r.PathValue("taskId")); err != nil {
			writeTaskError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeTaskError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/tasks/{taskId}/retry", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		if err := taskAPI.Retry(r.Context(), r.PathValue("taskId")); err != nil {
			writeTaskError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeTaskError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/tasks/{taskId}/log", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		events, err := taskAPI.Events(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeTaskError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		for _, event := range events {
			if err := json.NewEncoder(w).Encode(event); err != nil {
				return
			}
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
	mux.HandleFunc("GET /api/packs/{packId}/mods", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListPackMods(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/mods", func(w http.ResponseWriter, r *http.Request) {
		var body service.AddModInput
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.AddPackMod(r.Context(), r.PathValue("packId"), body, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/mods/local", func(w http.ResponseWriter, r *http.Request) {
		var body service.LocalModInput
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.AddLocalPackMod(r.Context(), r.PathValue("packId"), body, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("PATCH /api/packs/{packId}/mods/{modId}", func(w http.ResponseWriter, r *http.Request) {
		var body service.UpdateModInput
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.UpdatePackMod(r.Context(), r.PathValue("packId"), r.PathValue("modId"), body, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("DELETE /api/packs/{packId}/mods/{modId}", func(w http.ResponseWriter, r *http.Request) {
		if err := app.RemovePackMod(r.Context(), r.PathValue("packId"), r.PathValue("modId"), RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/packs/{packId}/mod-search", func(w http.ResponseWriter, r *http.Request) {
		in := service.ModSearchInput{Provider: r.URL.Query().Get("provider"), Query: r.URL.Query().Get("query"), MCVersion: r.URL.Query().Get("mcVersion"), Loader: r.URL.Query().Get("loader"), Cursor: r.URL.Query().Get("cursor"), Limit: queryLimit(r, "limit", 20)}
		if in.Limit < 0 {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "limit must be a positive integer")
			return
		}
		v, err := app.ModSearch(r.Context(), r.PathValue("packId"), in)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/resolve", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ResolvePack(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"lock": v, "status": "resolved"})
	})
	mux.HandleFunc("GET /api/packs/{packId}/locks", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListLocks(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/conflicts", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListConflicts(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/conflicts/{conflictId}/resolve", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ResolveConflict(r.Context(), r.PathValue("packId"), r.PathValue("conflictId"), "resolved", RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "resolved"})
	})
	mux.HandleFunc("POST /api/packs/{packId}/conflicts/{conflictId}/ignore", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ResolveConflict(r.Context(), r.PathValue("packId"), r.PathValue("conflictId"), "ignored", RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
	})
	mux.HandleFunc("GET /api/packs/{packId}/health", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.PackHealth(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}/content", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListContent(r.Context(), r.PathValue("packId"), r.URL.Query().Get("kind"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/content", func(w http.ResponseWriter, r *http.Request) {
		var body service.CreateContentInput
		if !decodeJSON(w, r, &body) {
			return
		}
		doc, rev, err := app.CreateContent(r.Context(), r.PathValue("packId"), body, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"document": doc, "revision": rev})
	})
	mux.HandleFunc("GET /api/packs/{packId}/content/{documentId}", func(w http.ResponseWriter, r *http.Request) {
		doc, rev, err := app.GetContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"document": doc, "revision": rev})
	})
	mux.HandleFunc("PUT /api/packs/{packId}/content/{documentId}/draft", func(w http.ResponseWriter, r *http.Request) {
		match, ok := parseIfMatch(r)
		if !ok {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "If-Match must be a non-negative revision")
			return
		}
		var body struct {
			Payload json.RawMessage `json:"payload"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		rev, err := app.SaveContentDraft(r.Context(), r.PathValue("packId"), r.PathValue("documentId"), service.SaveContentDraftInput{IfMatch: match, Payload: body.Payload}, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, rev)
	})
	mux.HandleFunc("POST /api/packs/{packId}/content/{documentId}/validate", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ValidateContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"), r.URL.Query().Get("revisionId"), RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/content/{documentId}/apply", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ApplyContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"), r.URL.Query().Get("revisionId"), RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "applied"})
	})
	mux.HandleFunc("POST /api/packs/{packId}/content/{documentId}/rollback", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RevisionID string `json:"revisionId"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.RollbackContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"), body.RevisionID, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}/content/{documentId}/history", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ContentHistory(r.Context(), r.PathValue("packId"), r.PathValue("documentId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/quests", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.GetQuest(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("PUT /api/packs/{packId}/quests/draft", func(w http.ResponseWriter, r *http.Request) {
		match, ok := parseIfMatch(r)
		if !ok {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "If-Match must be a non-negative revision")
			return
		}
		var body service.QuestDraft
		if !decodeJSON(w, r, &body) {
			return
		}
		v, issues, err := app.SaveQuestDraft(r.Context(), r.PathValue("packId"), body, match, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"revision": v, "issues": issues})
	})
	mux.HandleFunc("POST /api/packs/{packId}/quests/validate", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ValidateQuest(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"issues": v})
	})
	mux.HandleFunc("POST /api/packs/{packId}/quests/apply", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ApplyQuest(r.Context(), r.PathValue("packId"), RequestID(r.Context())); err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "applied"})
	})
	mux.HandleFunc("POST /api/packs/{packId}/quests/rollback", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RevisionID string `json:"revisionId"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.RollbackQuest(r.Context(), r.PathValue("packId"), body.RevisionID, RequestID(r.Context()))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}/quests/history", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.QuestHistory(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/quests/preview", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.QuestPreview(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
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
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrProviderNotFound):
		apiError(w, r, http.StatusNotFound, "provider_not_found", "provider resource not found")
	case errors.Is(err, service.ErrProviderUnavailable):
		apiError(w, r, http.StatusBadGateway, "provider_unavailable", "provider is unavailable")
	case errors.Is(err, service.ErrInvalidSHA1):
		apiError(w, r, http.StatusBadRequest, "invalid_sha1", "provider returned an invalid SHA-1")
	case errors.Is(err, service.ErrRevisionConflict):
		apiError(w, r, http.StatusConflict, "revision_conflict", "resource revision is stale")
	case errors.Is(err, service.ErrValidationFailed):
		apiError(w, r, http.StatusUnprocessableEntity, "validation_failed", "resource validation failed")
	case errors.Is(err, service.ErrCrossPackReference):
		apiError(w, r, http.StatusBadRequest, "cross_pack_reference", "resource references another pack")
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

func writeTaskError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case service.IsTaskNotFound(err):
		apiError(w, r, http.StatusNotFound, "task_not_found", "task not found")
	case service.IsTaskInvalidTransition(err):
		apiError(w, r, http.StatusConflict, "task_invalid_transition", "task state transition is not allowed")
	case service.IsTaskLeaseLost(err):
		apiError(w, r, http.StatusConflict, "task_lease_lost", "task lease is no longer valid")
	case service.IsTaskNotAvailable(err):
		apiError(w, r, http.StatusConflict, "task_not_available", "task is not available")
	case service.IsTaskIdempotencyConflict(err):
		apiError(w, r, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with different input")
	case service.IsTaskIdempotencyConsumed(err):
		apiError(w, r, http.StatusConflict, "idempotency_consumed", "idempotency key was already consumed")
	case service.IsTaskUnknownKind(err):
		apiError(w, r, http.StatusInternalServerError, "task_unknown_kind", "task kind is not registered")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		apiError(w, r, http.StatusRequestTimeout, "request_canceled", "task request was canceled")
	default:
		if service.IsTaskUnavailable(err) {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
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
