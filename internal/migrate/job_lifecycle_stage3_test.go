package migrate_test

import (
	"context"
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

const stage3Migration = "20261123000004"

// The Job lifecycle stage 3 migration (#1268) puts sync rows into a state the
// module's rules understand, and leaves every other kind alone.
func TestJobLifecycleStage3Migration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrationsBefore(stage3Migration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before stage 3: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`)
	// Old creators inserted without dispatch_complete (default true).
	job := func(id, jobType, status, age string, dispatchComplete bool, completed bool) {
		completedAt := "NULL"
		if completed {
			completedAt = "now() - interval '2 hours'"
		}
		exec(`INSERT INTO jobs (id, user_id, job_type, source, status, priority, dispatch_complete, created_at, completed_at)
		      VALUES (?, 'u1', ?, 'steam', ?, 'high', ?, now() - ?::interval, `+completedAt+`)`,
			id, jobType, status, dispatchComplete, age)
	}
	item := func(id, jobID, status string) {
		exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status) VALUES (?, ?, 'u1', ?, 't', ?)`,
			id, jobID, id, status)
	}

	// Created but not started before the upgrade: dispatched, not completed empty.
	job("fresh", "sync", "pending", "10 minutes", true, false)
	// Created but never started, more than a day ago.
	job("ancient", "sync", "pending", "2 days", true, false)
	// A failed sync the old "retry failed" reopened.
	job("reopened", "sync", "processing", "3 hours", false, true)
	item("reopened-review", "reopened", "pending_review")
	// A running sync: untouched, including its waiting child.
	job("running", "sync", "processing", "1 hour", false, false)
	item("running-waiting", "running", "pending")
	item("running-review", "running", "pending_review")
	// Unfinished items of finished syncs.
	job("cancelled", "sync", "cancelled", "5 hours", true, true)
	item("cancelled-review", "cancelled", "pending_review")
	item("cancelled-pending", "cancelled", "pending")
	job("failed", "sync", "failed", "5 hours", true, true)
	item("failed-review", "failed", "pending_review")
	job("done", "sync", "completed", "5 hours", true, true)
	item("done-pending", "done", "pending")
	item("done-review", "done", "pending_review")
	item("done-completed", "done", "completed")
	// Another kind: untouched.
	job("import", "import", "pending", "2 days", true, false)

	if _, err := bunmigrate.NewMigrator(db, migrations.Migrations).Migrate(ctx); err != nil {
		t.Fatalf("migrate stage 3: %v", err)
	}

	type jobRow struct {
		Status           string  `bun:"status"`
		ErrorMessage     *string `bun:"error_message"`
		DispatchComplete bool    `bun:"dispatch_complete"`
		CompletedAt      *string `bun:"completed_at"`
	}
	readJob := func(id string) jobRow {
		t.Helper()
		var r jobRow
		if err := db.NewRaw(`SELECT status, error_message, dispatch_complete, completed_at::text FROM jobs WHERE id = ?`, id).Scan(ctx, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	type itemRow struct {
		Status      string  `bun:"status"`
		Msg         *string `bun:"error_message"`
		ProcessedAt *string `bun:"processed_at"`
	}
	readItem := func(id string) itemRow {
		t.Helper()
		var r itemRow
		if err := db.NewRaw(`SELECT status, error_message, processed_at::text FROM job_items WHERE id = ?`, id).Scan(ctx, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	wantFailed := func(id, msg string) {
		t.Helper()
		if r := readJob(id); r.Status != "failed" || r.CompletedAt == nil || r.ErrorMessage == nil || *r.ErrorMessage != msg {
			t.Errorf("%s = %+v, want failed with %q", id, r, msg)
		}
	}

	if r := readJob("fresh"); r.Status != "pending" || r.DispatchComplete {
		t.Errorf("fresh = %+v, want pending with its dispatch not done", r)
	}
	wantFailed("ancient", "sync never started; start a new one")
	wantFailed("reopened", "sync did not finish; start a new one")
	if r := readItem("reopened-review"); r.Status != "cancelled" || r.ProcessedAt == nil {
		t.Errorf("reopened-review = %+v, want cancelled with its failed sync", r)
	}
	if r := readJob("running"); r.Status != "processing" || r.DispatchComplete {
		t.Errorf("running = %+v, want untouched", r)
	}
	for _, id := range []string{"running-waiting", "running-review"} {
		if r := readItem(id); r.Status == "cancelled" || r.Status == "failed" || r.ProcessedAt != nil {
			t.Errorf("%s = %+v, want untouched", id, r)
		}
	}
	for _, id := range []string{"cancelled-review", "cancelled-pending", "failed-review"} {
		if r := readItem(id); r.Status != "cancelled" || r.ProcessedAt == nil {
			t.Errorf("%s = %+v, want cancelled with processed_at", id, r)
		}
	}
	for _, id := range []string{"done-pending", "done-review"} {
		if r := readItem(id); r.Status != "failed" || r.ProcessedAt == nil || r.Msg == nil || *r.Msg != "interrupted by an upgrade; retry it" {
			t.Errorf("%s = %+v, want failed so it can be retried", id, r)
		}
	}
	if r := readItem("done-completed"); r.Status != "completed" {
		t.Errorf("done-completed = %+v, want untouched", r)
	}
	if r := readJob("import"); r.Status != "pending" || !r.DispatchComplete {
		t.Errorf("import = %+v, want untouched", r)
	}
}
