package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPAdapterSearchAndErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{{"ok", 200, nil}, {"not-found", 404, ErrNotFound}, {"rate", 429, ErrRateLimited}, {"server", 500, ErrUnavailable}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					_, _ = w.Write([]byte(`{"hits":[{"project_id":"p1","title":"Alpha"}],"pagination":{"total":1}}`))
				}
			}))
			defer s.Close()
			a, e := NewHTTPAdapter(Modrinth, s.URL, "", nil)
			if e != nil {
				t.Fatal(e)
			}
			got, e := a.Search(context.Background(), SearchRequest{Query: "a"})
			if tc.want != nil {
				if !errors.Is(e, tc.want) {
					t.Fatalf("error=%v want=%v", e, tc.want)
				}
				return
			}
			if e != nil || got.Total != 1 || got.Items[0].ID != "p1" {
				t.Fatalf("result=%#v error=%v", got, e)
			}
		})
	}
}
func TestHTTPAdapterTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { time.Sleep(100 * time.Millisecond) }))
	defer s.Close()
	a, e := NewHTTPAdapter(Modrinth, s.URL, "", &http.Client{Timeout: 10 * time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	_, e = a.Search(context.Background(), SearchRequest{})
	if !errors.Is(e, ErrUnavailable) {
		t.Fatalf("timeout=%v want unavailable", e)
	}
}

// TestHTTPAdapterMetadataAndDownloadJSON is the provider-boundary contract
// test for the normalized Modrinth-shaped DTOs. Service code must never need
// to know the response field names or dependency representation used upstream.
func TestHTTPAdapterMetadataAndDownloadJSON(t *testing.T) {
	const sha1 = "1111111111111111111111111111111111111111"
	var authCalls int
	var fixtureURL string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alpha.jar" {
			if r.Header.Get("Authorization") != "" {
				t.Error("API credentials leaked to asset download")
			}
			_, _ = w.Write([]byte("fixture-jar!"))
			return
		}
		if r.Header.Get("Authorization") != "secret" {
			t.Errorf("authorization=%q, want secret", r.Header.Get("Authorization"))
		} else {
			authCalls++
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/project/p1":
			_, _ = w.Write([]byte(`{"id":"p1","slug":"alpha","title":"Alpha Mod","description":"A test mod","icon_url":"https://cdn.example/icon.png","downloads":42}`))
		case "/v2/project/p1/version":
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"id":"v1","name":"Alpha 1.0","version_number":"1.0","game_versions":["1.20.1"],"loaders":["fabric"],"files":[{"id":"f1","name":"alpha.jar","downloadUrl":"%s/alpha.jar","sha1":"%s","sha256":"2222222222222222222222222222222222222222222222222222222222222222","size":12,"primary":true}],"dependencies":[{"project_id":"dep1","version_id":"dv1","dependency_type":"required","version_range":">=1.0"}]}]`, fixtureURL, sha1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	fixtureURL = s.URL
	a, err := NewHTTPAdapter(Modrinth, s.URL, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}

	project, err := a.Project(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "p1" || project.Name != "Alpha Mod" || project.Summary != "A test mod" || project.Downloads != 42 {
		t.Fatalf("project=%#v", project)
	}
	versions, err := a.Versions(context.Background(), "p1")
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions=%#v error=%v", versions, err)
	}
	if versions[0].ProjectID != "p1" || len(versions[0].Files) != 1 || len(versions[0].Dependencies) != 1 || versions[0].Dependencies[0].ProjectID != "dep1" {
		t.Fatalf("version=%#v", versions[0])
	}
	metadata, err := a.Metadata(context.Background(), "p1", "v1")
	if err != nil || metadata.Project.Name != "Alpha Mod" || len(metadata.Dependencies) != 1 {
		t.Fatalf("metadata=%#v error=%v", metadata, err)
	}
	download, err := a.Download(context.Background(), DownloadRequest{ProjectID: "p1", VersionID: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if download.FileName != "alpha.jar" || download.SHA1 != sha1 || download.Size != 12 || download.DownloadURL == "" {
		t.Fatalf("download=%#v", download)
	}
	if string(download.Content) != "fixture-jar!" {
		t.Fatalf("download content=%q", download.Content)
	}
	if authCalls < 5 {
		t.Fatalf("authenticated upstream calls=%d, want at least 5", authCalls)
	}
}

// TestHTTPAdapterCurseForgeEnvelopeNormalization prevents accidentally
// treating CurseForge's {data:...} envelope as if it were the Modrinth shape.
// It is intentionally strict because both adapters implement the same public
// provider contract.
func TestHTTPAdapterCurseForgeEnvelopeNormalization(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/mods/search":
			_, _ = w.Write([]byte(`{"data":[{"id":123,"name":"Alpha Mod","slug":"alpha","summary":"summary","logo":{"thumbnailUrl":"https://cdn.example/icon.png"},"downloadCount":1234}],"pagination":{"totalCount":1,"index":0,"pageSize":20}}`))
		case "/v1/mods/123":
			_, _ = w.Write([]byte(`{"data":{"id":123,"name":"Alpha Mod","slug":"alpha","summary":"summary","logo":{"thumbnailUrl":"https://cdn.example/icon.png"},"downloadCount":1234}}`))
		case "/v1/mods/123/files":
			_, _ = w.Write([]byte(`{"data":[{"id":456,"displayName":"Alpha 1.0","gameVersions":["1.20.1"],"loaders":["fabric"],"downloadUrl":"https://cdn.example/alpha.jar","fileLength":12,"hashes":[{"value":"1111111111111111111111111111111111111111","algo":1}],"primary":true}],"pagination":{"totalCount":1,"index":0,"pageSize":20}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	a, err := NewHTTPAdapter(CurseForge, s.URL, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	search, err := a.Search(context.Background(), SearchRequest{Query: "alpha"})
	if err != nil || len(search.Items) != 1 || search.Items[0].Name != "Alpha Mod" || search.Items[0].Downloads != 1234 {
		t.Fatalf("curseforge search=%#v error=%v", search, err)
	}
	project, err := a.Project(context.Background(), "123")
	if err != nil || project.Name != "Alpha Mod" || project.Summary != "summary" || project.Downloads != 1234 {
		t.Fatalf("curseforge project=%#v error=%v", project, err)
	}
	versions, err := a.Versions(context.Background(), "123")
	if err != nil || len(versions) != 1 || versions[0].Name != "Alpha 1.0" || len(versions[0].Files) != 1 || versions[0].Files[0].SHA1 == "" {
		t.Fatalf("curseforge versions=%#v error=%v", versions, err)
	}
}

// TestHTTPAdapterSearchParsesLoadersFromMixedCategories 钉住搜索结果的加载器解析。
//
// Modrinth 的 search hit 里 `categories` 是**加载器与普通分类混装**的
// （technology / storage / equipment 和 forge / neoforge 在同一个数组），
// 直接整坨当 loaders 用会让「仅支持 X」的提示全是废话，所以必须求交。
func TestHTTPAdapterSearchParsesLoadersFromMixedCategories(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"hits":[{"project_id":"p1","title":"Mekanism",
			"categories":["technology","storage","forge","neoforge","equipment"]}],
			"pagination":{"total":1}}`))
	}))
	defer s.Close()
	a, err := NewHTTPAdapter(Modrinth, s.URL, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Search(context.Background(), SearchRequest{Query: "mek"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items=%#v", got.Items)
	}
	loaders := got.Items[0].Loaders
	if len(loaders) != 2 || loaders[0] != "forge" || loaders[1] != "neoforge" {
		t.Fatalf("loaders=%v, want [forge neoforge] —— 普通分类必须被滤掉", loaders)
	}
}

// TestHTTPAdapterProjectParsesLoaders 保护「按 slug 直取」那条路径。
//
// 模组搜索的 slug 直取走的是项目详情接口，它绕开了平台侧的 facets 过滤，
// 所以加载器必须从详情里拿到手，才能自己再判一次能不能装。
func TestHTTPAdapterProjectParsesLoaders(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"p1","slug":"mekanism","title":"Mekanism",
			"loaders":["Forge"," NeoForge "],"categories":["technology"]}`))
	}))
	defer s.Close()
	a, err := NewHTTPAdapter(Modrinth, s.URL, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Project(context.Background(), "mekanism")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Loaders) != 2 || p.Loaders[0] != "forge" || p.Loaders[1] != "neoforge" {
		t.Fatalf("loaders=%v, want 规整成小写去空白的 [forge neoforge]", p.Loaders)
	}

	// 只有混装 categories 时按求交兜底。
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"p2","slug":"x","title":"X","categories":["storage","quilt","decoration"]}`))
	}))
	defer s2.Close()
	a2, err := NewHTTPAdapter(Modrinth, s2.URL, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := a2.Project(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Loaders) != 1 || p2.Loaders[0] != "quilt" {
		t.Fatalf("loaders=%v, want 兜底求交出 [quilt]", p2.Loaders)
	}
}

// TestPickLoadersReturnsEmptyForUnknown 守住「空 = 未知」的语义：
// 挑不出加载器时必须返回空，让调用方按未知处理，绝不能当成「一个都不支持」。
func TestPickLoadersReturnsEmptyForUnknown(t *testing.T) {
	if got := pickLoaders([]string{"technology", "storage"}); len(got) != 0 {
		t.Fatalf("pickLoaders = %v, want 空", got)
	}
	if got := pickLoaders(nil); len(got) != 0 {
		t.Fatalf("pickLoaders(nil) = %v, want 空", got)
	}
	if got := pickLoaders([]string{"FABRIC", "NeoForge"}); len(got) != 2 || got[0] != "fabric" || got[1] != "neoforge" {
		t.Fatalf("pickLoaders = %v, want [fabric neoforge]", got)
	}
}
