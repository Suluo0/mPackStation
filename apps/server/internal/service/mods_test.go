package service

import (
	"context"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

func TestP5ModChainSearchAddResolveAndHealth(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "P5", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	fixture := []byte(`{"projects":[{"id":"p1","name":"Alpha Mod"}],"versions":[{"id":"v1","projectId":"p1","name":"1.0","files":[{"name":"alpha.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]}],"metadata":[{"project":{"id":"p1","name":"Alpha Mod"},"dependencies":[{"projectId":"missing","type":"required","constraint":"*","reason":"required by Alpha"}]}]}`)
	ad, err := provider.NewCurseForgeFixture(fixture)
	if err != nil {
		t.Fatal(err)
	}
	a.SetProviderRegistry(provider.NewRegistry(ad))
	search, err := a.ModSearch(context.Background(), p.ID, ModSearchInput{Provider: "curseforge", Query: "alpha"})
	if err != nil || search.Total != 1 {
		t.Fatalf("search = %#v, %v", search, err)
	}
	m, err := a.AddPackMod(context.Background(), p.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1", Required: true}, "req-2")
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != "installed" || m.SHA1 == nil {
		t.Fatalf("mod = %#v", m)
	}
	mods, err := a.ListPackMods(context.Background(), p.ID)
	if err != nil || len(mods) != 1 {
		t.Fatalf("mods = %#v, %v", mods, err)
	}
	lock, err := a.ResolvePack(context.Background(), p.ID, "req-3")
	if err != nil {
		t.Fatal(err)
	}
	if lock.SnapshotSHA256 == "" {
		t.Fatal("lock hash missing")
	}
	conflicts, err := a.ListConflicts(context.Background(), p.ID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts = %#v, %v", conflicts, err)
	}
	health, err := a.PackHealth(context.Background(), p.ID)
	if err != nil || health.Healthy || health.PendingErrors != 1 {
		t.Fatalf("health = %#v, %v", health, err)
	}
	if err := a.ResolveConflict(context.Background(), p.ID, conflicts[0].ID, "resolved", "req-4"); err != nil {
		t.Fatal(err)
	}
	health, err = a.PackHealth(context.Background(), p.ID)
	if err != nil || !health.Healthy {
		t.Fatalf("resolved health = %#v, %v", health, err)
	}
}
