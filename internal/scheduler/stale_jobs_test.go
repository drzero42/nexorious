package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/scheduler"
)

func TestCleanupStaleJobs_StuckPendingNoItems(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'pending', 'low', 0, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	var errMsg *string
	if err := testDB.NewRaw(
		`SELECT status, error_message FROM jobs WHERE id = ?`, jobID,
	).Scan(ctx, &status, &errMsg); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status=failed, got %s", status)
	}
	if errMsg == nil || *errMsg != "stale_job_cleaned_up" {
		t.Fatalf("expected error_message=stale_job_cleaned_up, got %v", errMsg)
	}
}

func TestCleanupStaleJobs_StuckProcessingAllItemsTerminal(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 2, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	for range 2 {
		_, err := testDB.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, 'x', '{}', 'completed', '{}', '[]', now() - interval '5 hours')`,
			uuid.NewString(), jobID, userID, uuid.NewString(),
		).Exec(ctx)
		if err != nil {
			t.Fatalf("insert item: %v", err)
		}
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status=failed, got %s", status)
	}
}

func TestCleanupStaleJobs_StuckProcessingWithPendingItem_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'x', '{}', 'pending', '{}', '[]', now())`,
		uuid.NewString(), jobID, userID, "k",
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("expected status=processing (untouched), got %s", status)
	}
}

func TestCleanupStaleJobs_FreshJob_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'pending', 'low', 0, now() - interval '1 hour')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending (untouched), got %s", status)
	}
}

func TestCleanupStaleJobs_NonMetadataRefresh_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'import', 'manual', 'pending', 'low', 0, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "pending" {
		t.Fatalf("import job should not be touched, got status=%s", status)
	}
}

// ── sync job cleanup ──────────────────────────────────────────────────────────

func TestCleanupStaleJobs_SyncJob_StuckDispatch_Cleaned(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0, false, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	var errMsg *string
	if err := testDB.NewRaw(
		`SELECT status, error_message FROM jobs WHERE id = ?`, jobID,
	).Scan(ctx, &status, &errMsg); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status=failed, got %s", status)
	}
	if errMsg == nil || *errMsg != "stale_job_cleaned_up" {
		t.Fatalf("expected error_message=stale_job_cleaned_up, got %v", errMsg)
	}
}

func TestCleanupStaleJobs_SyncJob_DispatchComplete_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0, true, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("sync job with dispatch_complete=true should not be touched, got %s", status)
	}
}

func TestCleanupStaleJobs_SyncJob_WithActivePendingItem_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, false, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'x', '{}', 'pending', '{}', '[]', now())`,
		uuid.NewString(), jobID, userID, "k1",
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("sync job with active items should not be touched, got %s", status)
	}
}

func TestCleanupStaleJobs_SyncJob_FreshJob_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0, false, now() - interval '1 hour')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("fresh sync job should not be touched, got %s", status)
	}
}

func TestCleanupStaleJobs_SyncJob_WithPendingReviewItem_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, false, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'x', '{}', 'pending_review', '{}', '[]', now())`,
		uuid.NewString(), jobID, userID, "k2",
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("sync job with pending_review items should not be touched, got %s", status)
	}
}

func TestCleanupStaleJobs_CompletedJob_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, completed_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'completed', 'low', 0, now() - interval '5 hours', now())`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "completed" {
		t.Fatalf("completed job should not be touched, got status=%s", status)
	}
}
