package tasks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// TestEnqueueOrFail_NilRiverClient_MarksItemFailed locks in the contract that a
// nil River client never leaves a job_item stranded in 'pending'. It must move
// to 'failed' with a diagnostic error_message so the parent job can settle and
// the user sees the failure instead of silent inaction.
func TestEnqueueOrFail_NilRiverClient_MarksItemFailed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)
	itemID := insertTestJobItem(t, testDB, jobID, userID, map[string]any{})

	err := tasks.EnqueueOrFail(ctx, testDB, nil, itemID,
		tasks.IGDBMatchArgs{JobItemID: itemID})
	if !errors.Is(err, tasks.ErrNilRiverClient) {
		t.Fatalf("expected ErrNilRiverClient, got %v", err)
	}

	var status string
	var errMsg *string
	_ = testDB.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, itemID).
		Scan(ctx, &status, &errMsg)
	if status != "failed" {
		t.Errorf("expected status=failed, got %q", status)
	}
	if errMsg == nil || *errMsg == "" {
		t.Errorf("expected non-empty error_message, got %v", errMsg)
	}
}

// TestEnqueueOrFail_Success_LeavesItemPending verifies the happy path: when
// River accepts the insert the item stays in its current state ('pending'),
// and the river_job row exists.
func TestEnqueueOrFail_Success_LeavesItemPending(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)
	itemID := insertTestJobItem(t, testDB, jobID, userID, map[string]any{})

	rc := newTestRiverClient(t)
	if err := tasks.EnqueueOrFail(ctx, testDB, rc, itemID,
		tasks.IGDBMatchArgs{JobItemID: itemID}); err != nil {
		t.Fatalf("EnqueueOrFail returned %v, want nil", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("expected status=pending, got %q", status)
	}

	var count int
	_ = testDB.NewRaw(
		`SELECT count(*) FROM river_job WHERE args->>'job_item_id' = ? AND kind = 'igdb_match'`,
		itemID,
	).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("expected 1 river_job, got %d", count)
	}
}

// TestEnqueueOrFail_NilClient_OnlyTouchesPendingRows guards against a class of
// races where the item already moved to a terminal state. The UPDATE is
// scoped to status='pending' so an already-completed item cannot be clobbered.
func TestEnqueueOrFail_NilClient_OnlyTouchesPendingRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)
	itemID := insertTestJobItem(t, testDB, jobID, userID, map[string]any{})
	_, _ = testDB.NewRaw(
		`UPDATE job_items SET status = 'completed' WHERE id = ?`, itemID,
	).Exec(ctx)

	_ = tasks.EnqueueOrFail(ctx, testDB, nil, itemID,
		tasks.IGDBMatchArgs{JobItemID: itemID})

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("EnqueueOrFail clobbered a non-pending row: status=%q, want completed", status)
	}
}

// TestUsesGenericImportPipeline locks in which sources run the shared
// ImportMatch→pending_review→ImportFinalize chain. CSV qualifies despite not
// being a registry Mapper; nexorious (legacy single-stage) and sync do not.
func TestUsesGenericImportPipeline(t *testing.T) {
	if !tasks.UsesGenericImportPipeline(models.JobSourceCSV) {
		t.Errorf("csv should use the generic import pipeline")
	}
	if tasks.UsesGenericImportPipeline(models.JobSourceNexorious) {
		t.Errorf("nexorious uses the legacy single-stage import, not the generic pipeline")
	}
	if tasks.UsesGenericImportPipeline("steam") {
		t.Errorf("sync source steam should not use the import pipeline")
	}
}

// TestFinalizeArgsForSource_CSV: a resolved CSV pending_review item must finalize
// through the generic ImportFinalize worker.
func TestFinalizeArgsForSource_CSV(t *testing.T) {
	args, err := tasks.FinalizeArgsForSource(models.JobSourceCSV, "item-1")
	if err != nil {
		t.Fatalf("FinalizeArgsForSource(csv): %v", err)
	}
	if _, ok := args.(tasks.ImportFinalizeArgs); !ok {
		t.Errorf("expected ImportFinalizeArgs, got %T", args)
	}
}

// TestArgsForJobType_CSVImportUsesMatch: retrying a failed CSV item must re-enter
// at the generic match stage, not the legacy single-stage import worker.
func TestArgsForJobType_CSVImportUsesMatch(t *testing.T) {
	args, err := tasks.ArgsForJobType(models.JobTypeImport, models.JobSourceCSV, "item-1")
	if err != nil {
		t.Fatalf("ArgsForJobType: %v", err)
	}
	if _, ok := args.(tasks.ImportMatchArgs); !ok {
		t.Errorf("expected ImportMatchArgs for csv retry, got %T", args)
	}
}
