package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/drzero42/nexorious-go/internal/ratelimit"
)

func TestLocal_RespectsContextCancellation(t *testing.T) {
	// 1 token/second, burst 1 — use up the burst token then cancel.
	l := ratelimit.NewLocal(1, 1)
	_ = l.Wait(context.Background()) // consume the burst token

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := l.Wait(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}
