package httpapi

// P7 HTTP acceptance tests are deliberately written against the v7 public
// contract.  They keep the build/release invariants visible to a reviewer and
// are allowed to be red while the corresponding handlers are still being
// implemented.  Service tests alone do not prove that a browser can reach the
// contract or that its failures are stable.

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mpackstation/internal/provider"
	"mpackstation/internal/service"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

// TestP7HTTPContractRoutesMatchV7 is the route-level gate for architecture
// 6.6.  A route may return a domain error for the missing fixture, but it may
// not be silently absent or be reduced to ServeMux's generic 404.
func TestP7HTTPContractRoutesMatchV7(t *testing.T) {
	handler, db, _, _ := p7HTTPFixture(t)
	defer db.Close()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/packs/p7-missing/delivery-checks"},
		{http.MethodPost, "/api/packs/p7-missing/delivery-checks/run"},
		{http.MethodGet, "/api/packs/p7-missing/versions"},
		{http.MethodPost, "/api/packs/p7-missing/versions"},
		{http.MethodPost, "/api/packs/p7-missing/build"},
		{http.MethodGet, "/api/packs/p7-missing/artifacts"},
		{http.MethodGet, "/api/packs/p7-missing/artifacts/a-missing/download"},
		{http.MethodGet, "/api/packs/p7-missing/releases"},
		{http.MethodPost, "/api/packs/p7-missing/publish/curseforge"},
		{http.MethodPost, "/api/packs/p7-missing/publish/modrinth"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := p7Request(t, route.method, route.path, bytes.NewBufferString(`{}`), route.method != http.MethodGet)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code == http.StatusMethodNotAllowed || (res.Code == http.StatusNotFound && p7ErrorCode(res.Body.Bytes()) == "not_found") {
				t.Fatalf("P7-HTTP-001 missing v7 route: %s %s status=%d body=%s", route.method, route.path, res.Code, res.Body.String())
			}
		})
	}
}

// TestP7DeliveryChecksBlockAndAllowBuild proves that a blocked readiness gate
// stops disk output, while a passed gate permits a registered build.  It also
// proves the check is persisted against the exact pack version and fingerprint.
func TestP7DeliveryChecksBlockAndAllowBuild(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "p7-delivery", export); err != nil {
		t.Fatal(err)
	}
	base := service.BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "p7-delivery",
		Files: []service.BuildFile{{Path: "manifest.json", Content: []byte(`{"format":1}`)}},
	}
	blocked := base
	blocked.Checks = []service.DeliveryCheck{{Kind: "conflict", Status: "blocked", Detail: `{}`}}
	if _, err := app.BuildPack(context.Background(), blocked); !errors.Is(err, service.ErrDeliveryBlocked) {
		t.Fatalf("P7-BUILD-001 blocked build error=%v, want ErrDeliveryBlocked", err)
	}
	if n := p7Count(t, db, `SELECT COUNT(*) FROM artifacts WHERE pack_id=?`, packID); n != 0 {
		t.Fatalf("P7-BUILD-001 blocked build registered %d artifacts", n)
	}

	passed := base
	passed.Checks = []service.DeliveryCheck{{Kind: "conflict", Status: "passed", Detail: `{}`}, {Kind: "version", Status: "warning", Detail: `{"reason":"draft"}`}}
	built, err := app.BuildPack(context.Background(), passed)
	if err != nil {
		t.Fatalf("P7-BUILD-002 passed build: %v", err)
	}
	if built.Artifact.PackID != packID || built.Artifact.PackVersionID != versionID || built.Artifact.Status != "ready" {
		t.Fatalf("P7-BUILD-002 artifact provenance=%#v", built.Artifact)
	}
	var status, fp, recordedVersion string
	if err := db.QueryRow(`SELECT status,input_fingerprint,pack_version_id FROM delivery_checks WHERE pack_id=? AND kind='conflict'`, packID).Scan(&status, &fp, &recordedVersion); err != nil {
		t.Fatal(err)
	}
	if status != "passed" || fp != built.SourceFingerprint || recordedVersion != versionID {
		t.Fatalf("P7-BUILD-002 persisted check status=%q fp=%q version=%q", status, fp, recordedVersion)
	}
}

