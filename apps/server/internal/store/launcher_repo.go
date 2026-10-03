package store

import (
	"context"
	"database/sql"
	"errors"
)

// LauncherInstallRecord is one Minecraft version the launcher kernel actually
// produced in a local instance directory. VersionID is the string the launch
// command must use later — it is not the pack's mc_version once a loader is
// involved (see migrations/0022).
type LauncherInstallRecord struct {
	ID           string
	MinecraftDir string
	VersionID    string
	Loader       string
	MCVersion    string
	PackID       sql.NullString
	TaskID       string
	InstalledAt  int64
}

// RecordLauncherInstall upserts the install by (minecraft_dir, version_id):
// reinstalling the same version into the same directory is an update, not a
// second row, so the launch resolver always sees one truth per installed version.
func (r *Repository) RecordLauncherInstall(ctx context.Context, rec LauncherInstallRecord) (LauncherInstallRecord, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO launcher_installs(id,minecraft_dir,version_id,loader,mc_version,pack_id,task_id,installed_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(minecraft_dir,version_id) DO UPDATE SET
			loader=excluded.loader, mc_version=excluded.mc_version,
			pack_id=excluded.pack_id, task_id=excluded.task_id, installed_at=excluded.installed_at`,
		rec.ID, rec.MinecraftDir, rec.VersionID, rec.Loader, rec.MCVersion, rec.PackID, rec.TaskID, rec.InstalledAt)
	if err != nil {
		return LauncherInstallRecord{}, err
	}
	return r.getLauncherInstall(ctx, rec.MinecraftDir, rec.VersionID)
}

// LatestLauncherInstall resolves the version to launch for a directory.
// A pack-scoped install wins over an unattributed one in the same directory,
// because the pack is what the user is actually working on.
func (r *Repository) LatestLauncherInstall(ctx context.Context, packID, minecraftDir string) (LauncherInstallRecord, error) {
	if packID != "" {
		if rec, err := r.queryLauncherInstall(ctx, `WHERE pack_id=? AND minecraft_dir=? ORDER BY installed_at DESC LIMIT 1`, packID, minecraftDir); err == nil {
			return rec, nil
		} else if !errors.Is(err, ErrNotFound) {
			return LauncherInstallRecord{}, err
		}
	}
	return r.queryLauncherInstall(ctx, `WHERE minecraft_dir=? ORDER BY installed_at DESC LIMIT 1`, minecraftDir)
}

// ListLauncherInstalls returns the installed versions for a directory and/or a
// pack, newest first. Empty filters mean "no constraint on that column".
func (r *Repository) ListLauncherInstalls(ctx context.Context, packID, minecraftDir string) ([]LauncherInstallRecord, error) {
	clause := "WHERE 1=1"
	args := []any{}
	if packID != "" {
		clause += " AND pack_id=?"
		args = append(args, packID)
	}
	if minecraftDir != "" {
		clause += " AND minecraft_dir=?"
		args = append(args, minecraftDir)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,minecraft_dir,version_id,loader,mc_version,pack_id,task_id,installed_at
		FROM launcher_installs `+clause+` ORDER BY installed_at DESC LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LauncherInstallRecord, 0, 4)
	for rows.Next() {
		rec, err := scanLauncherInstall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *Repository) getLauncherInstall(ctx context.Context, dir, versionID string) (LauncherInstallRecord, error) {
	return r.queryLauncherInstall(ctx, `WHERE minecraft_dir=? AND version_id=?`, dir, versionID)
}

func (r *Repository) queryLauncherInstall(ctx context.Context, clause string, args ...any) (LauncherInstallRecord, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,minecraft_dir,version_id,loader,mc_version,pack_id,task_id,installed_at
		FROM launcher_installs `+clause, args...)
	rec, err := scanLauncherInstall(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LauncherInstallRecord{}, ErrNotFound
	}
	if err != nil {
		return LauncherInstallRecord{}, err
	}
	return rec, nil
}

type launcherInstallScanner interface {
	Scan(dest ...any) error
}

func scanLauncherInstall(row launcherInstallScanner) (LauncherInstallRecord, error) {
	var rec LauncherInstallRecord
	err := row.Scan(&rec.ID, &rec.MinecraftDir, &rec.VersionID, &rec.Loader, &rec.MCVersion, &rec.PackID, &rec.TaskID, &rec.InstalledAt)
	return rec, err
}
