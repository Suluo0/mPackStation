package blobstore

import (
	"bytes"
	"io"
	"testing"
)

func TestPutOpenAndDeduplicate(t *testing.T) {
	s, err := New(t.TempDir()); if err != nil { t.Fatal(err) }
	d, n, err := s.Put(bytes.NewBufferString("hello")); if err != nil || n != 5 { t.Fatalf("put: %v %d", err, n) }
	d2, _, err := s.Put(bytes.NewBufferString("hello")); if err != nil || d2 != d { t.Fatalf("dedupe: %v %q", err, d2) }
	f, err := s.Open(d); if err != nil { t.Fatal(err) }; defer f.Close(); b, _ := io.ReadAll(f); if string(b) != "hello" { t.Fatalf("content %q", b) }
}
func TestRejectBadDigest(t *testing.T) { s, _ := New(t.TempDir()); if _, err := s.Open("bad"); err == nil { t.Fatal("expected digest validation") } }