// TestP7BuildBindsPackVersionAndCapturesSource proves a version from another
// pack cannot be used and that each source snapshot is retained for audit.
func TestP7BuildBindsPackVersionAndCapturesSource(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	otherPack, err := app.CreatePack(context.Background(), service.CreatePackInput{Name: "P7 other", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p7-other")
	if err != nil {
		t.Fatal(err)
	}
	var otherVersion string
	if err := db.QueryRow(`SELECT pack_version_id FROM pack_current_version WHERE pack_id=?`, otherPack.ID).Scan(&otherVersion); err != nil {
		t.Fatal(err)
	}
	if err := app.RegisterExportDirectory(context.Background(), "p7-source", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	_, err = app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: otherVersion, ExportDirName: "p7-source", Files: []service.BuildFile{{Path: "a.txt", Content: []byte("x")}}})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("P7-BUILD-003 cross-pack version error=%v, want not found", err)
	}
	built, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-source", Files: []service.BuildFile{{Path: "a.txt", Content: []byte("x")}}, LockSnapshot: []byte(`{"lock":1}`), ContentSnapshot: []byte(`{"content":1}`), QuestSnapshot: []byte(`{"quest":1}`), BuildConfig: []byte(`{"config":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if n := p7Count(t, db, `SELECT COUNT(*) FROM pack_version_inputs WHERE pack_version_id=?`, versionID); n != 4 {
		t.Fatalf("P7-BUILD-003 captured input rows=%d, want 4", n)
	}
	if p7Count(t, db, `SELECT COUNT(*) FROM pack_version_inputs WHERE pack_version_id=?`, otherVersion) != 0 {
		t.Fatal("P7-BUILD-003 cross-pack attempt wrote source inputs")
	}
	if built.Artifact.PackVersionID != versionID {
		t.Fatalf("P7-BUILD-003 artifact version=%q, want %q", built.Artifact.PackVersionID, versionID)
	}
}

// TestP7BuildIsStableAndZipMetadataIsReproducible checks sorted manifest
// entries, fixed ZIP timestamps, byte-for-byte repeated builds, and one
// artifact row per source fingerprint.
func TestP7BuildIsStableAndZipMetadataIsReproducible(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	if err := app.RegisterExportDirectory(context.Background(), "p7-repro", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	firstInput := service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-repro", Files: []service.BuildFile{{Path: "z.txt", Content: []byte("z")}, {Path: "a.txt", Content: []byte("a")}}, BuildConfig: []byte(`{"format":1}`)}
	first, err := app.BuildPack(context.Background(), firstInput)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := firstInput
	secondInput.Files = []service.BuildFile{{Path: "a.txt", Content: []byte("a")}, {Path: "z.txt", Content: []byte("z")}}
	second, err := app.BuildPack(context.Background(), secondInput)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifact.ID != second.Artifact.ID || first.SourceFingerprint != second.SourceFingerprint || first.Artifact.SHA256 != second.Artifact.SHA256 || !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("P7-BUILD-004 repeated build changed identity or bytes: first=%#v second=%#v", first, second)
	}
	if n := p7Count(t, db, `SELECT COUNT(*) FROM artifacts WHERE pack_id=? AND pack_version_id=?`, packID, versionID); n != 1 {
		t.Fatalf("P7-BUILD-004 artifact rows=%d, want 1", n)
	}
	r, err := zip.OpenReader(first.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got := make([]string, 0, len(r.File))
	for _, f := range r.File {
		got = append(got, f.Name)
		if f.ModTime().Year() != 1980 || len(f.Extra) != 0 {
			t.Errorf("P7-BUILD-004 ZIP metadata for %q is not fixed: modtime=%v extra=%v", f.Name, f.ModTime(), f.Extra)
		}
	}
	if want := []string{"a.txt", "z.txt"}; !equalStrings(got, want) {
		t.Fatalf("P7-BUILD-004 ZIP order=%v, want %v", got, want)
	}
}

// TestP7ArtifactHashAndDownloadContract verifies the registered digest and
// the public download route. The HTTP route is intentionally checked in a
// separate step so a missing route is a clear contract failure.
func TestP7ArtifactHashAndDownloadContract(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "p7-download", export); err != nil {
		t.Fatal(err)
	}
	built, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-download", Files: []service.BuildFile{{Path: "manifest.json", Content: []byte(`{}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(built.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if got := hex.EncodeToString(digest[:]); got != built.Artifact.SHA256 || int64(len(body)) != built.Artifact.SizeBytes {
		t.Fatalf("P7-ARTIFACT-001 digest=%q/%d, registered=%q/%d", hex.EncodeToString(digest[:]), len(body), built.Artifact.SHA256, built.Artifact.SizeBytes)
	}
	handler := NewRouter(db, "test", "test")
	res := p7Do(t, handler, http.MethodGet, "/api/packs/"+packID+"/artifacts/"+built.Artifact.ID+"/download", nil, false)
	if res.Code != http.StatusOK {
		t.Fatalf("P7-ARTIFACT-001 download status=%d body=%s", res.Code, res.Body.String())
	}
	downloaded, _ := io.ReadAll(res.Body)
	if !bytes.Equal(downloaded, body) || res.Header().Get("Content-Disposition") == "" {
		t.Fatalf("P7-ARTIFACT-001 downloaded bytes/header mismatch")
	}
}

