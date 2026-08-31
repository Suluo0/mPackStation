package httpapi

import (
	"encoding/json"
	"mpackstation/internal/service"
	"net/http"
)

func registerContentRoutes(mux *http.ServeMux, app *service.API) {
	mux.HandleFunc("GET /api/packs/{packId}/content", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ListContent(r.Context(), r.PathValue("packId"), r.URL.Query().Get("kind"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"document": doc, "revision": rev})
	})
	mux.HandleFunc("GET /api/packs/{packId}/content/{documentId}", func(w http.ResponseWriter, r *http.Request) {
		doc, rev, err := app.GetContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, rev)
	})
	mux.HandleFunc("POST /api/packs/{packId}/content/{documentId}/validate", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ValidateContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"), r.URL.Query().Get("revisionId"), RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/content/{documentId}/apply", func(w http.ResponseWriter, r *http.Request) {
		rev, err := app.ApplyContent(r.Context(), r.PathValue("packId"), r.PathValue("documentId"), r.URL.Query().Get("revisionId"), RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "applied", "revision": rev})
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
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}/content/{documentId}/history", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ContentHistory(r.Context(), r.PathValue("packId"), r.PathValue("documentId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/quests", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.GetQuest(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"revision": v, "issues": issues})
	})
	mux.HandleFunc("POST /api/packs/{packId}/quests/validate", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.ValidateQuest(r.Context(), r.PathValue("packId"), RequestID(r.Context()))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"issues": v})
	})
	mux.HandleFunc("POST /api/packs/{packId}/quests/apply", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ApplyQuest(r.Context(), r.PathValue("packId"), RequestID(r.Context())); err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/packs/{packId}/quests/history", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.QuestHistory(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/quests/preview", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.QuestPreview(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
}
