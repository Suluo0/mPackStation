// Package obs provides small structured observability boundaries.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Logger is intentionally tiny so callers can inject a production logger.
type Logger interface {
	Info(context.Context, string, map[string]any)
	Error(context.Context, string, error, map[string]any)
}
type nopLogger struct{}

func (nopLogger) Info(context.Context, string, map[string]any)         {}
func (nopLogger) Error(context.Context, string, error, map[string]any) {}

// NopLogger returns a logger suitable when no sink is configured.
func NopLogger() Logger { return nopLogger{} }

// MemoryLogger captures events for tests and diagnostics.
type MemoryLogger struct {
	mu     sync.Mutex
	Events []Event
}
type Event struct {
	Level, Message string
	Err            error
	Fields         map[string]any
}

func (l *MemoryLogger) Info(_ context.Context, msg string, fields map[string]any) {
	l.add(Event{Level: "info", Message: msg, Fields: clone(fields)})
}
func (l *MemoryLogger) Error(_ context.Context, msg string, err error, fields map[string]any) {
	l.add(Event{Level: "error", Message: msg, Err: err, Fields: clone(fields)})
}
func (l *MemoryLogger) add(e Event) { l.mu.Lock(); defer l.mu.Unlock(); l.Events = append(l.Events, e) }
func clone(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

type requestIDKey struct{}

// WithRequestID attaches a request id to context, generating one when empty.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err == nil {
			id = hex.EncodeToString(b[:])
		}
	}
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID reads the request id from context.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}
