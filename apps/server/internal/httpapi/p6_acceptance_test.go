package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mpackstation/internal/service"
	"mpackstation/internal/store"
)

// P6 HTTP acceptance deliberately follows the v7 contract, including the
// plural /quests resource and PUT draft route. It is independent from the
// service tests: a passing service test cannot prove that the frontend can
// reach the use-case through the documented HTTP boundary.
func p6HTTPFixture(t *testing.T) (*service.API, *sql.DB, string, http.Handler) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "p6-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := service.New(db)
	pack, err := app.CreatePack(context.Background(), service.CreatePackInput{
		Name: "P6 HTTP", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15",
	}, "p6-http-pack")
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return app, db, pack.ID, NewRouterWithService(app, "test")
}

type p6ErrorEnvelope struct {
	Error struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		RequestID string         `json:"request_id"`
		Details   map[string]any `json:"details"`
	} `json:"error"`
}

func p6HTTPDo(t *testing.T, handler http.Handler, method, path, requestID string, body string, write bool, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Host = "localhost"
	req.Header.Set("Origin", "http://localhost")
	req.Header.Set("X-Request-ID", requestID)
	if write {
		req.Header.Set("X-MPack-Token", "test")
		req.Header.Set("Content-Type", "application/json")
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func p6RequireError(t *testing.T, res *httptest.ResponseRecorder, status int, code, requestID string) p6ErrorEnvelope {
	t.Helper()
	if res.Code != status {
		t.Fatalf("status=%d body=%s, want %d/%s", res.Code, res.Body.String(), status, code)
	}
	var envelope p6ErrorEnvelope
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v; body=%s", err, res.Body.String())
	}
	if envelope.Error.Code != code || envelope.Error.RequestID != requestID {
		t.Fatalf("error=%#v, want code=%q request_id=%q", envelope.Error, code, requestID)
	}
	if res.Header().Get("X-Request-ID") != requestID {
		t.Fatalf("response request id=%q, want %q", res.Header().Get("X-Request-ID"), requestID)
	}
	return envelope
}

func TestP6HTTPContentRevisionContract(t *testing.T) {
	_, _, packID, handler := p6HTTPFixture(t)
	base := "/api/packs/" + packID + "/content"
	payload := `{"schema_version":1,"type":"shaped","input":[{"item":"minecraft:iron_ingot"}],"output":{"item":"minecraft:iron_block","count":1}}`
	res := p6HTTPDo(t, handler, http.MethodPost, base, "p6-content-create", `{"kind":"recipe","slug":"iron","title":"Iron","payload":`+payload+`}`, true, "")
	if res.Code != http.StatusCreated || res.Header().Get("X-Request-ID") != "p6-content-create" {
		t.Fatalf("create status=%d body=%s request_id=%q", res.Code, res.Body.String(), res.Header().Get("X-Request-ID"))
	}
	var created struct {
		Document service.ContentDocument `json:"document"`
		Revision service.ContentRevision `json:"revision"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Document.ID == "" || created.Revision.ID == "" || created.Revision.Revision != 1 || created.Revision.State != "draft" {
		t.Fatalf("create=%#v", created)
	}
	var canonical map[string]any
	if err := json.Unmarshal(created.Revision.Payload, &canonical); err != nil || canonical["schema_version"] != float64(1) {
		t.Fatalf("canonical payload=%s err=%v", created.Revision.Payload, err)
	}

	res = p6HTTPDo(t, handler, http.MethodGet, base, "p6-content-list", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", res.Code, res.Body.String())
	}
	var list struct {
		Items      []service.ContentDocument `json:"items"`
		NextCursor any                       `json:"next_cursor"`
		Total      int                       `json:"total"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Total != 1 || list.Items[0].ID != created.Document.ID {
		t.Fatalf("list=%#v", list)
	}

	detailPath := base + "/" + created.Document.ID
	res = p6HTTPDo(t, handler, http.MethodGet, detailPath, "p6-content-get", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", res.Code, res.Body.String())
	}
	var detail struct {
		Document service.ContentDocument `json:"document"`
		Revision service.ContentRevision `json:"revision"`
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Document.ID != created.Document.ID || detail.Revision.ID != created.Revision.ID {
		t.Fatalf("detail=%#v", detail)
	}

	updatedPayload := `{"schema_version":1,"type":"shapeless","input":[{"item":"minecraft:iron_ingot"}],"output":{"item":"minecraft:iron_nugget","count":9}}`
	draftPath := detailPath + "/draft"
	res = p6HTTPDo(t, handler, http.MethodPut, draftPath, "p6-content-draft", `{"payload":`+updatedPayload+`}`, true, `"1"`)
	if res.Code != http.StatusOK {
		t.Fatalf("draft status=%d body=%s", res.Code, res.Body.String())
	}
	var revision2 service.ContentRevision
	if err := json.NewDecoder(res.Body).Decode(&revision2); err != nil {
		t.Fatal(err)
	}
	if revision2.Revision != 2 || revision2.State != "draft" {
		t.Fatalf("draft=%#v", revision2)
	}

	res = p6HTTPDo(t, handler, http.MethodPut, draftPath, "p6-content-stale", `{"payload":`+payload+`}`, true, `"1"`)
	p6RequireError(t, res, http.StatusConflict, "revision_conflict", "p6-content-stale")

	res = p6HTTPDo(t, handler, http.MethodPost, detailPath+"/validate?revisionId="+revision2.ID, "p6-content-validate", "", true, "")
	if res.Code != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", res.Code, res.Body.String())
	}
	var validation service.ContentValidation
	if err := json.NewDecoder(res.Body).Decode(&validation); err != nil {
		t.Fatal(err)
	}
	if validation.Status != "passed" || validation.RevisionID != revision2.ID || len(validation.Issues) != 0 {
		t.Fatalf("validation=%#v", validation)
	}

	res = p6HTTPDo(t, handler, http.MethodPost, detailPath+"/apply?revisionId="+revision2.ID, "p6-content-apply", "", true, "")
	if res.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", res.Code, res.Body.String())
	}
	res = p6HTTPDo(t, handler, http.MethodGet, detailPath, "p6-content-active", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("active status=%d body=%s", res.Code, res.Body.String())
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Document.ActiveRevisionID != revision2.ID || detail.Revision.ID != revision2.ID {
		t.Fatalf("active detail=%#v", detail)
	}

	res = p6HTTPDo(t, handler, http.MethodPost, detailPath+"/rollback", "p6-content-rollback", `{"revisionId":"`+created.Revision.ID+`"}`, true, "")
	if res.Code != http.StatusOK {
		t.Fatalf("rollback status=%d body=%s", res.Code, res.Body.String())
	}
	var rollback service.ContentRevision
	if err := json.NewDecoder(res.Body).Decode(&rollback); err != nil {
		t.Fatal(err)
	}
	if rollback.Revision != 3 || rollback.SourceRevisionID != created.Revision.ID || rollback.State != "draft" {
		t.Fatalf("rollback=%#v", rollback)
	}

	res = p6HTTPDo(t, handler, http.MethodGet, detailPath+"/history", "p6-content-history", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", res.Code, res.Body.String())
	}
	var history struct {
		Items []service.ContentRevision `json:"items"`
		Total int                       `json:"total"`
	}
	if err := json.NewDecoder(res.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 3 || len(history.Items) != 3 {
		t.Fatalf("history=%#v", history)
	}
	_ = list.NextCursor
}

func TestP6HTTPContentValidationAndErrorEnvelope(t *testing.T) {
	_, _, packID, handler := p6HTTPFixture(t)
	base := "/api/packs/" + packID + "/content"
	res := p6HTTPDo(t, handler, http.MethodPost, base, "p6-content-invalid-kind", `{"kind":"unknown","slug":"bad","title":"Bad","payload":{}}`, true, "")
	p6RequireError(t, res, http.StatusBadRequest, "invalid_argument", "p6-content-invalid-kind")

	res = p6HTTPDo(t, handler, http.MethodPost, base, "p6-content-invalid-json", `{"kind":"recipe","slug":"bad","title":"Bad","payload":{"schema_version":1`, true, "")
	p6RequireError(t, res, http.StatusBadRequest, "invalid_json", "p6-content-invalid-json")

	ore := `{"kind":"ore","slug":"range","title":"Range","payload":{"schema_version":1,"dimension":"overworld","block":"minecraft:stone","min_y":100,"max_y":1}}`
	res = p6HTTPDo(t, handler, http.MethodPost, base, "p6-ore-create", ore, true, "")
	if res.Code != http.StatusCreated {
		t.Fatalf("ore create status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Document service.ContentDocument `json:"document"`
		Revision service.ContentRevision `json:"revision"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	detailPath := base + "/" + created.Document.ID
	res = p6HTTPDo(t, handler, http.MethodPost, detailPath+"/validate?revisionId="+created.Revision.ID, "p6-ore-validate", "", true, "")
	if res.Code != http.StatusOK {
		t.Fatalf("ore validate status=%d body=%s", res.Code, res.Body.String())
	}
	var validation service.ContentValidation
	if err := json.NewDecoder(res.Body).Decode(&validation); err != nil {
		t.Fatal(err)
	}
	if validation.Status != "failed" || len(validation.Issues) == 0 || validation.Issues[0].Code != "ore_range_invalid" {
		t.Fatalf("ore validation=%#v", validation)
	}
	res = p6HTTPDo(t, handler, http.MethodPost, detailPath+"/apply?revisionId="+created.Revision.ID, "p6-ore-apply", "", true, "")
	p6RequireError(t, res, http.StatusUnprocessableEntity, "validation_failed", "p6-ore-apply")

	res = p6HTTPDo(t, handler, http.MethodPost, base, "p6-content-no-token", `{"kind":"recipe","slug":"x","title":"X","payload":{}}`, false, "")
	p6RequireError(t, res, http.StatusUnauthorized, "unauthorized", "p6-content-no-token")
}

func p6QuestDraftJSON(edges string, modRefs string) string {
	return `{"chapters":[{"id":"intro","title":"Intro","position":0}],"nodes":[{"id":"start","chapterId":"intro","title":"Start","position":0,"rewards":[{"kind":"item","item":"minecraft:stone","amount":1}],"modRefs":` + modRefs + `},{"id":"finish","chapterId":"intro","title":"Finish","position":1,"rewards":[{"kind":"experience","experience":5}]}],"edges":` + edges + `}`
}

func TestP6HTTPQuestRevisionContract(t *testing.T) {
	_, _, packID, handler := p6HTTPFixture(t)
	base := "/api/packs/" + packID + "/quests"
	draft := p6QuestDraftJSON(`[{"id":"e1","fromNodeId":"start","toNodeId":"finish"}]`, `[]`)
	res := p6HTTPDo(t, handler, http.MethodPut, base+"/draft", "p6-quest-save", draft, true, "0")
	if res.Code != http.StatusOK {
		t.Fatalf("quest save status=%d body=%s", res.Code, res.Body.String())
	}
	var saved struct {
		Revision service.QuestRevision     `json:"revision"`
		Issues   []service.ValidationIssue `json:"issues"`
	}
	if err := json.NewDecoder(res.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.Revision.ID == "" || saved.Revision.Revision != 1 || len(saved.Issues) != 0 {
		t.Fatalf("saved=%#v", saved)
	}

	res = p6HTTPDo(t, handler, http.MethodGet, base, "p6-quest-get", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("quest get status=%d body=%s", res.Code, res.Body.String())
	}
	var quest service.QuestBook
	if err := json.NewDecoder(res.Body).Decode(&quest); err != nil {
		t.Fatal(err)
	}
	if quest.Revision.ID != saved.Revision.ID || len(quest.Revision.Draft.Nodes) != 2 {
		t.Fatalf("quest=%#v", quest)
	}

	res = p6HTTPDo(t, handler, http.MethodPost, base+"/validate", "p6-quest-validate", "", true, "")
	if res.Code != http.StatusOK {
		t.Fatalf("quest validate status=%d body=%s", res.Code, res.Body.String())
	}
	var validated struct {
		Issues []service.ValidationIssue `json:"issues"`
	}
	if err := json.NewDecoder(res.Body).Decode(&validated); err != nil {
		t.Fatal(err)
	}
	if len(validated.Issues) != 0 {
		t.Fatalf("validated=%#v", validated)
	}
	res = p6HTTPDo(t, handler, http.MethodPost, base+"/apply", "p6-quest-apply", "", true, "")
	if res.Code != http.StatusOK {
		t.Fatalf("quest apply status=%d body=%s", res.Code, res.Body.String())
	}
	res = p6HTTPDo(t, handler, http.MethodGet, base+"/preview", "p6-quest-preview", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("quest preview status=%d body=%s", res.Code, res.Body.String())
	}
	var preview service.QuestDraft
	if err := json.NewDecoder(res.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Chapters) != 1 || len(preview.Nodes) != 2 || len(preview.Edges) != 1 {
		t.Fatalf("preview=%#v", preview)
	}

	res = p6HTTPDo(t, handler, http.MethodPut, base+"/draft", "p6-quest-stale", draft, true, "0")
	p6RequireError(t, res, http.StatusConflict, "revision_conflict", "p6-quest-stale")
	res = p6HTTPDo(t, handler, http.MethodGet, base+"/history", "p6-quest-history", "", false, "")
	if res.Code != http.StatusOK {
		t.Fatalf("quest history status=%d body=%s", res.Code, res.Body.String())
	}
	var history struct {
		Items []service.QuestRevision `json:"items"`
		Total int                     `json:"total"`
	}
	if err := json.NewDecoder(res.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 1 || len(history.Items) != 1 {
		t.Fatalf("quest history=%#v", history)
	}
}

func TestP6HTTPQuestGraphValidation(t *testing.T) {
	_, _, packID, handler := p6HTTPFixture(t)
	base := "/api/packs/" + packID + "/quests"
	cycle := p6QuestDraftJSON(`[{"id":"e1","fromNodeId":"start","toNodeId":"finish"},{"id":"e2","fromNodeId":"finish","toNodeId":"start"}]`, `[]`)
	res := p6HTTPDo(t, handler, http.MethodPut, base+"/draft", "p6-cycle-save", cycle, true, "0")
	if res.Code != http.StatusOK {
		t.Fatalf("cycle save status=%d body=%s", res.Code, res.Body.String())
	}
	var saved struct {
		Issues []service.ValidationIssue `json:"issues"`
	}
	if err := json.NewDecoder(res.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Issues) == 0 || saved.Issues[0].Code != "cycle" || saved.Issues[0].Severity != "error" {
		t.Fatalf("cycle issues=%#v", saved.Issues)
	}
	res = p6HTTPDo(t, handler, http.MethodPost, base+"/apply", "p6-cycle-apply", "", true, "")
	p6RequireError(t, res, http.StatusUnprocessableEntity, "validation_failed", "p6-cycle-apply")

	_, _, orphanPackID, orphanHandler := p6HTTPFixture(t)
	orphanBase := "/api/packs/" + orphanPackID + "/quests"
	orphan := p6QuestDraftJSON(`[]`, `[]`)
	res = p6HTTPDo(t, orphanHandler, http.MethodPut, orphanBase+"/draft", "p6-orphan-save", orphan, true, "0")
	if res.Code != http.StatusOK {
		t.Fatalf("orphan save status=%d body=%s", res.Code, res.Body.String())
	}
	if err := json.NewDecoder(res.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	foundOrphan := false
	for _, issue := range saved.Issues {
		if issue.Code == "orphan_node" && issue.Severity == "warning" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Fatalf("orphan issues=%#v", saved.Issues)
	}

	app, _, crossPackID, crossHandler := p6HTTPFixture(t)
	other, err := app.CreatePack(context.Background(), service.CreatePackInput{Name: "Other", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p6-other-pack")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := app.AddLocalPackMod(context.Background(), other.ID, service.LocalModInput{DisplayName: "Other Mod", FileName: "other.jar", SHA1: "2222222222222222222222222222222222222222", Size: 1}, "p6-other-mod")
	if err != nil {
		t.Fatal(err)
	}
	crossBase := "/api/packs/" + crossPackID + "/quests"
	cross := p6QuestDraftJSON(`[{"id":"e1","fromNodeId":"start","toNodeId":"finish"}]`, `[`+`"`+mod.ID+`"`+`]`)
	res = p6HTTPDo(t, crossHandler, http.MethodPut, crossBase+"/draft", "p6-cross-pack", cross, true, "0")
	p6RequireError(t, res, http.StatusBadRequest, "cross_pack_reference", "p6-cross-pack")
}
