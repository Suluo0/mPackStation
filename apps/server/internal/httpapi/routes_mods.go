package httpapi

import (
	"mpackstation/internal/service"
	"net/http"
	"strings"
)

func registerModRoutes(mux *http.ServeMux, app *service.API) {
	mux.HandleFunc("GET /api/packs/{packId}/mods", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListPackMods(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
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
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("DELETE /api/packs/{packId}/mods/{modId}", func(w http.ResponseWriter, r *http.Request) {
		if err := app.RemovePackMod(r.Context(), r.PathValue("packId"), r.PathValue("modId"), RequestID(r.Context())); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/packs/{packId}/mod-search", func(w http.ResponseWriter, r *http.Request) {
		// Contract query param is `q`; `query` kept as a legacy alias.
		q := r.URL.Query().Get("q")
		if q == "" {
			q = r.URL.Query().Get("query")
		}
		in := service.ModSearchInput{Provider: r.URL.Query().Get("provider"), Query: q, MCVersion: r.URL.Query().Get("mcVersion"), Loader: r.URL.Query().Get("loader"), Cursor: r.URL.Query().Get("cursor"), Limit: queryLimit(r, "limit", 20)}
		if in.Limit < 0 {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "limit must be a positive integer")
			return
		}
		// No provider param: fan the query out to every platform concurrently.
		if strings.TrimSpace(in.Provider) == "" {
			v, err := app.ModSearchAll(r.Context(), r.PathValue("packId"), in)
			if err != nil {
				writeError(w, r, err)
				return
			}
			WriteJSON(w, http.StatusOK, v)
			return
		}
		v, err := app.ModSearch(r.Context(), r.PathValue("packId"), in)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}/mod-versions", func(w http.ResponseWriter, r *http.Request) {
		provider, projectID := r.URL.Query().Get("provider"), r.URL.Query().Get("projectId")
		if strings.TrimSpace(provider) == "" || strings.TrimSpace(projectID) == "" {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "provider and projectId are required")
			return
		}
		v, err := app.ModVersions(r.Context(), r.PathValue("packId"), provider, projectID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/resolve", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ResolvePack(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"lock": v, "status": "resolved"})
	})
	mux.HandleFunc("GET /api/packs/{packId}/locks", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListLocks(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/conflicts", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListConflicts(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/conflicts/{conflictId}/resolve", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ResolveConflict(r.Context(), r.PathValue("packId"), r.PathValue("conflictId"), "resolved", RequestID(r.Context())); err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "resolved"})
	})
	mux.HandleFunc("POST /api/packs/{packId}/conflicts/{conflictId}/ignore", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ResolveConflict(r.Context(), r.PathValue("packId"), r.PathValue("conflictId"), "ignored", RequestID(r.Context())); err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
	})
	mux.HandleFunc("GET /api/packs/{packId}/health", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.PackHealth(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
}
