package provider

import (
	"context"
	"errors"
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
