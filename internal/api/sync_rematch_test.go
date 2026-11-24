package api_test

// Rematch and unskip write the library directly (#1338, ADR 0002).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

const rmStore = "playstation-store"

type rmFixture struct {
	t *testing.T
	e interface {
		ServeHTTP(http.ResponseWriter, *http.Request)
	}
	userID string
	token  string
}

func newRM(t *testing.T, name string) *rmFixture {
	t.Helper()
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB) // every route: sync, and the jobs badge
	userID, token := setupTagUser(t, testDB, e, name)
	return &rmFixture{t: t, e: e, userID: userID, token: token}
}

func (f *rmFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := testDB.ExecContext(context.Background(), q, args...); err != nil {
		f.t.Fatalf("exec %q: %v", q, err)
	}
}

func (f *rmFixture) count(q string, args ...any) int {
	f.t.Helper()
	var n int
	if err := testDB.NewRaw(q, args...).Scan(context.Background(), &n); err != nil {
		f.t.Fatalf("count %q: %v", q, err)
	}
	return n
}

func (f *rmFixture) str(q string, args ...any) string {
	f.t.Helper()
	var s string
	if err := testDB.NewRaw(q, args...).Scan(context.Background(), &s); err != nil {
		f.t.Fatalf("query %q: %v", q, err)
	}
	return s
}

func (f *rmFixture) igdbGame(id int) {
	f.exec(`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Game', now(), now()) ON CONFLICT DO NOTHING`, id)
}

// game inserts an external game. platform "" means no platform rows.
func (f *rmFixture) game(id, parent string, resolved int, skipped bool, platform string) {
	f.t.Helper()
	var p, r any
	if parent != "" {
		p = parent
	}
	if resolved != 0 {
		f.igdbGame(resolved)
		r = resolved
	}
	f.exec(`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, resolved_igdb_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'Spider-Man', ?, true, false, ?, ?, now(), now())`,
		id, f.userID, rmStore, id, skipped, p, r)
	if platform != "" {
		f.exec(`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, ?, 0, now())`,
			id+"-p", id, platform)
	}
}

func (f *rmFixture) job(id, status string) {
	f.exec(`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		VALUES (?, ?, 'sync', ?, ?, 'low', 0, true, now())`, id, f.userID, rmStore, status)
}

// item inserts a job item created age ago.
func (f *rmFixture) item(id, jobID, egID, status string, age time.Duration) {
	f.exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (?, ?, ?, ?, 'Spider-Man', ?, '{}', ?, '{}', '[]', ?)`,
		id, jobID, f.userID, id, egID, status, time.Now().Add(-age))
}

// link puts egID on user game ugID (game igdbID) on platform.
func (f *rmFixture) link(ugID string, igdbID int, egID, platform string) {
	f.igdbGame(igdbID)
	f.exec(`INSERT INTO user_games (id, user_id, game_id, play_status, created_at, updated_at) VALUES (?, ?, ?, 'completed', now(), now()) ON CONFLICT DO NOTHING`,
		ugID, f.userID, igdbID)
	var eg any
	if egID != "" {
		eg = egID
	}
	f.exec(`INSERT INTO user_game_platforms (id, user_game_id, external_game_id, platform, storefront, sync_from_source, is_available, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, true, now(), now())`, ugID+"-"+platform, ugID, eg, platform, rmStore)
}

func (f *rmFixture) rematch(id string, igdbID int, orphan string) *httptest.ResponseRecorder {
	f.t.Helper()
	body := map[string]any{"igdb_id": igdbID}
	if orphan != "" {
		body["orphan_action"] = orphan
	}
	return postJSONAuth(f.t, f.e, "/api/sync/external-games/"+id+"/rematch", body, f.token)
}

func (f *rmFixture) unskip(id string) *httptest.ResponseRecorder {
	f.t.Helper()
	return deleteAuth(f.t, f.e, "/api/sync/ignored/"+id, f.token)
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, code int) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("want %d, got %d: %s", code, rec.Code, rec.Body.String())
	}
}

// onGame counts egID's platform links on a user game of igdbID.
func (f *rmFixture) onGame(egID string, igdbID int) int {
	return f.count(`SELECT COUNT(*) FROM user_game_platforms ugp JOIN user_games ug ON ug.id = ugp.user_game_id
		WHERE ugp.external_game_id = ? AND ug.game_id = ? AND ug.user_id = ?`, egID, igdbID, f.userID)
}

func (f *rmFixture) itemStatus(id string) string {
	return f.str(`SELECT status FROM job_items WHERE id = ?`, id)
}

func (f *rmFixture) workerJobs() int {
	return f.count(`SELECT COUNT(*) FROM river_job WHERE kind IN ('user_game_write', 'igdb_match')`)
}

// ── Rematch ───────────────────────────────────────────────────────────────────

func TestRematch_FinishedSync_WritesLibraryAndLeavesTheItem(t *testing.T) {
	f := newRM(t, "rm-finished")
	f.game("eg", "", 100, false, "playstation-4")
	f.link("ug-old", 100, "eg", "playstation-4")
	f.link("ug-old", 100, "", "playstation-5") // another link keeps ug-old from being orphaned
	f.job("job", "completed")
	f.item("ji", "job", "eg", "completed", time.Hour)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if n := f.onGame("eg", 200); n != 1 {
		t.Errorf("links on the new match: want 1, got %d", n)
	}
	if n := f.onGame("eg", 100); n != 0 {
		t.Errorf("links on the old match: want 0, got %d", n)
	}
	if s := f.itemStatus("ji"); s != "completed" {
		t.Errorf("item: want completed (untouched), got %s", s)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "completed" {
		t.Errorf("job: want completed, got %s", s)
	}
	if n := f.workerJobs(); n != 0 {
		t.Errorf("worker river jobs: want 0, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes`); n != 0 {
		t.Errorf("changes rows outside an active sync: want 0, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch' AND args->>'game_id' = '200'`); n != 1 {
		t.Errorf("immediate metadata fetch: want 1, got %d", n)
	}
}

