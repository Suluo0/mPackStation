package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestV7CanonicalSchemaHasRequiredTables is a deliberately strict acceptance
// test. It is expected to fail while the historical v1 baseline is still the
// only schema. A migration may add tables in phases, but it must not remove
// any table in this canonical set once the corresponding phase is released.
func TestV7CanonicalSchemaHasRequiredTables(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	required := []string{
		"schema_migrations", "packs", "pack_mods", "jar_index", "pack_locks",
		"pack_versions", "pack_current_version", "conflicts", "mod_dependencies",
		"pack_alerts", "pack_mod_updates", "tasks", "task_events", "task_idem_keys",
		"outbox_events", "activities", "audit_events", "settings", "secrets",
		"onboarding_state", "remote_cache", "blob_grace", "import_previews",
		"content_documents", "content_revisions", "content_validation_runs",
		"quest_books", "quest_revisions", "quest_chapters", "quest_nodes", "quest_edges",
		"pack_version_inputs", "delivery_checks", "artifacts", "releases",
		"allowed_export_dirs",
	}
	for _, table := range required {
		if !sqliteObjectExists(t, db, "table", table) {
			t.Errorf("V7-DB-001 missing required table %q; implement the canonical migration before opening repository work", table)
		}
	}
}

func TestMigrationMetadataContainsImmutableIdentity(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	for _, column := range []string{"version", "name", "checksum", "applied_at"} {
		if !sqliteColumnExists(t, db, "schema_migrations", column) {
			t.Errorf("V7-DB-002 schema_migrations.%s is missing; migration identity cannot be checked", column)
		}
	}

	if !sqliteColumnExists(t, db, "schema_migrations", "checksum") {
		return
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE checksum IS NULL OR checksum = ''`).Scan(&count); err != nil {
		t.Fatalf("V7-DB-002 query migration checksum: %v", err)
	}
	if count != 0 {
		t.Fatalf("V7-DB-002 found %d migration rows without a non-empty checksum", count)
	}
}

func TestSQLiteStartupIntegrityChecks(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	var foreignKeys int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("V7-DB-003 read foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("V7-DB-003 foreign_keys = %d, want 1 on the opened connection", foreignKeys)
	}

	var quick string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&quick); err != nil {
		t.Fatalf("V7-DB-003 quick_check failed: %v", err)
	}
	if !strings.EqualFold(quick, "ok") {
		t.Fatalf("V7-DB-003 quick_check = %q, want ok", quick)
	}

	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("V7-DB-003 foreign_key_check failed: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, rowID, parent, foreignKey any
		if err := rows.Scan(&table, &rowID, &parent, &foreignKey); err != nil {
			t.Fatalf("V7-DB-003 read foreign_key_check result: %v", err)
		}
		t.Fatalf("V7-DB-003 foreign_key_check reported table=%v row=%v parent=%v fk=%v", table, rowID, parent, foreignKey)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("V7-DB-003 iterate foreign_key_check: %v", err)
	}

	var jsonValid int
	if err := db.QueryRow(`SELECT json_valid(?)`, `{}`).Scan(&jsonValid); err != nil {
		t.Fatalf("V7-DB-003 JSON1 probe failed: %v", err)
	}
	if jsonValid != 1 {
		t.Fatalf("V7-DB-003 JSON1 probe = %d, want 1", jsonValid)
	}
}

func TestPackModsUsesNullableSHA1AndUniqueIndexes(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	sha1, ok := sqliteColumn(t, db, "pack_mods", "sha1")
	if !ok {
		t.Fatalf("V7-DB-004 pack_mods.sha1 is missing")
	}
	if sha1.notNull {
		t.Fatalf("V7-DB-004 pack_mods.sha1 is NOT NULL; an undownloaded mod must be represented by NULL")
	}

	if !sqliteUniqueIndexWithTerms(t, db, "pack_mods", []string{"pack_id", "sha1", "local"}) {
		t.Errorf("V7-DB-004 missing named/identifiable partial unique index for local (pack_id, sha1) when sha1 IS NOT NULL")
	}
	if !sqliteUniqueIndexWithTerms(t, db, "pack_mods", []string{"pack_id", "source", "project_id"}) {
		t.Errorf("V7-DB-004 missing named/identifiable partial unique index for remote (pack_id, source, project_id)")
	}

	if !foreignKeyTargets(t, db, "pack_mods", "jar_index") {
		t.Errorf("V7-DB-004 pack_mods.sha1 must reference jar_index.sha1")
	}
}

func TestRevisionAppliedStateHasDatabaseUniqueness(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	for _, spec := range []struct {
		table string
		parent string
	}{
		{table: "content_revisions", parent: "document_id"},
		{table: "quest_revisions", parent: "quest_book_id"},
	} {
		if !sqliteObjectExists(t, db, "table", spec.table) {
			t.Errorf("V7-DB-005 missing %s; revision invariant is not executable", spec.table)
			continue
		}
		if !hasPartialAppliedUnique(t, db, spec.table, spec.parent) {
			t.Errorf("V7-DB-005 %s must allow at most one state='applied' row per %s", spec.table, spec.parent)
		}
	}
}

func TestPackVersionPointersAreSamePackConstrained(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	if !sqliteObjectExists(t, db, "table", "pack_current_version") {
		t.Fatalf("V7-DB-006 missing pack_current_version pointer table")
	}
	if !foreignKeyTargets(t, db, "pack_current_version", "pack_versions") {
		t.Errorf("V7-DB-006 current version pointer must have a foreign key to pack_versions")
	}
	if !hasCompositeForeignKey(t, db, "pack_current_version") {
		t.Errorf("V7-DB-006 current version pointer needs a composite (pack_id, pack_version_id) relationship to prevent cross-pack references")
	}
	if !hasCompositeForeignKey(t, db, "pack_versions") {
		t.Errorf("V7-DB-006 pack_versions.lock_id needs a composite same-pack relationship to pack_locks")
	}
}

func TestImportPreviewAndBuildInputProvenanceExist(t *testing.T) {
	db := openAcceptanceDB(t)
	defer db.Close()

	for _, table := range []string{"import_previews", "pack_version_inputs"} {
		if !sqliteObjectExists(t, db, "table", table) {
			t.Errorf("V7-DB-007 missing %s; restart-safe import/build provenance cannot be verified", table)
		}
	}
	for _, column := range []string{"token_hash", "input_hash", "source", "staged_path", "expires_at", "consumed_at"} {
		if sqliteObjectExists(t, db, "table", "import_previews") && !sqliteColumnExists(t, db, "import_previews", column) {
			t.Errorf("V7-DB-007 import_previews.%s is missing", column)
		}
	}
}

type sqliteColumnInfo struct {
	notNull bool
}

func sqliteColumn(t *testing.T, db *sql.DB, table, column string) (sqliteColumnInfo, bool) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		t.Fatalf("inspect table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		if name == column {
			return sqliteColumnInfo{notNull: notNull == 1}, true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(%s): %v", table, err)
	}
	return sqliteColumnInfo{}, false
}

func sqliteColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	_, ok := sqliteColumn(t, db, table, column)
	return ok
}

func sqliteObjectExists(t *testing.T, db *sql.DB, objectType, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, objectType, name).Scan(&count); err != nil {
		t.Fatalf("inspect sqlite_master for %s %s: %v", objectType, name, err)
	}
	return count == 1
}

func sqliteUniqueIndexWithTerms(t *testing.T, db *sql.DB, table string, terms []string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master WHERE type='index' AND tbl_name=?`, table)
	if err != nil {
		t.Fatalf("inspect indexes on %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var ddl sql.NullString
		var name string
		if err := rows.Scan(&name, &ddl); err != nil {
			t.Fatalf("scan index on %s: %v", table, err)
		}
		text := strings.ToLower(ddl.String)
		if strings.Contains(text, "unique index") && strings.Contains(text, "where") {
			matched := true
			for _, term := range terms {
				if !strings.Contains(text, strings.ToLower(term)) {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indexes on %s: %v", table, err)
	}
	return false
}

func foreignKeyTargets(t *testing.T, db *sql.DB, table, target string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_list(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		t.Fatalf("inspect foreign keys on %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, seq int
		var parent, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan foreign keys on %s: %v", table, err)
		}
		if parent == target {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign keys on %s: %v", table, err)
	}
	return false
}

func hasCompositeForeignKey(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_list(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		t.Fatalf("inspect foreign keys on %s: %v", table, err)
	}
	defer rows.Close()
	counts := map[int]int{}
	for rows.Next() {
		var id, seq int
		var parent, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan foreign keys on %s: %v", table, err)
		}
		counts[id]++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign keys on %s: %v", table, err)
	}
	for _, count := range counts {
		if count > 1 {
			return true
		}
	}
	return false
}

func hasPartialAppliedUnique(t *testing.T, db *sql.DB, table, parent string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name=?`, table)
	if err != nil {
		t.Fatalf("inspect indexes on %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var ddl sql.NullString
		if err := rows.Scan(&ddl); err != nil {
			t.Fatalf("scan revision index on %s: %v", table, err)
		}
		text := strings.ToLower(ddl.String)
		if strings.Contains(text, "unique") && strings.Contains(text, "where") && strings.Contains(text, "state") && strings.Contains(text, "applied") && strings.Contains(text, strings.ToLower(parent)) {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate revision indexes on %s: %v", table, err)
	}
	return false
}

func quoteSQLiteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func openAcceptanceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "acceptance.db"))
	if err != nil {
		t.Fatalf("open acceptance database: %v", err)
	}
	return db
}

func ExampleV7AcceptanceEvidence() {
	// This example is intentionally documentation-first: each failure names a
	// stable gate so a human can map it to the migration or issue being fixed.
	fmt.Println("V7-DB-001..007: canonical schema and database invariants")
	// Output: V7-DB-001..007: canonical schema and database invariants
}
