package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenRunsIntegrityCheckOnEveryStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integrity.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	// Introduce a foreign-key violation while checks are disabled, simulating a
	// damaged database left by an interrupted external process.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`INSERT INTO pack_mods(id, pack_id, source, project_id, version_id, display_name, added_at, updated_at) VALUES ('orphan','missing','curseforge','p','v','orphan',1,1)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	if reopened, err := Open(path); err == nil || !strings.Contains(err.Error(), "foreign key violation") {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("Open() = db=%v err=%v, want integrity failure", reopened, err)
	}
}
