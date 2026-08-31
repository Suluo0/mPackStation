package httpapi

import (
	"mpackstation/internal/service"
	"net/http"
)

func registerDashboardRoutes(mux *http.ServeMux, app *service.API) {
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.Dashboard(r.Context())
		if err != nil {
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
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
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
		}
	})
	mux.HandleFunc("GET /api/onboarding", func(w http.ResponseWriter, r *http.Request) {
		v, err := app.Onboarding(r.Context())
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		v, err := app.Onboarding(r.Context())
		if err != nil {
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, v)
		}
	})
}
