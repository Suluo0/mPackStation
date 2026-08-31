package store

// This file contains the repository boundary for P6 content and quest data.
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

var p6EvidenceSequence uint64

// ContentDocumentRecord is the persisted content document header.
type ContentDocumentRecord struct {
	ID, PackID, Kind, Slug, Title, ActiveRevisionID string
	CreatedAt, UpdatedAt                            int64
}

// ContentRevisionRecord is an append-only content revision.
type ContentRevisionRecord struct {
	ID, DocumentID, State, Payload, SourceRevisionID string
	Revision                                         int
	CreatedAt                                        int64
}

// ContentValidationRecord is one validation result for a revision.
type ContentValidationRecord struct {
	ID, RevisionID, Status, Issues, AffectedMods string
	CreatedAt                                    int64
}

// QuestBookRecord is the persisted quest book header.
type QuestBookRecord struct {
	ID, PackID, ActiveRevisionID string
	CreatedAt, UpdatedAt         int64
}

// QuestRevisionRecord is an append-only quest revision.
type QuestRevisionRecord struct {
	ID, QuestBookID, State string
	Revision               int
	CreatedAt              int64
}

// QuestChapterRecord is a chapter in a quest revision.
type QuestChapterRecord struct {
	ID, RevisionID, Title, Description, CoverColor string
	Position                                       int
}

// QuestNodeRecord is a node in a quest revision. JSON fields retain the
// extensible prerequisite, reward, and mod reference structures.
type QuestNodeRecord struct {
	ID, RevisionID, ChapterID, Title, Description, Icon string
	X, Y                                                float64
	Prerequisites, Rewards, ModRefs                     string
	Position                                            int
}

// QuestEdgeRecord is a directed edge in a quest revision.
type QuestEdgeRecord struct {
	ID, RevisionID, FromNodeID, ToNodeID string
}

func evidenceArgs(packID, requestID string, at int64, kind, action, aggregateType, aggregateID, eventType string, detail any) (ActivityRecord, []any, []any) {
	detailJSON, _ := json.Marshal(detail)
	seq := atomic.AddUint64(&p6EvidenceSequence, 1)
	return ActivityRecord{ID: fmt.Sprintf("activity-%s-%s-%d-%d", kind, aggregateID, at, seq), PackID: packID, Kind: kind, Action: action, Text: action, At: at},
		[]any{fmt.Sprintf("outbox-%s-%s-%d-%d", eventType, aggregateID, at, seq), packID, aggregateType, aggregateID, eventType, string(detailJSON), at, at},
		[]any{fmt.Sprintf("audit-%s-%s-%d-%d", action, aggregateID, at, seq), packID, action, string(detailJSON), requestID, at}
}

func addP6Evidence(ctx context.Context, r *Repository, packID, requestID string, at int64, kind, action, aggregateType, aggregateID, eventType string, detail any) error {
	a, outbox, audit := evidenceArgs(packID, requestID, at, kind, action, aggregateType, aggregateID, eventType, detail)
	if err := r.AddActivity(ctx, a, detailMap(detail), requestID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO outbox_events(id,pack_id,aggregate_type,aggregate_id,event_type,payload,attempts,next_attempt_at,created_at) VALUES (?,?,?,?,?,?,0,?,?)`, outbox...); err != nil {
		return fmt.Errorf("insert p6 outbox: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO audit_events(id,pack_id,principal_kind,principal_id,action,detail,request_id,created_at) VALUES (?,?, 'local','local',?,?,?,?)`, audit...); err != nil {
		return fmt.Errorf("insert p6 audit: %w", err)
	}
	return nil
}

func detailMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{"value": v}
}

// CreateContentDocument inserts a document and its first draft atomically.
func (r *Repository) CreateContentDocument(ctx context.Context, d ContentDocumentRecord, rev ContentRevisionRecord, requestID string) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO content_documents(id,pack_id,kind,slug,title,active_revision_id,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`, d.ID, d.PackID, d.Kind, d.Slug, d.Title, nullString(d.ActiveRevisionID), d.CreatedAt, d.UpdatedAt); err != nil {
			if isUnique(err) {
				return fmt.Errorf("%w: content document exists", ErrConflict)
			}
			return fmt.Errorf("create content document: %w", err)
		}
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO content_revisions(id,document_id,revision,state,payload,source_revision_id,created_at) VALUES (?,?,?,?,?,?,?)`, rev.ID, rev.DocumentID, rev.Revision, rev.State, rev.Payload, nullString(rev.SourceRevisionID), rev.CreatedAt); err != nil {
			return fmt.Errorf("create content revision: %w", err)
		}
		return addP6Evidence(ctx, tx, d.PackID, requestID, d.CreatedAt, "content", "content.create", "content_document", d.ID, "content.created", map[string]any{"document_id": d.ID, "revision_id": rev.ID})
	})
}