// TestP7ExportDirectorySafety prevents root, unapproved, and symlinked
// destinations from becoming build output locations.
func TestP7ExportDirectorySafety(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	if err := app.RegisterExportDirectory(context.Background(), "p7-unapproved-check", filepath.Join(t.TempDir(), "not-created")); err != nil {
		t.Fatal(err)
	}
	if _, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "not-registered", Files: []service.BuildFile{{Path: "a.txt", Content: []byte("a")}}}); !errors.Is(err, service.ErrExportDirNotAllowed) {
		t.Fatalf("P7-FILE-001 unapproved directory error=%v", err)
	}
	root := filepath.VolumeName(t.TempDir()) + string(os.PathSeparator)
	if err := app.RegisterExportDirectory(context.Background(), "p7-root", root); !errors.Is(err, service.ErrInvalidBuildInput) {
		t.Fatalf("P7-FILE-001 root registration error=%v, want invalid input", err)
	}
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "link")
	if err := os.Symlink(filepath.Dir(linkParent), link); err != nil {
		t.Logf("P7-FILE-001 symlink subcase N/A on this Windows token: %v", err)
	} else if err := app.RegisterExportDirectory(context.Background(), "p7-link", link); !errors.Is(err, service.ErrExportDirNotAllowed) {
		t.Fatalf("P7-FILE-001 symlink registration error=%v", err)
	}
	if _, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-unapproved-check", Files: []service.BuildFile{{Path: "../escape", Content: []byte("x")}}}); !errors.Is(err, service.ErrInvalidBuildInput) {
		t.Fatalf("P7-FILE-001 archive traversal error=%v", err)
	}
}

// TestP7CurseForgeAndModrinthPublishTasksAndPolling exercises both normalized
// provider DTO paths, durable publishing state, and remote-state polling.
func TestP7CurseForgeAndModrinthPublishTasksAndPolling(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	if err := app.RegisterExportDirectory(context.Background(), "p7-publish", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	built, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-publish", Files: []service.BuildFile{{Path: "manifest.json", Content: []byte(`{}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []provider.Name{provider.CurseForge, provider.Modrinth} {
		t.Run(string(name), func(t *testing.T) {
			adapter := &p7CountingAdapter{name: name, publishResult: provider.PublishResult{RemoteID: string(name) + "-remote", Status: "accepted"}, statusResult: provider.RemoteStatus{ID: string(name) + "-remote", Status: "succeeded"}}
			p7 := service.NewP7Service(db)
			p7.SetProviderRegistry(provider.NewRegistry(adapter))
			rel, err := p7.PublishPack(context.Background(), service.PublishInput{PackID: packID, PackVersionID: versionID, Provider: string(name), ArtifactID: built.Artifact.ID, IdempotencyKey: "p7-" + string(name), ProjectID: "project", VersionID: "version"})
			if err != nil || rel.Status != "publishing" {
				t.Fatalf("P7-PUBLISH-001 initial release=%#v err=%v", rel, err)
			}
			polled, err := p7.PollRelease(context.Background(), rel.ID)
			if err != nil || polled.Status != "succeeded" || adapter.publishCalls() != 1 || adapter.statusCalls() != 1 {
				t.Fatalf("P7-PUBLISH-001 polled=%#v err=%v publish=%d status=%d", polled, err, adapter.publishCalls(), adapter.statusCalls())
			}
		})
	}
}

