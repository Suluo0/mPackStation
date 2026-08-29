package obs

import (
	"context"
	"testing"
)

func TestRequestIDAndMemoryLogger(t *testing.T) { ctx := WithRequestID(context.Background(), "req-1"); if RequestID(ctx) != "req-1" { t.Fatal("request id missing") }; l := &MemoryLogger{}; l.Info(ctx, "ok", map[string]any{"request_id": RequestID(ctx)}); if len(l.Events) != 1 || l.Events[0].Fields["request_id"] != "req-1" { t.Fatalf("events=%+v", l.Events) } }
