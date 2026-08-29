package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"mpackstation/internal/store"
)

var (
	// ErrRevisionConflict means the caller edited a stale revision.
	ErrRevisionConflict = errors.New("revision conflict")
	// ErrValidationFailed means a revision contains blocking validation issues.
	ErrValidationFailed = errors.New("validation failed")
	// ErrCrossPackReference means a quest refers to a mod owned by another pack.
	ErrCrossPackReference = errors.New("cross-pack reference")
)

// ValidationIssue is stable, human-readable validation output.
type ValidationIssue struct {
	Code, Severity, Path, Message string
	Details                       map[string]any `json:"details,omitempty"`
}

// ContentDocument is the transport-neutral content header.
type ContentDocument struct {
	ID, PackID, Kind, Slug, Title string
	ActiveRevisionID              string `json:"activeRevisionId,omitempty"`
	CreatedAt                     string `json:"createdAt,omitempty"`
	UpdatedAt                     string `json:"updatedAt,omitempty"`
}

// ContentRevision is an immutable JSON revision.
type ContentRevision struct {
	ID, DocumentID, State, SourceRevisionID string
	Revision                                int
	Payload                                 json.RawMessage
	CreatedAt                               string
}

// ContentValidation is the latest validation result for a revision.
type ContentValidation struct {
	ID, RevisionID, Status string
	Issues                 []ValidationIssue
	AffectedMods           []string
	CreatedAt              string
}

// CreateContentInput creates a document and initial draft.
type CreateContentInput struct {
	Kind, Slug, Title string
	Payload           json.RawMessage
}

// SaveContentDraftInput appends a draft using optimistic concurrency.
type SaveContentDraftInput struct {
	IfMatch int
	Payload json.RawMessage
}