func TestRematch_PendingReview_CompletesItemAndJob(t *testing.T) {
	f := newRM(t, "rm-review")
	f.game("eg", "", 0, false, "playstation-4")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if s := f.itemStatus("ji"); s != "completed" {
		t.Errorf("item: want completed, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND external_game_id = 'eg' AND change_type = 'added'`); n != 1 {
		t.Errorf("added change: want 1, got %d", n)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "completed" {
		t.Errorf("job: want completed after its last item, got %s", s)
	}
	if n := f.onGame("eg", 200); n != 1 {
		t.Errorf("library link: want 1, got %d", n)
	}
}

func TestRematch_PendingReview_NoPlatformRows_CompletesItemWithoutChange(t *testing.T) {
	f := newRM(t, "rm-noplat")
	f.game("eg", "", 0, false, "")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if s := f.itemStatus("ji"); s != "completed" {
		t.Errorf("item: want completed, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes`); n != 0 {
		t.Errorf("changes: want 0, got %d", n)
	}
	if s := f.str(`SELECT resolved_igdb_id::text FROM external_games WHERE id = 'eg'`); s != "200" {
		t.Errorf("match: want 200, got %s", s)
	}
}

// Rematch is refused only while the game's own item is pending: the sync has
// not finished with it. A child's unfinished item does not refuse it.
func TestRematch_RefusedWhileTheGamesOwnItemIsPending(t *testing.T) {
	f := newRM(t, "rm-409")
	f.game("eg", "", 100, false, "playstation-4")
	job := newSyncJob(t, newTestLifecycle(t), f.userID, rmStore)
	job.add(t, f.userID, "eg")

	rec := f.rematch("eg", 200, "")
	wantCode(t, rec, http.StatusConflict)
	if want := `"a sync is still working on this game"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body: want %s, got %s", want, rec.Body.String())
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE resolved_igdb_id = 200`); n != 0 {
		t.Errorf("rematch changed the match of %d games", n)
	}
}

// A child's igdb_match still queued when its parent is rematched: the child's
// item completes with the rematch and its River job is cancelled, so the
// worker writes nothing.
func TestRematch_ChildWorkQueued_CompletesItAndCancelsItsRiverJob(t *testing.T) {
	f := newRM(t, "rm-child-queued")
	f.game("eg", "", 0, false, "playstation-4")
	f.game("ch", "eg", 0, false, "playstation-5")
	lc := newTestLifecycle(t)
	job := newSyncJob(t, lc, f.userID, rmStore)
	ids := job.add(t, f.userID, "eg", "ch")
	job.done(t)
	parentItem, childItem := ids[0], ids[1]
	if err := lc.AwaitReview(context.Background(), parentItem, itemOwnerID(t, parentItem), joblifecycle.Review{}); err != nil {
		t.Fatal(err)
	}
	childOwner := itemOwnerID(t, childItem)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	for _, id := range []string{parentItem, childItem} {
		if s := f.itemStatus(id); s != "completed" {
			t.Errorf("item %s: want completed, got %s", id, s)
		}
	}
	if s := riverJobState(t, testDB, childOwner); s != "cancelled" {
		t.Errorf("child's igdb_match: want cancelled, got %s", s)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, job.ID); s != "completed" {
		t.Errorf("job: want completed, got %s", s)
	}
	// The cancelled worker, run anyway, is refused before it writes.
	w := &tasks.IGDBMatchWorker{DB: testDB, LC: lc}
	if err := w.Work(context.Background(), &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{ID: childOwner, Attempt: 1, MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: childItem},
	}); err != nil {
		t.Fatalf("late worker: %v", err)
	}
	if s := f.itemStatus(childItem); s != "completed" {
		t.Errorf("late worker changed the child item: %s", s)
	}
}

func TestRematch_ParentInReviewWithWaitingChild_CompletesBoth(t *testing.T) {
	f := newRM(t, "rm-wait")
	f.game("eg", "", 0, false, "playstation-4")
	f.game("ch", "eg", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)
	f.item("ji-ch", "job", "ch", "pending", 0)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	for _, id := range []string{"ji", "ji-ch"} {
		if s := f.itemStatus(id); s != "completed" {
			t.Errorf("%s: want completed, got %s", id, s)
		}
	}
	if n := f.onGame("ch", 200); n != 1 {
		t.Errorf("child link on the new match: want 1, got %d", n)
	}
	// One added row for the Job, not one per edition.
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND change_type = 'added'`); n != 1 {
		t.Errorf("added changes: want 1, got %d", n)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "completed" {
		t.Errorf("job: want completed, got %s", s)
	}
}

func TestRematch_ParentWithoutItemOrFailed_CompletesWaitingChild(t *testing.T) {
	for _, parentItem := range []string{"", "failed"} {
		t.Run("parent item="+parentItem, func(t *testing.T) {
			f := newRM(t, "rm-orphan-child")
			f.game("eg", "", 0, false, "playstation-4")
			f.game("ch", "eg", 0, false, "playstation-5")
			f.job("job", "processing")
			if parentItem != "" {
				f.item("ji", "job", "eg", parentItem, 0)
			}
			f.item("ji-ch", "job", "ch", "pending", 0)

			wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

			if s := f.itemStatus("ji-ch"); s != "completed" {
				t.Errorf("child item: want completed, got %s", s)
			}
			if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND external_game_id = 'ch' AND change_type = 'added'`); n != 1 {
				t.Errorf("added change for the child: want 1, got %d", n)
			}
			if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "completed" {
				t.Errorf("job: want completed, got %s", s)
			}
		})
	}
}

