package tasks_test

// Sync on the Job lifecycle module (#1268): dispatch ownership, failures and
// interruptions, the write stage's single transaction (#1329), editions in
// review, recovery, effects and metrics.

import (
	"context"
	"errors"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/observability"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// A dispatch hard-killed after it marked the dispatch done is run again by
// River. It must not fetch the library a second time.
func TestDispatchSync_RunAgainAfterMarkedDone_DoesNotFetch(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, "pending")
	adapter := &hookAdapter{batches: [][]tasks.ExternalGameEntry{{{ExternalID: "1", Title: "Game One", Platforms: []string{"pc-windows"}}}}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})

	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if _, done := loadJobState(t, jobID); !done {
		t.Fatal("first run did not mark the dispatch done")
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if adapter.calls != 1 {
		t.Errorf("library fetched %d times, want 1", adapter.calls)
	}
}

// A database error while a batch's items are added is returned to River,
// which runs the dispatch again; the sync is not failed.
func TestDispatchSync_BatchDBError_RetriesWithoutFailing(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID, jobID := seedGOGSyncJob(t, "pending")
	for _, stmt := range []string{
		`CREATE FUNCTION sl_refuse_item() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''item insert refused''; END'`,
		`CREATE TRIGGER sl_refuse_item BEFORE INSERT ON job_items FOR EACH ROW EXECUTE FUNCTION sl_refuse_item()`,
	} {
		if _, err := testDB.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS sl_refuse_item ON job_items`)
		_, _ = testDB.ExecContext(ctx, `DROP FUNCTION IF EXISTS sl_refuse_item()`)
	})
	adapter := &hookAdapter{batches: [][]tasks.ExternalGameEntry{{{ExternalID: "1", Title: "Game One", Platforms: []string{"pc-windows"}}}}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: newTestLC(t)}

	err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"}))
	if err == nil {
		t.Fatal("want the database error returned to River")
	}
	if status, done := loadJobState(t, jobID); status != models.JobStatusProcessing || done {
		t.Errorf("job = %q / dispatch done %v, want processing and not done", status, done)
	}
	if n := countRows(t, `SELECT count(*) FROM river_job WHERE kind = ?`, joblifecycle.EffectsArgs{}.Kind()); n != 0 {
		t.Errorf("%d ending effects queued, want none", n)
	}
}

// credentialsOnCancelAdapter is a PlayStation-like adapter: a request cut off
// by a shutdown comes back as an authentication error.
type credentialsOnCancelAdapter struct{ cancel context.CancelFunc }

func (a credentialsOnCancelAdapter) GetLibrary(context.Context, int, func([]tasks.ExternalGameEntry) error) error {
	a.cancel()
	return tasks.ErrCredentials
}

// A shutdown during the fetch is not recorded as expired credentials.
func TestDispatchSync_ShutdownDuringFetch_NotRecordedAsExpired(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, "pending")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(credentialsOnCancelAdapter{cancel: cancel}), LC: newTestLC(t)}

	runInterrupted(t, ctx, w, jobID, userID)

	var flag bool
	if err := testDB.NewRaw(`SELECT credentials_error FROM user_sync_configs WHERE user_id = ?`, userID).Scan(context.Background(), &flag); err != nil {
		t.Fatal(err)
	}
	if flag {
		t.Error("a shutdown was recorded as expired credentials")
	}
	if status, _ := loadJobState(t, jobID); status != models.JobStatusProcessing {
		t.Errorf("job = %q, want processing (it resumes after the restart)", status)
	}
}

// Every lifecycle River kind has a finite timeout; a sync dispatch gets a
// generous one that tests can shorten.
func TestDispatchSync_Timeout(t *testing.T) {
	if got := (&tasks.DispatchSyncWorker{}).Timeout(nil); got != 2*time.Hour {
		t.Errorf("default timeout = %v, want 2h", got)
	}
	if got := (&tasks.DispatchSyncWorker{RunTimeout: time.Second}).Timeout(nil); got != time.Second {
		t.Errorf("injected timeout = %v, want 1s", got)
	}
}

// A dispatch that River discarded (its last attempt timed out or crashed) is
// failed by recovery, and the user gets sync.failed with the Job's message.
func TestSyncRecovery_DiscardedDispatch_FailsAndNotifies(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	lc := newTestLC(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	c, err := lc.CreateWithDispatch(ctx, joblifecycle.NewJob{UserID: userID, JobType: models.JobTypeSync, Source: "steam", Priority: models.JobPriorityLow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `UPDATE river_job SET state = 'discarded', finalized_at = now() WHERE id = ?`, dispatchOwner(t, c.JobID)); err != nil {
		t.Fatal(err)
	}

	if _, err := lc.Recover(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if got := jobStatus(t, c.JobID); got != models.JobStatusFailed {
		t.Fatalf("job = %q, want failed", got)
	}
	runSyncEffects(t, lc)
	var payload string
	if err := testDB.NewRaw(`SELECT payload::text FROM events WHERE dedup_key = ?`, c.JobID+":"+notify.TypeSyncFailed).Scan(ctx, &payload); err != nil {
		t.Fatalf("sync.failed event: %v", err)
	}
	if !strings.Contains(payload, "gave up") {
		t.Errorf("payload = %s, want the Job's error message", payload)
	}
}

// A sync cancelled after it started waiting on the user sends no
// sync.needs_review: the effect checks the Job is still active.
func TestSyncEffects_CancelledSyncSendsNoNeedsReview(t *testing.T) {
	truncateAllTables(t)
	lc := newTestLC(t)
	_, jobID, items := syncCompletionFixture(t, "steam", true, 1, models.JobItemStatusPendingReview)
	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	if err := lc.Cancel(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	runSyncEffects(t, lc)
	if n := eventCount(t, jobID+":"+notify.TypeSyncNeedsReview); n != 0 {
		t.Errorf("sync.needs_review events = %d, want 0", n)
	}
}

// writeFixture is a matched game with a pending item ready for the write
// stage.
func writeFixture(t *testing.T) (userID, jobID, egID, itemID string) {
	t.Helper()
	userID = edSetup(t)
	egID = edInsertGame(t, userID, edGame{extID: "W", available: true, resolved: 4242})
	jobID = edInsertJob(t, userID, "processing", true)
	itemID = edInsertItem(t, jobID, userID, egID, "pending")
	return userID, jobID, egID, itemID
}

// A write interrupted after the library change rolls back with the item's
// transition: neither the library row nor the 'added' row is left, and the
// run again writes one 'added' row (#1329).
func TestUserGameWorker_InterruptedWrite_LeavesNothing_RerunWritesOnce(t *testing.T) {
	ctx := context.Background()
	userID, jobID, _, itemID := writeFixture(t)
	for _, stmt := range []string{
		`CREATE SEQUENCE sl_change_tries`,
		`CREATE FUNCTION sl_refuse_first_change() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN IF nextval(''sl_change_tries'') = 1 THEN RAISE EXCEPTION ''interrupted''; END IF; RETURN NEW; END'`,
		`CREATE TRIGGER sl_refuse_first_change BEFORE INSERT ON changes FOR EACH ROW EXECUTE FUNCTION sl_refuse_first_change()`,
	} {
		if _, err := testDB.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS sl_refuse_first_change ON changes`)
		_, _ = testDB.ExecContext(ctx, `DROP FUNCTION IF EXISTS sl_refuse_first_change()`)
		_, _ = testDB.ExecContext(ctx, `DROP SEQUENCE IF EXISTS sl_change_tries`)
	})
	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	job := writeJob(t, tasks.UserGameArgs{JobItemID: itemID})

	if err := w.Work(ctx, job); err == nil {
		t.Fatal("want the interrupted write's error returned to River")
	}
	if n := ugCountFor(t, userID, 4242); n != 0 {
		t.Errorf("user games after the interrupted write: %d, want 0", n)
	}
	if n := edCount(t, `SELECT count(*) FROM changes WHERE job_id = ?`, jobID); n != 0 {
		t.Errorf("changes after the interrupted write: %d, want 0", n)
	}
	if s, _ := edItemStatus(t, itemID); s != models.JobItemStatusPending {
		t.Errorf("item = %q, want pending", s)
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if n := ugCountFor(t, userID, 4242); n != 1 {
		t.Errorf("user games after the re-run: %d, want 1", n)
	}
	if n := edCount(t, `SELECT count(*) FROM changes WHERE job_id = ? AND change_type = 'added'`, jobID); n != 1 {
		t.Errorf("'added' rows after the re-run: %d, want 1", n)
	}
}

// A user_game_write run again after its commit, and a second River job for
// the same item, write nothing (#1329).
func TestUserGameWorker_RunAgainOrDuplicate_WritesNothing(t *testing.T) {
	ctx := context.Background()
	userID, jobID, _, itemID := writeFixture(t)
	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	job := writeJob(t, tasks.UserGameArgs{JobItemID: itemID})

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("run again: %v", err)
	}
	if err := w.Work(ctx, riverJob(nextFakeOwner(), 1, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if n := ugCountFor(t, userID, 4242); n != 1 {
		t.Errorf("user games: %d, want 1", n)
	}
	if n := edCount(t, `SELECT count(*) FROM changes WHERE job_id = ?`, jobID); n != 1 {
		t.Errorf("changes rows: %d, want 1", n)
	}
	if n := edCount(t, `SELECT count(*) FROM user_game_platforms`); n != 1 {
		t.Errorf("user game platforms: %d, want 1", n)
	}
}

// The removed-game sweep leaves is_available = false and its 'removed' row
// together, and a dispatch that sweeps twice writes one row (#1329).
func TestDispatchSync_RemovedGameSweep_Twice_WritesOneRow(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID, jobID := seedGOGSyncJob(t, "pending")
	goneID := insertTestExternalGame(t, userID, "gog", "gone", "Gone Game", "pc-windows")
	batches := [][]tasks.ExternalGameEntry{{{ExternalID: "1", Title: "Kept", Platforms: []string{"pc-windows"}}}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&hookAdapter{batches: batches}), LC: newTestLC(t)}

	runDispatch(t, ctx, w, jobID, userID)
	// As if the first run was cut off after its sweep, before it marked the
	// dispatch done.
	if _, err := testDB.ExecContext(ctx, `UPDATE jobs SET dispatch_complete = false WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	runDispatch(t, ctx, w, jobID, userID)

	if n := countRows(t, `SELECT count(*) FROM external_games WHERE id = ? AND NOT is_available`, goneID); n != 1 {
		t.Error("the game that left is still available")
	}
	if n := countRows(t, `SELECT count(*) FROM changes WHERE job_id = ? AND change_type = 'removed'`, jobID); n != 1 {
		t.Errorf("'removed' rows: %d, want 1", n)
	}
}

// Two editions without a decision in one sync are both searched (no IGDB here)
// and both end in Pending review; the sync waits on the user and says so once.
func TestSync_TwoUndecidedEditions_BothInReview_NeedsReviewOnce(t *testing.T) {
	userID := edSetup(t)
	jobID := edInsertJob(t, userID, "pending", true)
	edDispatch(t, nil, jobID, userID, []tasks.ExternalGameEntry{edEntry("A")}, []tasks.ExternalGameEntry{edEntry("B")})
	lc := newTestLC(t)
	drainSync(t, &tasks.IGDBMatchWorker{DB: testDB, LC: lc}, &tasks.UserGameWorker{DB: testDB, LC: lc})

	for _, ext := range []string{"A", "B"} {
		id, eg := edGet(t, userID, ext)
		if eg.Resolved != nil || eg.IsSkipped {
			t.Errorf("%s = %+v, want no decision", ext, eg)
		}
		if s, _ := edItemStatus(t, edItemsFor(t, jobID, id)[0]); s != models.JobItemStatusPendingReview {
			t.Errorf("%s's item = %q, want pending_review", ext, s)
		}
	}
	if got := jobStatus(t, jobID); got != models.JobStatusProcessing {
		t.Fatalf("job = %q, want processing", got)
	}
	runSyncEffects(t, lc)
	runSyncEffects(t, lc)
	if n := eventCount(t, jobID+":"+notify.TypeSyncNeedsReview); n != 1 {
		t.Errorf("sync.needs_review events = %d, want 1", n)
	}
}

// scrapeMetric sums the values of a metric's series whose labels contain all
// of want.
func scrapeMetric(t *testing.T, name string, want ...string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	observability.MetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	total := 0
	re := regexp.MustCompile(`^` + name + `\{([^}]*)\} ([0-9.e+]+)$`)
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ok := true
		for _, w := range want {
			if !strings.Contains(m[1], w) {
				ok = false
			}
		}
		if ok {
			v, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				t.Fatal(err)
			}
			total += int(v)
		}
	}
	return total
}

