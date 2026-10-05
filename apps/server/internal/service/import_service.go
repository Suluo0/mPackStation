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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"mpackstation/internal/config"
	"mpackstation/internal/provider"

	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

const (
	ImportSourceCurseForgeURL = "curseforge_url"
	ImportSourceModrinthURL   = "modrinth_url"
	ImportSourceLocalZip      = "local_zip"
)

var (
	// ErrImportInvalidSource means the URL is well-formed https but its host
	// does not belong to the declared platform (semantic, 422).
	ErrImportInvalidSource = errors.New("invalid import source")
	ErrImportUnsafeArchive = errors.New("unsafe import archive")
	// ErrImportExpired means the preview expired before being consumed (410).
	ErrImportExpired = errors.New("import preview expired")
	// ErrImportConsumed means the preview was already consumed and the original
	// task can no longer be located (409 import_preview_consumed).
	ErrImportConsumed = errors.New("import preview consumed")
)

type ImportPreviewInput struct {
	Source  string
	URL     string
	Content []byte
	// Path 是本机整合包文件的绝对路径（2026-10-05 用户定稿：导入不走内容上传，
	// 后端直接读盘）。与 Content 二选一；local_zip 来源两者都接受。
	Path string `json:"path"`
}

// ImportPreview is the two-phase import handshake DTO. D-4: keys are always
// present; packName is "" when the source does not declare one yet (URL
// sources resolve the name at task time).
type ImportPreview struct {
	ID         string `json:"id"`
	Token      string `json:"token"`
	InputHash  string `json:"inputHash"`
	Source     string `json:"source"`
	ExpiresAt  string `json:"expiresAt"`
	EntryCount int    `json:"entryCount"`
	PackName   string `json:"packName"`
}

type ImportConfirmInput struct {
	PreviewID, Token, InputHash, IdempotencyKey string
}

type ImportService struct {
	repo    *store.Repository
	queue   *task.Queue
	dataDir string
	now     func() time.Time
	// registry 是组合根注入的 provider 适配器表，与 API 共用同一个实例：
	// 设置页热注册 CurseForge key 时导入侧同步生效。导入不自己造适配器。
	registry *provider.Registry
}

func NewImportService(db *sql.DB) *ImportService {
	s := &ImportService{now: time.Now}
	if db == nil {
		return s
	}
	s.repo = store.NewRepository(db)
	s.dataDir, _ = s.repo.DatabaseDir(context.Background())
	s.queue, _ = task.NewQueue(db)
	return s
}

// SetProviderRegistry 注入 provider 适配器（组合根调用一次）。
func (s *ImportService) SetProviderRegistry(r *provider.Registry) {
	if s != nil {
		s.registry = r
	}
}

// importAdapter 取既有 provider 适配器。清单只是"要哪些模组"的清单，下载一律
// 走这条既有管道，导入侧不再自带下载器。
func (s *ImportService) importAdapter(kind string) (provider.Adapter, error) {
	name := provider.Modrinth
	if kind == "curseforge_manifest" {
		name = provider.CurseForge
	}
	if s.registry == nil {
		return nil, &task.TaskError{Code: "import_provider_unavailable", Message: "provider 适配器未装配"}
	}
	ad, err := s.registry.Get(string(name))
	if err != nil {
		msg := fmt.Sprintf("%s 适配器不可用", name)
		if name == provider.CurseForge {
			msg += "：CurseForge 清单的模组要用 CF API 下载，请先在设置里配置 CurseForge Key"
		}
		return nil, &task.TaskError{Code: "import_provider_unavailable", Message: msg}
	}
	return ad, nil
}

