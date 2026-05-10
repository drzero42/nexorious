package scheduler_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/scheduler"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func insertUser(t *testing.T, ctx context.Context, db *bun.DB) string {
	t.Helper()
	id := fmt.Sprintf("user-%d", time.Now().UnixNano())
	_, err := db.NewRaw(
		`INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)`,
		id, id, "hash",
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestCleanupOldJobs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Old completed job (31 days ago).
	_, err := db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, completed_at)
		 VALUES ('job-old', ?, 'import', 'manual', 'completed', now() - interval '31 days')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert old job: %v", err)
	}

	// Recent completed job (1 day ago).
	_, err = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, completed_at)
		 VALUES ('job-recent', ?, 'import', 'manual', 'completed', now() - interval '1 day')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert recent job: %v", err)
	}

	// Active job (no completed_at).
	_, err = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status)
		 VALUES ('job-active', ?, 'import', 'manual', 'processing')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert active job: %v", err)
	}

	scheduler.CleanupOldJobs(ctx, db)

	var count int
	err = db.NewRaw(`SELECT COUNT(*) FROM jobs`).Scan(ctx, &count)
	if err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 jobs remaining, got %d", count)
	}

	// Confirm the old one is gone.
	var exists bool
	err = db.NewRaw(`SELECT EXISTS(SELECT 1 FROM jobs WHERE id = 'job-old')`).Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("check job-old: %v", err)
	}
	if exists {
		t.Fatal("expected old job to be deleted")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Expired session.
	_, err := db.NewRaw(
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES ('sess-expired', ?, 'hash1', 'rhash1', now() - interval '1 hour')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	// Valid session.
	_, err = db.NewRaw(
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES ('sess-valid', ?, 'hash2', 'rhash2', now() + interval '30 days')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert valid session: %v", err)
	}

	scheduler.CleanupExpiredSessions(ctx, db)

	var count int
	err = db.NewRaw(`SELECT COUNT(*) FROM user_sessions`).Scan(ctx, &count)
	if err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 session remaining, got %d", count)
	}

	var exists bool
	err = db.NewRaw(`SELECT EXISTS(SELECT 1 FROM user_sessions WHERE id = 'sess-valid')`).Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("check valid session: %v", err)
	}
	if !exists {
		t.Fatal("expected valid session to remain")
	}
}
