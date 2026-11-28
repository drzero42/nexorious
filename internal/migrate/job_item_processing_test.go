package migrate_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

const dropJobItemProcessingMigration = "20261128000003"

// The migration that removes the Job item status 'processing' (#1516) moves
// any item still holding it by the rules of the stage 2 migration, changes no
// other row, and narrows job_items_status_check so the value is rejected.
func TestDropJobItemProcessingMigration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrationsBefore(dropJobItemProcessingMigration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before dropping processing: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`)
	for _, status := range []string{"pending", "processing", "completed", "failed", "cancelled"} {
		exec(`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		      VALUES (?, 'u1', 'import', 'csv', ?, 'high')`, status, status)
	}
	// Every item names a River job, so the test sees which ones the migration clears.
	item := func(id, jobID, status string) {
		exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status, river_job_id)
		      VALUES (?, ?, 'u1', ?, 't', ?, 2)`, id, jobID, id, status)
	}
	for _, jobID := range []string{"pending", "processing", "completed", "failed", "cancelled"} {
		item(jobID+"-processing", jobID, "processing")
	}
	item("processing-review", "processing", "pending_review")
	item("completed-done", "completed", "completed")
	item("failed-failed", "failed", "failed")

	if _, err := bunmigrate.NewMigrator(db, migrations.Migrations).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	type itemRow struct {
		ID           string         `bun:"id"`
		Status       string         `bun:"status"`
		ErrorMessage sql.NullString `bun:"error_message"`
		ProcessedAt  sql.NullTime   `bun:"processed_at"`
		RiverJobID   sql.NullInt64  `bun:"river_job_id"`
	}
	var items []itemRow
	if err := db.NewRaw(`SELECT id, status, error_message, processed_at, river_job_id FROM job_items`).Scan(ctx, &items); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		// An active Job's item goes back to a worker; recovery adopts or replaces its River job.
		"pending-processing":    "pending",
		"processing-processing": "pending",
		// A completed Job's item can be retried; an ended Job's other items are cancelled.
		"completed-processing": "failed",
		"failed-processing":    "cancelled",
		"cancelled-processing": "cancelled",
		// Other statuses are left alone.
		"processing-review": "pending_review",
		"completed-done":    "completed",
		"failed-failed":     "failed",
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d", len(items), len(want))
	}
	for _, it := range items {
		if it.Status != want[it.ID] {
			t.Errorf("item %s status = %q, want %q", it.ID, it.Status, want[it.ID])
		}
		switch it.ID {
		case "completed-processing":
			if !strings.Contains(it.ErrorMessage.String, "retry it") || !it.ProcessedAt.Valid || it.RiverJobID.Valid {
				t.Errorf("item %s = %+v, want processed_at, no river_job_id and a retry message", it.ID, it)
			}
		case "failed-processing", "cancelled-processing":
			if !it.ProcessedAt.Valid || it.RiverJobID.Valid {
				t.Errorf("item %s = %+v, want processed_at and no river_job_id", it.ID, it)
			}
		default:
			if !it.RiverJobID.Valid || it.ProcessedAt.Valid {
				t.Errorf("item %s = %+v, want its river_job_id kept and no processed_at", it.ID, it)
			}
		}
	}

	insertItem := func(id, status string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status)
			VALUES (?, 'processing', 'u1', ?, 't', ?)`, id, id, status)
		return err
	}
	if err := insertItem("new-processing", "processing"); err == nil || !strings.Contains(err.Error(), "job_items_status_check") {
		t.Errorf("job_items.status 'processing' = %v, want a job_items_status_check violation", err)
	}

	// The down step accepts the value again.
	down, err := migrations.FS.ReadFile(dropJobItemProcessingMigration + "_drop_job_item_processing.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := insertItem("new-processing", "processing"); err != nil {
		t.Errorf("job_items.status 'processing' after down: %v", err)
	}
	if err := insertItem("new-bogus", "bogus"); err == nil || !strings.Contains(err.Error(), "job_items_status_check") {
		t.Errorf("job_items.status 'bogus' after down = %v, want a job_items_status_check violation", err)
	}
}