func (s *ImportService) Inspect(ctx context.Context, in ImportPreviewInput) (ImportPreview, error) {
	if s == nil || s.repo == nil {
		return ImportPreview{}, ErrUnavailable
	}
	in.Source = normalizeImportSource(in.Source)
	if err := validateImportInput(in); err != nil {
		return ImportPreview{}, err
	}
	data := in.Content
	if in.Source != ImportSourceLocalZip {
		if err := validateImportURL(in.URL, in.Source); err != nil {
			return ImportPreview{}, err
		}
		data = []byte(in.URL)
	}
	h := sha256.Sum256(data)
	inputHash := hex.EncodeToString(h[:])
	id := newID("import")
	tokenBytes := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", id, s.now().UnixNano())))
	token := hex.EncodeToString(tokenBytes[:])
	// 本机路径来源（2026-10-05 用户定稿）：后端直接读盘，不走 base64 内容上传。
	// 暂存文件（base64 上传的落点）在导入完成后会清掉；用户自己的原文件永不删除。
	userPath := strings.TrimSpace(in.Path)
	if in.Source == ImportSourceLocalZip && userPath != "" {
		if !filepath.IsAbs(userPath) {
			return ImportPreview{}, ErrInvalidArgument
		}
		fi, err := os.Stat(userPath)
		if err != nil || fi.IsDir() {
			return ImportPreview{}, ErrImportUnsafeArchive
		}
		if !strings.EqualFold(filepath.Ext(userPath), ".zip") && !strings.EqualFold(filepath.Ext(userPath), ".mrpack") {
			return ImportPreview{}, ErrImportUnsafeArchive
		}
		entryCount, packName, err := inspectArchive(userPath, true)
		if err != nil {
			return ImportPreview{}, err
		}
		now := s.now()
		exp := now.Add(10 * time.Minute)
		h := sha256.Sum256([]byte(fmt.Sprintf("path:%s:%d", userPath, fi.Size())))
		inputHash := hex.EncodeToString(h[:])
		id := newID("import")
		tokenBytes := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", id, s.now().UnixNano())))
		token := hex.EncodeToString(tokenBytes[:])
		if err := s.repo.CreateImportPreview(ctx, store.ImportPreviewRecord{ID: id, TokenHash: hashToken(token), InputHash: inputHash, Source: in.Source, StagedPath: userPath, ExpiresAt: sql.NullInt64{Int64: exp.UnixMilli(), Valid: true}, CreatedAt: sql.NullInt64{Int64: now.UnixMilli(), Valid: true}}); err != nil {
			return ImportPreview{}, err
		}
		return ImportPreview{ID: id, Token: token, InputHash: inputHash, Source: in.Source, ExpiresAt: exp.UTC().Format(time.RFC3339Nano), EntryCount: entryCount, PackName: packName}, nil
	}
	stageDir := filepath.Join(s.dataDir, "tmp")
	if stageDir == "tmp" || stageDir == "." {
		stageDir = os.TempDir()
	}
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return ImportPreview{}, err
	}
	stage, err := os.CreateTemp(stageDir, "mpack-import-*")
	if err != nil {
		return ImportPreview{}, err
	}
	stageName := stage.Name()
	defer stage.Close()
	if _, err := stage.Write(data); err != nil {
		_ = os.Remove(stageName)
		return ImportPreview{}, err
	}
	if err := stage.Sync(); err != nil {
		_ = os.Remove(stageName)
		return ImportPreview{}, err
	}
	entryCount, packName, err := inspectArchive(stageName, in.Source == ImportSourceLocalZip)
	if err != nil {
		_ = os.Remove(stageName)
		return ImportPreview{}, err
	}
	now := s.now()
	exp := now.Add(10 * time.Minute)
	if err := s.repo.CreateImportPreview(ctx, store.ImportPreviewRecord{ID: id, TokenHash: hashToken(token), InputHash: inputHash, Source: in.Source, StagedPath: stageName, ExpiresAt: sql.NullInt64{Int64: exp.UnixMilli(), Valid: true}, CreatedAt: sql.NullInt64{Int64: now.UnixMilli(), Valid: true}}); err != nil {
		_ = os.Remove(stageName)
		return ImportPreview{}, err
	}
	return ImportPreview{ID: id, Token: token, InputHash: inputHash, Source: in.Source, ExpiresAt: exp.UTC().Format(time.RFC3339Nano), EntryCount: entryCount, PackName: packName}, nil
}

