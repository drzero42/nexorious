package tasks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// riverClientWith builds a non-started River client whose Workers bundle is
// produced by register. Mirrors how cmd/nexorious/serve.go wires the client the
// API handler uses for Insert.
func riverClientWith(t *testing.T, register func(*river.Workers)) *river.Client[pgx.Tx] {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testConnStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	workers := river.NewWorkers()
	register(workers)
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	return rc
}

// TestStoreLinkRefreshDispatch_InsertRequiresRegisteredWorker reproduces the
// admin-trigger insert path. River's Client.Insert validates the job kind
// against the client's Workers bundle (validateJobArgs) and returns
// UnknownJobKindError for an unregistered kind. This is the ONLY way the admin
// endpoint's Insert can fail for these args — so a 500 from
// /api/games/store-links/refresh-job means the running process's bundle lacks
// the worker (e.g. a server not restarted after the worker was registered).
func TestStoreLinkRefreshDispatch_InsertRequiresRegisteredWorker(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Registered (as serve.go does) → Insert succeeds, exactly like the live
	// admin endpoint on current code.
	registered := riverClientWith(t, func(w *river.Workers) {
		river.AddWorker(w, &tasks.StoreLinkRefreshDispatchWorker{DB: testDB})
	})
	if _, err := registered.Insert(ctx, tasks.StoreLinkRefreshDispatchArgs{Force: true}, nil); err != nil {
		t.Fatalf("insert with worker registered should succeed, got: %v", err)
	}

	// Not registered → the failure the user observed.
	notRegistered := riverClientWith(t, func(w *river.Workers) {
		river.AddWorker(w, &tasks.MetadataRefreshItemWorker{DB: testDB})
	})
	_, err := notRegistered.Insert(ctx, tasks.StoreLinkRefreshDispatchArgs{Force: true}, nil)
	if err == nil {
		t.Fatal("insert without worker registered should fail")
	}
	var unknown *river.UnknownJobKindError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownJobKindError, got %T: %v", err, err)
	}
	if unknown.Kind != "store_link_refresh_dispatch" {
		t.Fatalf("unexpected kind: %q", unknown.Kind)
	}
}