func TestRematch_CompletesReviewItemInAnOlderReopenedJob(t *testing.T) {
	f := newRM(t, "rm-reopened")
	f.game("eg", "", 0, false, "playstation-4")
	f.job("old", "processing") // reopened by retry
	f.item("ji-old", "old", "eg", "pending_review", 48*time.Hour)
	f.job("new", "completed")
	f.item("ji-new", "new", "eg", "failed", time.Hour)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if s := f.itemStatus("ji-old"); s != "completed" {
		t.Errorf("old job's review item: want completed, got %s", s)
	}
	if s := f.itemStatus("ji-new"); s != "failed" {
		t.Errorf("finished job's item: want failed (untouched), got %s", s)
	}
}

func TestRematch_RacingASkip_DoesNotCompleteTheSkippedItem(t *testing.T) {
	f := newRM(t, "rm-skip-race")
	f.game("eg", "", 0, false, "playstation-4")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)
	ctx := context.Background()

	// A Skip holds the Job row, as the module does, and marks the item
	// skipped while the rematch runs.
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM jobs WHERE id = 'job' FOR UPDATE`); err != nil {
		t.Fatalf("lock job: %v", err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.rematch("eg", 200, "") }()
	time.Sleep(300 * time.Millisecond)
	if _, err := tx.ExecContext(ctx, `UPDATE job_items SET status = 'skipped' WHERE id = 'ji'`); err != nil {
		t.Fatalf("skip: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	wantCode(t, <-done, http.StatusNoContent)

	if s := f.itemStatus("ji"); s != "skipped" {
		t.Errorf("item: want skipped, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes`); n != 0 {
		t.Errorf("changes: want 0, got %d", n)
	}
}

