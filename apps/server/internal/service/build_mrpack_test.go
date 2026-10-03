package service

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// seedPack writes the same identity graph that AddPackMod produces, so the
// builder reads the pack's real authority instead of caller-supplied bytes.
// localOnly=true 时不给平台发布文件与下载地址（等价于只有本机 jar 的模组）。
func seedPack(t *testing.T, db *sql.DB, packID string, localOnly bool) {
	t.Helper()
	now := time.Now().UnixMilli()
	mustExec(t, db, `INSERT INTO mods(mod_id,display_name,kind) VALUES('jei','Just Enough Items (JEI)','normal')`)
	mustExec(t, db, `INSERT INTO pack_mods(id,pack_id,source,project_id,version_id,display_name,file_name,status,required,added_at,updated_at,origin,mod_id)
		VALUES('pm-jei',?,'modrinth','u6dRKJwZ','v-1','Just Enough Items (JEI)','','installed',1,?,?,'manual','jei')`, packID, now, now)
	mustExec(t, db, `INSERT INTO mod_versions(id,mod_id,declared_version,created_at,status) VALUES('mv-1','jei','19.57.0.450',?,'ready')`, now)
	mustExec(t, db, `INSERT INTO pack_mod_selections(pack_id,id,pack_mod_id,mod_id,version_id,acquisition,requested_version,created_at,status)
		VALUES(?, 'sel-1','pm-jei','jei','mv-1',?,?,?,'ready')`, packID, acquisition(localOnly), "19.57.0.450", now)
	mustExec(t, db, `UPDATE pack_mods SET current_selection_id='sel-1' WHERE id='pm-jei'`)
	if localOnly {
		return
	}
	mustExec(t, db, `INSERT INTO platform_projects(id,platform,external_project_id,slug,display_name) VALUES('pp-1','modrinth','u6dRKJwZ','jei','JEI')`)
	mustExec(t, db, `INSERT INTO platform_releases(id,project_id,external_version_id,version_name,published_at) VALUES('pr-1','pp-1','v-1','19.57.0.450',?)`, now)
	mustExec(t, db, `INSERT INTO platform_release_files(id,release_id,file_key,file_name,download_url,expected_size,expected_hashes,verification_status)
		VALUES('prf-1','pr-1','f-1','jei-1.20.1-fabric-19.57.0.450.jar','https://cdn.modrinth.com/data/u6dRKJwZ/versions/xx/jei.jar',2293143,
		 '{"sha1":"853deece496debf2c04941db954fc881ee786d10","sha256":"bac3dc6ff6238cb74716cf6109c21dee480f0156d81a62911cd16c9fe91204aa","sha512":"7a5be8bb90df93861d74fd8e3092ccd36e82c166ad2686e85dbcdcfd5f2d4235950a82f887533029a0b3e3186b5841ad02e326e6358f21583be3dc7b3d66bffd"}','verified')`)
	mustExec(t, db, `INSERT INTO selection_platform_pins(pack_id,selection_id,role,release_file_id) VALUES(?,'sel-1','primary','prf-1')`, packID)
}

func acquisition(localOnly bool) string {
	if localOnly {
		return "local"
	}
	return "modrinth"
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("seed %q: %v", firstLine(query), err)
	}
}

func firstLine(q string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(q), "\n")
	return line
}

func TestBuildAssemblesMrpackFromPackAuthority(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, false)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "mrpack", export); err != nil {
		t.Fatal(err)
	}

	// 关键断言：不传 files[] 也能构建 —— 装配来自库里的权威清单。
	result, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "mrpack",
		LockSnapshot: []byte(`{"mods":[{"id":"jei"}]}`),
	})
	if err != nil {
		t.Fatalf("assemble build: %v", err)
	}
	if result.Artifact.Kind != "mrpack" || !strings.HasSuffix(result.Artifact.FileName, ".mrpack") {
		t.Fatalf("artifact kind/name = %q/%q, want mrpack/.mrpack", result.Artifact.Kind, result.Artifact.FileName)
	}
	manifest := readManifestFromArtifact(t, result.Artifact.Path)
	// 字段名与真实产物对齐（formatVersion/game/versionId），见 build_mrpack.go 注释。
	if manifest.FormatVersion != 1 || manifest.Game != "minecraft" || manifest.Name == "" ||
		manifest.VersionID == "" {
		t.Fatalf("manifest header = %#v", manifest)
	}
	if len(manifest.Files) != 1 {
		t.Fatalf("manifest files = %d, want the one mod the pack actually has", len(manifest.Files))
	}
	file := manifest.Files[0]
	if file.Path != "mods/jei-1.20.1-fabric-19.57.0.450.jar" || file.FileSize != 2293143 ||
		file.Hashes.SHA1 != "853deece496debf2c04941db954fc881ee786d10" ||
		// sha512 缺失会让 Prism 一类严格读方拒绝安装，必须从 expected_hashes 透传到 manifest。
		len(file.Hashes.SHA512) != 128 ||
		len(file.Downloads) != 1 || !strings.HasPrefix(file.Downloads[0], "https://cdn.modrinth.com/") {
		t.Fatalf("manifest file = %#v", file)
	}
	// fixture 的包是 fabric 0.15：依赖必须带上加载器，安装方才知道装什么。
	if manifest.Dependencies["minecraft"] != "1.20.1" || manifest.Dependencies["fabric-loader"] != "0.15" {
		t.Fatalf("manifest dependencies = %#v", manifest.Dependencies)
	}
}

func TestBuildBlocksWhenModHasNoDownloadSource(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	seedPack(t, db, packID, true)
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "mrpack", export); err != nil {
		t.Fatal(err)
	}

	_, err := app.BuildPack(context.Background(), BuildInput{PackID: packID, PackVersionID: versionID, ExportDirName: "mrpack"})
	var de *DomainError
	if !errors.As(err, &de) || de.Status != 422 || de.Code != "build_mod_source_unresolved" {
		t.Fatalf("local-only mod build error = %v, want 422 build_mod_source_unresolved", err)
	}
	if !strings.Contains(de.Message, "Just Enough Items (JEI)") {
		t.Fatalf("message should name the offending mod: %q", de.Message)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE pack_id=?`, packID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("blocked build still registered %d artifact(s)", count)
	}
}

func TestBuildWithCallerSuppliedFilesStaysPlainZip(t *testing.T) {
	db, app, packID, versionID := newP7Fixture(t)
	defer db.Close()
	export := t.TempDir()
	if err := app.RegisterExportDirectory(context.Background(), "legacy", export); err != nil {
		t.Fatal(err)
	}
	result, err := app.BuildPack(context.Background(), BuildInput{
		PackID: packID, PackVersionID: versionID, ExportDirName: "legacy",
		Files: []BuildFile{{Path: "mods/z.jar", Content: []byte("z")}}, BuildConfig: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.Kind != "zip" || !strings.HasSuffix(result.Artifact.FileName, ".zip") {
		t.Fatalf("legacy path kind/name = %q/%q", result.Artifact.Kind, result.Artifact.FileName)
	}
}

func readManifestFromArtifact(t *testing.T, path string) mrpackManifest {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open artifact: %v", err)
	}
	defer zr.Close()
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Name != mrpackManifestPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		var manifest mrpackManifest
		if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		return manifest
	}
	body, _ := os.ReadFile(path)
	t.Fatalf("artifact has no %s (entries=%v, bytes=%d)", mrpackManifestPath, names, len(body))
	return mrpackManifest{}
}
