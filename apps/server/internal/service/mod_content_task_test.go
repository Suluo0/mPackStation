package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

// fakeProvider implements provider.Adapter with only Metadata + Download.
type fakeProvider struct {
	provider.Adapter
	jar []byte
}

func (f *fakeProvider) Name() provider.Name { return "modrinth" }
func (f *fakeProvider) Metadata(_ context.Context, _, _ string) (provider.Metadata, error) {
	return provider.Metadata{
		Project: provider.Project{ID: "test-project", Name: "Test Mod", Slug: "test-mod"},
		Version: provider.Version{ID: "test-version", VersionNumber: "1.0.0"},
	}, nil
}
func (f *fakeProvider) Download(_ context.Context, _ provider.DownloadRequest) (provider.DownloadResult, error) {
	return provider.DownloadResult{Content: f.jar, FileName: "testmod-1.0.0.jar"}, nil
}

func modContentFixture(t *testing.T) (*API, string, string) {
	return modContentFixtureWithJar(t, buildMinimalJar(t))
}

func modContentFixtureWithJar(t *testing.T, jar []byte) (*API, string, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(db)
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "Test", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "req-pack")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	// Inject fake provider BEFORE AddPackMod (it calls Metadata + Download).
	a.SetProviderRegistry(provider.NewRegistry(&fakeProvider{jar: jar}))
	mod, err := a.AddPackMod(context.Background(), p.ID, AddModInput{Provider: "modrinth", ProjectID: "test-project", VersionID: "test-version", Required: true}, "req-mod")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return a, p.ID, mod.ID
}

