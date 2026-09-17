package store

// mod_content_repo.go owns the SQL boundary for extracted mod content.
// Domain validation stays in service; this package only performs parameterized
// SQL and commits the related activity/outbox/audit records atomically.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
)

var modContentSeq uint64

// ModContentRecord is one extracted content row from a mod jar.
type ModContentRecord struct {
	ID, PackID, ModID, Modid, Version, Kind, Path, Key, Payload, ParseError string
	IsDynamic, ParsedAt                                                         int64
}

// ModContentRunRecord is one parse run summary. UNIQUE(mod_id, sha1) makes
// repeat parsing of the same jar version idempotent.
type ModContentRunRecord struct {
	ID, PackID, ModID, SHA1, Status, ErrorMessage          string
	TotalFiles, ParsedCount, DynamicCount, ErrorCount      int
	StartedAt, FinishedAt                                    int64
}

func ModContentEvidence(packID, requestID string, at int64, action, aggregateID string, detail any) (ActivityRecord, []any, []any) {
	detailJSON, _ := json.Marshal(detail)
	seq := atomic.AddUint64(&modContentSeq, 1)
	return ActivityRecord{ID: fmt.Sprintf("activity-modcontent-%s-%d-%d", aggregateID, at, seq), PackID: packID, Kind: "mod_content", Action: action, Text: action, At: at},
		[]any{fmt.Sprintf("outbox-modcontent-%s-%d-%d", aggregateID, at, seq), packID, "pack_mod", aggregateID, "mod_content." + action, string(detailJSON), 0, at},
		[]any{fmt.Sprintf("audit-modcontent-%s-%s-%d-%d", action, aggregateID, at, seq), packID, action, string(detailJSON), requestID, at}
}

func AddModContentEvidence(ctx context.Context, r *Repository, packID, requestID string, at int64, action, aggregateID string, detail any) error {
	a, outbox, audit := ModContentEvidence(packID, requestID, at, action, aggregateID, detail)
	if err := r.AddActivity(ctx, a, detailMap(detail), requestID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO outbox_events(id,pack_id,aggregate_type,aggregate_id,event_type,payload,attempts,next_attempt_at,created_at) VALUES (?,?,?,?,?,?,0,?,?)`, outbox...); err != nil {
		return fmt.Errorf("insert mod_content outbox: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO audit_events(id,pack_id,principal_kind,principal_id,action,detail,request_id,created_at) VALUES (?,?, 'local','local',?,?,?,?)`, audit...); err != nil {
		return fmt.Errorf("insert mod_content audit: %w", err)
	}
	return nil
}