func (s *ImportService) Confirm(ctx context.Context, in ImportConfirmInput) (*task.Task, bool, error) {
	if s == nil || s.repo == nil || s.queue == nil {
		return nil, false, ErrUnavailable
	}
	if strings.TrimSpace(in.PreviewID) == "" || strings.TrimSpace(in.Token) == "" || strings.TrimSpace(in.IdempotencyKey) == "" {
		return nil, false, ErrInvalidArgument
	}
	if strings.TrimSpace(in.InputHash) == "" || len(in.InputHash) != 64 {
		return nil, false, ErrInvalidArgument
	}
	p, err := s.repo.GetImportPreview(ctx, in.PreviewID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, false, fmt.Errorf("%w: unknown import preview", ErrInvalidArgument)
		}
		return nil, false, err
	}
	if p.TokenHash != hashToken(in.Token) {
		return nil, false, fmt.Errorf("%w: token does not match preview", ErrInvalidArgument)
	}
	if p.InputHash != in.InputHash {
		return nil, false, &DomainError{Status: 422, Code: "import_input_mismatch", Message: "inputHash does not match the previewed input"}
	}
	if p.ConsumedAt.Valid {
		// Same input confirmed again: replay the original task (D-6). Only when
		// the original task is gone do we surface 409 import_preview_consumed.
		if p.ConsumedTaskID != "" {
			if t, gerr := s.queue.Get(ctx, p.ConsumedTaskID); gerr == nil {
				return t, true, nil
			}
		}
		return nil, false, ErrImportConsumed
	}
	if p.ExpiresAt.Valid && p.ExpiresAt.Int64 <= s.now().UnixMilli() {
		return nil, false, ErrImportExpired
	}
	if _, err := s.repo.ConsumeImportPreview(ctx, in.PreviewID, p.TokenHash, in.InputHash, s.now().UnixMilli()); err != nil {
		if errors.Is(err, store.ErrConflict) {
			// Lost a concurrent consume race: re-read and replay that outcome.
			p2, gerr := s.repo.GetImportPreview(ctx, in.PreviewID)
			if gerr == nil && p2.ConsumedAt.Valid && p2.ConsumedTaskID != "" {
				if t, terr := s.queue.Get(ctx, p2.ConsumedTaskID); terr == nil {
					return t, true, nil
				}
			}
			return nil, false, ErrImportConsumed
		}
		return nil, false, err
	}
	payload, _ := json.Marshal(map[string]string{"previewId": p.ID})
	t, reused, err := s.queue.Submit(ctx, task.SubmitRequest{Kind: task.KindImport, Title: "Import pack", Payload: payload, IdempotencyKey: in.IdempotencyKey})
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.SetImportPreviewConsumedTask(ctx, p.ID, t.ID); err != nil {
		return nil, false, err
	}
	return t, reused, nil
}

func (s *ImportService) RegisterTaskHandler(reg *task.Registry) error {
	if s == nil || reg == nil {
		return errors.New("task registry is nil")
	}
	return reg.Register(task.KindImport, task.HandlerFunc(s.handleImportTask))
}
func (s *ImportService) RegisterTaskHandlerOnQueue(q *task.Queue) error {
	if s == nil || q == nil {
		return errors.New("task queue is nil")
	}
	return q.RegisterHandler(task.KindImport, task.HandlerFunc(s.handleImportTask))
}

