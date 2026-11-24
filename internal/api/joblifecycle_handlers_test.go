package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func insertJobWithCompletedAt(t *testing.T, id, userID, jobType, source, status string) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, 'low', now(), CASE WHEN ? IN ('completed','failed','cancelled') THEN now() END)`,
		id, userID, jobType, source, status, status,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
}

func jobItemRow(t *testing.T, id string) (status string, owner *int64) {
	t.Helper()
	if err := testDB.NewRaw(`SELECT status, river_job_id FROM job_items WHERE id = ?`, id).Scan(context.Background(), &status, &owner); err != nil {
		t.Fatalf("read item: %v", err)
	}
	return status, owner
}

// Retry on a failed or cancelled Job of a kind on the lifecycle module is
// refused with 409, before "no failed items" is considered: such a Job is
// never reopened.
func TestRetryFailed_LifecycleKindEndedJobIs409(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "lc-retry-409")

	for _, tc := range []struct{ id, jobType, source, status string }{
		{"lc-meta-failed", "metadata_refresh", "system", "failed"},
		{"lc-meta-cancelled", "metadata_refresh", "system", "cancelled"},
		{"lc-export-failed", "export", "nexorious", "failed"},
	} {
		insertJobWithCompletedAt(t, tc.id, userID, tc.jobType, tc.source, tc.status)
		if tc.jobType != "export" {
			insertJobItem(t, testDB, tc.id+"-f", tc.id, userID, "k", "G", "failed")
		}
		rec := postJSONAuth(t, e, "/api/jobs/"+tc.id+"/retry-failed", nil, token)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", tc.id, rec.Code, rec.Body.String())
		}
	}
	// A completed export with no failed items: 200, nothing to retry.
	insertJobWithCompletedAt(t, "lc-export-done", userID, "export", "csv", "completed")
	rec := postJSONAuth(t, e, "/api/jobs/lc-export-done/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("completed export: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Retry on a completed Job of a moved kind reopens it and gives each retried
// item its own River job.
func TestRetryFailed_LifecycleKindReopensCompletedJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "lc-retry-reopen")
	insertJobWithCompletedAt(t, "lc-meta-done", userID, "metadata_refresh", "system", "completed")
	insertJobItem(t, testDB, "lc-meta-done-f", "lc-meta-done", userID, "k1", "G", "failed")
	insertJobItem(t, testDB, "lc-meta-done-c", "lc-meta-done", userID, "k2", "G", "completed")

	rec := postJSONAuth(t, e, "/api/jobs/lc-meta-done/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = 'lc-meta-done'`).Scan(context.Background(), &status)
	if status != "processing" {
		t.Errorf("job = %s, want processing", status)
	}
	if s, owner := jobItemRow(t, "lc-meta-done-f"); s != "pending" || owner == nil || riverJobState(t, testDB, *owner) != "available" {
		t.Errorf("retried item = %s owner %v", s, owner)
	}
}

