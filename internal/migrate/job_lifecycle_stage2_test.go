package migrate_test

import (
	"context"
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"
)

const stage2Migration = "20261123000003"

// The Job lifecycle stage 2 migration (#1267) puts import rows into a state
// the module's rules understand, and leaves exports (which share the source
// values) alone.
func TestJobLifecycleStage2Migration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrationsBefore(stage2Migration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before stage 2: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`)
	job := func(id, jobType, source, status string) {
		exec(`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
		      VALUES (?, 'u1', ?, ?, ?, 'high', now() - interval '1 hour')`, id, jobType, source, status)
	}
	item := func(id, jobID, key, status string, resolved any) {
		exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status, resolved_igdb_id) VALUES (?, ?, 'u1', ?, 't', ?, ?)`,
			id, jobID, key, status, resolved)
	}

	// A nexorious import whose every record was malformed: only the pools item.
	job("nx-empty", "import", "nexorious", "pending")
	item("nx-empty-pools", "nx-empty", "__pools__", "completed", nil)
	// A nexorious import in flight, kept 'pending' by older versions.
	job("nx-flight", "import", "nexorious", "pending")
	item("nx-flight-1", "nx-flight", "igdb_1", "completed", nil)
	item("nx-flight-2", "nx-flight", "igdb_2", "pending", nil)
	// An active generic import with a resolved item stranded in 'processing'.
	job("csv-active", "import", "csv", "processing")
	item("csv-resolved", "csv-active", "game_0", "processing", 42)
	item("csv-review", "csv-active", "game_1", "pending_review", nil)
	// Unfinished items of finished imports.
	job("csv-cancelled", "import", "csv", "cancelled")
	item("cancelled-review", "csv-cancelled", "game_0", "pending_review", nil)
	item("cancelled-processing", "csv-cancelled", "game_1", "processing", 42)
	job("csv-done", "import", "csv", "completed")
	item("done-pending", "csv-done", "game_0", "pending", nil)
	item("done-review", "csv-done", "game_1", "pending_review", nil)
	item("done-processing", "csv-done", "game_2", "processing", 42)
	// An export with an import source value, and a sync: untouched.
	job("export", "export", "nexorious", "pending")
	job("sync", "sync", "steam", "pending")
	item("sync-item", "sync", "k", "processing", nil)

	// Stop before stage 3, so that only stage 2 acts on these rows: a later migration (#1516) moves the sync item's processing status.
	if _, err := bunmigrate.NewMigrator(db, migrationsBefore(stage3Migration)).Migrate(ctx); err != nil {
		t.Fatalf("migrate stage 2: %v", err)
	}

	type jobRow struct {
		Status       string  `bun:"status"`
		ErrorMessage *string `bun:"error_message"`
		StartedAt    *string `bun:"started_at"`
		CompletedAt  *string `bun:"completed_at"`
	}
	readJob := func(id string) jobRow {
		t.Helper()
		var r jobRow
		if err := db.NewRaw(`SELECT status, error_message, started_at::text, completed_at::text FROM jobs WHERE id = ?`, id).Scan(ctx, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	type itemRow struct {
		Status      string  `bun:"status"`
		Msg         *string `bun:"error_message"`
		ProcessedAt *string `bun:"processed_at"`
		Resolved    *int    `bun:"resolved_igdb_id"`
	}
	readItem := func(id string) itemRow {
		t.Helper()
		var r itemRow
		if err := db.NewRaw(`SELECT status, error_message, processed_at::text, resolved_igdb_id FROM job_items WHERE id = ?`, id).Scan(ctx, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	if r := readJob("nx-empty"); r.Status != "failed" || r.CompletedAt == nil || r.ErrorMessage == nil || *r.ErrorMessage != "the file had no usable game records" {
		t.Errorf("nx-empty = %+v, want failed", r)
	}
	if r := readJob("nx-flight"); r.Status != "processing" || r.StartedAt == nil {
		t.Errorf("nx-flight = %+v, want processing with started_at", r)
	}
	if r := readJob("csv-active"); r.Status != "processing" || r.StartedAt == nil {
		t.Errorf("csv-active = %+v, want started_at set", r)
	}
	if r := readItem("csv-resolved"); r.Status != "pending" || r.Resolved == nil || *r.Resolved != 42 {
		t.Errorf("stranded resolved item = %+v, want pending with its resolved id", r)
	}
	if r := readItem("csv-review"); r.Status != "pending_review" {
		t.Errorf("review item of an active import = %+v, want untouched", r)
	}
	for _, id := range []string{"cancelled-review", "cancelled-processing"} {
		if r := readItem(id); r.Status != "cancelled" || r.ProcessedAt == nil {
			t.Errorf("%s = %+v, want cancelled with processed_at", id, r)
		}
	}
	for _, id := range []string{"done-pending", "done-review", "done-processing"} {
		if r := readItem(id); r.Status != "failed" || r.ProcessedAt == nil || r.Msg == nil || *r.Msg != "interrupted by an upgrade; retry it" {
			t.Errorf("%s = %+v, want failed so it can be retried", id, r)
		}
	}
	if r := readItem("done-processing"); r.Resolved == nil {
		t.Error("done-processing lost its resolved id (a retry must go to finalize)")
	}
	if r := readJob("export"); r.Status != "pending" || r.StartedAt != nil {
		t.Errorf("export touched: %+v", r)
	}
	if r := readItem("sync-item"); r.Status != "processing" {
		t.Errorf("sync item touched: %+v", r)
	}
}
