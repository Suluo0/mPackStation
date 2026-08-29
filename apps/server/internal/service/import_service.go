package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

const (
	ImportSourceCurseForgeURL = "curseforge_url"
	ImportSourceModrinthURL   = "modrinth_url"
	ImportSourceLocalZip      = "local_zip"
)

var (
	ErrImportInvalidSource = errors.New("invalid import source")
	ErrImportUnsafeArchive = errors.New("unsafe import archive")
	ErrImportExpired       = errors.New("import preview expired")
)

type ImportPreviewInput struct {
	Source  string
	URL     string
	Content []byte
}

type ImportPreview struct {
	ID         string `json:"id,omitempty"`
	Token      string `json:"token,omitempty"`
	InputHash  string `json:"inputHash,omitempty"`
	Source     string `json:"source,omitempty"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	EntryCount int    `json:"entryCount"`
	PackName   string `json:"packName,omitempty"`
}

type ImportConfirmInput struct {
	PreviewID, Token, InputHash, IdempotencyKey string
}

type ImportService struct {
	repo    *store.Repository
	queue   *task.Queue
	dataDir string
	now     func() time.Time
}

func NewImportService(db *sql.DB) *ImportService {
	s := &ImportService{now: time.Now}
	if db == nil { return s }
	s.repo = store.NewRepository(db)
	s.dataDir, _ = s.repo.DatabaseDir(context.Background())
	s.queue, _ = task.NewQueue(db)
	return s
}

func NewImportServiceFromSource(source any) *ImportService {
	if db, ok := source.(*sql.DB); ok { return NewImportService(db) }
	return NewImportService(nil)
}

func (s *ImportService) Inspect(ctx context.Context, in ImportPreviewInput) (ImportPreview, error) {
	if s == nil || s.repo == nil { return ImportPreview{}, ErrUnavailable }
	if err := validateImportInput(in); err != nil { return ImportPreview{}, err }
	data := in.Content
	if in.Source != ImportSourceLocalZip {
		if err := validateImportURL(in.URL, in.Source); err != nil { return ImportPreview{}, err }
		data = []byte(in.URL)
	}
	h := sha256.Sum256(data)
	inputHash := hex.EncodeToString(h[:])
	id := newID("import")
	tokenBytes := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", id, s.now().UnixNano())))
	token := hex.EncodeToString(tokenBytes[:])
	stageDir := filepath.Join(s.dataDir, "tmp")
	if stageDir == "tmp" || stageDir == "." { stageDir = os.TempDir() }
	if err := os.MkdirAll(stageDir, 0o700); err != nil { return ImportPreview{}, err }
	stage, err := os.CreateTemp(stageDir, "mpack-import-*")
	if err != nil { return ImportPreview{}, err }
	stageName := stage.Name()
	defer stage.Close()
	if _, err := stage.Write(data); err != nil { _ = os.Remove(stageName); return ImportPreview{}, err }
	if err := stage.Sync(); err != nil { _ = os.Remove(stageName); return ImportPreview{}, err }
	entryCount, packName, err := inspectArchive(stageName, in.Source == ImportSourceLocalZip)
	if err != nil { _ = os.Remove(stageName); return ImportPreview{}, err }
	now := s.now(); exp := now.Add(10 * time.Minute)
	if err := s.repo.CreateImportPreview(ctx, store.ImportPreviewRecord{ID:id, TokenHash:hashToken(token), InputHash:inputHash, Source:in.Source, StagedPath:stageName, ExpiresAt:sql.NullInt64{Int64:exp.UnixMilli(), Valid:true}, CreatedAt:sql.NullInt64{Int64:now.UnixMilli(), Valid:true}}); err != nil { _ = os.Remove(stageName); return ImportPreview{}, err }
	return ImportPreview{ID:id, Token:token, InputHash:inputHash, Source:in.Source, ExpiresAt:exp.UTC().Format(time.RFC3339Nano), EntryCount:entryCount, PackName:packName}, nil
}

func (s *ImportService) Confirm(ctx context.Context, in ImportConfirmInput) (*task.Task, bool, error) {
	if s == nil || s.repo == nil || s.queue == nil { return nil, false, ErrUnavailable }
	if strings.TrimSpace(in.PreviewID) == "" || strings.TrimSpace(in.Token) == "" || strings.TrimSpace(in.InputHash) != "" && len(in.InputHash) != 64 || strings.TrimSpace(in.IdempotencyKey) == "" { return nil, false, ErrInvalidArgument }
	p, err := s.repo.ConsumeImportPreview(ctx, in.PreviewID, hashToken(in.Token), in.InputHash, s.now().UnixMilli())
	if err != nil { if errors.Is(err, store.ErrConflict) { return nil, false, ErrImportExpired }; return nil, false, err }
	payload, _ := json.Marshal(map[string]string{"previewId": p.ID})
	return s.queue.Submit(ctx, task.SubmitRequest{Kind: task.KindImport, Title: "Import pack", Payload: payload, IdempotencyKey: in.IdempotencyKey})
}

func (s *ImportService) RegisterTaskHandler(reg *task.Registry) error {
	if s == nil || reg == nil { return errors.New("task registry is nil") }
	return reg.Register(task.KindImport, task.HandlerFunc(s.handleImportTask))
}
func (s *ImportService) RegisterTaskHandlerOnQueue(q *task.Queue) error {
	if s == nil || q == nil { return errors.New("task queue is nil") }
	return q.RegisterHandler(task.KindImport, task.HandlerFunc(s.handleImportTask))
}

func (s *ImportService) handleImportTask(ctx context.Context, ex *task.Execution) error {
	var payload struct{ PreviewID string `json:"previewId"` }
	if err := json.Unmarshal(ex.Task.Payload, &payload); err != nil || payload.PreviewID == "" { return &task.TaskError{Code:"invalid_payload", Message:"invalid import task payload"} }
	p, err := s.repo.GetImportPreview(ctx, payload.PreviewID); if err != nil { return err }
	if err := ex.Progress(ctx, 10, "inspecting"); err != nil { return err }
	name, mc, loader, err := parsePackMetadata(p.StagedPath); if err != nil { return &task.TaskError{Code:"import_parse_failed", Message:"pack archive could not be parsed"} }
	api := &API{repo:s.repo, now:s.now}
	if _, err := api.CreatePack(ctx, CreatePackInput{Name:name, MCVersion:mc, Loader:loader}, "task:"+ex.Task.ID); err != nil { return err }
	_ = os.Remove(p.StagedPath)
	return ex.Progress(ctx, 100, "imported")
}

func validateImportInput(in ImportPreviewInput) error {
	switch in.Source { case ImportSourceLocalZip: if len(in.Content) == 0 { return ErrInvalidArgument }; case ImportSourceCurseForgeURL, ImportSourceModrinthURL: if strings.TrimSpace(in.URL)=="" { return ErrInvalidArgument }; default: return ErrImportInvalidSource }
	return nil
}
func validateImportURL(raw, source string) error {
	u, err := url.Parse(raw); if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" { return ErrImportInvalidSource }
	h := strings.ToLower(u.Hostname()); if source == ImportSourceCurseForgeURL && !(h == "curseforge.com" || strings.HasSuffix(h, ".curseforge.com")) { return ErrImportInvalidSource }; if source == ImportSourceModrinthURL && !(h == "modrinth.com" || strings.HasSuffix(h, ".modrinth.com")) { return ErrImportInvalidSource }
	return nil
}
func hashToken(token string) string { h:=sha256.Sum256([]byte(token)); return hex.EncodeToString(h[:]) }

func inspectArchive(path string, required bool) (int, string, error) {
	if !required { return 0, "", nil }
	r, err := zip.OpenReader(path); if err != nil { return 0,"",ErrImportUnsafeArchive }; defer r.Close()
	if len(r.File) > 50000 { return 0,"",ErrImportUnsafeArchive }
	var total int64; name := ""
	for _, f := range r.File { if err := validateArchiveName(f.Name); err != nil { return 0,"",err }; if f.UncompressedSize64 > 512<<20 || total+int64(f.UncompressedSize64) > 2<<30 { return 0,"",ErrImportUnsafeArchive }; total += int64(f.UncompressedSize64); if f.Name == "manifest.json" || f.Name == "modrinth.index.json" { b, e := readZipEntry(f); if e != nil { return 0,"",ErrImportUnsafeArchive }; var m map[string]any; if json.Unmarshal(b,&m)==nil { if v,ok:=m["name"].(string); ok { name=v } } } }
	return len(r.File), name, nil
}
func validateArchiveName(name string) error { n:=strings.ReplaceAll(name,"\\", "/"); if n=="" || strings.HasPrefix(n,"/") || strings.Contains(n,":") || strings.HasPrefix(n,"../") || strings.Contains(n,"/../") || n==".." { return ErrImportUnsafeArchive }; return nil }
func readZipEntry(f *zip.File) ([]byte,error) { r,e:=f.Open(); if e!=nil{return nil,e}; defer r.Close(); return io.ReadAll(io.LimitReader(r,1<<20)) }
func parsePackMetadata(path string) (string,string,string,error) { r,e:=zip.OpenReader(path); if e!=nil{return "","","",e}; defer r.Close(); name,mc,loader:="Imported pack","1.20.1","fabric"; for _,f:=range r.File { if f.Name!="manifest.json" && f.Name!="modrinth.index.json" {continue}; b,e:=readZipEntry(f); if e!=nil{continue}; var m map[string]any; if json.Unmarshal(b,&m)!=nil{continue}; if v,ok:=m["name"].(string);ok&&v!=""{name=v}; if v,ok:=m["minecraft"].(map[string]any);ok {if x,ok:=v["version"].(string);ok&&x!=""{mc=x}}; if v,ok:=m["dependencies"].(map[string]any);ok {for _,k:=range []string{"fabric-loader","forge","neoforge","quilt-loader"}{if _,ok:=v[k];ok {loader=strings.TrimSuffix(k,"-loader");if loader=="forge"||loader=="neoforge"{break}}}} }; return name,mc,loader,nil }
