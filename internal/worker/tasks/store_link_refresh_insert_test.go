package tasks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
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

func TestStoreLinkRefreshDispatch_HandlerOwned_PopulatesExistingRow(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	adminID := insertMetaRefreshAdminUser(t)

	// One resolvable external_games row (steam, no store_link → eligible).
	if _, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, created_at, updated_at)
		 VALUES (?, ?, 'steam', '440', 'Team Fortress 2', true, now(), now())`,
		uuid.NewString(), adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert external_game: %v", err)
	}

	// Handler-created pending row.
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'store_link_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	rc := newTestMetadataRiverClient(t)
	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.StoreLinkRefreshDispatchArgs]{
		Args: tasks.StoreLinkRefreshDispatchArgs{Force: true, JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'store_link_refresh'`).Scan(ctx, &count)
	if count != 1 {
		t.Fatalf("expected 1 job row, got %d", count)
	}
	var status string
	var total int
	_ = testDB.NewRaw(`SELECT status, total_items FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status, &total)
	if status != "processing" {
		t.Errorf("status: want processing, got %s", status)
	}
	if total != 1 {
		t.Errorf("total_items: want 1, got %d", total)
	}
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("job_items: want 1, got %d", itemCount)
	}
}

func TestStoreLinkRefreshDispatch_HandlerOwned_EmptyFinalizesCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	adminID := insertMetaRefreshAdminUser(t)
	// No external_games rows → 0 groups.

	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'store_link_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.StoreLinkRefreshDispatchArgs]{
		Args: tasks.StoreLinkRefreshDispatchArgs{Force: true, JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("status: want completed, got %s", status)
	}
}