// GetContentDocument returns a document header and its latest revision.
func (r *Repository) GetContentDocument(ctx context.Context, packID, id string) (ContentDocumentRecord, ContentRevisionRecord, error) {
	var d ContentDocumentRecord
	if err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,kind,slug,title,COALESCE(active_revision_id,''),created_at,updated_at FROM content_documents WHERE pack_id=? AND id=?`, packID, id).Scan(&d.ID, &d.PackID, &d.Kind, &d.Slug, &d.Title, &d.ActiveRevisionID, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return d, ContentRevisionRecord{}, ErrNotFound
		}
		return d, ContentRevisionRecord{}, err
	}
	var v ContentRevisionRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,document_id,revision,state,payload,COALESCE(source_revision_id,''),created_at FROM content_revisions WHERE document_id=? ORDER BY revision DESC LIMIT 1`, id).Scan(&v.ID, &v.DocumentID, &v.Revision, &v.State, &v.Payload, &v.SourceRevisionID, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, v, ErrNotFound
	}
	return d, v, err
}

// ListContentDocuments lists documents for a pack, optionally filtered by kind.
func (r *Repository) ListContentDocuments(ctx context.Context, packID, kind string) ([]ContentDocumentRecord, error) {
	q := `SELECT id,pack_id,kind,slug,title,COALESCE(active_revision_id,''),created_at,updated_at FROM content_documents WHERE pack_id=?`
	args := []any{packID}
	if kind != "" {
		q += ` AND kind=?`
		args = append(args, kind)
	}
	q += ` ORDER BY kind,slug,id`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContentDocumentRecord
	for rows.Next() {
		var d ContentDocumentRecord
		if err := rows.Scan(&d.ID, &d.PackID, &d.Kind, &d.Slug, &d.Title, &d.ActiveRevisionID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveContentDraft appends a draft if expectedRevision matches the latest
// revision. Canonical payload equality makes duplicate saves idempotent.
func (r *Repository) SaveContentDraft(ctx context.Context, packID, documentID string, expectedRevision int, rev ContentRevisionRecord, requestID string) (ContentRevisionRecord, error) {
	var result ContentRevisionRecord
	err := r.WithTx(ctx, func(tx *Repository) error {
		var latest ContentRevisionRecord
		if err := tx.db.QueryRowContext(ctx, `SELECT r.id,r.document_id,r.revision,r.state,r.payload,COALESCE(r.source_revision_id,''),r.created_at FROM content_revisions r JOIN content_documents d ON d.id=r.document_id WHERE d.pack_id=? AND d.id=? ORDER BY r.revision DESC LIMIT 1`, packID, documentID).Scan(&latest.ID, &latest.DocumentID, &latest.Revision, &latest.State, &latest.Payload, &latest.SourceRevisionID, &latest.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if latest.Revision != expectedRevision {
			return fmt.Errorf("%w: expected revision %d, current %d", ErrConflict, expectedRevision, latest.Revision)
		}
		if latest.Payload == rev.Payload {
			result = latest
			return nil
		}
		rev.Revision = latest.Revision + 1
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO content_revisions(id,document_id,revision,state,payload,source_revision_id,created_at) VALUES (?,?,?,?,?,?,?)`, rev.ID, rev.DocumentID, rev.Revision, "draft", rev.Payload, nullString(rev.SourceRevisionID), rev.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE content_documents SET updated_at=? WHERE id=? AND pack_id=?`, rev.CreatedAt, documentID, packID); err != nil {
			return err
		}
		if err := addP6Evidence(ctx, tx, packID, requestID, rev.CreatedAt, "content", "content.draft", "content_document", documentID, "content.draft_saved", map[string]any{"document_id": documentID, "revision_id": rev.ID}); err != nil {
			return err
		}
		result = rev
		return nil
	})
	return result, err
}

// ValidateContentRevision records validation output atomically.
func (r *Repository) ValidateContentRevision(ctx context.Context, packID, documentID string, v ContentValidationRecord, requestID string) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		var ok int
		if err := tx.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_revisions r JOIN content_documents d ON d.id=r.document_id WHERE d.pack_id=? AND d.id=? AND r.id=?`, packID, documentID, v.RevisionID).Scan(&ok); err != nil {
			return err
		}
		if ok != 1 {
			return ErrNotFound
		}
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO content_validation_runs(id,revision_id,status,issues,affected_mods,created_at) VALUES (?,?,?,?,?,?)`, v.ID, v.RevisionID, v.Status, v.Issues, v.AffectedMods, v.CreatedAt); err != nil {
			return err
		}
		return addP6Evidence(ctx, tx, packID, requestID, v.CreatedAt, "content", "content.validate", "content_revision", v.RevisionID, "content.validated", map[string]any{"revision_id": v.RevisionID, "status": v.Status})
	})
}

