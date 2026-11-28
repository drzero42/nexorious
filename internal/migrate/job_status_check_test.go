package migrate_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
	"github.com/drzero42/nexorious/internal/db/models"
)

const jobStatusCheckMigration = "20261128000001"

// The migration that adds the jobs.status and job_items.status CHECK
// constraints (#1513) first fails the rows whose status is unknown, among them
// the 'igdb_failed' items older installs may hold, and changes no other row.
func TestJobStatusCheckMigration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrationsBefore(jobStatusCheckMigration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before the status check: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`)
	job := func(id, status string) {
		exec(`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		      VALUES (?, 'u1', 'sync', 'steam', ?, 'high')`, id, status)
	}
	item := func(id, jobID, status string, errMsg any) {
		exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status, error_message)
		      VALUES (?, ?, 'u1', ?, 't', ?, ?)`, id, jobID, id, status, errMsg)
	}

	// A completed Job holding the removed 'igdb_failed' status and an unknown one.
	job("done", "completed")
	item("igdb", "done", "igdb_failed", nil)
	item("igdb-msg", "done", "igdb_failed", "IGDB timed out")
	item("bogus", "done", "bogus", nil)
	item("ok-completed", "done", "completed", nil)
	item("ok-review", "done", "pending_review", nil)
	// A Job with an unknown status: failed, its open items cancelled.
	job("weird", "running")
	item("weird-pending", "weird", "pending", nil)
	item("weird-review", "weird", "pending_review", nil)
	item("weird-done", "weird", "completed", nil)
	// An active Job is left alone.
	job("active", "processing")
	item("active-pending", "active", "pending", nil)

	if _, err := bunmigrate.NewMigrator(db, migrations.Migrations).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	type jobRow struct {
		ID           string         `bun:"id"`
		Status       string         `bun:"status"`
		ErrorMessage sql.NullString `bun:"error_message"`
		CompletedAt  sql.NullTime   `bun:"completed_at"`
	}
	var jobs []jobRow
	if err := db.NewRaw(`SELECT id, status, error_message, completed_at FROM jobs`).Scan(ctx, &jobs); err != nil {
		t.Fatal(err)
	}
	wantJobs := map[string]string{"done": "completed", "weird": "failed", "active": "processing"}
	for _, j := range jobs {
		if j.Status != wantJobs[j.ID] {
			t.Errorf("job %s status = %q, want %q", j.ID, j.Status, wantJobs[j.ID])
		}
		if j.ID == "weird" {
			if !strings.Contains(j.ErrorMessage.String, "'running'") || !j.CompletedAt.Valid {
				t.Errorf("job weird = %+v, want completed_at and an error naming 'running'", j)
			}
		}
	}

	type itemRow struct {
		ID           string         `bun:"id"`
		Status       string         `bun:"status"`
		ErrorMessage sql.NullString `bun:"error_message"`
		ProcessedAt  sql.NullTime   `bun:"processed_at"`
	}
	var items []itemRow
	if err := db.NewRaw(`SELECT id, status, error_message, processed_at FROM job_items`).Scan(ctx, &items); err != nil {
		t.Fatal(err)
	}
	wantItems := map[string]string{
		"igdb": "failed", "igdb-msg": "failed", "bogus": "failed",
		"ok-completed": "completed", "ok-review": "pending_review",
		"weird-pending": "cancelled", "weird-review": "cancelled", "weird-done": "completed",
		"active-pending": "pending",
	}
	for _, it := range items {
		if it.Status != wantItems[it.ID] {
			t.Errorf("item %s status = %q, want %q", it.ID, it.Status, wantItems[it.ID])
		}
		switch it.ID {
		case "igdb", "bogus":
			if !strings.Contains(it.ErrorMessage.String, "unknown status") || !it.ProcessedAt.Valid {
				t.Errorf("item %s = %+v, want processed_at and an unknown-status error", it.ID, it)
			}
		case "igdb-msg":
			if it.ErrorMessage.String != "IGDB timed out" {
				t.Errorf("item igdb-msg error = %q, want its own message kept", it.ErrorMessage.String)
			}
		case "weird-pending", "weird-review":
			if !it.ProcessedAt.Valid {
				t.Errorf("item %s has no processed_at", it.ID)
			}
		}
	}
}

// The CHECK constraints accept exactly the statuses of models.JobStatus and
// models.JobItemStatus: a status added to the Go types without a migration
// that widens the constraint fails here.
func TestJobStatusCheckMatchesModels(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`); err != nil {
		t.Fatal(err)
	}

	insertJob := func(id, status string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO jobs (id, user_id, job_type, source, status, priority)
			VALUES (?, 'u1', 'sync', 'steam', ?, 'high')`, id, status)
		return err
	}
	insertItem := func(id, status string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status)
			VALUES (?, 'j-host', 'u1', ?, 't', ?)`, id, id, status)
		return err
	}

	jobStatuses := append(models.ActiveJobStatuses(), models.EndedJobStatuses()...)
	for _, s := range jobStatuses {
		if err := insertJob("j-"+string(s), string(s)); err != nil {
			t.Errorf("jobs.status %q rejected: %v", s, err)
		}
	}
	if err := insertJob("j-host", string(models.JobStatusProcessing)); err != nil {
		t.Fatal(err)
	}
	for _, s := range models.JobItemStatuses() {
		if err := insertItem("i-"+string(s), string(s)); err != nil {
			t.Errorf("job_items.status %q rejected: %v", s, err)
		}
	}

	if err := insertJob("j-bogus", "bogus"); err == nil || !strings.Contains(err.Error(), "jobs_status_check") {
		t.Errorf("jobs.status 'bogus' = %v, want a jobs_status_check violation", err)
	}
	if err := insertItem("i-bogus", "igdb_failed"); err == nil || !strings.Contains(err.Error(), "job_items_status_check") {
		t.Errorf("job_items.status 'igdb_failed' = %v, want a job_items_status_check violation", err)
	}

	// The down step removes both constraints.
	down, err := migrations.FS.ReadFile(jobStatusCheckMigration + "_job_status_check.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := insertJob("j-bogus", "bogus"); err != nil {
		t.Errorf("jobs.status 'bogus' after down: %v", err)
	}
	if err := insertItem("i-bogus", "igdb_failed"); err != nil {
		t.Errorf("job_items.status 'igdb_failed' after down: %v", err)
	}
}