func TestRematch_MovesChildLinksToTheNewGame(t *testing.T) {
	f := newRM(t, "rm-move")
	f.game("eg", "", 100, false, "playstation-4")
	f.game("ch", "eg", 100, false, "playstation-5")
	f.link("ug-old", 100, "eg", "playstation-4")
	f.link("ug-old", 100, "ch", "playstation-5")

	// The child's link is the parent's only other link: that is no other link.
	wantCode(t, f.rematch("eg", 200, ""), http.StatusConflict)
	if n := f.onGame("ch", 100); n != 1 {
		t.Fatal("refused rematch changed the links")
	}

	wantCode(t, f.rematch("eg", 200, "keep"), http.StatusNoContent)
	if n := f.onGame("ch", 100); n != 0 {
		t.Errorf("child links left on the old game: %d", n)
	}
	if n := f.onGame("ch", 200); n != 1 {
		t.Errorf("child links on the new game: want 1, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-old'`); n != 1 {
		t.Error("keep removed the old user game")
	}
}

func TestRematch_FamilySplitOverTwoUserGames_AppliesTheActionToEach(t *testing.T) {
	f := newRM(t, "rm-split")
	f.game("eg", "", 100, false, "playstation-4")
	f.game("ch", "eg", 101, false, "playstation-5")
	f.link("ug-a", 100, "eg", "playstation-4")
	f.link("ug-b", 101, "ch", "playstation-5")

	wantCode(t, f.rematch("eg", 200, "remove"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id IN ('ug-a', 'ug-b')`); n != 0 {
		t.Errorf("orphaned user games left: %d", n)
	}
	if n := f.onGame("eg", 200) + f.onGame("ch", 200); n != 2 {
		t.Errorf("family links on the new game: want 2, got %d", n)
	}
}

func TestRematch_ToTheCurrentGameWithRemove_KeepsTheUserGame(t *testing.T) {
	f := newRM(t, "rm-same")
	f.game("eg", "", 100, false, "playstation-4")
	f.link("ug", 100, "eg", "playstation-4")

	wantCode(t, f.rematch("eg", 100, "remove"), http.StatusNoContent)

	if s := f.str(`SELECT COALESCE(play_status, '') FROM user_games WHERE id = 'ug'`); s != "completed" {
		t.Errorf("user game play status: want completed (kept), got %q", s)
	}
	if n := f.onGame("eg", 100); n != 1 {
		t.Errorf("link: want 1, got %d", n)
	}
}

func TestRematch_SkippedFamily_IsUnskippedAndMatched(t *testing.T) {
	f := newRM(t, "rm-skipped")
	f.game("eg", "", 0, true, "playstation-4")
	f.game("ch", "eg", 0, true, "playstation-5")

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ch') AND NOT is_skipped AND resolved_igdb_id = 200`); n != 2 {
		t.Errorf("unskipped and matched family members: want 2, got %d", n)
	}
}

func TestRematchUnskipSkip_ChildGame_Is400(t *testing.T) {
	f := newRM(t, "rm-child-400")
	f.game("eg", "", 0, false, "playstation-4")
	f.game("ch", "eg", 0, true, "playstation-5")
	wantCode(t, f.rematch("ch", 200, ""), http.StatusBadRequest)
	wantCode(t, f.unskip("ch"), http.StatusBadRequest)

	f.game("eg2", "", 0, false, "playstation-4")
	f.game("ch2", "eg2", 0, false, "playstation-5")
	wantCode(t, f.skip("ch2"), http.StatusBadRequest)
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'ch2' AND is_skipped`); n != 0 {
		t.Error("child skipped despite 400")
	}
}

// Skipping a Child game with an unfinished item in an active Job is refused
// inside the module's transaction: the item stays pending and the child is
// not skipped (#1419).
func TestSkip_ChildGameWithActiveItem_Is400(t *testing.T) {
	f := newRM(t, "rm-child-skip-item")
	f.game("eg", "", 0, false, "playstation-4")
	f.game("ch", "eg", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "ch", "pending", 0)

	wantCode(t, f.skip("ch"), http.StatusBadRequest)

	if s := f.itemStatus("ji"); s != "pending" {
		t.Errorf("item: want pending, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE is_skipped`); n != 0 {
		t.Errorf("skipped games: want 0, got %d", n)
	}
}

func TestRematch_RollsBackOnMidSequenceFailure(t *testing.T) {
	f := newRM(t, "rm-rollback")
	f.game("eg", "", 100, false, "playstation-4")
	f.game("ch", "eg", 100, false, "playstation-5")
	f.link("ug", 100, "eg", "playstation-4")
	f.link("ug", 100, "ch", "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)

	// Fail the child's library write, after everything else has been done.
	f.exec(`
		CREATE OR REPLACE FUNCTION test_fail_rematch_child() RETURNS trigger AS $$
		BEGIN
			IF NEW.external_game_id = 'ch' THEN
				RAISE EXCEPTION 'injected failure';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER test_fail_rematch_child BEFORE INSERT ON user_game_platforms
			FOR EACH ROW EXECUTE FUNCTION test_fail_rematch_child();`)
	t.Cleanup(func() {
		if _, err := testDB.ExecContext(context.Background(), `
			DROP TRIGGER IF EXISTS test_fail_rematch_child ON user_game_platforms;
			DROP FUNCTION IF EXISTS test_fail_rematch_child();`); err != nil {
			t.Errorf("drop failure trigger: %v", err)
		}
	})

	wantCode(t, f.rematch("eg", 200, "remove"), http.StatusInternalServerError)

	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id = 'ug'`); n != 1 {
		t.Error("user game removed despite rollback")
	}
	if n := f.onGame("eg", 100) + f.onGame("ch", 100); n != 2 {
		t.Errorf("old links: want 2, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE resolved_igdb_id = 200`); n != 0 {
		t.Errorf("re-resolved games: want 0, got %d", n)
	}
	if s := f.itemStatus("ji"); s != "pending_review" {
		t.Errorf("item: want pending_review, got %s", s)
	}
}

// A user_game_write for a child and a rematch of its parent both lock the Job
// row first, then the family, so they never deadlock: the rematch answers
// 204, the worker returns without an error, and the child's item ends
// completed whichever went first.
func TestRematch_ConcurrentChildWrite_NoDeadlock(t *testing.T) {
	for range 5 {
		f := newRM(t, "rm-deadlock")
		f.game("eg", "", 100, false, "playstation-4")
		f.game("ch", "eg", 100, false, "playstation-5")
		f.link("ug", 100, "eg", "playstation-4")
		f.link("ug", 100, "", "playstation-3")
		lc := newTestLifecycle(t)
		job := newSyncJob(t, lc, f.userID, rmStore)
		childItem := job.add(t, f.userID, "ch")[0]
		job.done(t)

		w := &tasks.UserGameWorker{DB: testDB, LC: lc}
		owner := itemOwnerID(t, childItem)
		var wg sync.WaitGroup
		var rec *httptest.ResponseRecorder
		var werr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			werr = w.Work(context.Background(), &river.Job[tasks.UserGameArgs]{
				JobRow: &rivertype.JobRow{ID: owner, Attempt: 1, MaxAttempts: 5},
				Args:   tasks.UserGameArgs{JobItemID: childItem},
			})
		}()
		go func() { defer wg.Done(); rec = f.rematch("eg", 200, "keep") }()
		wg.Wait()
		if werr != nil {
			t.Fatalf("worker: %v", werr)
		}
		wantCode(t, rec, http.StatusNoContent)
		if s := f.itemStatus(childItem); s != "completed" {
			t.Fatalf("child item: want completed, got %s (%s)", s, f.str(`SELECT COALESCE(error_message, '') FROM job_items WHERE id = ?`, childItem))
		}
	}
}