func (s *ImportService) handleImportTask(ctx context.Context, ex *task.Execution) error {
	var payload struct {
		PreviewID string `json:"previewId"`
	}
	if err := json.Unmarshal(ex.Task.Payload, &payload); err != nil || payload.PreviewID == "" {
		return &task.TaskError{Code: "invalid_payload", Message: "invalid import task payload"}
	}
	p, err := s.repo.GetImportPreview(ctx, payload.PreviewID)
	if err != nil {
		return err
	}
	if err := ex.Progress(ctx, 5, "parsing manifest"); err != nil {
		return err
	}
	mft, err := parseImportManifest(p.StagedPath)
	if err != nil {
		return &task.TaskError{Code: "import_parse_failed", Message: "pack archive could not be parsed: " + err.Error()}
	}
	api := &API{repo: s.repo, now: s.now}
	// 同名包占名（包括软删行——packs.name 的 UNIQUE 不区分状态）：名字追加短后缀
	// 重试一次。导入不许因为重名半途失败（2026-10-05 用户指令）。
	pack, err := api.CreatePack(ctx, CreatePackInput{Name: mft.Name, MCVersion: mft.MCVersion, Loader: mft.Loader, LoaderVersion: mft.LoaderVersion}, "task:"+ex.Task.ID)
	if err != nil && strings.Contains(err.Error(), "pack_name_duplicate") {
		pack, err = api.CreatePack(ctx, CreatePackInput{Name: fmt.Sprintf("%s (%s)", mft.Name, s.now().Format("0102-1504")), MCVersion: mft.MCVersion, Loader: mft.Loader, LoaderVersion: mft.LoaderVersion}, "task:"+ex.Task.ID)
	}
	if err != nil {
		return err
	}

	// 项目目录（2026-10-05 用户定稿）：导入的整合包按「每个项目一个目录」落盘，
	// 模组文件直接放进 mods/，并写我们自己的 metadata.json 记录。项目根目录
	// 在设置里可改（appsettings.json，OS 用户配置目录）。
	root, err := config.ResolveProjectRoot()
	if err != nil {
		return &task.TaskError{Code: "import_project_root", Message: err.Error()}
	}
	shortID := pack.ID
	if len(shortID) > 8 {
		shortID = shortID[len(shortID)-8:]
	}
	projDir := filepath.Join(root, config.SanitizeDirName(mft.Name)+"-"+shortID)
	modsDir := filepath.Join(projDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		return &task.TaskError{Code: "import_project_dir", Message: "create project directory failed: " + err.Error()}
	}

	// 模组下载一律走既有 provider 管道，导入只负责「清单 → 下载请求」的翻译：
	//  - Modrinth 清单：files[].downloads[0] 是 CDN 直链，形如
	//    https://cdn.modrinth.com/data/<projectID>/versions/<versionID>/<file>，
	//    取其中两段 ID 即可复用「按版本下载」。
	//  - CurseForge 清单：只有 projectID/fileID（fileID 就是版本 ID），同一管道。
	adapter, aerr := s.importAdapter(mft.Kind)
	if aerr != nil {
		return aerr
	}

	// 逐条走既有下载管道。心跳独立于下载：单个模组要下几十秒到几分钟
	// （Cobblemon 122MB 实测 50 秒+），期间没有 Progress 上报，30 秒任务租约
	// 照样过期——Progress 是同时续租的唯一入口（见 task.Queue.Progress）。
	var lastPct atomic.Int64
	lastPct.Store(10)
	stopBeat := make(chan struct{})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopBeat:
				return
			case <-t.C:
				_ = ex.Progress(ctx, float64(lastPct.Load()), "downloading…")
			}
		}
	}()
	total := len(mft.Files)
	for i := range mft.Files {
		spec := &mft.Files[i]
		pid, vid, ok := importDownloadIDs(spec, mft.Kind)
		if !ok {
			close(stopBeat)
			return &task.TaskError{Code: "import_download_unsupported",
				Message: "该模组的下载地址不是 Modrinth CDN 直链，既有下载管道覆盖不了：" + spec.URL}
		}
		res, derr := adapter.Download(ctx, provider.DownloadRequest{ProjectID: pid, VersionID: vid})
		if derr != nil {
			close(stopBeat)
			return &task.TaskError{Code: "import_download_failed",
				Message: fmt.Sprintf("模组下载失败：%s（%v）", filepath.Base(spec.Path), derr)}
		}
		if spec.Sha1 != "" && res.SHA1 != "" && !strings.EqualFold(spec.Sha1, res.SHA1) {
			close(stopBeat)
			return &task.TaskError{Code: "import_download_failed",
				Message: fmt.Sprintf("模组校验失败：%s（sha1 不匹配）", filepath.Base(spec.Path))}
		}
		rel := strings.TrimSpace(spec.Path)
		if rel == "" {
			rel = "mods/" + res.FileName
		}
		dest, perr := safeImportPath(projDir, rel)
		if perr != nil {
			close(stopBeat)
			return &task.TaskError{Code: "import_unsafe_path", Message: "清单里的模组路径不合法：" + rel}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			close(stopBeat)
			return &task.TaskError{Code: "import_project_dir", Message: err.Error()}
		}
		if err := os.WriteFile(dest, res.Content, 0o644); err != nil {
			close(stopBeat)
			return &task.TaskError{Code: "import_project_dir", Message: err.Error()}
		}
		spec.Sha1, spec.Size = res.SHA1, int64(len(res.Content))
		pct := 10 + 80*float64(i+1)/float64(max(1, total))
		lastPct.Store(int64(pct))
		_ = ex.Progress(ctx, pct, fmt.Sprintf("downloading %d/%d", i+1, total))
	}
	close(stopBeat)

	// overrides/：构建产物把平台之外的 jar 直接放在包里（overrides/mods/*.jar），
	// 这部分不联网，解压落盘即可。顺序固定先 overrides/ 后 client-overrides/，
	// 同路径后者胜出（mrpack 规范）。
	if _, err := extractOverrides(p.StagedPath, projDir); err != nil {
		return &task.TaskError{Code: "import_overrides_failed", Message: err.Error()}
	}

	// 自有 metadata 记录：项目目录里的模组台账（我们自己的格式，不依赖平台清单）。
	meta := map[string]any{
		"format":   "mpackstation-project",
		"version":  1,
		"imported": s.now().UnixMilli(),
		"source":   map[string]any{"kind": mft.Kind, "archive": filepath.Base(p.StagedPath)},
		"pack":     map[string]any{"name": mft.Name, "mcVersion": mft.MCVersion, "loader": mft.Loader, "loaderVersion": mft.LoaderVersion},
	}
	mods := make([]map[string]any, 0, len(mft.Files))
	for _, spec := range mft.Files {
		e := map[string]any{"fileName": filepath.Base(spec.Path), "sha1": spec.Sha1, "size": spec.Size, "required": spec.Required}
		if mft.Kind == "modrinth_index" {
			e["origin"] = map[string]any{"provider": "modrinth", "url": spec.URL}
		} else {
			e["origin"] = map[string]any{"provider": "curseforge", "projectID": spec.CFProject, "fileID": spec.CFFile}
		}
		mods = append(mods, e)
	}
	meta["mods"] = mods
	rawMeta, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projDir, "metadata.json"), rawMeta, 0o644); err != nil {
		return &task.TaskError{Code: "import_project_meta", Message: "write metadata.json failed: " + err.Error()}
	}

	// 台账入库：每个模组一条 pack_mods（local 来源，文件在项目目录里）。
	for i, spec := range mft.Files {
		if err := ex.Progress(ctx, 90+8*float64(i)/float64(max(1, len(mft.Files))), "recording "+filepath.Base(spec.Path)); err != nil {
			return err
		}
		display := strings.TrimSuffix(filepath.Base(spec.Path), filepath.Ext(spec.Path))
		if _, err := api.AddLocalPackMod(ctx, pack.ID, LocalModInput{
			DisplayName: display, FileName: filepath.Base(spec.Path),
			SHA1: spec.Sha1, Size: spec.Size, Required: spec.Required,
		}, "task:"+ex.Task.ID); err != nil {
			return &task.TaskError{Code: "import_record_failed", Message: fmt.Sprintf("模组记录失败：%s（%v）", filepath.Base(spec.Path), err)}
		}
	}
	// 暂存临时文件（base64 上传落点）清掉；用户自己的原文件永不删除。
	if isStagedTemp(p.StagedPath, s.dataDir) {
		_ = os.Remove(p.StagedPath)
	}
	return ex.Progress(ctx, 100, fmt.Sprintf("imported %d mods → %s", len(mft.Files), projDir))
}

