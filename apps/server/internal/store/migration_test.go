package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLegacyV1UpgradePreservesRows documents the one supported prototype
// upgrade: old empty-string sha1 values become NULL and old task enums are
// mapped to their v7 names without dropping the pack or its records.
func TestLegacyV1UpgradePreservesRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema fixture: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create legacy migration metadata: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES (1, 1)`); err != nil {
		t.Fatalf("insert legacy migration metadata: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO packs(id, name, created_at, updated_at, last_edited_at) VALUES ('p1','Legacy',1,1,1)`); err != nil {
		t.Fatalf("insert legacy pack: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO pack_mods(id, pack_id, source, project_id, version_id, display_name, sha1, added_at) VALUES ('m1','p1','curseforge','project','version','Example','',1)`); err != nil {
		t.Fatalf("insert legacy mod: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks(id, pack_id, kind, title, status, created_at) VALUES ('t1','p1','pack','Build','success',1)`); err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	defer upgraded.Close()
	var name string
	if err := upgraded.QueryRow(`SELECT name FROM packs WHERE id='p1'`).Scan(&name); err != nil {
		t.Fatalf("read migrated pack: %v", err)
	}
	if name != "Legacy" {
		t.Fatalf("migrated pack name = %q, want Legacy", name)
	}
	var sha sql.NullString
	if err := upgraded.QueryRow(`SELECT sha1 FROM pack_mods WHERE id='m1'`).Scan(&sha); err != nil {
		t.Fatalf("read migrated sha1: %v", err)
	}
	if sha.Valid {
		t.Fatalf("migrated empty sha1 = %q, want NULL", sha.String)
	}
	var status, kind string
	if err := upgraded.QueryRow(`SELECT status, kind FROM tasks WHERE id='t1'`).Scan(&status, &kind); err != nil {
		t.Fatalf("read migrated task: %v", err)
	}
	if status != "succeeded" || kind != "build" {
		t.Fatalf("migrated task = kind %q status %q, want build/succeeded", kind, status)
	}
	var version int
	if err := upgraded.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read migrated version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}
}

func TestMigrationChecksumTamperingIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tampered.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("initial Open: %v", err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum = ? WHERE version = 2`, strings.Repeat("0", 64)); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Open after checksum tampering error = %v, want checksum mismatch", err)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repeat.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != CurrentSchemaVersion {
		t.Fatalf("migration row count = %d, want %d", count, CurrentSchemaVersion)
	}
}
