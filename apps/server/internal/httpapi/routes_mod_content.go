package httpapi

// routes_mod_content.go registers the HTTP endpoints for mod content
// extraction: trigger a parse, list extracted content, get a single item,
// and inspect the latest parse run.

import (
	"mpackstation/internal/service"
	"net/http"
	"strconv"
)

func registerModContentRoutes(mux *http.ServeMux, app *service.API) {
	mux.HandleFunc("POST /api/packs/{packId}/mods/{modId}/content/icons/resolve", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.ResolveModContentIcons(r.Context(), r.PathValue("packId"), r.PathValue("modId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, result)
	})
	// Trigger a parse task for one installed mod. ?force=true re-parses even
	// when the same jar sha1 is already current (used after classifier fixes).
	mux.HandleFunc("POST /api/packs/{packId}/mods/{modId}/content/parse", func(w http.ResponseWriter, r *http.Request) {
		force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
		t, submitted, err := app.SubmitParseModContentOpts(r.Context(), r.PathValue("packId"), r.PathValue("modId"), force)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if !submitted {
			// Idempotent skip: same sha1 already parsed.
			apiError(w, r, http.StatusConflict, "parse_already_current", "content already parsed for this jar version")
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"taskId": t.ID, "status": t.Status})
	})

	// List extracted content for a mod, optionally filtered by kind.
	mux.HandleFunc("GET /api/packs/{packId}/mods/{modId}/content", func(w http.ResponseWriter, r *http.Request) {
		kind := r.URL.Query().Get("kind")
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		cursor := r.URL.Query().Get("cursor")
		items, nextCursor, total, run, err := app.ListModContent(r.Context(), r.PathValue("packId"), r.PathValue("modId"), kind, limit, cursor)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"items":       items,
			"next_cursor": nextCursorOrNull(nextCursor),
			"total":       total,
			"run":         run,
		})
	})

	// Get the latest parse run status.
	mux.HandleFunc("GET /api/packs/{packId}/mods/{modId}/content/run", func(w http.ResponseWriter, r *http.Request) {
		run, err := app.GetModContentRun(r.Context(), r.PathValue("packId"), r.PathValue("modId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"run": run})
	})

	// Get a single extracted content item by id.
	mux.HandleFunc("GET /api/packs/{packId}/mods/{modId}/content/{contentId}", func(w http.ResponseWriter, r *http.Request) {
		item, err := app.GetModContent(r.Context(), r.PathValue("packId"), r.PathValue("modId"), r.PathValue("contentId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"item": item})
	})
}

// nextCursorOrNull returns nil for an empty cursor so the JSON field is null
// rather than an empty string.
func nextCursorOrNull(cursor string) any {
	if cursor == "" {
		return nil
	}
	return cursor
}