func validateImportInput(in ImportPreviewInput) error {
	switch in.Source {
	case ImportSourceLocalZip:
		if len(in.Content) == 0 && strings.TrimSpace(in.Path) == "" {
			return ErrInvalidArgument
		}
	case ImportSourceCurseForgeURL, ImportSourceModrinthURL:
		if strings.TrimSpace(in.URL) == "" {
			return ErrInvalidArgument
		}
	default:
		// source outside the closed enum is a structural error (400).
		return ErrInvalidArgument
	}
	return nil
}
func normalizeImportSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "curseforge":
		return ImportSourceCurseForgeURL
	case "modrinth":
		return ImportSourceModrinthURL
	case "local":
		return ImportSourceLocalZip
	default:
		return strings.ToLower(strings.TrimSpace(source))
	}
}
func validateImportURL(raw, source string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		// Not a well-formed https URL: structural error (400).
		return ErrInvalidArgument
	}
	h := strings.ToLower(u.Hostname())
	if source == ImportSourceCurseForgeURL && !(h == "curseforge.com" || strings.HasSuffix(h, ".curseforge.com")) {
		return ErrImportInvalidSource
	}
	if source == ImportSourceModrinthURL && !(h == "modrinth.com" || strings.HasSuffix(h, ".modrinth.com")) {
		return ErrImportInvalidSource
	}
	return nil
}
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func inspectArchive(path string, required bool) (int, string, error) {
	if !required {
		return 0, "", nil
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0, "", ErrImportUnsafeArchive
	}
	defer r.Close()
	if len(r.File) > 50000 {
		return 0, "", ErrImportUnsafeArchive
	}
	var total int64
	name := ""
	for _, f := range r.File {
		if err := validateArchiveName(f.Name); err != nil {
			return 0, "", err
		}
		if f.UncompressedSize64 > 512<<20 || total+int64(f.UncompressedSize64) > 2<<30 {
			return 0, "", ErrImportUnsafeArchive
		}
		total += int64(f.UncompressedSize64)
		if f.Name == "manifest.json" || f.Name == "modrinth.index.json" {
			b, e := readZipEntry(f)
			if e != nil {
				return 0, "", ErrImportUnsafeArchive
			}
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				if v, ok := m["name"].(string); ok {
					name = v
				}
			}
		}
	}
	return len(r.File), name, nil
}
func validateArchiveName(name string) error {
	n := strings.ReplaceAll(name, "\\", "/")
	if n == "" || strings.HasPrefix(n, "/") || strings.Contains(n, ":") || strings.HasPrefix(n, "../") || strings.Contains(n, "/../") || n == ".." {
		return ErrImportUnsafeArchive
	}
	return nil
}
func readZipEntry(f *zip.File) ([]byte, error) {
	r, e := f.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 1<<20))
}