// UpsertModContentRun inserts or updates a run row. UNIQUE(mod_id, sha1)
// means a re-parse of the same version updates the existing run in place.
func (r *Repository) UpsertModContentRun(ctx context.Context, run ModContentRunRecord) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO mod_content_runs(id,pack_id,mod_id,sha1,status,total_files,parsed_count,dynamic_count,error_count,error_message,started_at,finished_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(mod_id,sha1) DO UPDATE SET status=excluded.status,total_files=excluded.total_files,parsed_count=excluded.parsed_count,dynamic_count=excluded.dynamic_count,error_count=excluded.error_count,error_message=excluded.error_message,finished_at=excluded.finished_at`,
		run.ID, run.PackID, run.ModID, run.SHA1, run.Status, run.TotalFiles, run.ParsedCount, run.DynamicCount, run.ErrorCount, run.ErrorMessage, run.StartedAt, nullInt64(run.FinishedAt))
	return err
}

// GetModContentRun returns the latest run for a mod (any sha1), or ErrNotFound.
func (r *Repository) LatestModContentRun(ctx context.Context, modID string) (ModContentRunRecord, error) {
	var run ModContentRunRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,mod_id,sha1,status,total_files,parsed_count,dynamic_count,error_count,error_message,started_at,COALESCE(finished_at,0) FROM mod_content_runs WHERE mod_id=? ORDER BY started_at DESC LIMIT 1`, modID).
		Scan(&run.ID, &run.PackID, &run.ModID, &run.SHA1, &run.Status, &run.TotalFiles, &run.ParsedCount, &run.DynamicCount, &run.ErrorCount, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

// GetModContentRunBySHA returns the run for an exact (mod_id, sha1).
func (r *Repository) GetModContentRunBySHA(ctx context.Context, modID, sha1 string) (ModContentRunRecord, error) {
	var run ModContentRunRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,mod_id,sha1,status,total_files,parsed_count,dynamic_count,error_count,error_message,started_at,COALESCE(finished_at,0) FROM mod_content_runs WHERE mod_id=? AND sha1=?`, modID, sha1).
		Scan(&run.ID, &run.PackID, &run.ModID, &run.SHA1, &run.Status, &run.TotalFiles, &run.ParsedCount, &run.DynamicCount, &run.ErrorCount, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

// ReplaceModContent deletes all existing rows for a mod and inserts the new
// set atomically. Called inside the task handler transaction.
func (r *Repository) ReplaceModContent(ctx context.Context, packID, modID string, items []ModContentRecord) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		if _, err := tx.db.ExecContext(ctx, `DELETE FROM mod_content WHERE mod_id=?`, modID); err != nil {
			return fmt.Errorf("delete old mod_content: %w", err)
		}
		for _, it := range items {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO mod_content(id,pack_id,mod_id,modid,version,kind,path,key,payload,is_dynamic,parse_error,parsed_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				it.ID, it.PackID, it.ModID, it.Modid, it.Version, it.Kind, it.Path, it.Key, it.Payload, it.IsDynamic, it.ParseError, it.ParsedAt); err != nil {
				return fmt.Errorf("insert mod_content %s: %w", it.Path, err)
			}
		}
		return nil
	})
}

// ReplaceModContentAndFinishRun atomically replaces all mod_content rows for a
// mod and updates the corresponding run row to its terminal state. This
// guarantees that content and run status never diverge (e.g. content replaced
// but run stuck in "running").
func (r *Repository) ReplaceModContentAndFinishRun(ctx context.Context, packID, modID string, items []ModContentRecord, run ModContentRunRecord) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		if _, err := tx.db.ExecContext(ctx, `DELETE FROM mod_content WHERE mod_id=?`, modID); err != nil {
			return fmt.Errorf("delete old mod_content: %w", err)
		}
		for _, it := range items {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO mod_content(id,pack_id,mod_id,modid,version,kind,path,key,payload,is_dynamic,parse_error,parsed_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				it.ID, it.PackID, it.ModID, it.Modid, it.Version, it.Kind, it.Path, it.Key, it.Payload, it.IsDynamic, it.ParseError, it.ParsedAt); err != nil {
				return fmt.Errorf("insert mod_content %s: %w", it.Path, err)
			}
		}
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO mod_content_runs(id,pack_id,mod_id,sha1,status,total_files,parsed_count,dynamic_count,error_count,error_message,started_at,finished_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(mod_id,sha1) DO UPDATE SET status=excluded.status,total_files=excluded.total_files,parsed_count=excluded.parsed_count,dynamic_count=excluded.dynamic_count,error_count=excluded.error_count,error_message=excluded.error_message,finished_at=excluded.finished_at`,
			run.ID, run.PackID, run.ModID, run.SHA1, run.Status, run.TotalFiles, run.ParsedCount, run.DynamicCount, run.ErrorCount, run.ErrorMessage, run.StartedAt, nullInt64(run.FinishedAt)); err != nil {
			return fmt.Errorf("upsert mod_content_runs: %w", err)
		}
		return nil
	})
}

// ListModContent returns paginated content rows for a mod, optionally filtered
// by kind. Cursor is the id of the last row from the previous page.
func (r *Repository) ListModContent(ctx context.Context, packID, modID, kind string, limit int, cursor string) ([]ModContentRecord, string, int, error) {
	q := `SELECT id,pack_id,mod_id,modid,version,kind,path,key,payload,is_dynamic,parse_error,parsed_at FROM mod_content WHERE pack_id=? AND mod_id=?`
	args := []any{packID, modID}
	if kind != "" {
		q += ` AND kind=?`
		args = append(args, kind)
	}
	var total int
	countQ := `SELECT COUNT(*) FROM mod_content WHERE pack_id=? AND mod_id=?`
	countArgs := []any{packID, modID}
	if kind != "" {
		countQ += ` AND kind=?`
		countArgs = append(countArgs, kind)
	}
	if err := r.db.QueryRowContext(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, "", 0, err
	}
	if cursor != "" {
		q += ` AND id > ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY id LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", 0, err
	}
	defer rows.Close()
	out := make([]ModContentRecord, 0, limit)
	for rows.Next() {
		var it ModContentRecord
		if err := rows.Scan(&it.ID, &it.PackID, &it.ModID, &it.Modid, &it.Version, &it.Kind, &it.Path, &it.Key, &it.Payload, &it.IsDynamic, &it.ParseError, &it.ParsedAt); err != nil {
			return nil, "", 0, err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}
	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].ID
		out = out[:limit]
	}
	return out, nextCursor, total, nil
}

// GetModContent returns a single content row by id.
func (r *Repository) GetModContent(ctx context.Context, packID, modID, id string) (ModContentRecord, error) {
	var it ModContentRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,mod_id,modid,version,kind,path,key,payload,is_dynamic,parse_error,parsed_at FROM mod_content WHERE pack_id=? AND mod_id=? AND id=?`, packID, modID, id).
		Scan(&it.ID, &it.PackID, &it.ModID, &it.Modid, &it.Version, &it.Kind, &it.Path, &it.Key, &it.Payload, &it.IsDynamic, &it.ParseError, &it.ParsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