// ApplyContent promotes a draft revision and archives the prior applied one.
func (r *Repository) ApplyContent(ctx context.Context, packID, documentID, revisionID, requestID string, at int64) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		var state string
		if err := tx.db.QueryRowContext(ctx, `SELECT r.state FROM content_revisions r JOIN content_documents d ON d.id=r.document_id WHERE d.pack_id=? AND d.id=? AND r.id=?`, packID, documentID, revisionID).Scan(&state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if state != "draft" {
			return fmt.Errorf("%w: revision is %s", ErrConflict, state)
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE content_revisions SET state='archived' WHERE document_id=? AND state='applied'`, documentID); err != nil {
			return err
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE content_revisions SET state='applied' WHERE document_id=? AND id=? AND state='draft'`, documentID, revisionID); err != nil {
			return err
		}
		res, err := tx.db.ExecContext(ctx, `UPDATE content_documents SET active_revision_id=?,updated_at=? WHERE id=? AND pack_id=?`, revisionID, at, documentID, packID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNotFound
		}
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO delivery_checks(id,pack_id,kind,status,detail,input_fingerprint,run_id,checked_at) VALUES (?,?, 'content','passed',?,?,?,?) ON CONFLICT(pack_id,kind) WHERE pack_version_id IS NULL DO UPDATE SET status=excluded.status,detail=excluded.detail,input_fingerprint=excluded.input_fingerprint,run_id=excluded.run_id,checked_at=excluded.checked_at`, fmt.Sprintf("delivery-content-%s", packID), packID, fmt.Sprintf(`{"revision_id":%q}`, revisionID), revisionID, fmt.Sprintf("p6-%d", at), at); err != nil {
			return fmt.Errorf("upsert content delivery check: %w", err)
		}
		return addP6Evidence(ctx, tx, packID, requestID, at, "content", "content.apply", "content_revision", revisionID, "content.applied", map[string]any{"document_id": documentID, "revision_id": revisionID})
	})
}

// RollbackContent creates a new draft copied from a historical revision.
func (r *Repository) RollbackContent(ctx context.Context, packID, documentID, targetRevisionID string, rev ContentRevisionRecord, requestID string) (ContentRevisionRecord, error) {
	var result ContentRevisionRecord
	err := r.WithTx(ctx, func(tx *Repository) error {
		var payload string
		if err := tx.db.QueryRowContext(ctx, `SELECT r.payload FROM content_revisions r JOIN content_documents d ON d.id=r.document_id WHERE d.pack_id=? AND d.id=? AND r.id=?`, packID, documentID, targetRevisionID).Scan(&payload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		var latest int
		if err := tx.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM content_revisions WHERE document_id=?`, documentID).Scan(&latest); err != nil {
			return err
		}
		rev.Revision, rev.Payload, rev.State, rev.SourceRevisionID = latest+1, payload, "draft", targetRevisionID
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO content_revisions(id,document_id,revision,state,payload,source_revision_id,created_at) VALUES (?,?,?,?,?,?,?)`, rev.ID, documentID, rev.Revision, rev.State, rev.Payload, rev.SourceRevisionID, rev.CreatedAt); err != nil {
			return err
		}
		if err := addP6Evidence(ctx, tx, packID, requestID, rev.CreatedAt, "content", "content.rollback", "content_revision", rev.ID, "content.rollback_created", map[string]any{"document_id": documentID, "source_revision_id": targetRevisionID}); err != nil {
			return err
		}
		result = rev
		return nil
	})
	return result, err
}

// ListContentHistory returns all revisions in ascending revision order.
func (r *Repository) ListContentHistory(ctx context.Context, packID, documentID string) ([]ContentRevisionRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT r.id,r.document_id,r.revision,r.state,r.payload,COALESCE(r.source_revision_id,''),r.created_at FROM content_revisions r JOIN content_documents d ON d.id=r.document_id WHERE d.pack_id=? AND d.id=? ORDER BY r.revision`, packID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContentRevisionRecord
	for rows.Next() {
		var v ContentRevisionRecord
		if err := rows.Scan(&v.ID, &v.DocumentID, &v.Revision, &v.State, &v.Payload, &v.SourceRevisionID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveQuestDraft creates or appends a complete quest graph snapshot.
func (r *Repository) SaveQuestDraft(ctx context.Context, packID string, book QuestBookRecord, rev QuestRevisionRecord, chapters []QuestChapterRecord, nodes []QuestNodeRecord, edges []QuestEdgeRecord, expectedRevision int, requestID string) (QuestRevisionRecord, error) {
	var result QuestRevisionRecord
	err := r.WithTx(ctx, func(tx *Repository) error {
		var existing QuestBookRecord
		err := tx.db.QueryRowContext(ctx, `SELECT id,pack_id,COALESCE(active_revision_id,''),created_at,updated_at FROM quest_books WHERE pack_id=?`, packID).Scan(&existing.ID, &existing.PackID, &existing.ActiveRevisionID, &existing.CreatedAt, &existing.UpdatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO quest_books(id,pack_id,active_revision_id,created_at,updated_at) VALUES (?,?,?,?,?)`, book.ID, packID, nullString(book.ActiveRevisionID), book.CreatedAt, book.UpdatedAt); err != nil {
				return err
			}
			existing = book
		} else if err != nil {
			return err
		}
		var latest int
		if err := tx.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM quest_revisions WHERE quest_book_id=?`, existing.ID).Scan(&latest); err != nil {
			return err
		}
		if latest != expectedRevision {
			return fmt.Errorf("%w: expected revision %d, current %d", ErrConflict, expectedRevision, latest)
		}
		rev.QuestBookID, rev.Revision, rev.State = existing.ID, latest+1, "draft"
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO quest_revisions(id,quest_book_id,revision,state,created_at) VALUES (?,?,?,?,?)`, rev.ID, rev.QuestBookID, rev.Revision, rev.State, rev.CreatedAt); err != nil {
			return err
		}
		chapterIDs := make(map[string]string, len(chapters))
		for _, c := range chapters {
			physical := rev.ID + "::c::" + c.ID
			chapterIDs[c.ID] = physical
			c.RevisionID = rev.ID
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO quest_chapters(id,revision_id,title,description,cover_color,position) VALUES (?,?,?,?,?,?)`, physical, c.RevisionID, c.Title, c.Description, c.CoverColor, c.Position); err != nil {
				return err
			}
		}
		nodeIDs := make(map[string]string, len(nodes))
		for _, n := range nodes {
			physical := rev.ID + "::n::" + n.ID
			nodeIDs[n.ID] = physical
			n.RevisionID = rev.ID
			chapterID := chapterIDs[n.ChapterID]
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO quest_nodes(id,revision_id,chapter_id,title,description,icon,x,y,prerequisites,rewards,mod_refs,position) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, physical, n.RevisionID, chapterID, n.Title, n.Description, n.Icon, n.X, n.Y, n.Prerequisites, n.Rewards, n.ModRefs, n.Position); err != nil {
				return err
			}
		}
		for _, e := range edges {
			e.RevisionID = rev.ID
			physical := rev.ID + "::e::" + e.ID
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO quest_edges(id,revision_id,from_node_id,to_node_id) VALUES (?,?,?,?)`, physical, e.RevisionID, nodeIDs[e.FromNodeID], nodeIDs[e.ToNodeID]); err != nil {
				return err
			}
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE quest_books SET updated_at=? WHERE id=?`, rev.CreatedAt, existing.ID); err != nil {
			return err
		}
		if err := addP6Evidence(ctx, tx, packID, requestID, rev.CreatedAt, "quest", "quest.draft", "quest_revision", rev.ID, "quest.draft_saved", map[string]any{"revision_id": rev.ID}); err != nil {
			return err
		}
		result = rev
		return nil
	})
	return result, err
}

