package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"mpackstation/internal/service"
	"net/http"
)

func registerPublishRoutes(mux *http.ServeMux, app *service.API, taskAPI *service.TaskAPI, p7 *service.P7Service, version string) {
	mux.HandleFunc("POST /api/export-dirs", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Name, Directory string }
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := app.RegisterExportDirectory(r.Context(), body.Name, body.Directory); err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"name": body.Name, "status": "ready"})
	})
	mux.HandleFunc("GET /api/packs/{packId}/delivery-checks", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		v, err := p7.ListDeliveryChecks(r.Context(), r.PathValue("packId"), r.URL.Query().Get("packVersionId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/delivery-checks/run", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		var body struct {
			PackVersionID string                  `json:"packVersionId"`
			Checks        []service.DeliveryCheck `json:"checks"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := p7.RunDeliveryChecks(r.Context(), r.PathValue("packId"), body.PackVersionID, body.Checks)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v})
	})
	mux.HandleFunc("GET /api/packs/{packId}/versions", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		v, err := app.ListPackVersions(r.Context(), r.PathValue("packId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/packs/{packId}/versions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Version   string `json:"version"`
			Channel   string `json:"channel"`
			Changelog string `json:"changelog"`
			Source    string `json:"source"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := app.CreatePackVersion(r.Context(), r.PathValue("packId"), body.Version, body.Channel, body.Changelog, body.Source)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/versions/{versionId}/build", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExportDirName   string                           `json:"exportDirName"`
			Files           []struct{ Path, Content string } `json:"files"`
			LockSnapshot    json.RawMessage                  `json:"lockSnapshot"`
			ContentSnapshot json.RawMessage                  `json:"contentSnapshot"`
			QuestSnapshot   json.RawMessage                  `json:"questSnapshot"`
			BuildConfig     json.RawMessage                  `json:"buildConfig"`
			Checks          []service.DeliveryCheck          `json:"checks"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		files := make([]service.BuildFile, 0, len(body.Files))
		for _, file := range body.Files {
			content, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil {
				apiError(w, r, http.StatusBadRequest, "invalid_argument", "file content must be base64")
				return
			}
			files = append(files, service.BuildFile{Path: file.Path, Content: content})
		}
		result, err := app.BuildPack(r.Context(), service.BuildInput{PackID: r.PathValue("packId"), PackVersionID: r.PathValue("versionId"), ExportDirName: body.ExportDirName, Files: files, LockSnapshot: body.LockSnapshot, ContentSnapshot: body.ContentSnapshot, QuestSnapshot: body.QuestSnapshot, BuildConfig: body.BuildConfig, Checks: body.Checks})
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, result)
	})
	mux.HandleFunc("POST /api/packs/{packId}/build", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PackVersionID string `json:"packVersionId"`
			ExportDirName string `json:"exportDirName"`
			Files         []struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"files"`
			LockSnapshot    json.RawMessage         `json:"lockSnapshot"`
			ContentSnapshot json.RawMessage         `json:"contentSnapshot"`
			QuestSnapshot   json.RawMessage         `json:"questSnapshot"`
			BuildConfig     json.RawMessage         `json:"buildConfig"`
			Checks          []service.DeliveryCheck `json:"checks"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		files := make([]service.BuildFile, 0, len(body.Files))
		for _, f := range body.Files {
			b, e := base64.StdEncoding.DecodeString(f.Content)
			if e != nil {
				apiError(w, r, http.StatusBadRequest, "invalid_argument", "file content must be base64")
				return
			}
			files = append(files, service.BuildFile{Path: f.Path, Content: b})
		}
		result, err := app.BuildPack(r.Context(), service.BuildInput{PackID: r.PathValue("packId"), PackVersionID: body.PackVersionID, ExportDirName: body.ExportDirName, Files: files, LockSnapshot: body.LockSnapshot, ContentSnapshot: body.ContentSnapshot, QuestSnapshot: body.QuestSnapshot, BuildConfig: body.BuildConfig, Checks: body.Checks})
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusCreated, result)
	})
	mux.HandleFunc("GET /api/packs/{packId}/artifacts", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		v, err := p7.ListArtifacts(r.Context(), r.PathValue("packId"), r.URL.Query().Get("packVersionId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("GET /api/packs/{packId}/artifacts/{artifactId}/download", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		a, b, err := p7.ReadArtifact(r.Context(), r.PathValue("packId"), r.PathValue("artifactId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+a.FileName+"\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /api/packs/{packId}/releases", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		v, err := p7.ListReleases(r.Context(), r.PathValue("packId"), r.URL.Query().Get("packVersionId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": v, "next_cursor": nil, "total": len(v)})
	})
	mux.HandleFunc("POST /api/releases", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		var body service.PublishInput
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := p7.PublishPack(r.Context(), body)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, v)
	})
	// Canonical asynchronous publication route. The legacy /api/releases route
	// remains synchronous for compatibility; this endpoint only persists a
	// publish task and returns its observable task state.
	mux.HandleFunc("POST /api/releases/async", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		var body service.PublishInput
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		t, _, err := p7.SubmitPublishTask(r.Context(), body)
		if err != nil {
			writeError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), t.ID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/publish/{provider}", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		var body struct {
			PackVersionID  string `json:"packVersionId"`
			ArtifactID     string `json:"artifactId"`
			IdempotencyKey string `json:"idempotencyKey"`
			ProjectID      string `json:"projectId"`
			VersionID      string `json:"versionId"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		v, err := p7.PublishPack(r.Context(), service.PublishInput{PackID: r.PathValue("packId"), PackVersionID: body.PackVersionID, Provider: r.PathValue("provider"), ArtifactID: body.ArtifactID, IdempotencyKey: body.IdempotencyKey, ProjectID: body.ProjectID, VersionID: body.VersionID})
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, v)
	})
	mux.HandleFunc("POST /api/packs/{packId}/publish/{provider}/async", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		if taskAPI == nil {
			apiError(w, r, http.StatusServiceUnavailable, "not_ready", "task service is not ready")
			return
		}
		var body struct{ PackVersionID, ArtifactID, IdempotencyKey, ProjectID, VersionID string }
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		t, _, err := p7.SubmitPublishTask(r.Context(), service.PublishInput{PackID: r.PathValue("packId"), PackVersionID: body.PackVersionID, Provider: r.PathValue("provider"), ArtifactID: body.ArtifactID, IdempotencyKey: body.IdempotencyKey, ProjectID: body.ProjectID, VersionID: body.VersionID})
		if err != nil {
			writeError(w, r, err)
			return
		}
		v, err := taskAPI.Get(r.Context(), t.ID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, v)
	})
	mux.HandleFunc("GET /api/releases/{releaseId}", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		v, err := p7.GetRelease(r.Context(), r.PathValue("releaseId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/releases/{releaseId}/poll", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		v, err := p7.PollRelease(r.Context(), r.PathValue("releaseId"))
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /api/releases/{releaseId}/retry", func(w http.ResponseWriter, r *http.Request) {
		if err := p7Ready(p7); err != nil {
			writeError(w, r, err)
			return
		}
		var body struct{ ProjectID, VersionID string }
		if !decodeJSON(w, r, &body) {
			return
		}
		v, err := p7.RetryPublish(r.Context(), r.PathValue("releaseId"), body.ProjectID, body.VersionID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, v)
	})
}
