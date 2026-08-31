package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mpackstation/internal/store"
)

var (
	// ErrInvalidBuildInput indicates a malformed or unsafe build request.
	ErrInvalidBuildInput = errors.New("invalid build input")
	// ErrExportDirNotAllowed indicates that the destination was not explicitly
	// registered as an approved export directory.
	ErrExportDirNotAllowed = errors.New("export directory is not allowed")
	// ErrDeliveryBlocked means at least one persisted delivery gate is blocked.
	ErrDeliveryBlocked = errors.New("delivery check blocked")
	// ErrArtifactMissing indicates a database artifact whose file disappeared.
	ErrArtifactMissing = errors.New("artifact file is missing")
)

// BuildFile is an already validated source entry. The build API deliberately
// accepts bytes rather than arbitrary source paths; blobstore/import services
// own filesystem access and can supply validated content here.
type BuildFile struct {
	Path    string
	Content []byte
}

// DeliveryCheck is the service representation of a readiness gate.
type DeliveryCheck struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func deliveryCheckDTO(c store.DeliveryCheckRecord) DeliveryCheck {
	return DeliveryCheck{Kind: c.Kind, Status: c.Status, Detail: c.Detail}
}

// BuildInput contains all inputs that affect a reproducible archive.
type BuildInput struct {
	PackID, PackVersionID, ExportDirName string
	// TaskID links an asynchronous build artifact to its durable task.
	TaskID          string
	Files           []BuildFile
	LockSnapshot    json.RawMessage
	ContentSnapshot json.RawMessage
	QuestSnapshot   json.RawMessage
	BuildConfig     json.RawMessage
	Checks          []DeliveryCheck
}

// Artifact is the safe service DTO returned after a successful build.
type Artifact struct {
	ID                string `json:"id"`
	PackID            string `json:"packId"`
	PackVersionID     string `json:"packVersionId"`
	FileName          string `json:"fileName"`
	SHA256            string `json:"sha256"`
	SourceFingerprint string `json:"sourceFingerprint"`
	Status            string `json:"status"`
	Kind              string `json:"kind"`
	Path              string `json:"-"`
	SizeBytes         int64  `json:"sizeBytes"`
	CreatedAt         string `json:"createdAt"`
}

// BuildResult includes both the generated artifact and the exact input
// fingerprint used for idempotency.
type BuildResult struct {
	Artifact          Artifact `json:"artifact"`
	SourceFingerprint string   `json:"sourceFingerprint"`
}

