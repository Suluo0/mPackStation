package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

type namedArchiveProvider struct {
	*fakeProvider
	name provider.Name
}

func (p *namedArchiveProvider) Name() provider.Name { return p.name }
func (p *namedArchiveProvider) Search(context.Context, provider.SearchRequest) (provider.SearchResult, error) {
	return provider.SearchResult{Items: []provider.Project{}}, nil
}

func TestPackScopedMinecraftLifecycleAndGenerationInvalidation(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "pack-scoped.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := New(db)
	pack, err := api.CreatePack(context.Background(), CreatePackInput{Name: "Scoped", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1"}, "create")
	if err != nil {
		t.Fatal(err)
	}
	members, err := api.ListPackContentSources(context.Background(), pack.ID)
	if err != nil || len(members) != 1 || members[0].CanonicalModID != "minecraft" || members[0].Origin != "builtin" {
		t.Fatalf("minecraft member = %#v, %v", members, err)
	}
	writeCatalogFixture(t, dir)
	if _, err = api.RebuildItemCatalog(context.Background(), pack.ID, "zh_cn", "generation"); err != nil {
		t.Fatal(err)
	}
	var readyVersion, selections, sources, runs, definitions, recipeTerms, tagEntries, textBindings, tagMembers, readyGeneration int
	checks := []struct {
		query  string
		target *int
	}{
		{`SELECT count(*) FROM mod_versions WHERE mod_id='minecraft' AND declared_version='1.21.1' AND status='ready'`, &readyVersion},
		{`SELECT count(*) FROM pack_mod_selections WHERE pack_id=? AND mod_id='minecraft' AND status='ready'`, &selections},
		{`SELECT count(*) FROM pack_content_sources WHERE pack_id=?`, &sources},
		{`SELECT count(*) FROM parse_runs WHERE pack_id=? AND phase='content' AND execution_status='succeeded'`, &runs},
		{`SELECT count(*) FROM content_definitions WHERE pack_id=?`, &definitions},
		{`SELECT count(*) FROM recipe_terms WHERE pack_id=?`, &recipeTerms},
		{`SELECT count(*) FROM tag_entries WHERE pack_id=?`, &tagEntries},
		{`SELECT count(*) FROM content_text_bindings WHERE pack_id=?`, &textBindings},
		{`SELECT count(*) FROM catalog_tag_members WHERE pack_id=?`, &tagMembers},
		{`SELECT count(*) FROM catalog_generations g JOIN packs p ON p.id=g.pack_id AND p.current_generation_id=g.id WHERE g.pack_id=? AND g.status='ready'`, &readyGeneration},
	}
	for _, check := range checks {
		args := []any{}
		if check.query != checks[0].query {
			args = append(args, pack.ID)
		}
		if err := db.QueryRow(check.query, args...).Scan(check.target); err != nil {
			t.Fatal(err)
		}
	}
	if readyVersion != 1 || selections != 1 || sources != 1 || runs != 1 || definitions == 0 || recipeTerms == 0 || tagEntries == 0 || textBindings == 0 || tagMembers == 0 || readyGeneration != 1 {
		t.Fatalf("version=%d selections=%d sources=%d runs=%d definitions=%d recipeTerms=%d tagEntries=%d textBindings=%d tagMembers=%d generation=%d", readyVersion, selections, sources, runs, definitions, recipeTerms, tagEntries, textBindings, tagMembers, readyGeneration)
	}
	nextVersion := "1.21.2"
	if _, err = api.UpdatePack(context.Background(), pack.ID, UpdatePackInput{MCVersion: &nextVersion}, "switch"); err != nil {
		t.Fatal(err)
	}
	var currentGeneration any
	var status string
	if err = db.QueryRow(`SELECT current_generation_id FROM packs WHERE id=?`, pack.ID).Scan(&currentGeneration); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT status FROM pack_mods WHERE pack_id=? AND mod_id='minecraft'`, pack.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if currentGeneration != nil || status != "pending" {
		t.Fatalf("after switch current=%v minecraft=%s", currentGeneration, status)
	}
}

func TestStableModIdentityIsSharedButMembershipIsPackScoped(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := New(db)
	one, _ := api.CreatePack(context.Background(), CreatePackInput{Name: "One", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "one")
	two, _ := api.CreatePack(context.Background(), CreatePackInput{Name: "Two", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "two")
	jar := buildMinimalJar(t)
	mr := &namedArchiveProvider{fakeProvider: &fakeProvider{jar: jar}, name: provider.Modrinth}
	cf := &namedArchiveProvider{fakeProvider: &fakeProvider{jar: jar}, name: provider.CurseForge}
	api.SetProviderRegistry(provider.NewRegistry(mr, cf))
	if _, err = api.AddPackMod(context.Background(), one.ID, AddModInput{Provider: "modrinth", ProjectID: "mr", VersionID: "v1"}, "add-one"); err != nil {
		t.Fatal(err)
	}
	if _, err = api.AddPackMod(context.Background(), two.ID, AddModInput{Provider: "curseforge", ProjectID: "cf", VersionID: "v1"}, "add-two"); err != nil {
		t.Fatal(err)
	}
	if _, err = api.AddPackMod(context.Background(), one.ID, AddModInput{Provider: "curseforge", ProjectID: "cf", VersionID: "v1"}, "duplicate"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("same mod identity in one pack should conflict, got %v", err)
	}
	var identities, versions, files, memberships int
	for query, target := range map[string]*int{
		`SELECT count(*) FROM mods WHERE mod_id='testmod'`:                              &identities,
		`SELECT count(*) FROM mod_versions WHERE mod_id='testmod' AND status='ready'`:   &versions,
		`SELECT count(*) FROM file_objects WHERE media_type='application/java-archive'`: &files,
		`SELECT count(*) FROM pack_mods WHERE mod_id='testmod'`:                         &memberships,
	} {
		if err = db.QueryRow(query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if identities != 1 || versions != 1 || files != 1 || memberships != 2 {
		t.Fatalf("identities=%d versions=%d files=%d memberships=%d", identities, versions, files, memberships)
	}
}
