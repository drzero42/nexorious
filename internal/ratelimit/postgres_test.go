package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/ratelimit"
)

// setupTestDB resets the shared rate-limiter table and returns the shared
// *bun.DB. The container is started once in TestMain.
func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`TRUNCATE TABLE rate_limiter_tokens RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate rate_limiter_tokens: %v", err)
	}
	return testDB
}

func TestPostgres_ConcurrentGoroutinesEachGetToken(t *testing.T) {
	db := setupTestDB(t)
	n := 5
	// Burst = n so all goroutines can get a token without waiting for refill.
	l := ratelimit.NewPostgres(db, "test-concurrent", 100, float64(n))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = l.Wait(ctx)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
}

func TestPostgres_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	// Burst=1, rate=0.01/s so after consuming the first token there's essentially
	// no refill within the test window.
	l := ratelimit.NewPostgres(db, "test-cancel", 0.01, 1)

	ctx := context.Background()
	// Consume the single burst token.
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first wait failed: %v", err)
	}

	// The next Wait should block; cancel it quickly.
	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := l.Wait(cancelCtx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// TestPostgres_ClampsInvalidParams verifies that out-of-range rate/burst values
// are clamped to safe minimums (rps<=0 → 1, burst<1 → 1) so the first Wait still
// succeeds.
func TestPostgres_ClampsInvalidParams(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		rps   float64
		burst float64
	}{
		{"zero rps clamped to 1", "test-zero-rps", 0, 5},
		{"low burst clamped to 1", "test-low-burst", 10, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			l := ratelimit.NewPostgres(db, tc.key, tc.rps, tc.burst)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := l.Wait(ctx); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestPostgres_AlreadyCancelledContext verifies Wait returns immediately when context is already done.
func TestPostgres_AlreadyCancelledContext(t *testing.T) {
	db := setupTestDB(t)
	l := ratelimit.NewPostgres(db, "test-pre-cancel", 10, 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling Wait
	err := l.Wait(ctx)
	if err == nil {
		t.Fatal("expected error from already-cancelled context, got nil")
	}
}
