package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

const p5SHA1 = "1111111111111111111111111111111111111111"

// TestP5AcceptanceModLifecycleAndCrossPackIsolation is the repository/service
// acceptance gate for the local single-instance mod model. It deliberately
// exercises two packs because the absence of tenant/workspace data isolation
// must not turn into the absence of pack-level isolation.
func TestP5AcceptanceModLifecycleAndCrossPackIsolation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "p5-lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := New(db)
	adapter, err := provider.NewCurseForgeFixture([]byte(`{
        "projects":[
          {"id":"p1","slug":"alpha","name":"Alpha Mod"},
          {"id":"p2","slug":"beta","name":"Beta Mod"}
        ],
        "versions":[
          {"id":"v1","projectId":"p1","name":"1.0","versionNumber":"1.0","files":[{"id":"f1","name":"alpha.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]},
          {"id":"v2","projectId":"p1","name":"2.0","versionNumber":"2.0","files":[{"id":"f2","name":"alpha-2.jar","sha1":"2222222222222222222222222222222222222222","size":13,"primary":true}]},
          {"id":"v3","projectId":"p2","name":"1.0","versionNumber":"1.0","files":[{"id":"f3","name":"beta.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]}
        ],
        "metadata":[
          {"project":{"id":"p1","slug":"alpha","name":"Alpha Mod"},"version":{"id":"v1","projectId":"p1","name":"1.0"},"dependencies":[]},
          {"project":{"id":"p1","slug":"alpha","name":"Alpha Mod"},"version":{"id":"v2","projectId":"p1","name":"2.0"},"dependencies":[]},
          {"project":{"id":"p2","slug":"beta","name":"Beta Mod"},"version":{"id":"v3","projectId":"p2","name":"1.0"},"dependencies":[]}
        ]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(adapter))

	packA, err := app.CreatePack(context.Background(), CreatePackInput{Name: "P5 A", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p5-a")
	if err != nil {
		t.Fatal(err)
	}
	packB, err := app.CreatePack(context.Background(), CreatePackInput{Name: "P5 B", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p5-b")
	if err != nil {
		t.Fatal(err)
	}

	modA, err := app.AddPackMod(context.Background(), packA.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1", Required: true}, "p5-add-a")
	if err != nil {
		t.Fatal(err)
	}
	if modA.Status != "installed" || modA.SHA1 == nil || *modA.SHA1 != p5SHA1 {
		t.Fatalf("added mod = %#v", modA)
	}
	if _, err := app.AddPackMod(context.Background(), packA.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1"}, "p5-duplicate"); !IsConflict(err) {
		t.Fatalf("duplicate mod error = %v, want conflict", err)
	}

	// The same content hash is globally indexed but independently selected by
	// each pack. The second selection must not be rejected as a cross-pack
	// duplicate.
	modB, err := app.AddPackMod(context.Background(), packB.ID, AddModInput{Provider: "curseforge", ProjectID: "p2", VersionID: "v3", Required: true}, "p5-add-b")
	if err != nil {
		t.Fatal(err)
	}
	var jarCount, modCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jar_index WHERE sha1=?`, p5SHA1).Scan(&jarCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pack_mods WHERE sha1=? AND status<>'removed'`, p5SHA1).Scan(&modCount); err != nil {
		t.Fatal(err)
	}
	if jarCount != 1 || modCount != 2 {
		t.Fatalf("shared SHA-1 counts: jar=%d mods=%d, want jar=1 mods=2", jarCount, modCount)
	}

	status := "disabled"
	updated, err := app.UpdatePackMod(context.Background(), packA.ID, modA.ID, UpdateModInput{Status: &status}, "p5-disable")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != status {
		t.Fatalf("disabled mod = %#v", updated)
	}
	version := "v2"
	updated, err = app.UpdatePackMod(context.Background(), packA.ID, modA.ID, UpdateModInput{VersionID: &version}, "p5-version")
	if err != nil {
		t.Fatal(err)
	}
	if updated.VersionID == nil || *updated.VersionID != version || updated.SHA1 == nil || *updated.SHA1 != "2222222222222222222222222222222222222222" || updated.Status != "installed" {
		t.Fatalf("version update = %#v", updated)
	}

	if err := app.RemovePackMod(context.Background(), packA.ID, modA.ID, "p5-remove"); err != nil {
		t.Fatal(err)
	}
	if mods, err := app.ListPackMods(context.Background(), packA.ID); err != nil || len(mods) != 0 {
		t.Fatalf("pack A mods after remove = %#v, %v", mods, err)
	}
	if mods, err := app.ListPackMods(context.Background(), packB.ID); err != nil || len(mods) != 1 || mods[0].ID != modB.ID {
		t.Fatalf("pack B mods after pack A remove = %#v, %v", mods, err)
	}
	if err := app.RemovePackMod(context.Background(), packB.ID, modA.ID, "p5-cross-pack-remove"); !IsNotFound(err) {
		t.Fatalf("cross-pack remove error = %v, want not found", err)
	}
	if _, err := app.UpdatePackMod(context.Background(), packB.ID, modA.ID, UpdateModInput{Status: &status}, "p5-cross-pack-update"); !IsNotFound(err) {
		t.Fatalf("cross-pack update error = %v, want not found", err)
	}
}

// TestP5AcceptanceLockSnapshotAndResolutionEvidence verifies that a resolve
// produces an immutable, hash-addressed snapshot and that repeat resolution
// is conflict-idempotent. It also asserts the required activity/outbox audit
// evidence for the lock write.
func TestP5AcceptanceLockSnapshotAndResolutionEvidence(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "p5-lock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := New(db)
	adapter, err := provider.NewCurseForgeFixture([]byte(`{
        "projects":[{"id":"p1","name":"Alpha Mod"}],
        "versions":[{"id":"v1","projectId":"p1","name":"1.0","files":[{"name":"alpha.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]}],
        "metadata":[{"project":{"id":"p1","name":"Alpha Mod"},"version":{"id":"v1","projectId":"p1"},"dependencies":[{"projectId":"missing","type":"required","constraint":">=1","reason":"required by Alpha"}]}]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(adapter))
	pack, err := app.CreatePack(context.Background(), CreatePackInput{Name: "P5 lock", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p5-pack")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPackMod(context.Background(), pack.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1"}, "p5-add"); err != nil {
		t.Fatal(err)
	}

	var beforeOutbox, beforeActivities int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE pack_id=?`, pack.ID).Scan(&beforeOutbox); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM activities WHERE pack_id=?`, pack.ID).Scan(&beforeActivities); err != nil {
		t.Fatal(err)
	}
	first, err := app.ResolvePack(context.Background(), pack.ID, "p5-resolve-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotSHA256 == "" || len(first.SnapshotJSON) == 0 {
		t.Fatalf("first lock = %#v", first)
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(first.SnapshotJSON), &snapshot); err != nil {
		t.Fatalf("snapshot JSON: %v", err)
	}
	if snapshot["packId"] != pack.ID {
		t.Fatalf("snapshot packId = %v, want %s", snapshot["packId"], pack.ID)
	}
	var depCount, conflictCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mod_dependencies WHERE pack_id=? AND lock_id=?`, pack.ID, first.ID).Scan(&depCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM conflicts WHERE pack_id=? AND status='pending'`, pack.ID).Scan(&conflictCount); err != nil {
		t.Fatal(err)
	}
	if depCount != 1 || conflictCount != 1 {
		t.Fatalf("resolution evidence: deps=%d conflicts=%d, want 1/1", depCount, conflictCount)
	}
	var afterOutbox, afterActivities int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE pack_id=?`, pack.ID).Scan(&afterOutbox); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM activities WHERE pack_id=?`, pack.ID).Scan(&afterActivities); err != nil {
		t.Fatal(err)
	}
	if afterOutbox <= beforeOutbox || afterActivities <= beforeActivities {
		t.Fatalf("resolve audit evidence missing: outbox %d->%d activities %d->%d", beforeOutbox, afterOutbox, beforeActivities, afterActivities)
	}

	second, err := app.ResolvePack(context.Background(), pack.ID, "p5-resolve-2")
	if err != nil {
		t.Fatal(err)
	}
	if second.SnapshotSHA256 == first.SnapshotSHA256 {
		t.Fatalf("repeat resolve reused lock hash %s; each lock must remain an immutable revision", first.SnapshotSHA256)
	}
	var pendingCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conflicts WHERE pack_id=? AND status='pending'`, pack.ID).Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("repeat resolve pending conflict count=%d, want 1", pendingCount)
	}

	// The old snapshot is immutable even after a later lock is generated.
	var oldSnapshot string
	if err := db.QueryRow(`SELECT snapshot_json FROM pack_locks WHERE id=?`, first.ID).Scan(&oldSnapshot); err != nil {
		t.Fatal(err)
	}
	if oldSnapshot != first.SnapshotJSON {
		t.Fatalf("first snapshot changed after second resolve")
	}

	// ResolveConflict must not allow a conflict ID from another pack to cross
	// the pack boundary. This is a data-level authorization invariant.
	other, err := app.CreatePack(context.Background(), CreatePackInput{Name: "P5 other", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p5-other")
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := app.ListConflicts(context.Background(), pack.ID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts = %#v, %v", conflicts, err)
	}
	if err := app.ResolveConflict(context.Background(), other.ID, conflicts[0].ID, "resolved", "p5-cross-pack-conflict"); !IsNotFound(err) {
		t.Fatalf("cross-pack conflict resolution error = %v, want not found", err)
	}
}

func TestP5AcceptanceUnconfiguredAndInvalidProviderResults(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "p5-errors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := New(db)
	pack, err := app.CreatePack(context.Background(), CreatePackInput{Name: "P5 errors", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p5-errors-pack")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ModSearch(context.Background(), pack.ID, ModSearchInput{Provider: "curseforge", Query: "alpha"}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("unconfigured provider error = %v, want provider unavailable", err)
	}

	bad, err := provider.NewCurseForgeFixture([]byte(`{
        "projects":[{"id":"p1","name":"Broken"}],
        "versions":[{"id":"v1","projectId":"p1","files":[{"name":"broken.jar","sha1":"not-a-sha1","primary":true}]}],
        "metadata":[{"project":{"id":"p1","name":"Broken"},"version":{"id":"v1","projectId":"p1"}}],
        "faults":{"search:missing":{"code":"404"},"search:throttled":{"code":"429"},"search:down":{"code":"503"}}
    }`))
	if err != nil {
		t.Fatal(err)
	}
	app.SetProviderRegistry(provider.NewRegistry(bad))
	if _, err := app.ModSearch(context.Background(), pack.ID, ModSearchInput{Provider: "curseforge", Query: "missing"}); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("404 provider error = %v, want provider not found", err)
	}
	if _, err := app.ModSearch(context.Background(), pack.ID, ModSearchInput{Provider: "curseforge", Query: "throttled"}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("429 provider error = %v, want unavailable mapping", err)
	}
	if _, err := app.ModSearch(context.Background(), pack.ID, ModSearchInput{Provider: "curseforge", Query: "down"}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("503 provider error = %v, want unavailable mapping", err)
	}
	if _, err := app.AddPackMod(context.Background(), pack.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1"}, "p5-invalid-sha1"); !errors.Is(err, ErrInvalidSHA1) {
		t.Fatalf("invalid SHA-1 error = %v, want invalid SHA-1", err)
	}
}
