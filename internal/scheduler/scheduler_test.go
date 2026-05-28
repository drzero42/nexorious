package scheduler_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/scheduler"
)

func TestCleanupOldJobs(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Old completed job (31 days ago).
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, completed_at)
		 VALUES ('job-old', ?, 'import', 'manual', 'completed', now() - interval '31 days')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert old job: %v", err)
	}

	// Recent completed job (1 day ago).
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, completed_at)
		 VALUES ('job-recent', ?, 'import', 'manual', 'completed', now() - interval '1 day')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert recent job: %v", err)
	}

	// Active job (no completed_at).
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status)
		 VALUES ('job-active', ?, 'import', 'manual', 'processing')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert active job: %v", err)
	}

	scheduler.CleanupOldJobs(ctx, testDB)

	var count int
	err = testDB.NewRaw(`SELECT COUNT(*) FROM jobs`).Scan(ctx, &count)
	if err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 jobs remaining, got %d", count)
	}

	// Confirm the old one is gone.
	var exists bool
	err = testDB.NewRaw(`SELECT EXISTS(SELECT 1 FROM jobs WHERE id = 'job-old')`).Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("check job-old: %v", err)
	}
	if exists {
		t.Fatal("expected old job to be deleted")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Expired session.
	_, err := testDB.NewRaw(
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at)
		 VALUES ('sess-expired', ?, 'hash1', now() - interval '1 hour')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	// Valid session.
	_, err = testDB.NewRaw(
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at)
		 VALUES ('sess-valid', ?, 'hash2', now() + interval '30 days')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert valid session: %v", err)
	}

	scheduler.CleanupExpiredSessions(ctx, testDB)

	var count int
	err = testDB.NewRaw(`SELECT COUNT(*) FROM user_sessions`).Scan(ctx, &count)
	if err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 session remaining, got %d", count)
	}

	var exists bool
	err = testDB.NewRaw(`SELECT EXISTS(SELECT 1 FROM user_sessions WHERE id = 'sess-valid')`).Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("check valid session: %v", err)
	}
	if !exists {
		t.Fatal("expected valid session to remain")
	}
}
