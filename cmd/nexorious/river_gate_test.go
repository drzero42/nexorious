package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func testGate(ready func() bool, runRecovery func(), start func() error) riverStartGate {
	return riverStartGate{
		ready: ready, runRecovery: runRecovery, start: start,
		poll: time.Millisecond, minBackoff: time.Millisecond, maxBackoff: 4 * time.Millisecond,
	}
}

// A Start that returns an error is retried, with recovery run before every
// attempt, until one succeeds (#1341 item 4: the gate used to give up after
// one try, leaving the process without River).
func TestRiverStartGate_RetriesFailedStart(t *testing.T) {
	var recovers, starts atomic.Int32
	g := testGate(
		func() bool { return true },
		func() { recovers.Add(1) },
		func() error {
			if starts.Add(1) < 3 {
				return errors.New("db not ready")
			}
			return nil
		},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !g.run(ctx) {
		t.Fatal("run returned false; want River started")
	}
	if got := starts.Load(); got != 3 {
		t.Errorf("start called %d times, want 3", got)
	}
	if got := recovers.Load(); got != 3 {
		t.Errorf("recover called %d times, want 3 (once before each attempt)", got)
	}
}

// Nothing runs before the app is ready, and a cancelled context ends the wait.
func TestRiverStartGate_WaitsForReadyAndStopsOnCancel(t *testing.T) {
	var ready atomic.Bool
	var recovers, starts atomic.Int32
	g := testGate(
		ready.Load,
		func() { recovers.Add(1) },
		func() error { starts.Add(1); return nil },
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)
	go func() { done <- g.run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	if <-done {
		t.Fatal("run returned true without the app being ready")
	}
	if recovers.Load() != 0 || starts.Load() != 0 {
		t.Errorf("recover/start ran before ready: recover=%d start=%d", recovers.Load(), starts.Load())
	}
}

// A context cancelled while backing off after a failed start ends the gate
// without another attempt.
func TestRiverStartGate_StopsRetryingOnCancel(t *testing.T) {
	var starts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	g := testGate(
		func() bool { return true },
		func() {},
		func() error { starts.Add(1); cancel(); return errors.New("fail") },
	)
	if g.run(ctx) {
		t.Fatal("run returned true after cancel")
	}
	if got := starts.Load(); got != 1 {
		t.Errorf("start called %d times, want 1", got)
	}
}
