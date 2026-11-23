package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Imports on the Job lifecycle module (#1267).

func countRows(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(q, args...).Scan(context.Background(), &n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestImportNexorious_RejectsUnusableFiles(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"no usable record", `{"version":"2.0","games":["x", 3]}`, "the file has no usable game records"},
		{"duplicate igdb_id", `{"version":"2.0","games":[{"igdb_id":5,"title":"A"},{"igdb_id":5,"title":"B"}]}`, "the file lists igdb_id 5 more than once"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			e := newTestEchoConfiguredIGDB(t, testDB, testCfg(), testIGDBClient(true))
			_, token := setupTagUser(t, testDB, e, "imp-unusable")
			rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", []byte(tc.body), token)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("status %d body %s, want 400 %q", rec.Code, rec.Body, tc.want)
			}
			if n := countRows(t, `SELECT count(*) FROM jobs`); n != 0 {
				t.Errorf("jobs created: %d", n)
			}
		})
	}
}

// The duplicate check runs under the advisory lock with the insert: two
// uploads at once create one Job.
func TestImportNexorious_ConcurrentUploadsCreateOneJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoConfiguredIGDB(t, testDB, testCfg(), testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "imp-concurrent")
	data := validExportJSON(t, 20)

	const n = 5
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			codes[i] = postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token).Code
		})
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if ok != 1 || countRows(t, `SELECT count(*) FROM jobs`) != 1 {
		t.Fatalf("codes %v, jobs %d; want exactly one created", codes, countRows(t, `SELECT count(*) FROM jobs`))
	}
}

// Create is one transaction: an upload that fails part-way leaves no Job, no
// items and no River jobs.
func TestImportNexorious_FailedUploadCreatesNothing(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoConfiguredIGDB(t, testDB, testCfg(), testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "imp-partway")
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := testDB.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`CREATE FUNCTION fail_item() RETURNS trigger LANGUAGE plpgsql AS $$
	      BEGIN IF NEW.source_title = 'Game 3' THEN RAISE EXCEPTION 'injected'; END IF; RETURN NEW; END $$`)
	exec(`CREATE TRIGGER fail_item BEFORE INSERT ON job_items FOR EACH ROW EXECUTE FUNCTION fail_item()`)
	t.Cleanup(func() {
		exec(`DROP TRIGGER fail_item ON job_items`)
		exec(`DROP FUNCTION fail_item()`)
	})

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", validExportJSON(t, 5), token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", rec.Code, rec.Body)
	}
	for _, q := range []string{`SELECT count(*) FROM jobs`, `SELECT count(*) FROM job_items`, `SELECT count(*) FROM river_job WHERE kind = 'import_item'`} {
		if n := countRows(t, q); n != 0 {
			t.Errorf("%s = %d, want 0", q, n)
		}
	}
}

func TestResolveAndSkip_Refusals(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "ji-refusals")

	insertJob(t, testDB, "job-csv", userID, "import", "csv", "processing")
	insertJobItem(t, testDB, "csv-pending", "job-csv", userID, "k1", "Pending", "pending")
	insertJob(t, testDB, "job-nx", userID, "import", "nexorious", "processing")
	insertJobItem(t, testDB, "nx-1", "job-nx", userID, "k2", "Nexorious", "pending")
	insertJob(t, testDB, "job-done", userID, "import", "csv", "completed")
	insertJobItem(t, testDB, "done-review", "job-done", userID, "k3", "Old", "pending_review")

	for _, tc := range []struct {
		item string
		want int
	}{
		{"csv-pending", http.StatusConflict},  // not in Pending review
		{"nx-1", http.StatusBadRequest},       // no review stage, checked before 409
		{"done-review", http.StatusConflict},  // Job not active
		{"no-such-item", http.StatusNotFound}, // not found
	} {
		for _, action := range []string{"resolve", "skip"} {
			rec := postJSONAuth(t, e, fmt.Sprintf("/api/job-items/%s/%s", tc.item, action), map[string]any{"igdb_id": 9}, token)
			if rec.Code != tc.want {
				t.Errorf("%s %s: status %d, want %d: %s", action, tc.item, rec.Code, tc.want, rec.Body)
			}
		}
	}
}

// Resolve leaves the item pending with a finalize River job; a source removed
// from the registry is still an import with a review stage.
func TestResolveItem_PendingWithFinalizeOwner(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "ji-resolve-owner")
	insertJob(t, testDB, "job-old", userID, "import", "darkadia", "processing")
	insertJobItem(t, testDB, "old-1", "job-old", userID, "k", "Old Game", "pending_review")

	rec := postJSONAuth(t, e, "/api/job-items/old-1/resolve", map[string]any{"igdb_id": 7}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if n := countRows(t, `SELECT count(*) FROM job_items ji JOIN river_job rj ON rj.id = ji.river_job_id
	                       WHERE ji.id = 'old-1' AND ji.status = 'pending' AND rj.kind = 'import_finalize'`); n != 1 {
		t.Error("resolved item is not pending with an import_finalize owner")
	}
}

func TestSkipItem_LastReviewItemCompletesJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "ji-skip-last")
	insertJob(t, testDB, "job-last", userID, "import", "csv", "processing")
	insertJobItem(t, testDB, "last-done", "job-last", userID, "k1", "Done", "completed")
	insertJobItem(t, testDB, "last-review", "job-last", userID, "k2", "Review", "pending_review")

	rec := postJSONAuth(t, e, "/api/job-items/last-review/skip", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if n := countRows(t, `SELECT count(*) FROM jobs WHERE id = 'job-last' AND status = 'completed'`); n != 1 {
		t.Error("skipping the last item in review did not complete the Job")
	}
}
