package provider

import (
	"context"
	"errors"
	"testing"
)

func TestFixtureAdapterNormalizesSearchAndDownload(t *testing.T) {
	data := []byte(`{"projects":[{"id":"p1","slug":"alpha","name":"Alpha Mod"}],"versions":[{"id":"v1","projectId":"p1","name":"1.0","versionNumber":"1.0","files":[{"id":"f1","name":"alpha.jar","downloadUrl":"https://cdn.example/alpha.jar","sha1":"1111111111111111111111111111111111111111","primary":true}]}],"metadata":[{"project":{"id":"p1","name":"Alpha Mod"},"dependencies":[]}],"faults":{"search:offline":{"code":"503"}}}`)
	a, err := NewModrinthFixture(data)
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.Search(context.Background(), SearchRequest{Query: "alpha", Limit: 10})
	if err != nil || len(r.Items) != 1 || r.Items[0].ID != "p1" {
		t.Fatalf("search = %#v, %v", r, err)
	}
	d, err := a.Download(context.Background(), DownloadRequest{ProjectID: "p1", VersionID: "v1"})
	if err != nil || d.SHA1 != "1111111111111111111111111111111111111111" {
		t.Fatalf("download = %#v, %v", d, err)
	}
	_, err = a.Search(context.Background(), SearchRequest{Query: "offline"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("fault error = %v, want unavailable", err)
	}
}