// ── Unskip ────────────────────────────────────────────────────────────────────

func TestUnskip_MatchedGame_WritesFamilyWithoutAJob(t *testing.T) {
	f := newRM(t, "unskip-matched")
	f.game("eg", "", 200, true, "playstation-4")
	f.game("ch", "eg", 100, true, "playstation-5") // an older match of its own
	f.link("ug-old", 100, "ch", "playstation-5")
	f.link("ug-old", 100, "", "playstation-3")

	wantCode(t, f.unskip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ch') AND NOT is_skipped AND resolved_igdb_id = 200`); n != 2 {
		t.Errorf("unskipped family on the parent's match: want 2, got %d", n)
	}
	if n := f.onGame("eg", 200) + f.onGame("ch", 200); n != 2 {
		t.Errorf("family links on the parent's match: want 2, got %d", n)
	}
	if n := f.onGame("ch", 100); n != 0 {
		t.Errorf("child links left on its old game: %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM jobs`) + f.count(`SELECT COUNT(*) FROM job_items`); n != 0 {
		t.Errorf("jobs and items created: %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch' AND args->>'game_id' = '200'`); n != 1 {
		t.Errorf("immediate metadata fetch: want 1, got %d", n)
	}
}

func TestUnskip_UnmatchedGame_OnlyClearsTheFlag(t *testing.T) {
	f := newRM(t, "unskip-unmatched")
	f.game("eg", "", 0, true, "playstation-4")

	wantCode(t, f.unskip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'eg' AND NOT is_skipped AND resolved_igdb_id IS NULL`); n != 1 {
		t.Error("want the game unskipped and still unmatched")
	}
	if n := f.count(`SELECT COUNT(*) FROM jobs`) + f.count(`SELECT COUNT(*) FROM river_job`); n != 0 {
		t.Errorf("jobs or river jobs created: %d", n)
	}
}

// A family from before #1424 can have an unmatched parent and a matched,
// linked child. Unskip makes the child follow its parent, as the dispatch's
// repair does: its links go (the user game is kept) and its match is cleared
// (#1425).
func TestUnskip_UnmatchedGame_ClearsAMatchedChild(t *testing.T) {
	f := newRM(t, "unskip-unmatched-child")
	f.game("eg", "", 0, true, "playstation-4")
	f.game("ch", "eg", 100, true, "playstation-5")
	f.link("ug-old", 100, "ch", "playstation-5")

	wantCode(t, f.unskip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ch') AND NOT is_skipped AND resolved_igdb_id IS NULL`); n != 2 {
		t.Errorf("unskipped, unmatched family: want 2, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM user_game_platforms WHERE external_game_id = 'ch'`); n != 0 {
		t.Errorf("child links left: %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-old'`); n != 1 {
		t.Error("the child's user game was removed")
	}
	if n := f.count(`SELECT COUNT(*) FROM jobs`) + f.count(`SELECT COUNT(*) FROM river_job`); n != 0 {
		t.Errorf("jobs or river jobs created: %d", n)
	}
}

func TestUnskip_FailureRollsBackTheFlag(t *testing.T) {
	f := newRM(t, "unskip-rollback")
	f.game("eg", "", 200, true, "playstation-4")
	f.exec(`
		CREATE OR REPLACE FUNCTION test_fail_unskip() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql;
		CREATE TRIGGER test_fail_unskip BEFORE INSERT ON user_game_platforms
			FOR EACH ROW EXECUTE FUNCTION test_fail_unskip();`)
	t.Cleanup(func() {
		if _, err := testDB.ExecContext(context.Background(), `
			DROP TRIGGER IF EXISTS test_fail_unskip ON user_game_platforms;
			DROP FUNCTION IF EXISTS test_fail_unskip();`); err != nil {
			t.Errorf("drop failure trigger: %v", err)
		}
	})

	wantCode(t, f.unskip("eg"), http.StatusInternalServerError)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'eg' AND is_skipped`); n != 1 {
		t.Error("the skip was cleared despite the failure")
	}
}

// ── The storefront view, the review badge and retry failed ────────────────────

type listedGame struct {
	ID                         string  `json:"id"`
	SyncStatus                 string  `json:"sync_status"`
	FailedJobItemID            *string `json:"failed_job_item_id"`
	HasUserGame                bool    `json:"has_user_game"`
	UserGameID                 *string `json:"user_game_id"`
	UserGameOtherPlatformCount int     `json:"user_game_other_platform_count"`
}

// list returns the storefront's listed games by id.
func (f *rmFixture) list() map[string]listedGame {
	f.t.Helper()
	rec := getAuth(f.t, f.e, "/api/sync/"+rmStore+"/external-games", f.token)
	wantCode(f.t, rec, http.StatusOK)
	var games []listedGame
	if err := json.Unmarshal(rec.Body.Bytes(), &games); err != nil {
		f.t.Fatalf("unmarshal list: %v", err)
	}
	out := map[string]listedGame{}
	for _, g := range games {
		out[g.ID] = g
	}
	return out
}

func (f *rmFixture) badge() int {
	f.t.Helper()
	rec := getAuth(f.t, f.e, "/api/jobs/pending-review-count", f.token)
	wantCode(f.t, rec, http.StatusOK)
	var resp struct {
		Count int `json:"pending_review_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		f.t.Fatalf("unmarshal badge: %v", err)
	}
	return resp.Count
}

func (f *rmFixture) status(id string) string {
	f.t.Helper()
	g, ok := f.list()[id]
	if !ok {
		return "(hidden)"
	}
	return g.SyncStatus
}

func (f *rmFixture) liveRiverJob(itemID, kind string) {
	args := `{}`
	if itemID != "" {
		args = `{"job_item_id":"` + itemID + `"}`
	}
	f.exec(`INSERT INTO river_job (args, kind, max_attempts, priority, queue, state, scheduled_at)
		VALUES (?::jsonb, ?, 5, 1, 'default', 'available', now())`, args, kind)
}

func TestListExternalGames_StatusComesFromTheNewestItem(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *rmFixture)
		want  string
		badge int
	}{
		{"old failed item, newer completed item", func(f *rmFixture) {
			f.game("eg", "", 100, false, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "failed", 48*time.Hour)
			f.job("j2", "completed")
			f.item("i2", "j2", "eg", "completed", time.Hour)
		}, "matched", 0},
		{"newest failed", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "failed", time.Hour)
		}, "failed", 0},
		{"skipped game whose newest item failed", func(f *rmFixture) {
			f.game("eg", "", 0, true, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "failed", time.Hour)
		}, "skipped", 0},
		{"newest in review in a cancelled job", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("j1", "cancelled")
			f.item("i1", "j1", "eg", "pending_review", time.Hour)
		}, "needs_review", 1},
		{"in review in an older reopened job, newest finished", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("old", "processing")
			f.item("i1", "old", "eg", "pending_review", 48*time.Hour)
			f.job("new", "completed")
			f.item("i2", "new", "eg", "failed", time.Hour)
		}, "needs_review", 1},
		{"newest pending in a cancelled job", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("j1", "cancelled")
			f.item("i1", "j1", "eg", "pending", time.Hour)
		}, "unmatched", 0},
		{"newest pending in an active job", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("j1", "processing")
			f.item("i1", "j1", "eg", "pending", 0)
		}, "(hidden)", 0},
		// The view no longer asks River: no item is unfinished in a finished
		// Job any more (the module cancels them with it, the upgrade fails or
		// cancels the old ones), so a live River job naming one is ignored.
		{"newest pending in a finished job, live river job", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "pending", 0)
			f.liveRiverJob("i1", "igdb_match")
		}, "unmatched", 0},
		// Failed, but Retry never reopens a failed Job: no retry is offered.
		{"newest failed in a failed job", func(f *rmFixture) {
			f.game("eg", "", 0, false, "playstation-4")
			f.job("j1", "failed")
			f.item("i1", "j1", "eg", "failed", time.Hour)
		}, "failed", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRM(t, "list-newest")
			tc.setup(f)
			if got := f.status("eg"); got != tc.want {
				t.Errorf("sync_status: want %s, got %s", tc.want, got)
			}
			if got := f.badge(); got != tc.badge {
				t.Errorf("badge: want %d, got %d", tc.badge, got)
			}
			g := f.list()["eg"]
			retryable := tc.want == "failed" && tc.name != "newest failed in a failed job"
			if (g.FailedJobItemID != nil) != retryable {
				t.Errorf("failed_job_item_id: %v with status %s", g.FailedJobItemID, tc.want)
			}
		})
	}
}

