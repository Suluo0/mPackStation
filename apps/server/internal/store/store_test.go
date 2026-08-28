package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenInitializesSchemaAndVersion(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "mpackstation.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var version, foreignKeys int
	if err := db.QueryRow(`SELECT version FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("schema_migrations query error = %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("foreign_keys query error = %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	if err := PingContext(context.Background(), db); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
}
