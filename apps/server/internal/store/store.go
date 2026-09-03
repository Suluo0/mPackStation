// Package store owns SQLite connections, migrations, and repository boundaries.
// No other package is allowed to execute business SQL directly.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// migrationFS contains immutable, numbered SQL migrations. A migration that
// has been applied must never be edited; append a new file instead.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationNamePattern = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

type migration struct {
	version int
	name    string
	path    string
	sql     string
	hash    string
}

// CurrentSchemaVersion is the highest migration shipped by this binary.
const CurrentSchemaVersion = 7

// V7AcceptanceEvidence is a documentation anchor used by the human-readable
// schema acceptance examples.
type V7AcceptanceEvidence struct{}

// Open opens (or creates) path and brings it to the latest schema. The
// returned database is configured for SQLite's single-writer model.
func Open(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	db, err := sql.Open("sqlite", absPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite has one writer. A single connection also ensures connection-scoped
	// PRAGMAs (notably foreign_keys) are applied consistently.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := configure(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Integrity is checked on every startup, including when no migration ran.
	// This prevents a previously-corrupted database from reaching HTTP listen.
	if err := CheckIntegrity(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// CheckIntegrity runs SQLite quick_check and foreign_key_check on the current
// database. It is intentionally exported for startup and integration tests.
func CheckIntegrity(db *sql.DB) error {
	if db == nil {
		return errors.New("database is nil")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin integrity check: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := checkIntegrity(tx); err != nil {
		return err
	}
	return nil
}

func configure(db *sql.DB) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("configure sqlite (%s): %w", pragma, err)
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationNamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid migration version %q", match[1])
		}
		contents, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		hash := sha256.Sum256(contents)
		migrations = append(migrations, migration{
			version: version,
			name:    match[2],
			path:    entry.Name(),
			sql:     string(contents),
			hash:    fmt.Sprintf("%x", hash[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for i, m := range migrations {
		want := i + 1
		if m.version != want {
			return nil, fmt.Errorf("migration sequence gap: got %04d, want %04d", m.version, want)
		}
	}
	return migrations, nil
}

func migrate(db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return errors.New("no migrations bundled")
	}
	if err := ensureMigrationTable(db); err != nil {
		return err
	}
	rows, err := readMigrationRows(db)
	if err != nil {
		return err
	}
	byVersion := make(map[int]migration, len(migrations))
	for _, m := range migrations {
		byVersion[m.version] = m
	}
	for version, row := range rows {
		m, ok := byVersion[version]
		if !ok {
			return fmt.Errorf("database schema version %d is not supported by this binary", version)
		}
		// The prototype's schema_migrations table predated name/checksum. It is
		// trusted only when it is clearly the v1 shape.
		if row.checksum == "" && version == 1 && legacySchemaPresent(db) {
			if err := backfillLegacyMigration(db, m); err != nil {
				return err
			}
			row.name, row.checksum = m.name, m.hash
			rows[version] = row
		}
		if row.name != m.name || row.checksum != m.hash {
			return fmt.Errorf("migration %04d checksum mismatch (database=%s, binary=%s)", version, row.checksum, m.hash)
		}
	}

	current := 0
	for version := range rows {
		if version > current {
			current = version
		}
	}
	if current > migrations[len(migrations)-1].version {
		return fmt.Errorf("database schema version %d is newer than server version %d", current, migrations[len(migrations)-1].version)
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return err
		}
	}
	return nil
}

type migrationRow struct {
	name, checksum string
}

func ensureMigrationTable(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema metadata: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&exists); err != nil {
		return fmt.Errorf("inspect schema metadata: %w", err)
	}
	if exists == 0 {
		if _, err := tx.Exec(`CREATE TABLE schema_migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            checksum TEXT NOT NULL,
            applied_at INTEGER NOT NULL
        )`); err != nil {
			return fmt.Errorf("create schema metadata: %w", err)
		}
	} else {
		columns, err := tableColumns(tx, "schema_migrations")
		if err != nil {
			return err
		}
		if _, ok := columns["name"]; !ok {
			if _, err := tx.Exec(`ALTER TABLE schema_migrations ADD COLUMN name TEXT NOT NULL DEFAULT ''`); err != nil {
				return fmt.Errorf("add migration name: %w", err)
			}
		}
		if _, ok := columns["checksum"]; !ok {
			if _, err := tx.Exec(`ALTER TABLE schema_migrations ADD COLUMN checksum TEXT NOT NULL DEFAULT ''`); err != nil {
				return fmt.Errorf("add migration checksum: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema metadata: %w", err)
	}
	return nil
}

func tableColumns(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query(`PRAGMA table_info("` + strings.ReplaceAll(table, `"`, `""`) + `")`)
	if err != nil {
		return nil, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan table %s: %w", table, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table %s: %w", table, err)
	}
	return columns, nil
}

func readMigrationRows(db *sql.DB) (map[int]migrationRow, error) {
	rows, err := db.Query(`SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read schema migrations: %w", err)
	}
	defer rows.Close()
	result := make(map[int]migrationRow)
	previous := 0
	for rows.Next() {
		var version int
		var row migrationRow
		if err := rows.Scan(&version, &row.name, &row.checksum); err != nil {
			return nil, fmt.Errorf("scan schema migration: %w", err)
		}
		if version != previous+1 {
			return nil, fmt.Errorf("schema migration history has a gap at %d", previous+1)
		}
		result[version] = row
		previous = version
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema migration rows: %w", err)
	}
	return result, nil
}

func backfillLegacyMigration(db *sql.DB, m migration) error {
	if _, err := db.Exec(`UPDATE schema_migrations SET name = ?, checksum = ? WHERE version = 1 AND (name = '' OR checksum = '')`, m.name, m.hash); err != nil {
		return fmt.Errorf("record legacy migration checksum: %w", err)
	}
	return nil
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %04d: %w", m.version, err)
	}
	defer func() { _ = tx.Rollback() }()
	if m.version == 2 && legacySchemaPresentTx(tx) {
		if err := upgradeLegacyV1(tx); err != nil {
			return fmt.Errorf("upgrade legacy v1: %w", err)
		}
	}
	if _, err := tx.Exec(m.sql); err != nil {
		return fmt.Errorf("apply migration %04d (%s): %w", m.version, m.name, err)
	}
	if err := checkIntegrity(tx); err != nil {
		return fmt.Errorf("migration %04d integrity check: %w", m.version, err)
	}
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`, m.version, m.name, m.hash, now); err != nil {
		return fmt.Errorf("record migration %04d: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %04d: %w", m.version, err)
	}
	return nil
}

func checkIntegrity(tx *sql.Tx) error {
	var quick string
	if err := tx.QueryRow(`PRAGMA quick_check`).Scan(&quick); err != nil {
		return fmt.Errorf("quick_check: %w", err)
	}
	if quick != "ok" {
		return fmt.Errorf("quick_check returned %q", quick)
	}
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, rowid, parent string
		var fk sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fk); err != nil {
			return fmt.Errorf("scan foreign_key_check: %w", err)
		}
		return fmt.Errorf("foreign key violation in %s row %s (parent %s)", table, rowid, parent)
	}
	return rows.Err()
}

func legacySchemaPresent(db *sql.DB) bool {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='packs'`).Scan(&count); err != nil || count == 0 {
		return false
	}
	columns, err := tableColumnsDB(db, "packs")
	return err == nil && !columns["status"]
}

func legacySchemaPresentTx(tx *sql.Tx) bool {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='packs'`).Scan(&count); err != nil || count == 0 {
		return false
	}
	columns, err := tableColumns(tx, "packs")
	return err == nil && !columns["status"]
}

func tableColumnsDB(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info("` + strings.ReplaceAll(table, `"`, `""`) + `")`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

// PingContext keeps readiness checks bounded even when the database is unavailable.
func PingContext(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}
