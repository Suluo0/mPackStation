package service

import (
	"fmt"

	"mpackstation/internal/store"
)

// DomainError carries a stable, fine-grained error code plus the HTTP status
// the contract assigns to it (standards.md D-3/D-5). httpapi serializes it
// verbatim; sentinel-based mapping remains as fallback for unmigrated paths.
// Wrapped preserves errors.Is/As semantics for the underlying cause (e.g.
// store.ErrNotFound) so service-level tests keep working.
type DomainError struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
	Wrapped error
}

func (e *DomainError) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Wrapped)
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the underlying cause for errors.Is/As.
func (e *DomainError) Unwrap() error { return e.Wrapped }

// ValidationError carries per-field validation issues for 422 responses
// (standards.md §5.3). Domain selects the error code family: "content"
// yields content_invalid; "quest" yields quest_cycle / quest_orphan_node /
// quest_invalid_reference according to the dominant issue code.
type ValidationError struct {
	Domain string
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s validation failed: %d issue(s)", e.Domain, len(e.Issues))
}

// NotFoundError builds a 404 DomainError whose code names the concrete
// resource (pack_not_found, mod_not_found, ...), preserving ErrNotFound
// semantics through Unwrap.
func NotFoundError(code, message string) *DomainError {
	return &DomainError{Status: 404, Code: code, Message: message, Wrapped: store.ErrNotFound}
}
