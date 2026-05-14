package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/drzero42/nexorious-go/internal/ratelimit"
)

func TestLocal_WaitSucceeds(t *testing.T) {
	l := ratelimit.NewLocal(100, 10)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

// TestLocal_ZeroRPS verifies that rps<=0 is clamped to 1 (guard branch in NewLocal).
func TestLocal_ZeroRPS(t *testing.T) {
	// Should not panic; guard clamps rps to 1.
	l := ratelimit.NewLocal(0, 5)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLocal_NegativeRPS verifies that negative rps is clamped to 1.
func TestLocal_NegativeRPS(t *testing.T) {
	l := ratelimit.NewLocal(-10, 5)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLocal_ZeroBurst verifies that burst<1 is clamped to 1 (guard branch in NewLocal).
func TestLocal_ZeroBurst(t *testing.T) {
	l := ratelimit.NewLocal(10, 0)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLocal_NegativeBurst verifies that negative burst is clamped to 1.
func TestLocal_NegativeBurst(t *testing.T) {
	l := ratelimit.NewLocal(10, -5)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