func TestListExternalGames_RematchSupersedesOldItems(t *testing.T) {
	t.Run("newest item failed", func(t *testing.T) {
		f := newRM(t, "list-rematch-failed")
		f.game("eg", "", 0, false, "playstation-4")
		f.job("j1", "completed")
		f.item("i1", "j1", "eg", "failed", time.Hour)

		wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

		g := f.list()["eg"]
		if g.SyncStatus != "matched" || g.FailedJobItemID != nil {
			t.Errorf("want matched with no failed item, got %s / %v", g.SyncStatus, g.FailedJobItemID)
		}
	})
	t.Run("in review in a cancelled job", func(t *testing.T) {
		f := newRM(t, "list-rematch-cancelled")
		f.game("eg", "", 0, false, "playstation-4")
		f.job("j1", "cancelled")
		f.item("i1", "j1", "eg", "pending_review", time.Hour)

		wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

		if s := f.status("eg"); s != "matched" {
			t.Errorf("sync_status: want matched, got %s", s)
		}
		if n := f.badge(); n != 0 {
			t.Errorf("badge: want 0, got %d", n)
		}
		if s := f.itemStatus("i1"); s != "pending_review" {
			t.Errorf("item in the cancelled job: want pending_review (untouched), got %s", s)
		}
	})
}