// Cancel of a moved kind cancels the Job's open items too, and the
// progress counts report them.
func TestCancelJob_LifecycleKindCancelsItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "lc-cancel")
	insertJob(t, testDB, "lc-cancel-job", userID, "metadata_refresh", "system", "processing")
	insertJobItem(t, testDB, "lc-cancel-i1", "lc-cancel-job", userID, "k1", "G1", "pending")
	insertJobItem(t, testDB, "lc-cancel-i2", "lc-cancel-job", userID, "k2", "G2", "pending")
	insertJobItem(t, testDB, "lc-cancel-i3", "lc-cancel-job", userID, "k3", "G3", "completed")

	rec := postJSONAuth(t, e, "/api/jobs/lc-cancel-job/cancel", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{"lc-cancel-i1", "lc-cancel-i2"} {
		if s, _ := jobItemRow(t, id); s != "cancelled" {
			t.Errorf("%s = %s, want cancelled", id, s)
		}
	}
	rec = getAuth(t, e, "/api/jobs/lc-cancel-job", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get job: %d", rec.Code)
	}
	var resp struct {
		Progress map[string]float64 `json:"progress"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Progress["cancelled"] != 2 || resp.Progress["total"] != 3 {
		t.Errorf("progress = %v, want cancelled 2 of 3", resp.Progress)
	}
	// A second cancel finds the Job ended.
	if rec := postJSONAuth(t, e, "/api/jobs/lc-cancel-job/cancel", nil, token); rec.Code != http.StatusConflict {
		t.Errorf("second cancel: %d", rec.Code)
	}
}

// In an admin-triggered store-link refresh the Job is the admin's while each
// item belongs to the storefront's user: an item retry is authorised on the
// Job's owner, so the admin can retry it and the item's user cannot reopen the
// admin's Job.
func TestRetryItem_LifecycleKindAuthorisedOnJobOwner(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	adminID, adminToken := setupAdminUser(t, testDB, e, "lc-retry-item")
	userID, userToken := setupRegularUser(t, testDB, e, "lc-retry-item")
	insertJobWithCompletedAt(t, "lc-sl-job", adminID, "store_link_refresh", "system", "completed")
	insertJobItem(t, testDB, "lc-sl-item", "lc-sl-job", userID, "steam", "steam", "failed")

	if rec := postJSONAuth(t, e, "/api/job-items/lc-sl-item/retry", nil, userToken); rec.Code != http.StatusNotFound {
		t.Fatalf("item's user: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if s, _ := jobItemRow(t, "lc-sl-item"); s != "failed" {
		t.Fatalf("refused retry changed the item: %s", s)
	}
	rec := postJSONAuth(t, e, "/api/job-items/lc-sl-item/retry", nil, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("job owner: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	s, owner := jobItemRow(t, "lc-sl-item")
	if s != "pending" || owner == nil {
		t.Fatalf("retried item = %s owner %v", s, owner)
	}
	var kind string
	_ = testDB.NewRaw(`SELECT kind FROM river_job WHERE id = ?`, *owner).Scan(context.Background(), &kind)
	if kind != "store_link_refresh_item" {
		t.Errorf("retried item queued as %q, want store_link_refresh_item", kind)
	}
	// Retrying an item that is not failed: 409.
	if rec := postJSONAuth(t, e, "/api/job-items/lc-sl-item/retry", nil, adminToken); rec.Code != http.StatusConflict {
		t.Errorf("retry of a pending item: %d", rec.Code)
	}
}

// The admin refresh triggers create the Job with its dispatch River job in
// one transaction, and a second trigger while one is active returns the same
// Job.
func TestStartMaintenanceRefresh_CreatesJobWithDispatch(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupAdminUser(t, testDB, e, "lc-refresh")

	for _, path := range []string{"/api/games/metadata/refresh-job", "/api/games/store-links/refresh-job"} {
		var ids []string
		for range 2 {
			rec := postJSONAuth(t, e, path, nil, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
			}
			var resp struct {
				JobID string `json:"job_id"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			ids = append(ids, resp.JobID)
		}
		if ids[0] == "" || ids[0] != ids[1] {
			t.Fatalf("%s: job ids %v, want the same active job twice", path, ids)
		}
		var status string
		var owner *int64
		var dispatchDone bool
		if err := testDB.NewRaw(`SELECT status, river_job_id, dispatch_complete FROM jobs WHERE id = ?`, ids[0]).
			Scan(context.Background(), &status, &owner, &dispatchDone); err != nil {
			t.Fatal(err)
		}
		if status != "pending" || owner == nil || dispatchDone {
			t.Fatalf("%s: job = %s owner %v dispatch_complete %v", path, status, owner, dispatchDone)
		}
		if state := riverJobState(t, testDB, *owner); state != "available" {
			t.Errorf("%s: dispatch river job = %s", path, state)
		}
	}
}