func (a *API) CreateContent(ctx context.Context, packID string, in CreateContentInput, requestID string) (ContentDocument, ContentRevision, error) {
	if err := a.ready(); err != nil {
		return ContentDocument{}, ContentRevision{}, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return ContentDocument{}, ContentRevision{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	slug := strings.TrimSpace(in.Slug)
	title := strings.TrimSpace(in.Title)
	if !validContentKind(kind) || slug == "" || len(slug) > 128 || title == "" || len(title) > 256 {
		return ContentDocument{}, ContentRevision{}, ErrInvalidArgument
	}
	payload, err := canonicalContentPayload(kind, in.Payload)
	if err != nil {
		return ContentDocument{}, ContentRevision{}, err
	}
	now := time.Now().UnixMilli()
	docID, revID := newID("content"), newID("content-revision")
	d := store.ContentDocumentRecord{ID: docID, PackID: packID, Kind: kind, Slug: slug, Title: title, CreatedAt: now, UpdatedAt: now}
	r := store.ContentRevisionRecord{ID: revID, DocumentID: docID, Revision: 1, State: "draft", Payload: string(payload), CreatedAt: now}
	if err := a.repo.CreateContentDocument(ctx, d, r, requestID); err != nil {
		return ContentDocument{}, ContentRevision{}, err
	}
	return contentDocDTO(d), contentRevDTO(r), nil
}

func (a *API) GetContent(ctx context.Context, packID, documentID string) (ContentDocument, ContentRevision, error) {
	if err := a.ready(); err != nil {
		return ContentDocument{}, ContentRevision{}, err
	}
	d, r, e := a.repo.GetContentDocument(ctx, packID, documentID)
	if e != nil {
		return ContentDocument{}, ContentRevision{}, e
	}
	return contentDocDTO(d), contentRevDTO(r), nil
}

func (a *API) ListContent(ctx context.Context, packID, kind string) ([]ContentDocument, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if kind != "" && !validContentKind(kind) {
		return nil, ErrInvalidArgument
	}
	if _, e := a.repo.GetPack(ctx, packID); e != nil {
		return nil, e
	}
	rows, e := a.repo.ListContentDocuments(ctx, packID, kind)
	if e != nil {
		return nil, e
	}
	out := make([]ContentDocument, 0, len(rows))
	for _, d := range rows {
		out = append(out, contentDocDTO(d))
	}
	return out, nil
}

func (a *API) SaveContentDraft(ctx context.Context, packID, documentID string, in SaveContentDraftInput, requestID string) (ContentRevision, error) {
	if err := a.ready(); err != nil {
		return ContentRevision{}, err
	}
	d, _, e := a.repo.GetContentDocument(ctx, packID, documentID)
	if e != nil {
		return ContentRevision{}, e
	}
	payload, e := canonicalContentPayload(d.Kind, in.Payload)
	if e != nil {
		return ContentRevision{}, e
	}
	if in.IfMatch < 1 {
		return ContentRevision{}, ErrInvalidArgument
	}
	r := store.ContentRevisionRecord{ID: newID("content-revision"), DocumentID: documentID, Payload: string(payload), CreatedAt: time.Now().UnixMilli()}
	r, e = a.repo.SaveContentDraft(ctx, packID, documentID, in.IfMatch, r, requestID)
	if errors.Is(e, store.ErrConflict) {
		return ContentRevision{}, fmt.Errorf("%w: %v", ErrRevisionConflict, e)
	}
	if e != nil {
		return ContentRevision{}, e
	}
	return contentRevDTO(r), nil
}

func (a *API) ValidateContent(ctx context.Context, packID, documentID, revisionID, requestID string) (ContentValidation, error) {
	if err := a.ready(); err != nil {
		return ContentValidation{}, err
	}
	d, r, e := a.repo.GetContentDocument(ctx, packID, documentID)
	if e != nil {
		return ContentValidation{}, e
	}
	if revisionID != "" {
		hist, e := a.repo.ListContentHistory(ctx, packID, documentID)
		if e != nil {
			return ContentValidation{}, e
		}
		found := false
		for _, x := range hist {
			if x.ID == revisionID {
				r = x
				found = true
				break
			}
		}
		if !found {
			return ContentValidation{}, store.ErrNotFound
		}
	}
	issues := validateContentSemantics(d.Kind, []byte(r.Payload))
	status := "passed"
	for _, i := range issues {
		if i.Severity == "error" {
			status = "failed"
			break
		} else if status == "passed" {
			status = "warning"
		}
	}
	issueJSON, _ := json.Marshal(issues)
	v := store.ContentValidationRecord{ID: newID("content-validation"), RevisionID: r.ID, Status: status, Issues: string(issueJSON), AffectedMods: "[]", CreatedAt: time.Now().UnixMilli()}
	if e = a.repo.ValidateContentRevision(ctx, packID, documentID, v, requestID); e != nil {
		return ContentValidation{}, e
	}
	return ContentValidation{ID: v.ID, RevisionID: r.ID, Status: status, Issues: issues, AffectedMods: []string{}, CreatedAt: iso(v.CreatedAt)}, nil
}

func (a *API) ApplyContent(ctx context.Context, packID, documentID, revisionID, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	d, r, e := a.repo.GetContentDocument(ctx, packID, documentID)
	if e != nil {
		return e
	}
	if revisionID == "" {
		revisionID = r.ID
	}
	hist, e := a.repo.ListContentHistory(ctx, packID, documentID)
	if e != nil {
		return e
	}
	var target store.ContentRevisionRecord
	for _, x := range hist {
		if x.ID == revisionID {
			target = x
			break
		}
	}
	if target.ID == "" {
		return store.ErrNotFound
	}
	if target.ID != r.ID {
		return fmt.Errorf("%w: only the latest revision can be applied", ErrRevisionConflict)
	}
	if isBlocking(validateContentSemantics(d.Kind, []byte(target.Payload))) {
		return ErrValidationFailed
	}
	return a.repo.ApplyContent(ctx, packID, documentID, revisionID, requestID, time.Now().UnixMilli())
}

func (a *API) RollbackContent(ctx context.Context, packID, documentID, targetRevisionID, requestID string) (ContentRevision, error) {
	if err := a.ready(); err != nil {
		return ContentRevision{}, err
	}
	if _, _, e := a.repo.GetContentDocument(ctx, packID, documentID); e != nil {
		return ContentRevision{}, e
	}
	r, e := a.repo.RollbackContent(ctx, packID, documentID, targetRevisionID, store.ContentRevisionRecord{ID: newID("content-revision"), DocumentID: documentID, CreatedAt: time.Now().UnixMilli()}, requestID)
	if e != nil {
		return ContentRevision{}, e
	}
	return contentRevDTO(r), nil
}

func (a *API) ContentHistory(ctx context.Context, packID, documentID string) ([]ContentRevision, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	x, e := a.repo.ListContentHistory(ctx, packID, documentID)
	if e != nil {
		return nil, e
	}
	out := make([]ContentRevision, 0, len(x))
	for _, r := range x {
		out = append(out, contentRevDTO(r))
	}
	return out, nil
}

func validContentKind(k string) bool { return k == "recipe" || k == "structure" || k == "ore" }
func contentDocDTO(d store.ContentDocumentRecord) ContentDocument {
	return ContentDocument{ID: d.ID, PackID: d.PackID, Kind: d.Kind, Slug: d.Slug, Title: d.Title, ActiveRevisionID: d.ActiveRevisionID, CreatedAt: iso(d.CreatedAt), UpdatedAt: iso(d.UpdatedAt)}
}
func contentRevDTO(r store.ContentRevisionRecord) ContentRevision {
	return ContentRevision{ID: r.ID, DocumentID: r.DocumentID, State: r.State, SourceRevisionID: r.SourceRevisionID, Revision: r.Revision, Payload: json.RawMessage(r.Payload), CreatedAt: iso(r.CreatedAt)}
}

var contentFields = map[string]map[string]bool{"recipe": {"schema_version": true, "type": true, "input": true, "output": true, "conditions": true, "metadata": true}, "structure": {"schema_version": true, "file": true, "size": true, "rotation": true, "anchor": true, "parameters": true, "preview": true, "metadata": true}, "ore": {"schema_version": true, "dimension": true, "block": true, "min_y": true, "max_y": true, "count": true, "frequency": true, "distribution": true, "biomes": true, "metadata": true}}

func canonicalContentPayload(kind string, raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > 256*1024 {
		return nil, ErrInvalidArgument
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	if e := dec.Decode(&v); e != nil {
		return nil, ErrInvalidArgument
	}
	var extra any
	if e := dec.Decode(&extra); e != io.EOF {
		return nil, ErrInvalidArgument
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, ErrInvalidArgument
	}
	for k := range m {
		if !contentFields[kind][k] {
			return nil, fmt.Errorf("%w: unknown content field %s", ErrInvalidArgument, k)
		}
	}
	b, e := json.Marshal(m)
	if e != nil {
		return nil, ErrInvalidArgument
	}
	return b, nil
}

func validateContentSemantics(kind string, raw []byte) []ValidationIssue {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return []ValidationIssue{{Code: "invalid_json", Severity: "error", Path: "$", Message: "payload must be an object"}}
	}
	issues := []ValidationIssue{}
	sv, ok := m["schema_version"].(float64)
	if !ok || sv < 1 || sv != float64(int(sv)) {
		issues = append(issues, ValidationIssue{Code: "schema_version_required", Severity: "error", Path: "$.schema_version", Message: "schema_version must be a positive integer"})
	}
	required := map[string][]string{"recipe": {"type", "input", "output"}, "structure": {"file", "size"}, "ore": {"dimension", "block", "min_y", "max_y"}}
	for _, k := range required[kind] {
		if _, ok := m[k]; !ok {
			issues = append(issues, ValidationIssue{Code: "field_required", Severity: "error", Path: "$." + k, Message: "required field is missing"})
		}
	}
	if kind == "ore" {
		min, mok := m["min_y"].(float64)
		max, xok := m["max_y"].(float64)
		if mok && xok && min > max {
			issues = append(issues, ValidationIssue{Code: "ore_range_invalid", Severity: "error", Path: "$.min_y", Message: "min_y must not exceed max_y"})
		}
	}
	return issues
}
func isBlocking(x []ValidationIssue) bool {
	for _, i := range x {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

// Quest domain DTOs.
type QuestChapter struct {
	ID, Title, Description, CoverColor string
	Position                           int `json:"position"`
}
type QuestNode struct {
	ID, ChapterID, Title, Description, Icon string
	X, Y                                    float64
	Prerequisites, Rewards, ModRefs         []any
	Position                                int `json:"position"`
}
type QuestEdge struct{ ID, FromNodeID, ToNodeID string }
type QuestDraft struct {
	Chapters []QuestChapter `json:"chapters"`
	Nodes    []QuestNode    `json:"nodes"`
	Edges    []QuestEdge    `json:"edges"`
}
type QuestRevision struct {
	ID, QuestBookID, State string
	Revision               int
	CreatedAt              string
	Draft                  QuestDraft
}
type QuestBook struct {
	ID, PackID, ActiveRevisionID string
	Revision                     QuestRevision
}
type QuestReward struct {
	Kind       string `json:"kind"`
	Item       string `json:"item,omitempty"`
	Amount     int    `json:"amount,omitempty"`
	Experience int    `json:"experience,omitempty"`
	Command    string `json:"command,omitempty"`
	UnlockID   string `json:"unlockId,omitempty"`
}

func (a *API) GetQuest(ctx context.Context, packID string) (QuestBook, error) {
	if err := a.ready(); err != nil {
		return QuestBook{}, err
	}
	b, v, c, n, e, x := a.repo.GetQuestRevision(ctx, packID)
	if x != nil {
		return QuestBook{}, x
	}
	return questDTO(b, v, c, n, e), nil
}
func (a *API) SaveQuestDraft(ctx context.Context, packID string, in QuestDraft, ifMatch int, requestID string) (QuestRevision, []ValidationIssue, error) {
	if err := a.ready(); err != nil {
		return QuestRevision{}, nil, err
	}
	if ifMatch < 0 {
		return QuestRevision{}, nil, ErrInvalidArgument
	}
	issues, e := a.validateQuest(ctx, packID, in)
	if e != nil {
		return QuestRevision{}, nil, e
	}
	for _, i := range issues {
		if i.Severity == "error" && (i.Code == "duplicate_id" || i.Code == "duplicate_position" || i.Code == "missing_chapter" || i.Code == "missing_node" || i.Code == "self_edge" || i.Code == "invalid_reward" || i.Code == "cross_pack_reference") {
			return QuestRevision{}, issues, ErrInvalidArgument
		}
	}
	now := time.Now().UnixMilli()
	book := store.QuestBookRecord{ID: newID("quest-book"), PackID: packID, CreatedAt: now, UpdatedAt: now}
	rev := store.QuestRevisionRecord{ID: newID("quest-revision"), CreatedAt: now}
	chs := make([]store.QuestChapterRecord, 0, len(in.Chapters))
	for _, c := range in.Chapters {
		chs = append(chs, store.QuestChapterRecord{ID: c.ID, Title: c.Title, Description: c.Description, CoverColor: c.CoverColor, Position: c.Position})
	}
	nodes := make([]store.QuestNodeRecord, 0, len(in.Nodes))
	for _, n := range in.Nodes {
		pre, _ := json.Marshal(n.Prerequisites)
		rew, _ := json.Marshal(n.Rewards)
		refs, _ := json.Marshal(n.ModRefs)
		nodes = append(nodes, store.QuestNodeRecord{ID: n.ID, ChapterID: n.ChapterID, Title: n.Title, Description: n.Description, Icon: n.Icon, X: n.X, Y: n.Y, Prerequisites: string(pre), Rewards: string(rew), ModRefs: string(refs), Position: n.Position})
	}
	edges := make([]store.QuestEdgeRecord, 0, len(in.Edges))
	for _, ed := range in.Edges {
		edges = append(edges, store.QuestEdgeRecord{ID: ed.ID, FromNodeID: ed.FromNodeID, ToNodeID: ed.ToNodeID})
	}
	v, e := a.repo.SaveQuestDraft(ctx, packID, book, rev, chs, nodes, edges, ifMatch, requestID)
	if errors.Is(e, store.ErrConflict) {
		return QuestRevision{}, issues, fmt.Errorf("%w: %v", ErrRevisionConflict, e)
	}
	if e != nil {
		return QuestRevision{}, issues, e
	}
	return questRevDTO(v, in), issues, nil
}
func (a *API) ValidateQuest(ctx context.Context, packID string, requestID string) ([]ValidationIssue, error) {
	b, e := a.GetQuest(ctx, packID)
	if e != nil {
		return nil, e
	}
	issues, e := a.validateQuest(ctx, packID, b.Revision.Draft)
	if e != nil {
		return nil, e
	}
	status := "passed"
	if isBlocking(issues) {
		status = "failed"
	} else if len(issues) > 0 {
		status = "warning"
	}
	raw, _ := json.Marshal(issues)
	v := store.ContentValidationRecord{ID: newID("quest-validation"), Status: status, Issues: string(raw), AffectedMods: "[]", CreatedAt: time.Now().UnixMilli()}
	if e = a.repo.ValidateQuest(ctx, packID, b.Revision.ID, v, requestID); e != nil {
		return nil, e
	}
	return issues, nil
}
func (a *API) ApplyQuest(ctx context.Context, packID, requestID string) error {
	b, e := a.GetQuest(ctx, packID)
	if e != nil {
		return e
	}
	issues, e := a.validateQuest(ctx, packID, b.Revision.Draft)
	if e != nil {
		return e
	}
	if isBlocking(issues) {
		return ErrValidationFailed
	}
	return a.repo.ApplyQuest(ctx, packID, b.Revision.ID, requestID, time.Now().UnixMilli())
}
func (a *API) RollbackQuest(ctx context.Context, packID, targetRevisionID, requestID string) (QuestRevision, error) {
	if err := a.ready(); err != nil {
		return QuestRevision{}, err
	}
	b, v, c, n, e, x := a.repo.GetQuestRevisionByID(ctx, packID, targetRevisionID)
	if x != nil {
		return QuestRevision{}, x
	}
	_, current, _, _, _, x := a.repo.GetQuestRevision(ctx, packID)
	if x != nil {
		return QuestRevision{}, x
	}
	draft := questDTO(b, v, c, n, e).Revision.Draft
	out, _, x := a.SaveQuestDraft(ctx, packID, draft, current.Revision, requestID)
	return out, x
}
func (a *API) QuestHistory(ctx context.Context, packID string) ([]QuestRevision, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	h, e := a.repo.ListQuestHistory(ctx, packID)
	if e != nil {
		return nil, e
	}
	out := make([]QuestRevision, 0, len(h))
	for _, v := range h {
		_, rv, c, n, ed, x := a.repo.GetQuestRevisionByID(ctx, packID, v.ID)
		if x != nil {
			return nil, x
		}
		out = append(out, questRevDTO(rv, questDTO(store.QuestBookRecord{}, rv, c, n, ed).Revision.Draft))
	}
	return out, nil
}
func (a *API) QuestPreview(ctx context.Context, packID string) (QuestDraft, error) {
	b, e := a.GetQuest(ctx, packID)
	if e != nil {
		return QuestDraft{}, e
	}
	return b.Revision.Draft, nil
}
func questDTO(b store.QuestBookRecord, v store.QuestRevisionRecord, c []store.QuestChapterRecord, n []store.QuestNodeRecord, e []store.QuestEdgeRecord) QuestBook {
	d := QuestDraft{Chapters: make([]QuestChapter, 0, len(c)), Nodes: make([]QuestNode, 0, len(n)), Edges: make([]QuestEdge, 0, len(e))}
	for _, x := range c {
		d.Chapters = append(d.Chapters, QuestChapter{ID: logicalQuestID(x.ID, v.ID, "c"), Title: x.Title, Description: x.Description, CoverColor: x.CoverColor, Position: x.Position})
	}
	for _, x := range n {
		var pre, rew, refs []any
		_ = json.Unmarshal([]byte(x.Prerequisites), &pre)
		_ = json.Unmarshal([]byte(x.Rewards), &rew)
		_ = json.Unmarshal([]byte(x.ModRefs), &refs)
		d.Nodes = append(d.Nodes, QuestNode{ID: logicalQuestID(x.ID, v.ID, "n"), ChapterID: logicalQuestID(x.ChapterID, v.ID, "c"), Title: x.Title, Description: x.Description, Icon: x.Icon, X: x.X, Y: x.Y, Prerequisites: pre, Rewards: rew, ModRefs: refs, Position: x.Position})
	}
	for _, x := range e {
		d.Edges = append(d.Edges, QuestEdge{ID: logicalQuestID(x.ID, v.ID, "e"), FromNodeID: logicalQuestID(x.FromNodeID, v.ID, "n"), ToNodeID: logicalQuestID(x.ToNodeID, v.ID, "n")})
	}
	return QuestBook{ID: b.ID, PackID: b.PackID, ActiveRevisionID: b.ActiveRevisionID, Revision: questRevDTO(v, d)}
}

func logicalQuestID(physical, revisionID, kind string) string {
	prefix := revisionID + "::" + kind + "::"
	if strings.HasPrefix(physical, prefix) {
		return strings.TrimPrefix(physical, prefix)
	}
	return physical
}
func questRevDTO(v store.QuestRevisionRecord, d QuestDraft) QuestRevision {
	return QuestRevision{ID: v.ID, QuestBookID: v.QuestBookID, State: v.State, Revision: v.Revision, CreatedAt: iso(v.CreatedAt), Draft: d}
}

func (a *API) validateQuest(ctx context.Context, packID string, d QuestDraft) ([]ValidationIssue, error) {
	issues := []ValidationIssue{}
	chapters := map[string]bool{}
	positions := map[int]bool{}
	for _, c := range d.Chapters {
		if c.ID == "" || chapters[c.ID] {
			issues = append(issues, ValidationIssue{Code: "duplicate_id", Severity: "error", Path: "chapters", Message: "chapter IDs must be unique"})
		}
		chapters[c.ID] = true
		if positions[c.Position] {
			issues = append(issues, ValidationIssue{Code: "duplicate_position", Severity: "error", Path: "chapters", Message: "chapter positions must be unique"})
		}
		positions[c.Position] = true
		if c.Position < 0 {
			issues = append(issues, ValidationIssue{Code: "invalid_position", Severity: "error", Path: "chapters", Message: "position must be non-negative"})
		}
	}
	nodes := map[string]bool{}
	nodeChapter := map[string]string{}
	nodePos := map[string]map[int]bool{}
	for _, n := range d.Nodes {
		if n.ID == "" || nodes[n.ID] {
			issues = append(issues, ValidationIssue{Code: "duplicate_id", Severity: "error", Path: "nodes", Message: "node IDs must be unique"})
		}
		nodes[n.ID] = true
		if !chapters[n.ChapterID] {
			issues = append(issues, ValidationIssue{Code: "missing_chapter", Severity: "error", Path: "nodes." + n.ID, Message: "node chapter does not exist"})
		}
		if nodePos[n.ChapterID] == nil {
			nodePos[n.ChapterID] = map[int]bool{}
		}
		if nodePos[n.ChapterID][n.Position] {
			issues = append(issues, ValidationIssue{Code: "duplicate_position", Severity: "error", Path: "nodes." + n.ID, Message: "node positions must be unique within chapter"})
		}
		nodePos[n.ChapterID][n.Position] = true
		nodeChapter[n.ID] = n.ChapterID
		if len(n.Rewards) > 0 {
			for _, r := range n.Rewards {
				if !validReward(r) {
					issues = append(issues, ValidationIssue{Code: "invalid_reward", Severity: "error", Path: "nodes." + n.ID + ".rewards", Message: "reward kind or value is invalid"})
				}
			}
		}
		for _, ref := range n.ModRefs {
			if s, ok := ref.(string); ok && s != "" {
				owner, e := a.repo.PackIDForModReference(ctx, s)
				if e == nil && owner != packID {
					issues = append(issues, ValidationIssue{Code: "cross_pack_reference", Severity: "error", Path: "nodes." + n.ID + ".modRefs", Message: "mod reference belongs to another pack"})
				} else if errors.Is(e, store.ErrNotFound) {
					issues = append(issues, ValidationIssue{Code: "missing_mod_reference", Severity: "error", Path: "nodes." + n.ID + ".modRefs", Message: "mod reference is not selected in this pack"})
				}
			}
		}
	}
	adj := map[string][]string{}
	indeg := map[string]int{}
	for id := range nodes {
		indeg[id] = 0
	}
	edgeSeen := map[string]bool{}
	for _, ed := range d.Edges {
		if !nodes[ed.FromNodeID] || !nodes[ed.ToNodeID] {
			issues = append(issues, ValidationIssue{Code: "missing_node", Severity: "error", Path: "edges." + ed.ID, Message: "edge endpoint does not exist"})
			continue
		}
		if ed.FromNodeID == ed.ToNodeID {
			issues = append(issues, ValidationIssue{Code: "self_edge", Severity: "error", Path: "edges." + ed.ID, Message: "edge cannot point to itself"})
			continue
		}
		key := ed.FromNodeID + "\x00" + ed.ToNodeID
		if edgeSeen[key] {
			issues = append(issues, ValidationIssue{Code: "duplicate_edge", Severity: "error", Path: "edges." + ed.ID, Message: "duplicate edge"})
		}
		edgeSeen[key] = true
		adj[ed.FromNodeID] = append(adj[ed.FromNodeID], ed.ToNodeID)
		indeg[ed.ToNodeID]++
	}
	originalIndeg := make(map[string]int, len(indeg))
	for id, n := range indeg {
		originalIndeg[id] = n
	}
	q := []string{}
	for id, n := range indeg {
		if n == 0 {
			q = append(q, id)
		}
	}
	seen := 0
	for len(q) > 0 {
		id := q[0]
		q = q[1:]
		seen++
		for _, to := range adj[id] {
			indeg[to]--
			if indeg[to] == 0 {
				q = append(q, to)
			}
		}
	}
	if seen < len(nodes) {
		issues = append(issues, ValidationIssue{Code: "cycle", Severity: "error", Path: "edges", Message: "quest graph contains a cycle"})
	}
	if len(nodes) > 1 {
		for id := range nodes {
			if len(adj[id]) == 0 && originalIndeg[id] == 0 {
				issues = append(issues, ValidationIssue{Code: "orphan_node", Severity: "warning", Path: "nodes." + id, Message: "node is isolated"})
			}
		}
	}
	_ = nodeChapter
	return issues, nil
}
func validReward(r any) bool {
	m, ok := r.(map[string]any)
	if !ok {
		return false
	}
	kind, _ := m["kind"].(string)
	switch kind {
	case "item":
		item, _ := m["item"].(string)
		amt := rewardNumber(m["amount"])
		return item != "" && amt > 0 && amt <= 100000
	case "experience":
		x := rewardNumber(m["experience"])
		return x > 0 && x <= 1000000
	case "command":
		c, _ := m["command"].(string)
		return c != "" && len(c) <= 2048
	case "unlock":
		u, _ := m["unlockId"].(string)
		return u != "" && len(u) <= 256
	default:
		return false
	}
}

func rewardNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		x, _ := n.Float64()
		return x
	default:
		return 0
	}
}

// Keep a deterministic order in previews and test fixtures.
func (d QuestDraft) Sorted() QuestDraft {
	sort.SliceStable(d.Chapters, func(i, j int) bool { return d.Chapters[i].Position < d.Chapters[j].Position })
	sort.SliceStable(d.Nodes, func(i, j int) bool {
		if d.Nodes[i].ChapterID == d.Nodes[j].ChapterID {
			return d.Nodes[i].Position < d.Nodes[j].Position
		}
		return d.Nodes[i].ChapterID < d.Nodes[j].ChapterID
	})
	sort.SliceStable(d.Edges, func(i, j int) bool { return d.Edges[i].ID < d.Edges[j].ID })
	return d
}