func TestListExternalGames_OrphanFieldsArePerFamily(t *testing.T) {
	f := newRM(t, "list-orphan")
	f.game("eg", "", 100, false, "playstation-4")
	f.game("ch", "eg", 100, false, "playstation-5")
	f.link("ug", 100, "eg", "playstation-4")
	f.link("ug", 100, "ch", "playstation-5")

	g := f.list()["eg"]
	if !g.HasUserGame || g.UserGameID == nil || *g.UserGameID != "ug" {
		t.Errorf("want has_user_game on ug, got %v / %v", g.HasUserGame, g.UserGameID)
	}
	if g.UserGameOtherPlatformCount != 0 {
		t.Errorf("user_game_other_platform_count: want 0 (the child's link is in the family), got %d", g.UserGameOtherPlatformCount)
	}

	f.link("ug", 100, "", "playstation-3") // a link from outside the family
	if n := f.list()["eg"].UserGameOtherPlatformCount; n != 1 {
		t.Errorf("user_game_other_platform_count: want 1, got %d", n)
	}
}

func TestListExternalGames_UnskippedNeverMatchedGameIsUnmatched(t *testing.T) {
	f := newRM(t, "list-unmatched")
	f.game("eg", "", 0, true, "playstation-4")
	wantCode(t, f.unskip("eg"), http.StatusNoContent)
	if s := f.status("eg"); s != "unmatched" {
		t.Errorf("sync_status: want unmatched, got %s", s)
	}
}