// GetQuestRevision returns the current book and latest revision graph.
func (r *Repository) GetQuestRevision(ctx context.Context, packID string) (QuestBookRecord, QuestRevisionRecord, []QuestChapterRecord, []QuestNodeRecord, []QuestEdgeRecord, error) {
	var b QuestBookRecord
	if err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,COALESCE(active_revision_id,''),created_at,updated_at FROM quest_books WHERE pack_id=?`, packID).Scan(&b.ID, &b.PackID, &b.ActiveRevisionID, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return b, QuestRevisionRecord{}, nil, nil, nil, ErrNotFound
		}
		return b, QuestRevisionRecord{}, nil, nil, nil, err
	}
	var v QuestRevisionRecord
	if err := r.db.QueryRowContext(ctx, `SELECT id,quest_book_id,revision,state,created_at FROM quest_revisions WHERE quest_book_id=? ORDER BY revision DESC LIMIT 1`, b.ID).Scan(&v.ID, &v.QuestBookID, &v.Revision, &v.State, &v.CreatedAt); err != nil {
		return b, v, nil, nil, nil, err
	}
	cs, err := r.listQuestChapters(ctx, v.ID)
	if err != nil {
		return b, v, nil, nil, nil, err
	}
	ns, err := r.listQuestNodes(ctx, v.ID)
	if err != nil {
		return b, v, nil, nil, nil, err
	}
	es, err := r.listQuestEdges(ctx, v.ID)
	return b, v, cs, ns, es, err
}

// GetQuestRevisionByID loads an immutable historical quest graph.
func (r *Repository) GetQuestRevisionByID(ctx context.Context, packID, revisionID string) (QuestBookRecord, QuestRevisionRecord, []QuestChapterRecord, []QuestNodeRecord, []QuestEdgeRecord, error) {
	var b QuestBookRecord
	var v QuestRevisionRecord
	if err := r.db.QueryRowContext(ctx, `SELECT b.id,b.pack_id,COALESCE(b.active_revision_id,''),b.created_at,b.updated_at,q.id,q.quest_book_id,q.revision,q.state,q.created_at FROM quest_books b JOIN quest_revisions q ON q.quest_book_id=b.id WHERE b.pack_id=? AND q.id=?`, packID, revisionID).Scan(&b.ID, &b.PackID, &b.ActiveRevisionID, &b.CreatedAt, &b.UpdatedAt, &v.ID, &v.QuestBookID, &v.Revision, &v.State, &v.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return b, v, nil, nil, nil, ErrNotFound
		}
		return b, v, nil, nil, nil, err
	}
	cs, err := r.listQuestChapters(ctx, v.ID)
	if err != nil {
		return b, v, nil, nil, nil, err
	}
	ns, err := r.listQuestNodes(ctx, v.ID)
	if err != nil {
		return b, v, nil, nil, nil, err
	}
	es, err := r.listQuestEdges(ctx, v.ID)
	return b, v, cs, ns, es, err
}
func (r *Repository) listQuestChapters(ctx context.Context, id string) ([]QuestChapterRecord, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,revision_id,title,description,cover_color,position FROM quest_chapters WHERE revision_id=? ORDER BY position,id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []QuestChapterRecord
	for rows.Next() {
		var x QuestChapterRecord
		if e := rows.Scan(&x.ID, &x.RevisionID, &x.Title, &x.Description, &x.CoverColor, &x.Position); e != nil {
			return nil, e
		}
		o = append(o, x)
	}
	return o, rows.Err()
}
func (r *Repository) listQuestNodes(ctx context.Context, id string) ([]QuestNodeRecord, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,revision_id,chapter_id,title,description,icon,x,y,prerequisites,rewards,mod_refs,position FROM quest_nodes WHERE revision_id=? ORDER BY chapter_id,position,id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []QuestNodeRecord
	for rows.Next() {
		var x QuestNodeRecord
		if e := rows.Scan(&x.ID, &x.RevisionID, &x.ChapterID, &x.Title, &x.Description, &x.Icon, &x.X, &x.Y, &x.Prerequisites, &x.Rewards, &x.ModRefs, &x.Position); e != nil {
			return nil, e
		}
		o = append(o, x)
	}
	return o, rows.Err()
}
func (r *Repository) listQuestEdges(ctx context.Context, id string) ([]QuestEdgeRecord, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,revision_id,from_node_id,to_node_id FROM quest_edges WHERE revision_id=? ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []QuestEdgeRecord
	for rows.Next() {
		var x QuestEdgeRecord
		if e := rows.Scan(&x.ID, &x.RevisionID, &x.FromNodeID, &x.ToNodeID); e != nil {
			return nil, e
		}
		o = append(o, x)
	}
	return o, rows.Err()
}

// ApplyQuest promotes a draft quest revision and archives the previous one.
func (r *Repository) ApplyQuest(ctx context.Context, packID, revisionID, requestID string, at int64) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		var bookID string
		var state string
		if e := tx.db.QueryRowContext(ctx, `SELECT q.quest_book_id,q.state FROM quest_revisions q JOIN quest_books b ON b.id=q.quest_book_id WHERE b.pack_id=? AND q.id=?`, packID, revisionID).Scan(&bookID, &state); e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		if state != "draft" {
			return fmt.Errorf("%w: revision is %s", ErrConflict, state)
		}
		if _, e := tx.db.ExecContext(ctx, `UPDATE quest_revisions SET state='archived' WHERE quest_book_id=? AND state='applied'`, bookID); e != nil {
			return e
		}
		if _, e := tx.db.ExecContext(ctx, `UPDATE quest_revisions SET state='applied' WHERE id=? AND quest_book_id=?`, revisionID, bookID); e != nil {
			return e
		}
		if _, e := tx.db.ExecContext(ctx, `UPDATE quest_books SET active_revision_id=?,updated_at=? WHERE id=?`, revisionID, at, bookID); e != nil {
			return e
		}
		if _, e := tx.db.ExecContext(ctx, `INSERT INTO delivery_checks(id,pack_id,kind,status,detail,input_fingerprint,run_id,checked_at) VALUES (?,?, 'quest','passed',?,?,?,?) ON CONFLICT(pack_id,kind) WHERE pack_version_id IS NULL DO UPDATE SET status=excluded.status,detail=excluded.detail,input_fingerprint=excluded.input_fingerprint,run_id=excluded.run_id,checked_at=excluded.checked_at`, fmt.Sprintf("delivery-quest-%s", packID), packID, fmt.Sprintf(`{"revision_id":%q}`, revisionID), revisionID, fmt.Sprintf("p6-%d", at), at); e != nil {
			return fmt.Errorf("upsert quest delivery check: %w", e)
		}
		return addP6Evidence(ctx, tx, packID, requestID, at, "quest", "quest.apply", "quest_revision", revisionID, "quest.applied", map[string]any{"revision_id": revisionID})
	})
}

// ValidateQuest records validation evidence. The canonical v7 schema has no
// quest_validation_runs table, so quest validation is intentionally retained
// in activity/outbox/audit rather than violating the content revision FK.
func (r *Repository) ValidateQuest(ctx context.Context, packID, revisionID string, v ContentValidationRecord, requestID string) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		var n int
		if e := tx.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM quest_revisions q JOIN quest_books b ON b.id=q.quest_book_id WHERE b.pack_id=? AND q.id=?`, packID, revisionID).Scan(&n); e != nil {
			return e
		}
		if n != 1 {
			return ErrNotFound
		}
		return addP6Evidence(ctx, tx, packID, requestID, v.CreatedAt, "quest", "quest.validate", "quest_revision", revisionID, "quest.validated", map[string]any{"revision_id": revisionID, "status": v.Status})
	})
}

