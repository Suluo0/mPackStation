package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"mpackstation/internal/store"
)

// O9 守护用例：目录读取失败要能区分「从没构建过」「包不存在」「构建过但已过期」，
// 而不是三者共用一个 409 catalog_stale。
func TestCatalogReadSeparatesNotBuiltStaleAndMissingPack(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "catalog-code.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := New(db)
	ctx := context.Background()
	pack, err := api.CreatePack(ctx, CreatePackInput{Name: "Codes", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1"}, "create")
	if err != nil {
		t.Fatal(err)
	}

	var de *DomainError
	code := func(err error) (int, string) {
		if !errors.As(err, &de) {
			t.Fatalf("expected DomainError, got %v", err)
		}
		return de.Status, de.Code
	}

	// 1. 从未构建
	if s, c := code(mustFail(t, catalogOf(api, ctx, pack.ID))); s != 409 || c != "catalog_not_built" {
		t.Errorf("not built: got %d %s, want 409 catalog_not_built", s, c)
	}
	// 2. 包不存在
	if s, c := code(mustFail(t, catalogOf(api, ctx, "pack-does-not-exist"))); s != 404 || c != "pack_not_found" {
		t.Errorf("missing pack: got %d %s, want 404 pack_not_found", s, c)
	}

	writeCatalogFixture(t, dir)
	if _, err = api.RebuildItemCatalog(ctx, pack.ID, "zh_cn", "rebuild"); err != nil {
		t.Fatal(err)
	}

	// 3. 目录已构建，但查一个不在目录里的条目/标签：不能说成包不存在。
	if s, c := code(mustFail(t, itemOf(api, ctx, pack.ID))); s != 404 || c != "catalog_item_not_found" {
		t.Errorf("unknown item: got %d %s, want 404 catalog_item_not_found", s, c)
	}
	if s, c := code(mustFail(t, tagOf(api, ctx, pack.ID))); s != 404 || c != "catalog_tag_not_found" {
		t.Errorf("unknown tag: got %d %s, want 404 catalog_tag_not_found", s, c)
	}

	// 4. 包内容变化后目录过期：仍是 409，但码与「从未构建」不同。
	mustExec(t, db, `UPDATE packs SET config_revision=config_revision+1`)
	if s, c := code(mustFail(t, catalogOf(api, ctx, pack.ID))); s != 409 || c != "catalog_stale" {
		t.Errorf("stale: got %d %s, want 409 catalog_stale", s, c)
	}
	if s, c := code(mustFail(t, iconOf(api, ctx, pack.ID))); s != 409 || c != "catalog_stale" {
		t.Errorf("stale icon: got %d %s, want 409 catalog_stale", s, c)
	}
}

func mustFail(t *testing.T, err error) error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	return err
}

func catalogOf(api *API, ctx context.Context, packID string) error {
	_, err := api.GetItemCatalog(ctx, packID, "zh_cn")
	return err
}

func itemOf(api *API, ctx context.Context, packID string) error {
	_, err := api.GetCatalogItem(ctx, packID, "minecraft:not_a_thing", "zh_cn")
	return err
}

func tagOf(api *API, ctx context.Context, packID string) error {
	_, err := api.GetCatalogTag(ctx, packID, "item", "minecraft:nope", "zh_cn")
	return err
}

func iconOf(api *API, ctx context.Context, packID string) error {
	_, err := api.GetCatalogIcon(ctx, packID, "minecraft:iron_ingot")
	return err
}
