package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFixedClock(t *testing.T) {
	want := time.UnixMilli(42)
	if got := FixedClock(want).Now(); !got.Equal(want) {
		t.Fatalf("got %v", got)
	}
}
func TestPathPolicy(t *testing.T) {
	p := PathPolicy{Root: t.TempDir()}
	if _, err := p.Resolve("../escape"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := p.Resolve("/absolute"); err == nil {
		t.Fatal("expected absolute rejection")
	}
	inside, err := p.Resolve("ok/file.txt")
	if err != nil || !filepath.IsAbs(inside) {
		t.Fatalf("resolve: %v %q", err, inside)
	}
	link := filepath.Join(p.Root, "link")
	if err := os.Symlink(filepath.Join(p.Root, "ok"), link); err == nil {
		if _, e := p.Resolve("link"); e == nil {
			t.Fatal("expected symlink rejection")
		}
	}
}