func runParse(t *testing.T, a *API, packID, modID string) {
	t.Helper()
	_, err := a.parseAndPersistModContent(context.Background(), packID, modID, "test-task", nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
}

func TestHandleParseModContentTask_FullFlow(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	runParse(t, a, packID, modID)

	ctx := context.Background()
	items, _, total, run, err := a.ListModContent(ctx, packID, modID, "", 100, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(items) != 5 {
		t.Errorf("items len = %d, want 5", len(items))
	}
	if run.Status != "succeeded" {
		t.Errorf("run status = %q, want succeeded", run.Status)
	}
	if run.ParsedCount != 5 {
		t.Errorf("run parsedCount = %d, want 5", run.ParsedCount)
	}
	if run.DynamicCount != 1 {
		t.Errorf("run dynamicCount = %d, want 1", run.DynamicCount)
	}

	// Recipe kind filter.
	recipes, _, recipeTotal, _, err := a.ListModContent(ctx, packID, modID, "recipe", 100, "")
	if err != nil {
		t.Fatalf("list recipes: %v", err)
	}
	if recipeTotal != 2 {
		t.Errorf("recipe total = %d, want 2", recipeTotal)
	}
	dynFound := false
	for _, r := range recipes {
		if r.IsDynamic {
			dynFound = true
		}
	}
	if !dynFound {
		t.Error("no dynamic recipe found")
	}
}

func TestHandleParseModContentTask_Idempotent(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	runParse(t, a, packID, modID)
	// Second run: same sha1, short-circuits.
	runParse(t, a, packID, modID)
	ctx := context.Background()
	_, _, total, _, err := a.ListModContent(ctx, packID, modID, "", 100, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("total after re-run = %d, want 5 (idempotent)", total)
	}
}

func TestSubmitParseModContent_QueueUnavailable(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	a.SetProviderRegistry(provider.NewRegistry(&fakeProvider{jar: buildMinimalJar(t)}))
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "T", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "r")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := a.AddPackMod(context.Background(), p.ID, AddModInput{Provider: "modrinth", ProjectID: "p", VersionID: "v"}, "r")
	if err != nil {
		t.Fatal(err)
	}
	// No queue set.
	_, _, err = a.SubmitParseModContent(context.Background(), p.ID, mod.ID)
	if err == nil {
		t.Fatal("expected error when queue is nil, got nil")
	}
}

func TestListModContent_NotParsed(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	ctx := context.Background()
	_, _, _, _, err := a.ListModContent(ctx, packID, modID, "", 100, "")
	if err == nil {
		t.Fatal("expected ErrModContentNotParsed, got nil")
	}
}

func TestListModContent_InvalidKind(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	runParse(t, a, packID, modID)
	ctx := context.Background()
	_, _, _, _, err := a.ListModContent(ctx, packID, modID, "invalid_kind", 100, "")
	if err == nil {
		t.Fatal("expected ErrInvalidContentKind, got nil")
	}
}

func TestGetModContentRun(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	ctx := context.Background()
	_, err := a.GetModContentRun(ctx, packID, modID)
	if err == nil {
		t.Fatal("expected error for unparsed mod, got nil")
	}
	runParse(t, a, packID, modID)
	run, err := a.GetModContentRun(ctx, packID, modID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", run.Status)
	}
}

func TestGetModContent_ById(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	runParse(t, a, packID, modID)
	ctx := context.Background()
	items, _, _, _, _ := a.ListModContent(ctx, packID, modID, "", 100, "")
	if len(items) == 0 {
		t.Fatal("no items")
	}
	got, err := a.GetModContent(ctx, packID, modID, items[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != items[0].ID {
		t.Errorf("id = %q, want %q", got.ID, items[0].ID)
	}
}

// Ensure json import is used (for future payload assertions).
var _ = json.RawMessage{}

func TestSubmitParseModContent_CreatesTask(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db-submit.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	a.SetProviderRegistry(provider.NewRegistry(&fakeProvider{jar: buildMinimalJar(t)}))
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "Submit", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "r")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := a.AddPackMod(context.Background(), p.ID, AddModInput{Provider: "modrinth", ProjectID: "p", VersionID: "v"}, "r")
	if err != nil {
		t.Fatal(err)
	}
	q, err := task.NewQueue(db)
	if err != nil {
		t.Fatal(err)
	}
	a.SetTaskQueue(q)
	taskObj, submitted, err := a.SubmitParseModContent(context.Background(), p.ID, mod.ID)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !submitted {
		t.Error("submitted = false, want true")
	}
	if taskObj == nil {
		t.Fatal("task is nil")
	}
	if taskObj.Kind != task.KindParseModContent {
		t.Errorf("kind = %q, want %q", taskObj.Kind, task.KindParseModContent)
	}
	var payload parseModContentPayload
	if err := json.Unmarshal(taskObj.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.PackID != p.ID || payload.ModID != mod.ID {
		t.Errorf("payload = %+v, want packID=%s modID=%s", payload, p.ID, mod.ID)
	}
}

func TestHandleParseModContentTask_VersionChangeReParses(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	ctx := context.Background()
	firstMod, err := a.repo.GetPackMod(ctx, modID)
	if err != nil {
		t.Fatal(err)
	}
	runParse(t, a, packID, modID)
	_, _, total1, _, _ := a.ListModContent(ctx, packID, modID, "", 100, "")
	var next bytes.Buffer
	zw := zip.NewWriter(&next)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"testmod","version":"2.0.0"}`)
	addZipFile(t, zw, "data/testmod/recipe/new.json", `{"type":"minecraft:crafting_shapeless","ingredients":[],"result":{"id":"testmod:new"}}`)
	zw.Close()
	a.SetProviderRegistry(provider.NewRegistry(&fakeProvider{jar: next.Bytes()}))
	nextVersion := "test-version-2"
	if _, err := a.UpdatePackMod(ctx, packID, modID, UpdateModInput{VersionID: &nextVersion}, "version-change"); err != nil {
		t.Fatal(err)
	}
	runParse(t, a, packID, modID)
	_, _, total2, _, err := a.ListModContent(ctx, packID, modID, "", 100, "")
	if err != nil {
		t.Fatalf("list after re-parse: %v", err)
	}
	if total2 == total1 {
		t.Errorf("version switch reused the previous content count %d", total1)
	}
	// Verify there are now two run records (one per sha1).
	runA, err := a.repo.GetModContentRunBySHA(ctx, modID, firstMod.SHA1)
	if err != nil {
		t.Errorf("get run A: %v", err)
	}
	if runA.Status != "succeeded" {
		t.Errorf("run A status = %q, want succeeded", runA.Status)
	}
	updatedMod, _ := a.repo.GetPackMod(ctx, modID)
	runB, err := a.repo.GetModContentRunBySHA(ctx, modID, updatedMod.SHA1)
	if err != nil {
		t.Errorf("get run B: %v", err)
	}
	if runB.Status != "succeeded" {
		t.Errorf("run B status = %q, want succeeded", runB.Status)
	}
}

func TestListModContent_Pagination(t *testing.T) {
	// Build a jar with 150 recipes to test pagination.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"pagermod","version":"1.0"}`)
	for i := 0; i < 150; i++ {
		addZipFile(t, zw, fmt.Sprintf("data/pagermod/recipe/r%03d.json", i),
			`{"type":"minecraft:crafting_shapeless","ingredients":[],"result":{"id":"pagermod:x"}}`)
	}
	zw.Close()
	a, packID, modID := modContentFixtureWithJar(t, buf.Bytes())
	ctx := context.Background()
	if _, err := a.parseAndPersistModContent(ctx, packID, modID, "test", nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// First page: limit=100.
	items1, cursor1, total, _, err := a.ListModContent(ctx, packID, modID, "recipe", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 150 {
		t.Errorf("total = %d, want 150", total)
	}
	if len(items1) != 100 {
		t.Errorf("page1 len = %d, want 100", len(items1))
	}
	if cursor1 == "" {
		t.Fatal("cursor1 is empty, want non-empty")
	}
	// Second page.
	items2, cursor2, _, _, err := a.ListModContent(ctx, packID, modID, "recipe", 100, cursor1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 50 {
		t.Errorf("page2 len = %d, want 50", len(items2))
	}
	if cursor2 != "" {
		t.Errorf("cursor2 = %q, want empty", cursor2)
	}
}

func TestHandleParseModContentTask_LocalModNoBytes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db-local.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "Local", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "r")
	if err != nil {
		t.Fatal(err)
	}
	// Insert a local mod directly via store (no provider download).
	modID := "pm-local-test"
	localSHA := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	// Insert jar_index row to satisfy FK on pack_mods.sha1.
	if err := a.repo.UpsertJarIndex(context.Background(), store.JarIndexRecord{SHA1: localSHA, FilePath: "jar://" + localSHA, ParsedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := a.repo.AddPackMod(context.Background(), &store.PackModRecord{
		ID: modID, PackID: p.ID, Source: "local", DisplayName: "Local Mod",
		FileName: "local.jar", SHA1: localSHA,
		Status: "installed", AddedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.parseAndPersistModContent(context.Background(), p.ID, modID, "test", nil)
	if err == nil {
		t.Fatal("expected error for local mod without bytes, got nil")
	}
	// Verify run is marked failed.
	run, err := a.repo.GetModContentRunBySHA(context.Background(), modID, localSHA)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" {
		t.Errorf("run status = %q, want failed", run.Status)
	}
}

func TestParseModContent_ErrorCountRecorded(t *testing.T) {
	// Build a jar with one valid recipe and one invalid JSON recipe.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"errmod","version":"1.0"}`)
	addZipFile(t, zw, "data/errmod/recipe/good.json", `{"type":"minecraft:crafting_shapeless","ingredients":[],"result":{"id":"errmod:ok"}}`)
	addZipFile(t, zw, "data/errmod/recipe/bad.json", `{invalid json`)
	zw.Close()
	a, packID, modID := modContentFixtureWithJar(t, buf.Bytes())
	ctx := context.Background()
	if _, err := a.parseAndPersistModContent(ctx, packID, modID, "test", nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	run, err := a.GetModContentRun(ctx, packID, modID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ErrorCount != 1 {
		t.Errorf("run.ErrorCount = %d, want 1 (invalid JSON recipe)", run.ErrorCount)
	}
	if run.Status != "succeeded" {
		t.Errorf("run status = %q, want succeeded", run.Status)
	}
}

func TestReplaceModContent_Atomic(t *testing.T) {
	a, packID, modID := modContentFixture(t)
	ctx := context.Background()
	runParse(t, a, packID, modID)
	// Verify initial state: 5 content rows.
	items, _, total, _, err := a.ListModContent(ctx, packID, modID, "", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("initial total = %d, want 5", total)
	}
	// ReplaceModContentAndFinishRun with a run that has an invalid status
	// should fail the CHECK constraint and roll back both content and run.
	mod, _ := a.repo.GetPackMod(ctx, modID)
	badRun := store.ModContentRunRecord{
		ID: "mcrun-bad", PackID: packID, ModID: modID, SHA1: mod.SHA1,
		Status: "invalid_status", // violates CHECK constraint
	}
	err = a.repo.ReplaceModContentAndFinishRun(ctx, packID, modID, []store.ModContentRecord{}, badRun)
	if err == nil {
		t.Fatal("expected error for invalid run status, got nil")
	}
	// Verify rollback: content rows should still be 5 (not deleted to 0).
	_, _, totalAfter, _, err := a.ListModContent(ctx, packID, modID, "", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter != 5 {
		t.Errorf("total after failed atomic replace = %d, want 5 (rollback)", totalAfter)
	}
	_ = items
}

func TestSubmitParseModContent_ModNotFound_Code(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db-nf.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "NF", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "r")
	if err != nil {
		t.Fatal(err)
	}
	q, _ := task.NewQueue(db)
	a.SetTaskQueue(q)
	_, _, err = a.SubmitParseModContent(context.Background(), p.ID, "nonexistent-mod")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var de *DomainError
	if !errors.As(err, &de) {
		t.Fatalf("error is not DomainError: %T", err)
	}
	if de.Code != "mod_not_found" {
		t.Errorf("code = %q, want mod_not_found", de.Code)
	}
	if de.Status != 404 {
		t.Errorf("status = %d, want 404", de.Status)
	}
}