// importManifest 是两类清单的统一中间表示：Modrinth 的 modrinth.index.json 与
// CurseForge 的 manifest.json（zip 包里本来就带，2026-10-05 确认）。字段对齐后
// 下游（下载/记录/写 metadata）不再关心来源差异。
type importManifest struct {
	Kind          string // modrinth_index | curseforge_manifest
	Name          string
	MCVersion     string
	Loader        string
	LoaderVersion string
	Files         []importFileSpec
}

type importFileSpec struct {
	Path      string // 清单里的目标相对路径（mods/xxx.jar）
	Sha1      string // mrpack 清单自带，下载后强校验；CF 下载后现算
	Size      int64
	URL       string // mrpack 直链（cdn.modrinth.com）
	Required  bool
	CFProject int64
	CFFile    int64
}

// parseImportManifest 从暂存的 zip/mrpack 里解析清单。两种格式都有官方定义：
// mrpack = modrinth.index.json（files[].downloads 是直链）；CF zip = manifest.json
// （files[].projectID/fileID，下载要过 CF API，密钥是硬前置）。
func parseImportManifest(path string) (*importManifest, error) {
	r, e := zip.OpenReader(path)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	out := &importManifest{Name: "Imported pack", MCVersion: "1.20.1", Loader: "fabric"}
	seen := false
	for _, f := range r.File {
		name := strings.TrimPrefix(f.Name, "/")
		if name != "modrinth.index.json" && name != "manifest.json" {
			continue
		}
		b, e := readZipEntry(f)
		if e != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(b, &raw) != nil {
			continue
		}
		seen = true
		if v, ok := raw["name"].(string); ok && v != "" {
			out.Name = v
		}
		switch name {
		case "modrinth.index.json":
			out.Kind = "modrinth_index"
			// mrpack 规范把 MC 版本放在 dependencies.minecraft（字符串值），
			// 没有顶层 minecraft 对象；只认后者会让版本恒为初始默认值。
			if deps, ok := raw["dependencies"].(map[string]any); ok {
				if v, ok := deps["minecraft"].(string); ok && v != "" {
					out.MCVersion = v
				}
				for _, k := range []string{"fabric-loader", "neoforge", "forge", "quilt-loader"} {
					if v, ok := deps[k].(string); ok && v != "" {
						out.Loader = strings.TrimSuffix(k, "-loader")
						out.LoaderVersion = v
						break
					}
				}
			}
			// 兼容非规范的顶层 minecraft.version（早期或第三方生成器）。
			if m, ok := raw["minecraft"].(map[string]any); ok {
				if v, ok := m["version"].(string); ok && v != "" {
					out.MCVersion = v
				}
			}
			files, _ := raw["files"].([]any)
			for _, it := range files {
				fm, ok := it.(map[string]any)
				if !ok {
					continue
				}
				spec := importFileSpec{Path: jsonStr(fm["path"]), Required: true}
				if h, ok := fm["hashes"].(map[string]any); ok {
					spec.Sha1 = jsonStr(h["sha1"])
				}
				if s, ok := fm["fileSize"].(float64); ok {
					spec.Size = int64(s)
				}
				if env, ok := fm["env"].(map[string]any); ok {
					if c, ok := env["client"].(string); ok && c == "unsupported" {
						spec.Required = false
					}
				}
				if dl, ok := fm["downloads"].([]any); ok && len(dl) > 0 {
					spec.URL = jsonStr(dl[0])
				}
				if spec.Path != "" && spec.URL != "" {
					out.Files = append(out.Files, spec)
				}
			}
		case "manifest.json":
			out.Kind = "curseforge_manifest"
			if m, ok := raw["minecraft"].(map[string]any); ok {
				if v, ok := m["version"].(string); ok && v != "" {
					out.MCVersion = v
				}
				if mls, ok := m["modLoaders"].([]any); ok {
					for _, ml := range mls {
						mo, ok := ml.(map[string]any)
						if !ok {
							continue
						}
						id := jsonStr(mo["id"]) // 形如 "forge-47.2.0"
						for _, k := range []string{"neoforge", "forge", "fabric", "quilt"} {
							if strings.HasPrefix(id, k+"-") {
								out.Loader = k
								out.LoaderVersion = strings.TrimPrefix(id, k+"-")
								break
							}
						}
						if out.Loader != "" {
							break
						}
					}
				}
			}
			files, _ := raw["files"].([]any)
			for _, it := range files {
				fm, ok := it.(map[string]any)
				if !ok {
					continue
				}
				pid, _ := fm["projectID"].(float64)
				fid, _ := fm["fileID"].(float64)
				if pid == 0 || fid == 0 {
					continue
				}
				out.Files = append(out.Files, importFileSpec{
					Path:      fmt.Sprintf("mods/cf-%d-%d.jar", int64(pid), int64(fid)),
					CFProject: int64(pid),
					CFFile:    int64(fid),
					Required:  fm["required"] != false,
				})
			}
		}
		if seen {
			break // 正常只有一个清单；并存时以先读到的一份为准
		}
	}
	if !seen {
		return nil, fmt.Errorf("neither modrinth.index.json nor manifest.json in archive")
	}
	if out.Loader == "" {
		out.Loader = "fabric"
	}
	return out, nil
}

