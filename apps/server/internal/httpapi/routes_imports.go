package httpapi

import (
	"encoding/base64"
	"mpackstation/internal/service"
	"net/http"
)

func registerImportRoutes(mux *http.ServeMux, importer *service.ImportService) {
	mux.HandleFunc("POST /api/packs/import/inspect", func(w http.ResponseWriter, r *http.Request) {
		if importer == nil {
			writeError(w, r, service.ErrUnavailable)
			return
		}
		var body struct{ Source, URL, Content string }
		if !decodeJSON(w, r, &body) {
			return
		}
		content, err := base64.StdEncoding.DecodeString(body.Content)
		if body.Source == service.ImportSourceLocalZip && err != nil {
			apiError(w, r, http.StatusBadRequest, "invalid_argument", "content must be base64")
			return
		}
		v, err := importer.Inspect(r.Context(), service.ImportPreviewInput{Source: body.Source, URL: body.URL, Content: content})
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/import", func(w http.ResponseWriter, r *http.Request) {
		if importer == nil {
			writeError(w, r, service.ErrUnavailable)
			return
		}
		var body struct{ PreviewID, Token, InputHash, IdempotencyKey string }
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		v, reused, err := importer.Confirm(r.Context(), service.ImportConfirmInput{PreviewID: body.PreviewID, Token: body.Token, InputHash: body.InputHash, IdempotencyKey: body.IdempotencyKey})
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"importId": body.PreviewID, "taskId": v.ID, "packId": v.PackID, "reused": reused})
	})
}