type PackVersion struct {
	ID        string `json:"id"`
	PackID    string `json:"packId"`
	Version   string `json:"version"`
	Channel   string `json:"channel"`
	Changelog string `json:"changelog"`
	Source    string `json:"source"`
	LockID    string `json:"lockId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func packVersionDTO(v store.PackVersionRecord) PackVersion {
	return PackVersion{ID: v.ID, PackID: v.PackID, Version: v.Version, Channel: v.Channel, Changelog: v.Changelog, Source: v.Source, LockID: v.LockID.String, CreatedAt: iso(v.CreatedAt), UpdatedAt: iso(v.UpdatedAt)}
}
func (a *API) ListPackVersions(ctx context.Context, packID string) ([]PackVersion, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListPackVersions(ctx, packID)
	if err != nil {
		return nil, err
	}
	out := make([]PackVersion, 0, len(rows))
	for _, v := range rows {
		out = append(out, packVersionDTO(v))
	}
	return out, nil
}
func (a *API) CreatePackVersion(ctx context.Context, packID, version, channel, changelog, source string) (PackVersion, error) {
	if err := a.ready(); err != nil {
		return PackVersion{}, err
	}
	if strings.TrimSpace(version) == "" {
		return PackVersion{}, ErrInvalidBuildInput
	}
	if channel == "" {
		channel = "draft"
	}
	if source == "" {
		source = "manual"
	}
	if channel != "draft" && channel != "release" {
		return PackVersion{}, ErrInvalidBuildInput
	}
	if source != "manual" && source != "imported" && source != "build" {
		return PackVersion{}, ErrInvalidBuildInput
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return PackVersion{}, err
	}
	now := a.nowMillis()
	v := store.PackVersionRecord{ID: newID("version"), PackID: packID, Version: version, Channel: channel, Changelog: changelog, Source: source, CreatedAt: now, UpdatedAt: now}
	if err := a.repo.CreatePackVersion(ctx, v); err != nil {
		return PackVersion{}, err
	}
	return packVersionDTO(v), nil
}

// RegisterExportDirectory adds an explicit, marker-verified destination. It
// is intentionally separate from BuildPack so a build can never silently
// choose an arbitrary user directory.
func (a *API) RegisterExportDirectory(ctx context.Context, name, directory string) error {
	if err := a.ready(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	abs, err := canonicalExportDir(directory)
	if err != nil || name == "" || name == "." {
		return ErrInvalidBuildInput
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return ErrExportDirNotAllowed
	}
	marker := filepath.Join(abs, ".mpackstation-export")
	if f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return ErrExportDirNotAllowed
		}
	} else {
		_ = f.Close()
	}
	if err := verifyExportDir(abs); err != nil {
		return ErrExportDirNotAllowed
	}
	now := time.Now().UnixMilli()
	return a.repo.RegisterExportDir(ctx, store.ExportDirRecord{Name: name, AbsolutePath: abs, MarkerVerifiedAt: now, CreatedAt: now})
}

// ListDeliveryChecks returns persisted checks for a pack version.
func (a *API) ListDeliveryChecks(ctx context.Context, packID, versionID string) ([]DeliveryCheck, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListDeliveryChecks(ctx, packID, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]DeliveryCheck, 0, len(rows))
	for _, r := range rows {
		out = append(out, deliveryCheckDTO(r))
	}
	return out, nil
}

// RunDeliveryChecks persists an explicit readiness run; actual domain checks
// remain owned by the corresponding services and are supplied as findings.
func (a *API) RunDeliveryChecks(ctx context.Context, packID, versionID string, checks []DeliveryCheck) ([]DeliveryCheck, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	now := a.nowMillis()
	rows := make([]store.DeliveryCheckRecord, 0, len(checks))
	fp := hashJSON([]byte(packID + ":" + versionID))
	for _, c := range checks {
		if !validDeliveryCheck(c.Kind, c.Status) {
			return nil, ErrInvalidBuildInput
		}
		d, err := normalizeSnapshot(json.RawMessage(c.Detail))
		if err != nil {
			return nil, err
		}
		rows = append(rows, store.DeliveryCheckRecord{ID: newID("delivery"), PackID: packID, PackVersionID: versionID, Kind: c.Kind, Status: c.Status, Detail: string(d), InputFingerprint: fp, RunID: fp[:16], CheckedAt: now})
	}
	if err := a.repo.SaveDeliveryChecks(ctx, packID, versionID, rows); err != nil {
		return nil, err
	}
	return a.ListDeliveryChecks(ctx, packID, versionID)
}

func (a *API) ListArtifacts(ctx context.Context, packID, versionID string) ([]Artifact, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListArtifacts(ctx, packID, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(rows))
	for _, r := range rows {
		out = append(out, artifactDTO(r))
	}
	return out, nil
}

func (a *API) ReadArtifact(ctx context.Context, packID, artifactID string) (Artifact, []byte, error) {
	if err := a.ready(); err != nil {
		return Artifact{}, nil, err
	}
	r, err := a.repo.GetArtifact(ctx, artifactID)
	if err != nil {
		if IsNotFound(err) {
			return Artifact{}, nil, NotFoundError("artifact_not_found", "artifact not found")
		}
		return Artifact{}, nil, err
	}
	if r.PackID != packID {
		return Artifact{}, nil, NotFoundError("artifact_not_found", "artifact not found")
	}
	if err := verifyArtifactFile(r.Path, r.SHA256, r.SizeBytes); err != nil {
		return Artifact{}, nil, err
	}
	b, err := os.ReadFile(r.Path)
	if err != nil {
		return Artifact{}, nil, ErrArtifactMissing
	}
	return artifactDTO(r), b, nil
}

// BuildPack creates a deterministic local zip and registers it only after the
// complete file is closed, synced, validated, and atomically renamed.
func (a *API) BuildPack(ctx context.Context, in BuildInput) (BuildResult, error) {
	if err := a.ready(); err != nil {
		return BuildResult{}, err
	}
	if err := validateBuildInput(in); err != nil {
		return BuildResult{}, err
	}
	version, err := a.repo.GetPackVersion(ctx, in.PackID, in.PackVersionID)
	if err != nil {
		if IsNotFound(err) {
			return BuildResult{}, NotFoundError("pack_version_not_found", "pack version not found")
		}
		return BuildResult{}, err
	}
	// A version with a designated lock must build from that exact immutable
	// snapshot. This prevents callers from silently supplying an unrelated lock.
	if version.LockID.Valid {
		lock, lerr := a.repo.GetLock(ctx, in.PackID, version.LockID.String)
		if lerr != nil {
			return BuildResult{}, lerr
		}
		provided, nerr := normalizeSnapshot(in.LockSnapshot)
		if nerr != nil {
			return BuildResult{}, nerr
		}
		providedHash := hashJSON(provided)
		canonicalLockHash := hashJSON([]byte(lock.SnapshotJSON))
		if providedHash != lock.SnapshotSHA256 && providedHash != canonicalLockHash {
			return BuildResult{}, ErrInvalidBuildInput
		}
	}
	dir, err := a.repo.GetExportDir(ctx, in.ExportDirName)
	if err != nil {
		return BuildResult{}, ErrExportDirNotAllowed
	}
	if err := verifyRegisteredExportDir(dir.AbsolutePath); err != nil {
		return BuildResult{}, ErrExportDirNotAllowed
	}

	normalized, fingerprint, inputRows, checks, err := buildManifest(in, a.nowMillis())
	if err != nil {
		return BuildResult{}, err
	}
	for _, check := range checks {
		if check.Status == "blocked" {
			return BuildResult{}, ErrDeliveryBlocked
		}
	}
	if len(checks) == 0 {
		blocked, berr := a.repo.HasBlockedDeliveryCheck(ctx, in.PackID, in.PackVersionID)
		if berr != nil {
			return BuildResult{}, berr
		}
		if blocked {
			return BuildResult{}, ErrDeliveryBlocked
		}
	}
	if old, err := a.repo.GetArtifactByFingerprint(ctx, in.PackID, in.PackVersionID, "zip", fingerprint); err == nil {
		if old.Status != "ready" {
			return BuildResult{}, ErrArtifactMissing
		}
		if err := verifyArtifactFile(old.Path, old.SHA256, old.SizeBytes); err != nil {
			return BuildResult{}, ErrArtifactMissing
		}
		return BuildResult{Artifact: artifactDTO(old), SourceFingerprint: fingerprint}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return BuildResult{}, err
	}

	// Input provenance is written before disk I/O. This is safe because no
	// artifact row exists until the rename succeeds, and it makes a failed run
	// diagnosable after restart.
	if err := a.repo.RecordBuildInputs(ctx, in.PackID, in.PackVersionID, inputRows, checks); err != nil {
		return BuildResult{}, err
	}
	fileName := safeVersionName(version.Version) + "-" + fingerprint[:16] + ".zip"
	tmp, err := os.CreateTemp(dir.AbsolutePath, ".mpackstation-build-*.tmp")
	if err != nil {
		return BuildResult{}, ErrExportDirNotAllowed
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := writeDeterministicZip(tmp, normalized); err != nil {
		return BuildResult{}, ErrInvalidBuildInput
	}
	if err := tmp.Sync(); err != nil {
		return BuildResult{}, ErrExportDirNotAllowed
	}
	if err := tmp.Close(); err != nil {
		return BuildResult{}, ErrExportDirNotAllowed
	}
	if err := verifyZip(tmpName, normalized); err != nil {
		return BuildResult{}, ErrInvalidBuildInput
	}
	finalPath := filepath.Join(dir.AbsolutePath, fileName)
	if err := os.Rename(tmpName, finalPath); err != nil {
		// Another builder may have committed the same deterministic artifact.
		if existing, eerr := a.repo.GetArtifactByFingerprint(ctx, in.PackID, in.PackVersionID, "zip", fingerprint); eerr == nil {
			if verr := verifyArtifactFile(existing.Path, existing.SHA256, existing.SizeBytes); verr == nil {
				return BuildResult{Artifact: artifactDTO(existing), SourceFingerprint: fingerprint}, nil
			}
		}
		return BuildResult{}, ErrExportDirNotAllowed
	}
	committed = true
	sha, size, err := hashFile(finalPath)
	if err != nil {
		_ = os.Remove(finalPath)
		return BuildResult{}, ErrExportDirNotAllowed
	}
	row := store.ArtifactRecord{ID: newID("artifact"), PackID: in.PackID, PackVersionID: in.PackVersionID, TaskID: in.TaskID, Path: finalPath, FileName: fileName, SHA256: sha, SizeBytes: size, SourceFingerprint: fingerprint, Status: "ready", Kind: "zip", CreatedAt: a.nowMillis()}
	registered, err := a.repo.RegisterArtifact(ctx, row)
	if err != nil {
		_ = os.Remove(finalPath)
		return BuildResult{}, err
	}
	if registered.ID != row.ID {
		// A concurrent builder won the idempotency race. Its file is canonical;
		// this duplicate file is safe to remove because it is not registered.
		if registered.Path != finalPath {
			_ = os.Remove(finalPath)
		}
	}
	return BuildResult{Artifact: artifactDTO(registered), SourceFingerprint: fingerprint}, nil
}

func (a *API) nowMillis() int64 {
	if a != nil && a.now != nil {
		return a.now().UnixMilli()
	}
	return time.Now().UnixMilli()
}

func artifactDTO(a store.ArtifactRecord) Artifact {
	return Artifact{ID: a.ID, PackID: a.PackID, PackVersionID: a.PackVersionID, FileName: a.FileName, Path: a.Path, SHA256: a.SHA256, SizeBytes: a.SizeBytes, SourceFingerprint: a.SourceFingerprint, Status: a.Status, Kind: a.Kind, CreatedAt: iso(a.CreatedAt)}
}

type normalizedBuildFile struct {
	Path string
	Data []byte
}

func validateBuildInput(in BuildInput) error {
	if strings.TrimSpace(in.PackID) == "" || strings.TrimSpace(in.PackVersionID) == "" || strings.TrimSpace(in.ExportDirName) == "" || len(in.Files) == 0 {
		return ErrInvalidBuildInput
	}
	seen := make(map[string]struct{}, len(in.Files))
	for _, file := range in.Files {
		path, err := safeArchivePath(file.Path)
		if err != nil || len(file.Content) > 512<<20 {
			return ErrInvalidBuildInput
		}
		if _, ok := seen[path]; ok {
			return ErrInvalidBuildInput
		}
		seen[path] = struct{}{}
	}
	return nil
}

func buildManifest(in BuildInput, now int64) ([]normalizedBuildFile, string, []store.PackVersionInputRecord, []store.DeliveryCheckRecord, error) {
	files := make([]normalizedBuildFile, 0, len(in.Files))
	for _, f := range in.Files {
		path, err := safeArchivePath(f.Path)
		if err != nil {
			return nil, "", nil, nil, ErrInvalidBuildInput
		}
		data := append([]byte(nil), f.Content...)
		files = append(files, normalizedBuildFile{Path: path, Data: data})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	lock, err := normalizeSnapshot(in.LockSnapshot)
	if err != nil {
		return nil, "", nil, nil, err
	}
	content, err := normalizeSnapshot(in.ContentSnapshot)
	if err != nil {
		return nil, "", nil, nil, err
	}
	quest, err := normalizeSnapshot(in.QuestSnapshot)
	if err != nil {
		return nil, "", nil, nil, err
	}
	config, err := normalizeSnapshot(in.BuildConfig)
	if err != nil {
		return nil, "", nil, nil, err
	}
	type fileFingerprint struct {
		Path string `json:"path"`
		Hash string `json:"sha256"`
		Size int    `json:"size"`
	}
	manifest := struct {
		Files   []fileFingerprint `json:"files"`
		Lock    json.RawMessage   `json:"lock"`
		Content json.RawMessage   `json:"content"`
		Quest   json.RawMessage   `json:"quest"`
		Config  json.RawMessage   `json:"config"`
	}{Lock: lock, Content: content, Quest: quest, Config: config}
	manifest.Files = make([]fileFingerprint, 0, len(files))
	for _, file := range files {
		hash := sha256.Sum256(file.Data)
		manifest.Files = append(manifest.Files, fileFingerprint{Path: file.Path, Hash: hex.EncodeToString(hash[:]), Size: len(file.Data)})
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", nil, nil, ErrInvalidBuildInput
	}
	fp := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(fp[:])
	inputRows := []store.PackVersionInputRecord{
		{ID: newID("input"), Kind: "lock", SourceID: "snapshot", InputHash: hashJSON(lock), Payload: string(lock), CreatedAt: now},
		{ID: newID("input"), Kind: "content", SourceID: "snapshot", InputHash: hashJSON(content), Payload: string(content), CreatedAt: now},
		{ID: newID("input"), Kind: "quest", SourceID: "snapshot", InputHash: hashJSON(quest), Payload: string(quest), CreatedAt: now},
		{ID: newID("input"), Kind: "build_config", SourceID: "snapshot", InputHash: hashJSON(config), Payload: string(config), CreatedAt: now},
	}
	checks := make([]store.DeliveryCheckRecord, 0, len(in.Checks))
	for _, check := range in.Checks {
		if !validDeliveryCheck(check.Kind, check.Status) {
			return nil, "", nil, nil, ErrInvalidBuildInput
		}
		detail, err := normalizeSnapshot(json.RawMessage(check.Detail))
		if err != nil {
			return nil, "", nil, nil, err
		}
		checks = append(checks, store.DeliveryCheckRecord{ID: newID("delivery"), PackID: in.PackID, PackVersionID: in.PackVersionID, Kind: check.Kind, Status: check.Status, Detail: string(detail), InputFingerprint: fingerprint, RunID: fingerprint[:16], CheckedAt: now})
	}
	return files, fingerprint, inputRows, checks, nil
}

func validDeliveryCheck(kind, status string) bool {
	switch kind {
	case "dependency", "conflict", "missing_file", "content", "version", "quest":
	default:
		return false
	}
	return status == "passed" || status == "warning" || status == "blocked"
}

func normalizeSnapshot(raw json.RawMessage) ([]byte, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []byte(`{}`), nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, ErrInvalidBuildInput
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, ErrInvalidBuildInput
	}
	return b, nil
}

func hashJSON(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func safeArchivePath(raw string) (string, error) {
	if raw == "" || strings.IndexByte(raw, 0) >= 0 {
		return "", ErrInvalidBuildInput
	}
	v := strings.ReplaceAll(raw, `\`, "/")
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || (len(v) >= 2 && v[1] == ':') {
		return "", ErrInvalidBuildInput
	}
	v = pathCleanSlash(v)
	if v == "." || v == "" || v == ".." || strings.HasPrefix(v, "../") {
		return "", ErrInvalidBuildInput
	}
	return v, nil
}

func pathCleanSlash(v string) string {
	parts := strings.Split(v, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			} else {
				out = append(out, "..")
			}
		default:
			out = append(out, part)
		}
	}
	return strings.Join(out, "/")
}

func safeVersionName(version string) string {
	var b strings.Builder
	for _, r := range version {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "version"
	}
	return b.String()
}

func canonicalExportDir(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", ErrExportDirNotAllowed
	}
	abs, err := filepath.Abs(raw)
	if err != nil || !filepath.IsAbs(abs) {
		return "", ErrExportDirNotAllowed
	}
	abs = filepath.Clean(abs)
	if filepath.Dir(abs) == abs || (filepath.VolumeName(abs) != "" && filepath.Clean(abs) == filepath.VolumeName(abs)+string(filepath.Separator)) {
		return "", ErrExportDirNotAllowed
	}
	return abs, nil
}

func verifyExportDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ErrExportDirNotAllowed
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(dir) {
		return ErrExportDirNotAllowed
	}
	if info, err := os.Stat(filepath.Join(dir, ".mpackstation-export")); err != nil || !info.Mode().IsRegular() {
		return ErrExportDirNotAllowed
	}
	return nil
}

func verifyRegisteredExportDir(dir string) error { return verifyExportDir(dir) }

func writeDeterministicZip(dst io.Writer, files []normalizedBuildFile) error {
	zw := zip.NewWriter(dst)
	for _, file := range files {
		h := &zip.FileHeader{Name: file.Path, Method: zip.Deflate}
		// Set only the legacy DOS fields. FileHeader.SetModTime also emits an
		// extended timestamp extra field, which would make metadata differ across
		// ZIP writers. 0x21 is 1980-01-01 in DOS date encoding.
		h.ModifiedDate = 0x21
		h.ModifiedTime = 0
		h.Extra = nil
		w, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := w.Write(file.Data); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func verifyZip(name string, expected []normalizedBuildFile) error {
	r, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer r.Close()
	if len(r.File) != len(expected) {
		return ErrInvalidBuildInput
	}
	for i, file := range r.File {
		if file.Name != expected[i].Path || file.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidBuildInput
		}
		reader, err := file.Open()
		if err != nil {
			return ErrInvalidBuildInput
		}
		body, readErr := io.ReadAll(io.LimitReader(reader, 512<<20))
		_ = reader.Close()
		if readErr != nil || string(body) != string(expected[i].Data) {
			return ErrInvalidBuildInput
		}
	}
	return nil
}

func hashFile(name string) (string, int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func verifyArtifactFile(name, expectedHash string, expectedSize int64) error {
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != expectedSize {
		return ErrArtifactMissing
	}
	hash, size, err := hashFile(name)
	if err != nil || size != expectedSize || !strings.EqualFold(hash, expectedHash) {
		return ErrArtifactMissing
	}
	return nil
}