func jsonStr(v any) string {
	s, _ := v.(string)
	return s
}

// isStagedTemp 判断路径是不是导入流程自己的暂存文件（base64 上传落点）。
// 用户通过路径导入的原文件不属于我们，永不删除。
func isStagedTemp(path, dataDir string) bool {
	stageDir := filepath.Join(dataDir, "tmp")
	if stageDir == "tmp" || stageDir == "." {
		stageDir = os.TempDir()
	}
	rel, err := filepath.Rel(stageDir, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// importDownloadIDs 把一个清单文件翻译成既有下载管道认识的一对 ID。
// Modrinth 从 CDN 直链里取，CurseForge 清单直接给了 projectID/fileID。
func importDownloadIDs(spec *importFileSpec, kind string) (string, string, bool) {
	if kind == "curseforge_manifest" {
		if spec.CFProject == 0 || spec.CFFile == 0 {
			return "", "", false
		}
		return strconv.FormatInt(spec.CFProject, 10), strconv.FormatInt(spec.CFFile, 10), true
	}
	return modrinthIDsFromURL(spec.URL)
}

// modrinthIDsFromURL 从 mrpack 清单的 CDN 直链里取出 projectID/versionID：
// https://cdn.modrinth.com/data/<projectID>/versions/<versionID>/<file>
func modrinthIDsFromURL(raw string) (string, string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] == "data" && parts[i+2] == "versions" {
			return parts[i+1], parts[i+3], true
		}
	}
	return "", "", false
}

