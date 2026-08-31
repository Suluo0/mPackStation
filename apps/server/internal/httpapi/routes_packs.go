package httpapi

import (
	"mpackstation/internal/service"
	"net/http"
)

func registerPackRoutes(mux *http.ServeMux, app *service.API) {
	mux.HandleFunc("GET /api/packs", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListPacks(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs", func(w http.ResponseWriter, r *http.Request) {
		var body service.CreatePackInput
		if !decodeJSON(w, r, &body) {
			return
		}
		if !checkMCVersionCandidate(w, r, app, body.MCVersion) {
			return
		}
		v, err := app.CreatePack(r.Context(), body, RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.GetPack(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("PATCH /api/packs/{packId}", func(w http.ResponseWriter, r *http.Request) {
		var body service.UpdatePackInput
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.MCVersion != nil && !checkMCVersionCandidate(w, r, app, *body.MCVersion) {
			return
		}
		v, err := app.UpdatePack(r.Context(), r.PathValue("packId"), body, RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/archive", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ArchivePack(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/unarchive", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.UnarchivePack(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("DELETE /api/packs/{packId}", func(w http.ResponseWriter, r *http.Request) {
		if err := app.DeletePack(r.Context(), r.PathValue("packId"), RequestID(r.Context())); err != nil {
			writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
