// Package blobstore owns durable file writes and temporary objects.
package blobstore

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mpackstation/internal/platform"
)

// Store is a content-addressed local filesystem store.
type Store struct { root, temp string; policy platform.PathPolicy }

// New creates a store rooted at root, with a private temporary directory.
func New(root string) (*Store, error) {
	if root == "" { return nil, errors.New("blob root is empty") }
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil { return nil, fmt.Errorf("create blob root: %w", err) }
	return &Store{root: root, temp: filepath.Join(root, "tmp"), policy: platform.PathPolicy{Root: root}}, nil
}

// Put writes data atomically and returns its SHA-1 content address.
func (s *Store) Put(r io.Reader) (string, int64, error) {
	f, err := os.CreateTemp(s.temp, "blob-"); if err != nil { return "", 0, err }
	tmp := f.Name(); defer os.Remove(tmp)
	h := sha1.New(); n, err := io.Copy(io.MultiWriter(f, h), r); if err != nil { f.Close(); return "", 0, err }
	if err = f.Sync(); err != nil { f.Close(); return "", 0, err }; if err = f.Close(); err != nil { return "", 0, err }
	digest := hex.EncodeToString(h.Sum(nil)); rel := filepath.Join("blobs", digest[:2], digest)
	dest, err := s.policy.Resolve(rel); if err != nil { return "", 0, err }; if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { return "", 0, err }
	if _, err := os.Stat(dest); err == nil { return digest, n, nil } else if !errors.Is(err, os.ErrNotExist) { return "", 0, err }
	if err := os.Rename(tmp, dest); err != nil { if _, statErr := os.Stat(dest); statErr == nil { return digest, n, nil }; return "", 0, err }
	return digest, n, nil
}

// Open opens an addressed blob for reading.
func (s *Store) Open(digest string) (*os.File, error) { if len(digest) != 40 { return nil, errors.New("invalid sha1 digest") }; p, err := s.policy.Resolve(filepath.Join("blobs", digest[:2], digest)); if err != nil { return nil, err }; return os.Open(p) }

// Remove deletes a blob after the caller has completed reference checks.
func (s *Store) Remove(digest string) error { if len(digest) != 40 { return errors.New("invalid sha1 digest") }; p, err := s.policy.Resolve(filepath.Join("blobs", digest[:2], digest)); if err != nil { return err }; if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) { return err }; return nil }
