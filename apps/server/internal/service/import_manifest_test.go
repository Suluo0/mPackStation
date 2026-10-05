package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeZipEntry 把若干份内容打进一个 zip，用于构造导入清单夹具。
func writeZipEntry(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// mrpack 规范把 MC 版本放在 dependencies.minecraft（字符串），没有顶层
// minecraft 对象；这里锁定该字段的解析，防止再次回落到初始默认值。
func TestParseImportManifestModrinthIndex(t *testing.T) {
	index := `{
		"formatVersion": 1,
		"name": "Kindling Test Pack",
		"versionId": "0.1.0-test1",
		"dependencies": {"minecraft": "1.21.1", "neoforge": "21.1.247"},
		"files": [
			{
				"path": "mods/example-1.0.jar",
				"hashes": {"sha1": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				"fileSize": 1234,
				"downloads": ["https://cdn.modrinth.com/data/abc/versions/v1/example-1.0.jar"]
			},
			{
				"path": "resourcepacks/gui.zip",
				"hashes": {"sha1": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
				"fileSize": 4321,
				"downloads": ["https://cdn.modrinth.com/data/def/versions/v2/gui.zip"]
			}
		]
	}`
	path := filepath.Join(t.TempDir(), "pack.mrpack")
	writeZipEntry(t, path, map[string]string{"modrinth.index.json": index})

	m, err := parseImportManifest(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Kind != "modrinth_index" {
		t.Errorf("Kind = %q, want modrinth_index", m.Kind)
	}
	if m.Name != "Kindling Test Pack" {
		t.Errorf("Name = %q, want Kindling Test Pack", m.Name)
	}
	if m.MCVersion != "1.21.1" {
		t.Errorf("MCVersion = %q, want 1.21.1", m.MCVersion)
	}
	if m.Loader != "neoforge" || m.LoaderVersion != "21.1.247" {
		t.Errorf("Loader = %q/%q, want neoforge/21.1.247", m.Loader, m.LoaderVersion)
	}
	if len(m.Files) != 2 {
		t.Fatalf("Files = %d, want 2", len(m.Files))
	}
	if m.Files[0].Path != "mods/example-1.0.jar" || m.Files[0].Sha1 == "" || m.Files[0].Size != 1234 {
		t.Errorf("Files[0] = %+v, want mods/example-1.0.jar with sha1 and size 1234", m.Files[0])
	}
	if !m.Files[0].Required {
		t.Error("Files[0].Required = false, want true")
	}
	if m.Files[1].Path != "resourcepacks/gui.zip" {
		t.Errorf("Files[1].Path = %q, want resourcepacks/gui.zip", m.Files[1].Path)
	}
}

// CF 的 manifest.json 用 minecraft.version + minecraft.modLoaders，这条路径
// 与 mrpack 不同，一并锁定。
func TestParseImportManifestCurseForgeManifest(t *testing.T) {
	manifest := `{
		"minecraft": {
			"version": "1.20.1",
			"modLoaders": [{"id": "forge-47.2.0", "primary": true}]
		},
		"name": "CF Pack",
		"files": [
			{"projectID": 12345, "fileID": 67890, "required": true}
		]
	}`
	path := filepath.Join(t.TempDir(), "cf.zip")
	writeZipEntry(t, path, map[string]string{"manifest.json": manifest})

	m, err := parseImportManifest(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Kind != "curseforge_manifest" {
		t.Errorf("Kind = %q, want curseforge_manifest", m.Kind)
	}
	if m.MCVersion != "1.20.1" {
		t.Errorf("MCVersion = %q, want 1.20.1", m.MCVersion)
	}
	if m.Loader != "forge" || m.LoaderVersion != "47.2.0" {
		t.Errorf("Loader = %q/%q, want forge/47.2.0", m.Loader, m.LoaderVersion)
	}
	if len(m.Files) != 1 || m.Files[0].CFProject != 12345 || m.Files[0].CFFile != 67890 {
		t.Errorf("Files = %+v, want one CF entry 12345/67890", m.Files)
	}
}