func initMetrics(t *testing.T) {
	t.Helper()
	prov, err := observability.Init(&config.Config{OTELServiceName: "nexorious-test", OTELMetricsEnabled: true}, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })
}

// Items are counted as they end, also in a sync that later fails; the
// failed sync counts once in the outcome counter.
func TestSyncMetrics_ItemsOfASyncThatLaterFails(t *testing.T) {
	initMetrics(t)
	truncateAllTables(t)
	ctx := context.Background()
	userID, jobID := seedGOGSyncJob(t, "pending")
	lc := newTestLC(t)
	adapter := &hookAdapter{
		batches: [][]tasks.ExternalGameEntry{{{ExternalID: "1", Title: "Game One", Platforms: []string{"pc-windows"}}}},
		err:     errors.New("upstream exploded"),
	}
	// The item ends while the dispatch is still fetching.
	adapter.afterFetch = func() {
		var itemID string
		if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemID); err != nil {
			t.Errorf("load item: %v", err)
			return
		}
		endSyncItem(t, lc, itemID, models.JobItemStatusCompleted)
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: lc}
	runDispatch(t, ctx, w, jobID, userID)

	if got := jobStatus(t, jobID); got != models.JobStatusFailed {
		t.Fatalf("job = %q, want failed", got)
	}
	if n := scrapeMetric(t, "nexorious_sync_items_total", `outcome="completed"`, `source="gog"`); n != 1 {
		t.Errorf("completed items counted = %d, want 1", n)
	}
	if n := scrapeMetric(t, "nexorious_sync_total", `status="failed"`, `source="gog"`); n != 1 {
		t.Errorf("failed syncs counted = %d, want 1", n)
	}
}

