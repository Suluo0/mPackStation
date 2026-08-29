package store

import (
	"context"
	"database/sql"
	"fmt"
)

type ImportPreviewRecord struct {
	ID, TokenHash, InputHash, Source, StagedPath string
	ExpiresAt, ConsumedAt, CreatedAt             sql.NullInt64
}

func (r *Repository) CreateImportPreview(ctx context.Context, p ImportPreviewRecord) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO import_previews(id,token_hash,input_hash,source,staged_path,expires_at,consumed_at,created_at) VALUES(?,?,?,?,?,?,?,?)`, p.ID, p.TokenHash, p.InputHash, p.Source, p.StagedPath, p.ExpiresAt.Int64, nullInt64Arg(p.ConsumedAt), p.CreatedAt.Int64)
	if err != nil { return fmt.Errorf("create import preview: %w", err) }
	return nil
}

func (r *Repository) GetImportPreview(ctx context.Context, id string) (ImportPreviewRecord, error) {
	var p ImportPreviewRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,token_hash,input_hash,source,staged_path,expires_at,consumed_at,created_at FROM import_previews WHERE id=?`, id).Scan(&p.ID, &p.TokenHash, &p.InputHash, &p.Source, &p.StagedPath, &p.ExpiresAt, &p.ConsumedAt, &p.CreatedAt)
	if err == sql.ErrNoRows { return ImportPreviewRecord{}, ErrNotFound }
	if err != nil { return ImportPreviewRecord{}, fmt.Errorf("get import preview: %w", err) }
	return p, nil
}

func (r *Repository) ConsumeImportPreview(ctx context.Context, id, tokenHash, inputHash string, now int64) (ImportPreviewRecord, error) {
	var out ImportPreviewRecord
	err := r.WithTx(ctx, func(tx *Repository) error {
		var p ImportPreviewRecord
		err := tx.db.QueryRowContext(ctx, `SELECT id,token_hash,input_hash,source,staged_path,expires_at,consumed_at,created_at FROM import_previews WHERE id=?`, id).Scan(&p.ID, &p.TokenHash, &p.InputHash, &p.Source, &p.StagedPath, &p.ExpiresAt, &p.ConsumedAt, &p.CreatedAt)
		if err == sql.ErrNoRows { return ErrNotFound }
		if err != nil { return err }
		if p.TokenHash != tokenHash || p.InputHash != inputHash { return ErrConflict }
		if p.ConsumedAt.Valid || p.ExpiresAt.Int64 <= now { return ErrConflict }
		res, err := tx.db.ExecContext(ctx, `UPDATE import_previews SET consumed_at=? WHERE id=? AND consumed_at IS NULL AND expires_at>?`, now, id, now)
		if err != nil { return fmt.Errorf("consume import preview: %w", err) }
		n, err := res.RowsAffected(); if err != nil || n != 1 { return ErrConflict }
		p.ConsumedAt = sql.NullInt64{Int64: now, Valid: true}; out = p
		return nil
	})
	if err != nil { return ImportPreviewRecord{}, err }
	return out, nil
}

func nullInt64Arg(v sql.NullInt64) any { if v.Valid { return v.Int64 }; return nil }
