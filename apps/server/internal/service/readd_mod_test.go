package service

import (
	"context"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

func TestReAddRemovedPackMod(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db)
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "Readd", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	fixture := []byte(`{"projects":[{"id":"p1","name":"Alpha Mod"}],"versions":[{"id":"v1","projectId":"p1","name":"1.0","files":[{"name":"alpha.jar","sha1":"1111111111111111111111111111111111111111","size":12,"primary":true}]}],"metadata":[{"project":{"id":"p1","name":"Alpha Mod"},"dependencies":[]}]}`)
	ad, err := provider.NewCurseForgeFixture(fixture)
	if err != nil {
		t.Fatal(err)
	}
	a.SetProviderRegistry(provider.NewRegistry(ad))
	ctx := context.Background()
	m1, err := a.AddPackMod(ctx, p.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1", Required: true}, "add-1")
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := a.repo.RemovePackMod(ctx, p.ID, m1.ID, 1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	m2, err := a.AddPackMod(ctx, p.ID, AddModInput{Provider: "curseforge", ProjectID: "p1", VersionID: "v1", Required: true}, "add-2")
	if err != nil {
		t.Fatalf("re-add after remove: %v", err)
	}
	if m2.Status != "installed" {
		t.Fatalf("re-add status = %q", m2.Status)
	}
	mods, err := a.ListPackMods(ctx, p.ID)
	if err != nil || len(mods) != 1 {
		t.Fatalf("mods after re-add = %#v, %v", mods, err)
	}
}
