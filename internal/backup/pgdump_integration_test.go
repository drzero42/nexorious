package backup_test

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious/internal/backup"
)

// A backup must not carry work in flight or restore bookkeeping (#1341): no
// river_job or river_leader rows and no nexorious_restore schema. It must still
// load into an empty database and keep River's job id sequence, so job ids keep
// increasing after a restore.
func TestRunPgDump_ExcludesQueueAndRestoreMarker(t *testing.T) {
	conn := requirePsqlTools(t)
	truncateAllTables(t)
	ctx := context.Background()

	var maxJobID int64
	if err := testDB.QueryRowContext(ctx, `
		INSERT INTO river_job (args, kind, max_attempts, queue, state)
		SELECT '{}', 'dump_test', 1, 'default', 'available' FROM generate_series(1, 3)
		RETURNING id`).Scan(&maxJobID); err != nil {
		t.Fatalf("insert river_job: %v", err)
	}
	if err := testDB.QueryRowContext(ctx, `SELECT max(id) FROM river_job`).Scan(&maxJobID); err != nil {
		t.Fatalf("max river_job id: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `
		INSERT INTO river_leader (leader_id, elected_at, expires_at)
		VALUES ('dump-test', now(), now() + interval '1 hour')`); err != nil {
		t.Fatalf("insert river_leader: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `
		CREATE SCHEMA nexorious_restore;
		CREATE TABLE nexorious_restore.marker (phase text);
		INSERT INTO nexorious_restore.marker VALUES ('cleanup')`); err != nil {
		t.Fatalf("create marker schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DELETE FROM river_leader WHERE leader_id = 'dump-test'`)
		_, _ = testDB.ExecContext(ctx, `DROP SCHEMA IF EXISTS nexorious_restore CASCADE`)
	})

	dumpPath := filepath.Join(t.TempDir(), "database.sql")
	if err := backup.RunPgDump(conn, dumpPath); err != nil {
		t.Fatalf("RunPgDump: %v", err)
	}

	const target = "pgdump_exclusions_t"
	if _, err := testDB.ExecContext(ctx, "DROP DATABASE IF EXISTS "+target+" WITH (FORCE)"); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, "CREATE DATABASE "+target); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, "DROP DATABASE IF EXISTS "+target+" WITH (FORCE)")
	})

	targetConn := conn
	targetConn.DBName = target
	if err := backup.RunPsqlRestore(targetConn, dumpPath); err != nil {
		t.Fatalf("load dump into empty database: %v", err)
	}

	u, err := url.Parse(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + target
	restored := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(u.String()))), pgdialect.New())
	t.Cleanup(func() { _ = restored.Close() })

	var n int
	if err := restored.QueryRowContext(ctx, `SELECT count(*) FROM bun_migrations`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("restored database has no migrations recorded (n=%d, err=%v)", n, err)
	}
	for _, table := range []string{"river_job", "river_leader"} {
		if err := restored.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s: got %d rows in the backup, want 0", table, n)
		}
	}
	var markerSchema sql.NullString
	if err := restored.QueryRowContext(ctx, `SELECT to_regnamespace('nexorious_restore')::text`).Scan(&markerSchema); err != nil {
		t.Fatal(err)
	}
	if markerSchema.Valid {
		t.Error("nexorious_restore schema is in the backup")
	}
	var lastValue int64
	if err := restored.QueryRowContext(ctx, `SELECT last_value FROM river_job_id_seq`).Scan(&lastValue); err != nil {
		t.Fatalf("read river_job_id_seq: %v", err)
	}
	if lastValue < maxJobID {
		t.Errorf("river_job_id_seq last_value = %d, want >= %d (sequence lost)", lastValue, maxJobID)
	}
}
