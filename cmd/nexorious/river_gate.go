package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
)

// riverStartGate starts River once the app is ready. It is the one place River
// is started (#1341); a restore switches it off for good and restarts the
// process instead of starting River again.
//
// It waits for ready, runs runRecovery, then calls start. A start that fails is
// retried, with runRecovery run again before each attempt and an exponential
// backoff between attempts, until one succeeds or ctx is done: before #1341 a
// single failed Start left the process without River until a restart.
type riverStartGate struct {
	ready       func() bool
	runRecovery func()
	start       func() error

	poll       time.Duration // how often readiness is checked
	minBackoff time.Duration // wait after the first failed start
	maxBackoff time.Duration // cap on the doubling wait
}

// run blocks until River has started (true) or ctx is done (false).
func (g riverStartGate) run(ctx context.Context) bool {
	backoff := g.minBackoff
	for {
		if !g.ready() {
			if !sleepCtx(ctx, g.poll) {
				return false
			}
			continue
		}
		if ctx.Err() != nil {
			return false
		}
		g.runRecovery()
		err := g.start()
		if err == nil {
			slog.Info("app ready — River client started")
			return true
		}
		slog.Error("failed to start River client — retrying", logging.KeyErr, err,
			"retry_in", backoff, logging.Cat(logging.CategoryDB))
		if !sleepCtx(ctx, backoff) {
			return false
		}
		backoff = min(backoff*2, g.maxBackoff)
	}
}

// sleepCtx waits for d, returning false if ctx is done first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
