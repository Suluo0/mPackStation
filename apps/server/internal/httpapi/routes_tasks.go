package httpapi

import (
	"encoding/json"
	"mpackstation/internal/service"
	"net/http"
)

func registerTaskRoutes(mux *http.ServeMux, app *service.API, taskAPI *service.TaskAPI) {
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
			writeError(w, r, err)
		} else {
			WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
		}
	})
	mux.HandleFunc("GET /api/tasks/{taskId}", func(w http.ResponseWriter, r *http.Request) {
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), r.PathValue("taskId"))
		if err != nil {
			writeError(w, r, err)
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
			writeError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		for _, event := range events {
			if err := json.NewEncoder(w).Encode(event); err != nil {
				return
			}
		}
	})
}
