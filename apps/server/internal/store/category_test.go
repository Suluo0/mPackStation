package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// 0027：用户自定义分类要从两条读取路径（ListPackMods / ListPackMembers）都能出来。
func TestPackModCategoryAcrossReads(t *testing.T) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "mpackstation.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UnixMilli()
	if _, err := db.ExecContext(ctx, `INSERT INTO packs(id,name,created_at,updated_at,last_edited_at) VALUES('p1','p1',?,?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pack_mods(id,pack_id,source,display_name,status,added_at,updated_at,category,project_id) VALUES('m1','p1','modrinth','JEI','installed',?,?,'优化','proj-jei')`, now, now); err != nil {
		t.Fatal(err)
	}
	r := NewRepository(db)
	throughMods, err := r.ListPackMods(ctx, "p1")
	if err != nil || len(throughMods) != 1 || throughMods[0].Category != "优化" {
		t.Fatalf("ListPackMods category = %+v (err=%v)", throughMods, err)
	}
	throughMembers, err := r.ListPackMembers(ctx, "p1")
	if err != nil || len(throughMembers) != 1 || throughMembers[0].Category != "优化" {
		t.Fatalf("ListPackMembers category = %+v (err=%v)", throughMembers, err)
	}
}