// safeImportPath 把清单里的相对路径落到项目目录内，拒绝任何越出项目目录的路径。
func safeImportPath(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", ErrImportUnsafeArchive
	}
	dest := filepath.Join(root, clean)
	if !strings.HasPrefix(dest, filepath.Clean(root)+string(os.PathSeparator)) {
		return "", ErrImportUnsafeArchive
	}
	return dest, nil
}

// extractOverrides 把 mrpack 的 overrides/ 与 client-overrides/ 解到实例根目录，
// 返回落盘文件数。macOS 的 .DS_Store 一类垃圾条目跳过。
func extractOverrides(archivePath, destRoot string) (int, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, ErrImportUnsafeArchive
	}
	defer r.Close()
	root := filepath.Clean(destRoot)
	count := 0
	for _, group := range []string{"overrides/", "client-overrides/"} {
		for _, f := range r.File {
			if !strings.HasPrefix(f.Name, group) || f.FileInfo().IsDir() {
				continue
			}
			if err := validateArchiveName(f.Name); err != nil {
				return count, err
			}
			rel := strings.TrimPrefix(f.Name, group)
			if rel == "" || strings.HasSuffix(rel, ".DS_Store") {
				continue
			}
			dest, perr := safeImportPath(root, rel)
			if perr != nil {
				return count, perr
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return count, err
			}
			if err := writeZipEntryToFile(f, dest); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// writeZipEntryToFile 流式落盘一个 zip 条目（overrides 里可能有几十 MB 的 jar，
// 不能用 readZipEntry 的 1MB 上限）。
func writeZipEntryToFile(f *zip.File, dest string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(src, 1<<30)); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	return out.Close()
}