// TestP7PublishFailureDuplicateAndExplicitRetry ensures a failed
// non-idempotent operation is not replayed by the same request and only an
// explicit retry can issue another provider call.
func TestP7PublishFailureDuplicateAndExplicitRetry(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	if err := app.RegisterExportDirectory(context.Background(), "p7-retry", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	built, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-retry", Files: []service.BuildFile{{Path: "manifest.json", Content: []byte(`{}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &p7CountingAdapter{name: provider.CurseForge, publishErr: provider.ErrUnavailable}
	p7 := service.NewP7Service(db)
	p7.SetProviderRegistry(provider.NewRegistry(adapter))
	in := service.PublishInput{PackID: packID, PackVersionID: versionID, Provider: string(provider.CurseForge), ArtifactID: built.Artifact.ID, IdempotencyKey: "p7-failed-once", ProjectID: "p", VersionID: "v"}
	failed, err := p7.PublishPack(context.Background(), in)
	if !errors.Is(err, service.ErrPublishFailed) || failed.Status != "failed" || adapter.publishCalls() != 1 {
		t.Fatalf("P7-PUBLISH-002 first failed=%#v err=%v calls=%d", failed, err, adapter.publishCalls())
	}
	repeated, err := p7.PublishPack(context.Background(), in)
	if err != nil || repeated.ID != failed.ID || repeated.Status != "failed" || adapter.publishCalls() != 1 {
		t.Fatalf("P7-PUBLISH-002 duplicate=%#v err=%v calls=%d; duplicate must not auto-retry", repeated, err, adapter.publishCalls())
	}
	if _, err := p7.RetryPublish(context.Background(), failed.ID, "p", "v"); !errors.Is(err, service.ErrPublishFailed) || adapter.publishCalls() != 2 {
		t.Fatalf("P7-PUBLISH-002 explicit retry err=%v calls=%d", err, adapter.publishCalls())
	}
}

// TestP7PollingFailurePreservesPublishingState proves a status transport
// failure is observable without destroying the last durable remote state.
func TestP7PollingFailurePreservesPublishingState(t *testing.T) {
	db, app, packID, versionID := newP7ServiceFixture(t)
	defer db.Close()
	if err := app.RegisterExportDirectory(context.Background(), "p7-poll", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	built, err := app.BuildPack(context.Background(), service.BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "p7-poll", Files: []service.BuildFile{{Path: "a", Content: []byte("a")}}})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &p7CountingAdapter{name: provider.Modrinth, publishResult: provider.PublishResult{RemoteID: "mr-1", Status: "accepted"}, statusErr: provider.ErrUnavailable}
	p7 := service.NewP7Service(db)
	p7.SetProviderRegistry(provider.NewRegistry(adapter))
	rel, err := p7.PublishPack(context.Background(), service.PublishInput{PackID: packID, PackVersionID: versionID, Provider: string(provider.Modrinth), ArtifactID: built.Artifact.ID, IdempotencyKey: "p7-poll-failure", ProjectID: "p", VersionID: "v"})
	if err != nil || rel.Status != "publishing" {
		t.Fatalf("P7-PUBLISH-003 initial=%#v err=%v", rel, err)
	}
	unchanged, err := p7.PollRelease(context.Background(), rel.ID)
	if !errors.Is(err, service.ErrProviderStatusUnavailable) || unchanged.Status != "publishing" {
		t.Fatalf("P7-PUBLISH-003 unavailable poll=%#v err=%v", unchanged, err)
	}
	adapter.statusErr = nil
	adapter.statusResult = provider.RemoteStatus{ID: "mr-1", Status: "succeeded"}
	finished, err := p7.PollRelease(context.Background(), rel.ID)
	if err != nil || finished.Status != "succeeded" {
		t.Fatalf("P7-PUBLISH-003 recovery poll=%#v err=%v", finished, err)
	}
}

// TestP7TaskDuplicateCancelAndRecovery covers the task-side publication
// lifecycle: canonical duplicate submission, cancellation, and restart
// recovery of an expired lease.
func TestP7TaskDuplicateCancelAndRecovery(t *testing.T) {
	db, _, _, _ := newP7ServiceFixture(t)
	defer db.Close()
	q, err := task.NewQueue(db, task.WithLeaseTTL(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	first, duplicate, err := q.Submit(context.Background(), task.SubmitRequest{Kind: task.KindPublish, Title: "P7 publish", Payload: []byte(`{"artifact":"a","provider":"cf"}`), IdempotencyKey: "p7-task-dup"})
	if err != nil || duplicate {
		t.Fatalf("P7-TASK-001 first submit task=%#v duplicate=%v err=%v", first, duplicate, err)
	}
	second, duplicate, err := q.Submit(context.Background(), task.SubmitRequest{Kind: task.KindPublish, Title: "P7 publish", Payload: []byte(`{"provider":"cf","artifact":"a"}`), IdempotencyKey: "p7-task-dup"})
	if err != nil || !duplicate || second.ID != first.ID {
		t.Fatalf("P7-TASK-001 equivalent duplicate task=%#v duplicate=%v err=%v", second, duplicate, err)
	}
	if _, _, err := q.Submit(context.Background(), task.SubmitRequest{Kind: task.KindPublish, Title: "P7 publish", Payload: []byte(`{"artifact":"different"}`), IdempotencyKey: "p7-task-dup"}); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("P7-TASK-001 changed duplicate error=%v", err)
	}
	if err := q.Cancel(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	canceled, err := q.Get(context.Background(), first.ID)
	if err != nil || canceled.Status != task.StatusCanceled {
		t.Fatalf("P7-TASK-002 canceled=%#v err=%v", canceled, err)
	}
	if err := q.Cancel(context.Background(), first.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("P7-TASK-002 duplicate cancel error=%v", err)
	}

	recoverable, _, err := q.Submit(context.Background(), task.SubmitRequest{Kind: task.KindPublish, Title: "P7 recovery", Payload: []byte(`{"artifact":"recover"}`), IdempotencyKey: "p7-task-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	leased, err := q.Lease(context.Background(), "p7-worker-a")
	if err != nil || leased.ID != recoverable.ID {
		t.Fatalf("P7-TASK-003 lease=%#v err=%v", leased, err)
	}
	if _, err := db.Exec(`UPDATE tasks SET lease_expires_at=? WHERE id=?`, time.Now().Add(-time.Second).UnixMilli(), recoverable.ID); err != nil {
		t.Fatal(err)
	}
	count, err := q.Recover(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("P7-TASK-003 recovered count=%d err=%v", count, err)
	}
	recovered, err := q.Get(context.Background(), recoverable.ID)
	if err != nil || recovered.Status != task.StatusQueued || recovered.RecoverCount != 1 || recovered.LeaseOwner != "" {
		t.Fatalf("P7-TASK-003 recovered=%#v err=%v", recovered, err)
	}
}

// TestP7HTTPErrorEnvelopeIsStable checks request-id propagation and stable
// errors for authentication, host validation, and a malformed build request.
func TestP7HTTPErrorEnvelopeIsStable(t *testing.T) {
	handler, db, _, _ := p7HTTPFixture(t)
	defer db.Close()

	unauth := httptest.NewRequest(http.MethodPost, "/api/export-dirs", bytes.NewBufferString(`{}`))
	unauth.Host = "localhost"
	unauth.Header.Set("Origin", "http://localhost")
	unauth.Header.Set("X-Request-ID", "p7-unauth")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, unauth)
	p7RequireError(t, res, http.StatusUnauthorized, "unauthorized", "p7-unauth")

	badHost := p7Request(t, http.MethodPost, "/api/export-dirs", bytes.NewBufferString(`{}`), true)
	badHost.Host = "evil.example"
	badHost.Header.Set("X-Request-ID", "p7-host")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, badHost)
	p7RequireError(t, res, http.StatusBadRequest, "invalid_host", "p7-host")

	res = p7Do(t, handler, http.MethodPost, "/api/packs/p7-missing/versions/missing/build", bytes.NewBufferString(`{"files":[{"path":"a","content":"%%%"}]}`), true)
	p7RequireError(t, res, http.StatusBadRequest, "invalid_argument", "p7-http-build")
}

type p7CountingAdapter struct {
	name          provider.Name
	mu            sync.Mutex
	publishCount  int
	statusCount   int
	publishResult provider.PublishResult
	publishErr    error
	statusResult  provider.RemoteStatus
	statusErr     error
}

func (a *p7CountingAdapter) Name() provider.Name { return a.name }
func (a *p7CountingAdapter) Search(context.Context, provider.SearchRequest) (provider.SearchResult, error) {
	return provider.SearchResult{}, provider.ErrNotFound
}
func (a *p7CountingAdapter) Project(context.Context, string) (provider.Project, error) {
	return provider.Project{}, provider.ErrNotFound
}
func (a *p7CountingAdapter) Versions(context.Context, string) ([]provider.Version, error) {
	return nil, provider.ErrNotFound
}
func (a *p7CountingAdapter) Metadata(context.Context, string, string) (provider.Metadata, error) {
	return provider.Metadata{}, provider.ErrNotFound
}
func (a *p7CountingAdapter) Download(context.Context, provider.DownloadRequest) (provider.DownloadResult, error) {
	return provider.DownloadResult{}, provider.ErrNotFound
}
func (a *p7CountingAdapter) Publish(context.Context, provider.PublishRequest) (provider.PublishResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.publishCount++
	return a.publishResult, a.publishErr
}
func (a *p7CountingAdapter) RemoteStatus(context.Context, string) (provider.RemoteStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statusCount++
	return a.statusResult, a.statusErr
}
func (a *p7CountingAdapter) publishCalls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.publishCount
}
func (a *p7CountingAdapter) statusCalls() int { a.mu.Lock(); defer a.mu.Unlock(); return a.statusCount }

func p7HTTPFixture(t *testing.T) (http.Handler, *sql.DB, string, string) {
	t.Helper()
	db, _, packID, versionID := newP7ServiceFixture(t)
	return NewRouter(db, "test", "test"), db, packID, versionID
}

func newP7ServiceFixture(t *testing.T) (*sql.DB, *service.API, string, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "p7-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := service.New(db)
	pack, err := app.CreatePack(context.Background(), service.CreatePackInput{Name: "P7 HTTP", MCVersion: "1.20.1", Loader: "fabric", LoaderVersion: "0.15"}, "p7-fixture")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	var versionID string
	if err := db.QueryRow(`SELECT pack_version_id FROM pack_current_version WHERE pack_id=?`, pack.ID).Scan(&versionID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, app, pack.ID, versionID
}

func p7Request(t *testing.T, method, path string, body io.Reader, write bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.Host = "localhost"
	req.Header.Set("Origin", "http://localhost")
	req.Header.Set("X-Request-ID", "p7-http-build")
	if write {
		req.Header.Set("X-MPack-Token", "test")
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func p7Do(t *testing.T, handler http.Handler, method, path string, body io.Reader, write bool) *httptest.ResponseRecorder {
	t.Helper()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, p7Request(t, method, path, body, write))
	return res
}

func p7RequireError(t *testing.T, res *httptest.ResponseRecorder, status int, code, requestID string) {
	t.Helper()
	if res.Code != status {
		t.Fatalf("P7-HTTP-002 status=%d body=%s, want %d/%s", res.Code, res.Body.String(), status, code)
	}
	var envelope struct {
		Error struct {
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			RequestID string         `json:"request_id"`
			Details   map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("P7-HTTP-002 invalid error envelope: %v", err)
	}
	if envelope.Error.Code != code || envelope.Error.Message == "" || envelope.Error.RequestID != requestID || res.Header().Get("X-Request-ID") != requestID {
		t.Fatalf("P7-HTTP-002 envelope=%#v header=%q", envelope.Error, res.Header().Get("X-Request-ID"))
	}
}

func p7ErrorCode(body []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	return envelope.Error.Code
}

func p7Count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
