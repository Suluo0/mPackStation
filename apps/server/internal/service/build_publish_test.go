package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

func TestP7BuildIsReproducibleAndIdempotent(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "tests", export); err != nil {
		t.Fatal(err)
	}
	in := BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "tests",
		Files:        []BuildFile{{Path: "mods/z.jar", Content: []byte("z")}, {Path: "config/a.txt", Content: []byte("a")}},
		LockSnapshot: []byte(`{"mods":[{"id":"z"}]}`), BuildConfig: []byte(`{"format":1}`),
		Checks: []DeliveryCheck{{Kind: "version", Status: "passed"}, {Kind: "missing_file", Status: "passed"}},
	}
	first, err := app.BuildPack(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.BuildPack(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceFingerprint != second.SourceFingerprint || first.Artifact.ID != second.Artifact.ID || first.Artifact.SHA256 != second.Artifact.SHA256 || string(firstBytes) != string(secondBytes) {
		t.Fatalf("repeat build changed identity: first=%#v second=%#v", first, second)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE pack_id=?`, packID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("artifact rows=%d, want one", count)
	}
	var inputCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pack_version_inputs WHERE pack_version_id=?`, versionID).Scan(&inputCount); err != nil {
		t.Fatal(err)
	}
	if inputCount != 4 {
		t.Fatalf("input rows=%d, want four source snapshots", inputCount)
	}
}

func TestP7BuildRejectsUnsafeInputsAndBlockedDelivery(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	if err := app.RegisterExportDirectory(context.Background(), "tests", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	base := BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "tests", Files: []BuildFile{{Path: "ok.txt", Content: []byte("ok")}}}
	for name, in := range map[string]BuildInput{
		"traversal": func() BuildInput {
			x := base
			x.Files = []BuildFile{{Path: "../escape.txt", Content: []byte("x")}}
			return x
		}(),
		"absolute": func() BuildInput {
			x := base
			x.Files = []BuildFile{{Path: `C:\escape.txt`, Content: []byte("x")}}
			return x
		}(),
		"bad-json": func() BuildInput { x := base; x.LockSnapshot = []byte(`{"broken"`); return x }(),
		"blocked": func() BuildInput {
			x := base
			x.Checks = []DeliveryCheck{{Kind: "conflict", Status: "blocked"}}
			return x
		}(),
	} {
		_, err := app.BuildPack(context.Background(), in)
		if !errors.Is(err, ErrInvalidBuildInput) && !errors.Is(err, ErrDeliveryBlocked) {
			t.Errorf("%s error=%v, want input or delivery rejection", name, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE pack_id=?`, packID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected builds registered %d artifacts", count)
	}
}

func TestP7PublishLocalIsIdempotentAndFailedRetryIsExplicit(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "tests", export); err != nil {
		t.Fatal(err)
	}
	built, err := app.BuildPack(context.Background(), BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "tests", Files: []BuildFile{{Path: "manifest.json", Content: []byte(`{}`)}}, Checks: []DeliveryCheck{{Kind: "version", Status: "passed"}}})
	if err != nil {
		t.Fatal(err)
	}
	p7 := NewP7Service(db)
	first, err := p7.PublishPack(context.Background(), PublishInput{PackID: packID, PackVersionID: versionID, Provider: "local", ArtifactID: built.Artifact.ID, IdempotencyKey: "local-1"})
	if err != nil || first.Status != "succeeded" {
		t.Fatalf("local publish=%#v err=%v", first, err)
	}
	second, err := p7.PublishPack(context.Background(), PublishInput{PackID: packID, PackVersionID: versionID, Provider: "local", ArtifactID: built.Artifact.ID, IdempotencyKey: "local-1"})
	if err != nil || second.ID != first.ID || second.Status != "succeeded" {
		t.Fatalf("idempotent local publish first=%#v second=%#v err=%v", first, second, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM releases WHERE provider='local' AND idempotency_key='local-1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("release rows=%d, want one", count)
	}

	failed, err := p7.PublishPack(context.Background(), PublishInput{PackID: packID, PackVersionID: versionID, Provider: "curseforge", ArtifactID: built.Artifact.ID, IdempotencyKey: "remote-1", ProjectID: "project", VersionID: "version"})
	if !errors.Is(err, ErrPublishFailed) || failed.Status != "failed" || failed.ErrorMessage == "" {
		t.Fatalf("unconfigured publish=%#v err=%v", failed, err)
	}
	// Reusing the same request is idempotent and must not auto-retry the
	// non-idempotent provider side effect.
	repeated, err := p7.PublishPack(context.Background(), PublishInput{PackID: packID, PackVersionID: versionID, Provider: "curseforge", ArtifactID: built.Artifact.ID, IdempotencyKey: "remote-1", ProjectID: "project", VersionID: "version"})
	if err != nil || repeated.ID != failed.ID || repeated.Status != "failed" {
		t.Fatalf("repeated failed publish=%#v err=%v", repeated, err)
	}
	fixture, err := provider.NewCurseForgeFixture([]byte(`{"projects":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	p7.SetProviderRegistry(provider.NewRegistry(fixture))
	retried, err := p7.RetryPublish(context.Background(), failed.ID, "project", "version")
	if err != nil {
		t.Fatalf("explicit retry err=%v release=%#v", err, retried)
	}
	if retried.Status != "publishing" {
		t.Fatalf("explicit retry status=%s, want publishing", retried.Status)
	}
	if strings.Contains(retried.ErrorMessage, "project") {
		t.Fatalf("release leaked provider detail: %#v", retried)
	}
}

func newP7Fixture(t *testing.T) (*sql.DB, *API, string, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "p7.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := New(db)
	pack, err := app.CreatePack(context.Background(), CreatePackInput{Name: "P7", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p7-test")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	var versionID string
	if err := db.QueryRow(`SELECT pack_version_id FROM pack_current_version WHERE pack_id=?`, pack.ID).Scan(&versionID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db, app, pack.ID, versionID
}