// PackIDForModReference resolves an ID or project ID to its owning pack.
func (r *Repository) PackIDForModReference(ctx context.Context, ref string) (string, error) {
	var packID string
	err := r.db.QueryRowContext(ctx, `SELECT pack_id FROM pack_mods WHERE status<>'removed' AND (id=? OR project_id=?) ORDER BY CASE WHEN id=? THEN 0 ELSE 1 END LIMIT 1`, ref, ref, ref).Scan(&packID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return packID, err
}

// ListQuestHistory lists revisions in ascending order.
func (r *Repository) ListQuestHistory(ctx context.Context, packID string) ([]QuestRevisionRecord, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT q.id,q.quest_book_id,q.revision,q.state,q.created_at FROM quest_revisions q JOIN quest_books b ON b.id=q.quest_book_id WHERE b.pack_id=? ORDER BY q.revision`, packID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []QuestRevisionRecord
	for rows.Next() {
		var x QuestRevisionRecord
		if e := rows.Scan(&x.ID, &x.QuestBookID, &x.Revision, &x.State, &x.CreatedAt); e != nil {
			return nil, e
		}
		o = append(o, x)
	}
	return o, rows.Err()
}

func isUnique(err error) bool {
	return err != nil && (contains(err.Error(), "unique") || contains(err.Error(), "UNIQUE"))
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
