package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"mpackstation/internal/store"
)

func p6Fixture(t *testing.T) (*API, *sql.DB, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(db)
	p, err := a.CreatePack(context.Background(), CreatePackInput{Name: "P6", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p6-pack")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return a, db, p.ID
}

func TestP6ContentRevisionLifecycleAndEvidence(t *testing.T) {
	a, db, packID := p6Fixture(t)
	ctx := context.Background()
	d, r, err := a.CreateContent(ctx, packID, CreateContentInput{Kind: "recipe", Slug: "iron", Title: "Iron", Payload: []byte(`{"schema_version":1,"type":"shaped","input":[{"item":"minecraft:iron_ingot"}],"output":{"item":"minecraft:iron_block","count":1}}`)}, "req-create")
	if err != nil {
		t.Fatal(err)
	}
	if r.Revision != 1 || d.ActiveRevisionID != nil {
		t.Fatalf("initial = %#v %#v", d, r)
	}
	// Canonical serialization makes an equivalent save idempotent.
	r2, err := a.SaveContentDraft(ctx, packID, d.ID, SaveContentDraftInput{IfMatch: 1, Payload: []byte(`{"output":{"count":1,"item":"minecraft:iron_block"},"input":[{"item":"minecraft:iron_ingot"}],"type":"shaped","schema_version":1}`)}, "req-dedupe")
	if err != nil {
		t.Fatal(err)
	}
	if r2.ID != r.ID || r2.Revision != 1 {
		t.Fatalf("dedupe = %#v", r2)
	}
	r3, err := a.SaveContentDraft(ctx, packID, d.ID, SaveContentDraftInput{IfMatch: 1, Payload: []byte(`{"schema_version":1,"type":"shapeless","input":[{"item":"minecraft:iron_ingot"}],"output":{"item":"minecraft:iron_nugget","count":9}}`)}, "req-draft")
	if err != nil || r3.Revision != 2 {
		t.Fatalf("draft = %#v %v", r3, err)
	}
	if _, err := a.SaveContentDraft(ctx, packID, d.ID, SaveContentDraftInput{IfMatch: 1, Payload: r3.Payload}, "req-stale"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale error = %v", err)
	}
	v, err := a.ValidateContent(ctx, packID, d.ID, r3.ID, "req-validate")
	if err != nil || v.Status != "passed" {
		t.Fatalf("validate = %#v %v", v, err)
	}
	if _, err := a.ApplyContent(ctx, packID, d.ID, r3.ID, "req-apply"); err != nil {
		t.Fatal(err)
	}
	got, _, err := a.GetContent(ctx, packID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveRevisionID == nil || *got.ActiveRevisionID != r3.ID {
		t.Fatalf("active = %#v", got)
	}
	rolled, err := a.RollbackContent(ctx, packID, d.ID, r.ID, "req-rollback")
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Revision != 3 || rolled.State != "draft" || rolled.SourceRevisionID == nil || *rolled.SourceRevisionID != r.ID {
		t.Fatalf("rollback = %#v", rolled)
	}
	h, err := a.ContentHistory(ctx, packID, d.ID)
	if err != nil || len(h) != 3 {
		t.Fatalf("history = %#v %v", h, err)
	}
	var activities, outbox, audits, checks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM activities WHERE pack_id=? AND kind='content'`, packID).Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE pack_id=? AND aggregate_type IN ('content_document','content_revision')`, packID).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE pack_id=? AND action LIKE 'content.%'`, packID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_checks WHERE pack_id=? AND kind='content'`, packID).Scan(&checks); err != nil {
		t.Fatal(err)
	}
	if activities < 4 || outbox < 4 || audits < 4 || checks != 1 {
		t.Fatalf("evidence activities=%d outbox=%d audits=%d checks=%d", activities, outbox, audits, checks)
	}
}

func TestP6ContentValidationRejectsUnknownAndBlocksApply(t *testing.T) {
	a, _, packID := p6Fixture(t)
	ctx := context.Background()
	if _, _, err := a.CreateContent(ctx, packID, CreateContentInput{Kind: "unknown", Slug: "x", Title: "X", Payload: []byte(`{}`)}, "req"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("kind error = %v", err)
	}
	if _, _, err := a.CreateContent(ctx, packID, CreateContentInput{Kind: "recipe", Slug: "x", Title: "X", Payload: []byte(`{"schema_version":1,"bad":true}`)}, "req"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unknown field error = %v", err)
	}
	d, r, err := a.CreateContent(ctx, packID, CreateContentInput{Kind: "ore", Slug: "x", Title: "X", Payload: []byte(`{"schema_version":1,"dimension":"overworld","block":"minecraft:stone","min_y":100,"max_y":1}`)}, "req")
	if err != nil {
		t.Fatal(err)
	}
	issues, err := a.ValidateContent(ctx, packID, d.ID, r.ID, "req-v")
	if err != nil {
		t.Fatal(err)
	}
	if !isBlocking(issues.Issues) {
		t.Fatalf("issues = %#v", issues)
	}
	var ve *ValidationError
	if _, err := a.ApplyContent(ctx, packID, d.ID, r.ID, "req-a"); !errors.As(err, &ve) || ve.Domain != "content" {
		t.Fatalf("apply error = %v", err)
	}
}

func questBase() QuestDraft {
	return QuestDraft{
		Chapters: []QuestChapter{{ID: "intro", Title: "Intro", Position: 0}},
		Nodes:    []QuestNode{{ID: "start", ChapterID: "intro", Title: "Start", Position: 0, Rewards: []any{map[string]any{"kind": "item", "item": "minecraft:stone", "amount": 1}}}, {ID: "finish", ChapterID: "intro", Title: "Finish", Position: 1, Rewards: []any{map[string]any{"kind": "experience", "experience": 5}}}},
		Edges:    []QuestEdge{{ID: "e1", FromNodeID: "start", ToNodeID: "finish"}},
	}
}

func TestP6QuestGraphLifecycleAndValidation(t *testing.T) {
	a, db, packID := p6Fixture(t)
	ctx := context.Background()
	draft := questBase()
	r, issues, err := a.SaveQuestDraft(ctx, packID, draft, 0, "q-save")
	if err != nil || r.Revision != 1 || len(issues) != 0 {
		t.Fatalf("save = %#v %#v %v", r, issues, err)
	}
	if _, err := a.ValidateQuest(ctx, packID, "q-validate"); err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyQuest(ctx, packID, "q-apply"); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetQuest(ctx, packID)
	if err != nil {
		t.Fatal(err)
	}
	if q.ActiveRevisionID == nil || *q.ActiveRevisionID != r.ID || len(q.Revision.Draft.Nodes) != 2 {
		t.Fatalf("quest = %#v", q)
	}
	draft.Nodes[1].Title = "Updated"
	r2, _, err := a.SaveQuestDraft(ctx, packID, draft, 1, "q-save2")
	if err != nil || r2.Revision != 2 {
		t.Fatalf("save2 = %#v %v", r2, err)
	}
	if _, err := a.RollbackQuest(ctx, packID, r.ID, "q-rollback"); err != nil {
		t.Fatal(err)
	}
	h, err := a.QuestHistory(ctx, packID)
	if err != nil || len(h) != 3 {
		t.Fatalf("history = %#v %v", h, err)
	}
	var checks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_checks WHERE pack_id=? AND kind='quest'`, packID).Scan(&checks); err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("quest checks=%d", checks)
	}
}

func TestP6QuestRejectsCycleOrCrossPackReference(t *testing.T) {
	a, _, packID := p6Fixture(t)
	ctx := context.Background()
	cycle := questBase()
	cycle.Edges = []QuestEdge{{ID: "e1", FromNodeID: "start", ToNodeID: "finish"}, {ID: "e2", FromNodeID: "finish", ToNodeID: "start"}}
	r, issues, err := a.SaveQuestDraft(ctx, packID, cycle, 0, "q-cycle")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 || issues[0].Code != "cycle" {
		t.Fatalf("cycle issues=%#v", issues)
	}
	var ve *ValidationError
	if err := a.ApplyQuest(ctx, packID, "q-apply"); !errors.As(err, &ve) || ve.Domain != "quest" {
		t.Fatalf("cycle apply=%v", err)
	}
	_ = r
	isolated := questBase()
	isolated.Edges = nil
	_, isolatedIssues, err := a.SaveQuestDraft(ctx, packID, isolated, 1, "q-isolated")
	if err != nil {
		t.Fatal(err)
	}
	foundOrphan := false
	for _, i := range isolatedIssues {
		if i.Code == "orphan_node" && i.Severity == "warning" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Fatalf("isolated issues=%#v", isolatedIssues)
	}
	other, err := a.CreatePack(ctx, CreatePackInput{Name: "Other", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "other-pack")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := a.AddLocalPackMod(ctx, other.ID, LocalModInput{DisplayName: "Other Mod", FileName: "other.jar", SHA1: "2222222222222222222222222222222222222222", Size: 1}, "other-mod")
	if err != nil {
		t.Fatal(err)
	}
	cross := questBase()
	cross.Nodes[0].ModRefs = []any{mod.ID}
	_, crossIssues, err := a.SaveQuestDraft(ctx, packID, cross, 2, "q-cross")
	var cve *ValidationError
	if !errors.As(err, &cve) || cve.Domain != "quest" {
		t.Fatalf("cross-pack error=%v issues=%#v", err, crossIssues)
	}
	found := false
	for _, i := range crossIssues {
		if i.Code == "cross_pack_reference" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cross-pack issue=%#v", crossIssues)
	}
}