// A sync with a failed item that is retried and completes again adds one item
// to the item counter and nothing to the outcome counter.
func TestSyncMetrics_RetriedItemCountsOnceMore_OutcomeOnce(t *testing.T) {
	initMetrics(t)
	truncateAllTables(t)
	ctx := context.Background()
	lc := newTestLC(t)
	_, jobID, items := syncCompletionFixture(t, "steam", true, 1)
	endSyncItem(t, lc, items[0], models.JobItemStatusFailed)
	if scrapeMetric(t, "nexorious_sync_total", `source="steam"`) != 1 || scrapeMetric(t, "nexorious_sync_items_total", `source="steam"`) != 1 {
		t.Fatal("first ending not counted once")
	}
	if _, err := lc.Retry(ctx, jobID, items); err != nil {
		t.Fatal(err)
	}
	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	if got := jobStatus(t, jobID); got != models.JobStatusCompleted {
		t.Fatalf("job = %q, want completed", got)
	}
	if n := scrapeMetric(t, "nexorious_sync_items_total", `source="steam"`); n != 2 {
		t.Errorf("items counted = %d, want 2", n)
	}
	if n := scrapeMetric(t, "nexorious_sync_total", `source="steam"`); n != 1 {
		t.Errorf("sync outcomes counted = %d, want 1", n)
	}
}
