package httpapi

import (
	"net/http"
	"strings"

	"mpackstation/internal/service"
)

func registerCatalogRoutes(mux *http.ServeMux, app *service.API) {
	mux.HandleFunc("GET /api/packs/{packId}/catalog/icon", func(w http.ResponseWriter, r *http.Request) {
		icon, err := app.GetCatalogIcon(r.Context(), r.PathValue("packId"), r.URL.Query().Get("itemId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", icon.Mime)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(icon.Data)
	})
	mux.HandleFunc("GET /api/packs/{packId}/catalog/status", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.GetCatalogStatus(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/packs/{packId}/catalog/rebuild", func(w http.ResponseWriter, r *http.Request) {
		item, err := app.SubmitCatalogInit(r.Context(), r.PathValue("packId"), r.URL.Query().Get("locale"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"taskId": item.ID, "status": item.Status})
	})
	mux.HandleFunc("GET /api/packs/{packId}/catalog", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.GetItemCatalog(r.Context(), r.PathValue("packId"), r.URL.Query().Get("locale"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /api/packs/{packId}/catalog/items/{itemId...}", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.GetCatalogItem(r.Context(), r.PathValue("packId"), strings.TrimPrefix(r.PathValue("itemId"), "/"), r.URL.Query().Get("locale"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /api/packs/{packId}/catalog/tags/{tagId...}", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.GetCatalogTag(r.Context(), r.PathValue("packId"), r.URL.Query().Get("registry"), strings.TrimPrefix(r.PathValue("tagId"), "/"), r.URL.Query().Get("locale"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, result)
	})
}
