package httpapi

import (
	"mpackstation/internal/service"
	"net/http"
	"time"
)

func registerSystemRoutes(mux *http.ServeMux, app *service.API, version string) {
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
	mux.HandleFunc("GET /api/system/health", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.SystemHealth(r.Context())
		if err != nil {
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	mux.HandleFunc("GET /api/system/status", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.SystemStatus(r.Context())
		if err != nil {
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
	// CurseForge key management (settings page). PUT validates the key against
	// the live platform before persisting; DELETE clears the stored key. Both
	// take effect immediately, no restart needed. The key value is never
	// returned by any read endpoint.
	mux.HandleFunc("PUT /api/system/providers/curseforge/key", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Key string `json:"key"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if err := app.SetCurseForgeKey(r.Context(), in.Key); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/system/providers/curseforge/key", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ClearCurseForgeKey(r.Context()); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/tools/prism/install", func(w http.ResponseWriter, r *http.Request) {
		t, reused, err := app.SubmitPrismInstall(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		if t == nil {
			WriteJSON(w, http.StatusOK, map[string]any{"started": false, "reason": "already_installed"})
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"started": true, "reused": reused, "taskId": t.ID})
	})
	mux.HandleFunc("POST /api/tools/prism/login", func(w http.ResponseWriter, r *http.Request) {
		if err := app.LaunchPrismLogin(r.Context()); err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"launched": true})
	})
	mux.HandleFunc("GET /api/meta/mc-versions", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.MCVersions(r.Context())
		if err != nil {
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
}
