// Package platform contains operating-system boundaries used by other layers.
package platform

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Clock isolates wall time for deterministic tests.
type Clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock returns a clock backed by the system.
func RealClock() Clock { return realClock{} }

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// FixedClock returns a deterministic clock for tests.
func FixedClock(t time.Time) Clock { return fixedClock{t: t} }

// NewID returns a random, URL-safe identifier.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// PathPolicy constrains paths to a single root and rejects symlink escapes.
type PathPolicy struct{ Root string }

// Resolve validates a relative path beneath Root.
func (p PathPolicy) Resolve(name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return "", errors.New("path must be a non-empty relative path")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes root")
	}
	root, err := filepath.Abs(p.Root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, clean)
	if rel, err := filepath.Rel(root, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes root")
	}
	// Existing path components must resolve beneath root (symlink/junction safe).
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("symlink path is not allowed")
	}
	return target, nil
}
