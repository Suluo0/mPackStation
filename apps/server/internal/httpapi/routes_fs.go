package httpapi

import (
	"mpackstation/internal/service"
	"net/http"
)

func registerFSRoutes(mux *http.ServeMux, app *service.API) {
	// Local path picker support for launcher / export directory fields.
	// GET is enough for browsing; registration of export dirs stays on POST /api/export-dirs.
	mux.HandleFunc("GET /api/fs/browse", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.BrowseDirectories(r.Context(), r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
}
