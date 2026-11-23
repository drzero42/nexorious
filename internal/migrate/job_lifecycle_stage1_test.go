package migrate_test

import (
	"context"
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

const stage1Migration = "20261123000002"

// migrationsBefore returns every migration whose name sorts before name.
func migrationsBefore(name string) *bunmigrate.Migrations {
	out := bunmigrate.NewMigrations()
	for _, m := range migrations.Migrations.Sorted() {
		if m.Name >= name {
			break
		}
		out.Add(m)
	}
	return out
}

// The Job lifecycle stage 1 migration (#1266) puts the rows of the kinds that
// move onto internal/joblifecycle into a state its rules understand, and
// leaves every other kind alone.
func TestJobLifecycleStage1Migration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	// Everything before the stage 1 migration.
	mg := bunmigrate.NewMigrator(db, migrationsBefore(stage1Migration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before stage 1: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`)
	job := func(id, jobType, status, age string) {
		exec(`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
		      VALUES (?, 'u1', ?, 'system', ?, 'low', now() - ?::interval)`, id, jobType, status, age)
	}
	item := func(id, jobID, status string) {
		exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status) VALUES (?, ?, 'u1', ?, 't', ?)`,
			id, jobID, id, status)
	}
	// A refresh an admin started that never began its dispatch (no items):
	// dispatch_complete defaults to true, which would complete it empty.
	job("refresh-pending", "metadata_refresh", "pending", "1 hour")
	// An export stuck processing for days (#1303), and a recent one.
	job("export-stuck", "export", "processing", "3 days")
	job("export-recent", "export", "pending", "5 minutes")
	// A refresh that has started (has items) is not a never-dispatched one.
	job("refresh-running", "store_link_refresh", "processing", "1 hour")
	item("running-item", "refresh-running", "pending")
	// Unfinished items of finished Jobs.
	job("refresh-failed", "metadata_refresh", "failed", "1 hour")
	item("failed-job-item", "refresh-failed", "pending")
	job("refresh-done", "store_link_refresh", "completed", "1 hour")
	item("done-job-item", "refresh-done", "pending")
	// Kinds not on the module are left alone.
	job("sync-active", "sync", "pending", "3 days")
	job("import-failed", "import", "failed", "1 hour")
	item("import-item", "import-failed", "pending")

	// Up to stage 1 only: stage 2 (#1267) moves the import kinds.
	if _, err := bunmigrate.NewMigrator(db, migrationsBefore(stage2Migration)).Migrate(ctx); err != nil {
		t.Fatalf("migrate stage 1: %v", err)
	}

	type jobRow struct {
		Status           string  `bun:"status"`
		DispatchComplete bool    `bun:"dispatch_complete"`
		ErrorMessage     *string `bun:"error_message"`
		CompletedAt      *string `bun:"completed_at"`
	}
	readJob := func(id string) jobRow {
		t.Helper()
		var r jobRow
		if err := db.NewRaw(`SELECT status, dispatch_complete, error_message, completed_at::text FROM jobs WHERE id = ?`, id).Scan(ctx, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	readItem := func(id string) (status string, msg *string) {
		t.Helper()
		if err := db.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, id).Scan(ctx, &status, &msg); err != nil {
			t.Fatal(err)
		}
		return status, msg
	}

	if r := readJob("refresh-pending"); r.Status != "pending" || r.DispatchComplete {
		t.Errorf("refresh-pending = %+v, want pending with its dispatch not done (recovery runs it)", r)
	}
	if r := readJob("export-stuck"); r.Status != "failed" || r.CompletedAt == nil || r.ErrorMessage == nil || *r.ErrorMessage != "interrupted; start it again" {
		t.Errorf("export-stuck = %+v, want failed", r)
	}
	if r := readJob("export-recent"); r.Status != "pending" || r.DispatchComplete {
		t.Errorf("export-recent = %+v, want pending with its dispatch not done", r)
	}
	if r := readJob("refresh-running"); r.Status != "processing" || !r.DispatchComplete {
		t.Errorf("refresh-running = %+v, want untouched", r)
	}
	if s, _ := readItem("running-item"); s != "pending" {
		t.Errorf("running-item = %s, want pending", s)
	}
	if s, _ := readItem("failed-job-item"); s != "cancelled" {
		t.Errorf("item of a failed job = %s, want cancelled", s)
	}
	if s, msg := readItem("done-job-item"); s != "failed" || msg == nil {
		t.Errorf("item of a completed job = %s (%v), want failed so it can be retried", s, msg)
	}
	if r := readJob("sync-active"); r.Status != "pending" || !r.DispatchComplete {
		t.Errorf("sync job touched: %+v", r)
	}
	if s, _ := readItem("import-item"); s != "pending" {
		t.Errorf("import item touched: %s", s)
	}
	var cols int
	if err := db.NewRaw(`SELECT count(*) FROM information_schema.columns WHERE column_name = 'river_job_id' AND table_name IN ('jobs', 'job_items')`).Scan(ctx, &cols); err != nil {
		t.Fatal(err)
	}
	if cols != 2 {
		t.Errorf("river_job_id columns = %d, want 2", cols)
	}
}