func TestPendingReviewCount_IgnoresSyncItemOfADeletedGame(t *testing.T) {
	f := newRM(t, "badge-gone")
	f.job("j1", "processing")
	f.exec(`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES ('i1', 'j1', ?, 'k', 'Gone', '{}', 'pending_review', '{}', '[]', now())`, f.userID)
	if n := f.badge(); n != 0 {
		t.Errorf("badge: want 0, got %d", n)
	}
}

func TestRetryFailedExternalGames_RetriesOnlyGamesShownAsFailed(t *testing.T) {
	f := newRM(t, "retry-failed")
	// Shown as failed, with a failed child.
	f.game("eg", "", 0, false, "playstation-4")
	f.game("ch", "eg", 0, false, "playstation-5")
	f.job("j1", "completed")
	f.item("i-eg", "j1", "eg", "failed", time.Hour)
	f.item("i-ch", "j1", "ch", "failed", time.Hour)
	// An old failure superseded by a newer completed item.
	f.game("old", "", 100, false, "playstation-4")
	f.item("i-old", "j1", "old", "failed", 48*time.Hour)
	f.job("j2", "completed")
	f.item("i-old2", "j2", "old", "completed", time.Hour)
	// A skipped game whose newest item failed.
	f.game("sk", "", 0, true, "playstation-4")
	f.item("i-sk", "j1", "sk", "failed", time.Hour)

	wantCode(t, postJSONAuth(t, f.e, "/api/sync/"+rmStore+"/external-games/retry-failed", nil, f.token), http.StatusNoContent)

	for id, want := range map[string]string{"i-eg": "pending", "i-ch": "pending", "i-old": "failed", "i-sk": "failed"} {
		if s := f.itemStatus(id); s != want {
			t.Errorf("%s: want %s, got %s", id, want, s)
		}
	}
	if n := f.count(`SELECT COUNT(*) FROM river_job WHERE kind = 'igdb_match' AND args->>'job_item_id' IN ('i-eg', 'i-ch')`); n != 2 {
		t.Errorf("queued igdb_match jobs: want 2, got %d", n)
	}
}
