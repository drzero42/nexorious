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
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/externalgame"
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
	seq    int // games inserted later are younger
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

// game inserts an External game titled Spider-Man, so the games a test
// inserts with the same match and skip are editions of each other. platform
// "" means no platform rows. Each game is younger than the one before.
func (f *rmFixture) game(id string, resolved int, skipped bool, platform string) {
	f.t.Helper()
	f.titled(id, "Spider-Man", resolved, skipped, platform)
}

// titled is game with another title.
func (f *rmFixture) titled(id, title string, resolved int, skipped bool, platform string) {
	f.t.Helper()
	var r any
	if resolved != 0 {
		f.igdbGame(resolved)
		r = resolved
	}
	f.seq++
	f.exec(`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, true, false, ?, now() - interval '1 day' + ? * interval '1 second', now())`,
		id, f.userID, rmStore, id, title, skipped, r, f.seq)
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

func TestRematch_EndedSync_WritesLibraryAndLeavesTheItem(t *testing.T) {
	f := newRM(t, "rm-ended")
	f.game("eg", 100, false, "playstation-4")
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
	f.game("eg", 0, false, "playstation-4")
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
	f.game("eg", 0, false, "")
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

// Rematch is refused only while the game's own item is pending: the sync is
// not done with it. Another edition's open item does not refuse it.
func TestRematch_RefusedWhileTheGamesOwnItemIsPending(t *testing.T) {
	f := newRM(t, "rm-409")
	f.game("eg", 100, false, "playstation-4")
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

// Another edition's igdb_match still queued when the game is rematched: that
// item completes with the rematch and its River job is cancelled, so the
// worker writes nothing.
func TestRematch_EditionWorkQueued_CompletesItAndCancelsItsRiverJob(t *testing.T) {
	f := newRM(t, "rm-edition-queued")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	lc := newTestLifecycle(t)
	job := newSyncJob(t, lc, f.userID, rmStore)
	ids := job.add(t, f.userID, "eg", "ed")
	job.done(t)
	ownItem, otherItem := ids[0], ids[1]
	if err := lc.AwaitReview(context.Background(), ownItem, itemOwnerID(t, ownItem), joblifecycle.Review{}); err != nil {
		t.Fatal(err)
	}
	otherOwner := itemOwnerID(t, otherItem)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	for _, id := range []string{ownItem, otherItem} {
		if s := f.itemStatus(id); s != "completed" {
			t.Errorf("item %s: want completed, got %s", id, s)
		}
	}
	if s := riverJobState(t, testDB, otherOwner); s != "cancelled" {
		t.Errorf("the other edition's igdb_match: want cancelled, got %s", s)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, job.ID); s != "completed" {
		t.Errorf("job: want completed, got %s", s)
	}
	// The cancelled worker, run anyway, is refused before it writes.
	w := &tasks.IGDBMatchWorker{DB: testDB, LC: lc}
	if err := w.Work(context.Background(), &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{ID: otherOwner, Attempt: 1, MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: otherItem},
	}); err != nil {
		t.Fatalf("late worker: %v", err)
	}
	if s := f.itemStatus(otherItem); s != "completed" {
		t.Errorf("late worker changed the other item: %s", s)
	}
}

// Two editions in review are one entry and count once on the badge; one
// Rematch completes both items and the Job and writes both onto one library
// game, with one 'added' row.
func TestRematch_TwoEditionsInReview_OneEntryOneRematch(t *testing.T) {
	f := newRM(t, "rm-two-review")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)
	f.item("ji-ed", "job", "ed", "pending_review", 0)

	if l := f.list(); len(l) != 1 || l["eg"].SyncStatus != "needs_review" {
		t.Fatalf("list = %+v, want one entry eg in needs_review", l)
	}
	if n := f.badge(); n != 1 {
		t.Errorf("badge: want 1, got %d", n)
	}

	wantCode(t, f.rematch("ed", 200, ""), http.StatusNoContent)

	for _, id := range []string{"ji", "ji-ed"} {
		if s := f.itemStatus(id); s != "completed" {
			t.Errorf("%s: want completed, got %s", id, s)
		}
	}
	if n := f.onGame("eg", 200) + f.onGame("ed", 200); n != 2 {
		t.Errorf("links on the new match: want 2, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, f.userID); n != 1 {
		t.Errorf("user games: want 1, got %d", n)
	}
	// One added row for the Job, not one per edition.
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND change_type = 'added'`); n != 1 {
		t.Errorf("added changes: want 1, got %d", n)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "completed" {
		t.Errorf("job: want completed, got %s", s)
	}
	if n := f.badge(); n != 0 {
		t.Errorf("badge after the rematch: want 0, got %d", n)
	}
}

// The given game has no open item (none, or a failed one); another edition's
// pending item is completed by the Rematch.
func TestRematch_OtherEditionsPendingItem_IsCompleted(t *testing.T) {
	for _, ownItem := range []string{"", "failed"} {
		t.Run("own item="+ownItem, func(t *testing.T) {
			f := newRM(t, "rm-other-pending")
			f.game("eg", 0, false, "playstation-4")
			f.game("ed", 0, false, "playstation-5")
			f.job("job", "processing")
			if ownItem != "" {
				f.item("ji", "job", "eg", ownItem, 0)
			}
			f.item("ji-ed", "job", "ed", "pending", 0)

			wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

			if s := f.itemStatus("ji-ed"); s != "completed" {
				t.Errorf("the other edition's item: want completed, got %s", s)
			}
			if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND external_game_id = 'ed' AND change_type = 'added'`); n != 1 {
				t.Errorf("added change for the other edition: want 1, got %d", n)
			}
			if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "completed" {
				t.Errorf("job: want completed, got %s", s)
			}
		})
	}
}

func TestRematch_CompletesReviewItemInAnOlderReopenedJob(t *testing.T) {
	f := newRM(t, "rm-reopened")
	f.game("eg", 0, false, "playstation-4")
	f.job("old", "processing") // reopened by retry
	f.item("ji-old", "old", "eg", "pending_review", 48*time.Hour)
	f.job("new", "completed")
	f.item("ji-new", "new", "eg", "failed", time.Hour)

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if s := f.itemStatus("ji-old"); s != "completed" {
		t.Errorf("old job's review item: want completed, got %s", s)
	}
	if s := f.itemStatus("ji-new"); s != "failed" {
		t.Errorf("ended job's item: want failed (untouched), got %s", s)
	}
}

func TestRematch_RacingASkip_DoesNotCompleteTheSkippedItem(t *testing.T) {
	f := newRM(t, "rm-skip-race")
	f.game("eg", 0, false, "playstation-4")
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

func TestRematch_MovesEveryEditionsLinks(t *testing.T) {
	f := newRM(t, "rm-move")
	f.game("eg", 100, false, "playstation-4")
	f.game("ed", 100, false, "playstation-5")
	f.link("ug-old", 100, "eg", "playstation-4")
	f.link("ug-old", 100, "ed", "playstation-5")

	// The other edition's link is the game's only other link: that is no
	// other link.
	wantCode(t, f.rematch("eg", 200, ""), http.StatusConflict)
	if n := f.onGame("ed", 100); n != 1 {
		t.Fatal("refused rematch changed the links")
	}

	wantCode(t, f.rematch("eg", 200, "keep"), http.StatusNoContent)
	if n := f.onGame("ed", 100); n != 0 {
		t.Errorf("edition links left on the old game: %d", n)
	}
	if n := f.onGame("ed", 200); n != 1 {
		t.Errorf("edition links on the new game: want 1, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-old'`); n != 1 {
		t.Error("keep removed the old user game")
	}
}

// Editions whose links sit on two library games (an edition's link left on a
// game it is no longer matched to): the orphan action applies to each.
func TestRematch_EditionsOnTwoUserGames_AppliesTheActionToEach(t *testing.T) {
	f := newRM(t, "rm-split")
	f.game("eg", 100, false, "playstation-4")
	f.game("ed", 100, false, "playstation-5")
	f.link("ug-a", 100, "eg", "playstation-4")
	f.link("ug-b", 101, "ed", "playstation-5")

	wantCode(t, f.rematch("eg", 200, "remove"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id IN ('ug-a', 'ug-b')`); n != 0 {
		t.Errorf("orphaned user games left: %d", n)
	}
	if n := f.onGame("eg", 200) + f.onGame("ed", 200); n != 2 {
		t.Errorf("edition links on the new game: want 2, got %d", n)
	}
}

func TestRematch_ToTheCurrentGameWithRemove_KeepsTheUserGame(t *testing.T) {
	f := newRM(t, "rm-same")
	f.game("eg", 100, false, "playstation-4")
	f.link("ug", 100, "eg", "playstation-4")

	wantCode(t, f.rematch("eg", 100, "remove"), http.StatusNoContent)

	if s := f.str(`SELECT COALESCE(play_status, '') FROM user_games WHERE id = 'ug'`); s != "completed" {
		t.Errorf("user game play status: want completed (kept), got %q", s)
	}
	if n := f.onGame("eg", 100); n != 1 {
		t.Errorf("link: want 1, got %d", n)
	}
}

func TestRematch_SkippedEditions_AreUnskippedAndMatched(t *testing.T) {
	f := newRM(t, "rm-skipped")
	f.game("eg", 0, true, "playstation-4")
	f.game("ed", 0, true, "playstation-5")

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ed') AND NOT is_skipped AND resolved_igdb_id = 200`); n != 2 {
		t.Errorf("unskipped and matched editions: want 2, got %d", n)
	}
}

// Rematch, Skip and Unskip through any edition's id act on the whole set; no
// edition is refused for being younger.
func TestRematchSkipUnskip_ThroughAnyEdition_ActOnTheSet(t *testing.T) {
	f := newRM(t, "rm-any-edition")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	set := `SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ed') AND `

	wantCode(t, f.skip("ed"), http.StatusNoContent)
	if n := f.count(set + `is_skipped`); n != 2 {
		t.Errorf("skipped editions: want 2, got %d", n)
	}
	wantCode(t, f.unskip("ed"), http.StatusNoContent)
	if n := f.count(set + `NOT is_skipped`); n != 2 {
		t.Errorf("unskipped editions: want 2, got %d", n)
	}
	wantCode(t, f.rematch("ed", 200, ""), http.StatusNoContent)
	if n := f.count(set + `resolved_igdb_id = 200`); n != 2 {
		t.Errorf("rematched editions: want 2, got %d", n)
	}
}

// Skip of an edition whose own item is pending skips that item and the whole
// set (#1419).
func TestSkip_EditionWithItsOwnPendingItem_SkipsThatItem(t *testing.T) {
	f := newRM(t, "rm-skip-own-item")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "ed", "pending", 0)

	wantCode(t, f.skip("ed"), http.StatusNoContent)

	if s := f.itemStatus("ji"); s != "skipped" {
		t.Errorf("item: want skipped, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE is_skipped`); n != 2 {
		t.Errorf("skipped games: want 2, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND external_game_id = 'ed' AND change_type = 'skipped'`); n != 1 {
		t.Errorf("skipped change: want 1, got %d", n)
	}
}

// A same-title game on another match is not an edition: a Rematch leaves it
// alone.
func TestRematch_LeavesASameTitleGameOnAnotherMatchAlone(t *testing.T) {
	f := newRM(t, "rm-other-match")
	f.game("eg", 100, false, "playstation-4")
	f.game("other", 300, false, "playstation-5")

	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	if s := f.str(`SELECT resolved_igdb_id::text FROM external_games WHERE id = 'other'`); s != "300" {
		t.Errorf("other game's match: want 300, got %s", s)
	}
}

func TestRematch_RollsBackOnMidSequenceFailure(t *testing.T) {
	f := newRM(t, "rm-rollback")
	f.game("eg", 100, false, "playstation-4")
	f.game("ed", 100, false, "playstation-5")
	f.link("ug", 100, "eg", "playstation-4")
	f.link("ug", 100, "ed", "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)

	// Fail the other edition's library write, after everything else has been done.
	f.exec(`
		CREATE OR REPLACE FUNCTION test_fail_rematch_edition() RETURNS trigger AS $$
		BEGIN
			IF NEW.external_game_id = 'ed' THEN
				RAISE EXCEPTION 'injected failure';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER test_fail_rematch_edition BEFORE INSERT ON user_game_platforms
			FOR EACH ROW EXECUTE FUNCTION test_fail_rematch_edition();`)
	t.Cleanup(func() {
		if _, err := testDB.ExecContext(context.Background(), `
			DROP TRIGGER IF EXISTS test_fail_rematch_edition ON user_game_platforms;
			DROP FUNCTION IF EXISTS test_fail_rematch_edition();`); err != nil {
			t.Errorf("drop failure trigger: %v", err)
		}
	})

	wantCode(t, f.rematch("eg", 200, "remove"), http.StatusInternalServerError)

	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id = 'ug'`); n != 1 {
		t.Error("user game removed despite rollback")
	}
	if n := f.onGame("eg", 100) + f.onGame("ed", 100); n != 2 {
		t.Errorf("old links: want 2, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE resolved_igdb_id = 200`); n != 0 {
		t.Errorf("re-resolved games: want 0, got %d", n)
	}
	if s := f.itemStatus("ji"); s != "pending_review" {
		t.Errorf("item: want pending_review, got %s", s)
	}
}

// A user_game_write for one edition and a rematch through another both lock
// the Job row first, then the editions, so they never deadlock: the rematch
// answers 204, the worker returns without an error, and the edition's item
// ends completed whichever went first.
func TestRematch_ConcurrentEditionWrite_NoDeadlock(t *testing.T) {
	for range 5 {
		f := newRM(t, "rm-deadlock")
		f.game("eg", 100, false, "playstation-4")
		f.game("ed", 100, false, "playstation-5")
		f.link("ug", 100, "eg", "playstation-4")
		f.link("ug", 100, "", "playstation-3")
		lc := newTestLifecycle(t)
		job := newSyncJob(t, lc, f.userID, rmStore)
		editionItem := job.add(t, f.userID, "ed")[0]
		job.done(t)

		w := &tasks.UserGameWorker{DB: testDB, LC: lc}
		owner := itemOwnerID(t, editionItem)
		var wg sync.WaitGroup
		var rec *httptest.ResponseRecorder
		var werr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			werr = w.Work(context.Background(), &river.Job[tasks.UserGameArgs]{
				JobRow: &rivertype.JobRow{ID: owner, Attempt: 1, MaxAttempts: 5},
				Args:   tasks.UserGameArgs{JobItemID: editionItem},
			})
		}()
		go func() { defer wg.Done(); rec = f.rematch("eg", 200, "keep") }()
		wg.Wait()
		if werr != nil {
			t.Fatalf("worker: %v", werr)
		}
		wantCode(t, rec, http.StatusNoContent)
		if s := f.itemStatus(editionItem); s != "completed" {
			t.Fatalf("edition item: want completed, got %s (%s)", s, f.str(`SELECT COALESCE(error_message, '') FROM job_items WHERE id = ?`, editionItem))
		}
	}
}

// ── Unskip ────────────────────────────────────────────────────────────────────

func TestUnskip_MatchedEditions_WritesEachWithoutAJob(t *testing.T) {
	f := newRM(t, "unskip-matched")
	f.game("eg", 200, true, "playstation-4")
	f.game("ed", 200, true, "playstation-5")

	wantCode(t, f.unskip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ed') AND NOT is_skipped AND resolved_igdb_id = 200`); n != 2 {
		t.Errorf("unskipped editions on the match: want 2, got %d", n)
	}
	if n := f.onGame("eg", 200) + f.onGame("ed", 200); n != 2 {
		t.Errorf("edition links on the match: want 2, got %d", n)
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
	f.game("eg", 0, true, "playstation-4")

	wantCode(t, f.unskip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'eg' AND NOT is_skipped AND resolved_igdb_id IS NULL`); n != 1 {
		t.Error("want the game unskipped and still unmatched")
	}
	if n := f.count(`SELECT COUNT(*) FROM jobs`) + f.count(`SELECT COUNT(*) FROM river_job`); n != 0 {
		t.Errorf("jobs or river jobs created: %d", n)
	}
}

// A same-title game on another match is not an edition: an Unskip of an
// unmatched game leaves it skipped.
func TestUnskip_LeavesASameTitleGameOnAnotherMatchAlone(t *testing.T) {
	f := newRM(t, "unskip-other-match")
	f.game("eg", 0, true, "playstation-4")
	f.game("other", 100, true, "playstation-5")

	wantCode(t, f.unskip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'other' AND is_skipped AND resolved_igdb_id = 100`); n != 1 {
		t.Error("the same-title game on another match was changed")
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'eg' AND NOT is_skipped`); n != 1 {
		t.Error("eg was not unskipped")
	}
}

func TestUnskip_FailureRollsBackTheFlag(t *testing.T) {
	f := newRM(t, "unskip-rollback")
	f.game("eg", 200, true, "playstation-4")
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
			f.game("eg", 100, false, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "failed", 48*time.Hour)
			f.job("j2", "completed")
			f.item("i2", "j2", "eg", "completed", time.Hour)
		}, "matched", 0},
		{"newest failed", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "failed", time.Hour)
		}, "failed", 0},
		{"skipped game whose newest item failed", func(f *rmFixture) {
			f.game("eg", 0, true, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "failed", time.Hour)
		}, "skipped", 0},
		{"newest in review in a cancelled job", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
			f.job("j1", "cancelled")
			f.item("i1", "j1", "eg", "pending_review", time.Hour)
		}, "needs_review", 1},
		{"in review in an older reopened job, newest ended", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
			f.job("old", "processing")
			f.item("i1", "old", "eg", "pending_review", 48*time.Hour)
			f.job("new", "completed")
			f.item("i2", "new", "eg", "failed", time.Hour)
		}, "needs_review", 1},
		{"newest pending in a cancelled job", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
			f.job("j1", "cancelled")
			f.item("i1", "j1", "eg", "pending", time.Hour)
		}, "unmatched", 0},
		{"newest pending in an active job", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
			f.job("j1", "processing")
			f.item("i1", "j1", "eg", "pending", 0)
		}, "(hidden)", 0},
		// The view no longer asks River: no item is open in an ended
		// Job any more (the module cancels them with it, the upgrade fails or
		// cancels the old ones), so a live River job naming one is ignored.
		{"newest pending in an ended job, live river job", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
			f.job("j1", "completed")
			f.item("i1", "j1", "eg", "pending", 0)
			f.liveRiverJob("i1", "igdb_match")
		}, "unmatched", 0},
		// Failed, but Retry never reopens a failed Job: no retry is offered.
		{"newest failed in a failed job", func(f *rmFixture) {
			f.game("eg", 0, false, "playstation-4")
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
		f.game("eg", 0, false, "playstation-4")
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
		f.game("eg", 0, false, "playstation-4")
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

func TestListExternalGames_OrphanFieldsArePerSet(t *testing.T) {
	f := newRM(t, "list-orphan")
	f.game("eg", 100, false, "playstation-4")
	f.game("ed", 100, false, "playstation-5")
	f.link("ug", 100, "eg", "playstation-4")
	f.link("ug", 100, "ed", "playstation-5")

	g := f.list()["eg"]
	if !g.HasUserGame || g.UserGameID == nil || *g.UserGameID != "ug" {
		t.Errorf("want has_user_game on ug, got %v / %v", g.HasUserGame, g.UserGameID)
	}
	if g.UserGameOtherPlatformCount != 0 {
		t.Errorf("user_game_other_platform_count: want 0 (the other edition's link is in the set), got %d", g.UserGameOtherPlatformCount)
	}

	f.link("ug", 100, "", "playstation-3") // a link from outside the set
	if n := f.list()["eg"].UserGameOtherPlatformCount; n != 1 {
		t.Errorf("user_game_other_platform_count: want 1, got %d", n)
	}
}

func TestListExternalGames_UnskippedNeverMatchedGameIsUnmatched(t *testing.T) {
	f := newRM(t, "list-unmatched")
	f.game("eg", 0, true, "playstation-4")
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
	// Shown as failed, with a failed edition.
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("j1", "completed")
	f.item("i-eg", "j1", "eg", "failed", time.Hour)
	f.item("i-ed", "j1", "ed", "failed", time.Hour)
	// An old failure superseded by a newer completed item.
	f.game("old", 100, false, "playstation-4")
	f.item("i-old", "j1", "old", "failed", 48*time.Hour)
	f.job("j2", "completed")
	f.item("i-old2", "j2", "old", "completed", time.Hour)
	// A skipped game whose newest item failed.
	f.game("sk", 0, true, "playstation-4")
	f.item("i-sk", "j1", "sk", "failed", time.Hour)

	wantCode(t, postJSONAuth(t, f.e, "/api/sync/"+rmStore+"/external-games/retry-failed", nil, f.token), http.StatusNoContent)

	for id, want := range map[string]string{"i-eg": "pending", "i-ed": "pending", "i-old": "failed", "i-sk": "failed"} {
		if s := f.itemStatus(id); s != want {
			t.Errorf("%s: want %s, got %s", id, want, s)
		}
	}
	if n := f.count(`SELECT COUNT(*) FROM river_job WHERE kind = 'igdb_match' AND args->>'job_item_id' IN ('i-eg', 'i-ed')`); n != 2 {
		t.Errorf("queued igdb_match jobs: want 2, got %d", n)
	}
}

// ── Editions in the storefront view and the badge ─────────────────────────────

// One edition still in flight: the entry is hidden and not on the badge,
// although the other edition is in review.
func TestListExternalGames_EditionInFlight_HidesTheEntry(t *testing.T) {
	f := newRM(t, "list-in-flight")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)
	f.item("ji-ed", "job", "ed", "pending", 0)

	if l := f.list(); len(l) != 0 {
		t.Errorf("list = %+v, want the entry hidden", l)
	}
	if n := f.badge(); n != 0 {
		t.Errorf("badge: want 0, got %d", n)
	}
}

// One edition failed: the entry shows as failed and offers that edition's
// item for retry.
func TestListExternalGames_EditionFailed_OffersItsItem(t *testing.T) {
	f := newRM(t, "list-failed")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("job", "completed")
	f.item("ji", "job", "eg", "completed", time.Hour)
	f.item("ji-ed", "job", "ed", "failed", time.Hour)

	l := f.list()
	g, ok := l["eg"]
	if len(l) != 1 || !ok {
		t.Fatalf("list = %+v, want one entry eg", l)
	}
	if g.SyncStatus != "failed" || g.FailedJobItemID == nil || *g.FailedJobItemID != "ji-ed" {
		t.Errorf("entry = %s / %v, want failed offering ji-ed", g.SyncStatus, g.FailedJobItemID)
	}
}

// The entry is the oldest edition still in the storefront library, and it is
// available when any edition is.
func TestListExternalGames_EntryIsTheOldestAvailableEdition(t *testing.T) {
	f := newRM(t, "list-representative")
	f.game("gone", 100, false, "playstation-4")
	f.game("eg", 100, false, "playstation-5")
	f.game("young", 100, false, "playstation-5")
	f.exec(`UPDATE external_games SET is_available = false WHERE id = 'gone'`)

	type entry struct {
		ID          string   `json:"id"`
		IsAvailable bool     `json:"is_available"`
		Platforms   []string `json:"platforms"`
	}
	rec := getAuth(t, f.e, "/api/sync/"+rmStore+"/external-games", f.token)
	wantCode(t, rec, http.StatusOK)
	var entries []entry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "eg" || !entries[0].IsAvailable {
		t.Fatalf("entries = %+v, want one available entry eg", entries)
	}
	if got := strings.Join(entries[0].Platforms, ","); got != "playstation-4,playstation-5" {
		t.Errorf("platforms = %s, want those of every edition", got)
	}

	// None left in the library: the oldest, unavailable.
	f.exec(`UPDATE external_games SET is_available = false`)
	rec = getAuth(t, f.e, "/api/sync/"+rmStore+"/external-games", f.token)
	entries = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "gone" || entries[0].IsAvailable {
		t.Errorf("entries = %+v, want one unavailable entry gone", entries)
	}
}

// The list, the badge and the lock agree on which games form a set: every
// listed entry is one locked set, every game is in exactly one, and the
// badge counts the entries in review.
func TestEditions_ListBadgeAndLockAgree(t *testing.T) {
	f := newRM(t, "editions-agree")
	// Two editions in review, matched editions, a same-title game on another
	// match, a skipped one, an unmatched one with no item, and another title.
	f.game("a1", 0, false, "playstation-4")
	f.game("a2", 0, false, "playstation-5")
	f.game("m1", 100, false, "playstation-4")
	f.game("m2", 100, false, "playstation-5")
	f.game("m3", 300, false, "playstation-4")
	f.game("s1", 100, true, "playstation-4")
	f.titled("t1", "Other", 0, false, "playstation-4")
	f.titled("t2", "Other", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ia1", "job", "a1", "pending_review", 0)
	f.item("ia2", "job", "a2", "pending_review", 0)
	f.item("it1", "job", "t1", "pending_review", 0)

	list := f.list()
	var all []string
	if err := testDB.NewRaw(`SELECT id FROM external_games WHERE user_id = ? ORDER BY id`, f.userID).Scan(context.Background(), &all); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, id := range all {
		var set []models.ExternalGame
		err := testDB.RunInTx(context.Background(), nil, func(ctx context.Context, tx bun.Tx) error {
			var err error
			_, set, err = externalgame.LockEditions(ctx, tx, f.userID, id)
			return err
		})
		if err != nil {
			t.Fatalf("lock %s: %v", id, err)
		}
		var entry string
		for _, eg := range set {
			if _, ok := list[eg.ID]; ok {
				if entry != "" {
					t.Errorf("set of %s has two entries: %s and %s", id, entry, eg.ID)
				}
				entry = eg.ID
			}
		}
		if entry == "" {
			t.Errorf("set of %s has no entry in the list", id)
		}
		if prev, ok := seen[id]; ok && prev != entry {
			t.Errorf("%s is in two sets", id)
		}
		seen[id] = entry
	}
	if len(list) != 5 {
		t.Errorf("entries = %d, want 5 (a, m, m3, s1, t)", len(list))
	}
	review := 0
	for _, g := range list {
		if g.SyncStatus == "needs_review" {
			review++
		}
	}
	if n := f.badge(); n != review || n != 2 {
		t.Errorf("badge = %d, entries in review = %d, want 2", n, review)
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

// Two Rematches at once through different editions' ids do not deadlock and
// end on one match.
func TestRematch_TwoAtOnceThroughDifferentEditions_EndOnOneMatch(t *testing.T) {
	for range 10 {
		f := newRM(t, "rm-two-at-once")
		f.game("eg", 100, false, "playstation-4")
		f.game("ed", 100, false, "playstation-5")
		f.igdbGame(200)
		f.igdbGame(300)

		var wg sync.WaitGroup
		recs := make([]*httptest.ResponseRecorder, 2)
		wg.Add(2)
		go func() { defer wg.Done(); recs[0] = f.rematch("eg", 200, "keep") }()
		go func() { defer wg.Done(); recs[1] = f.rematch("ed", 300, "keep") }()
		wg.Wait()
		for _, rec := range recs {
			if rec.Code != http.StatusNoContent && rec.Code != http.StatusConflict {
				t.Fatalf("rematch: %d %s", rec.Code, rec.Body.String())
			}
		}
		if n := f.count(`SELECT COUNT(DISTINCT resolved_igdb_id) FROM external_games WHERE id IN ('eg', 'ed')`); n != 1 {
			t.Fatalf("matches across the editions: want 1, got %d", n)
		}
	}
}

// waitForLockWait waits until some backend is blocked on a lock.
func waitForLockWait(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := testDB.NewRaw(`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock'`).Scan(context.Background(), &n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no backend blocked on a lock")
}

// takePrecedentInTx takes precedent for egID in a transaction, as a match
// worker does, and returns the commit; the transaction holds the precedent
// FOR SHARE until then.
func takePrecedentInTx(t *testing.T, egID string) (commit func()) {
	t.Helper()
	ctx := context.Background()
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	took, err := externalgame.TakePrecedent(ctx, tx, egID)
	if err != nil || !took {
		t.Fatalf("take precedent: took %v, %v", took, err)
	}
	return func() {
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

// A Rematch or an Unskip of a game while a match worker takes precedent from
// it: the new edition ends on the same match and skip as the rest of the set,
// in either order.
func TestRematchUnskip_RacingAPrecedentTake_NewEditionFollowsTheSet(t *testing.T) {
	type op struct {
		name    string
		skipped bool
		run     func(f *rmFixture) *httptest.ResponseRecorder
		want    string // the match and skip every edition must end on
	}
	ops := []op{
		{"rematch", false, func(f *rmFixture) *httptest.ResponseRecorder { return f.rematch("eg", 200, "keep") }, "200 false"},
		{"unskip", true, func(f *rmFixture) *httptest.ResponseRecorder { return f.unskip("eg") }, "100 false"},
	}
	for _, o := range ops {
		for _, takeFirst := range []bool{true, false} {
			name := o.name + "/take first"
			if !takeFirst {
				name = o.name + "/" + o.name + " first"
			}
			t.Run(name, func(t *testing.T) {
				f := newRM(t, "rm-race-take")
				f.game("eg", 100, o.skipped, "playstation-4")
				f.game("new", 0, false, "playstation-5")
				f.igdbGame(200)

				if takeFirst {
					commit := takePrecedentInTx(t, "new")
					done := make(chan *httptest.ResponseRecorder, 1)
					go func() { done <- o.run(f) }()
					waitForLockWait(t)
					commit()
					wantCode(t, <-done, http.StatusNoContent)
				} else {
					wantCode(t, o.run(f), http.StatusNoContent)
					takePrecedentInTx(t, "new")()
				}

				for _, id := range []string{"eg", "new"} {
					got := f.str(`SELECT resolved_igdb_id::text || ' ' || is_skipped::text FROM external_games WHERE id = ?`, id)
					if got != o.want {
						t.Errorf("%s = %s, want %s", id, got, o.want)
					}
				}
			})
		}
	}
}

// A game whose editions change under every try of the editions lock answers
// 409 after the third try, and writes nothing. Each try reads the game's
// state, then blocks on an edition another transaction holds; that
// transaction moves the game to another match and commits, so the try locks a
// set without the game and starts again.
func TestRematch_EditionsChangeUnderEveryTry_Answers409(t *testing.T) {
	f := newRM(t, "rm-editions-changed")
	f.game("e1", 0, false, "")   // an edition of g as it is now
	f.game("e2", 101, false, "") // of g once it is on 101
	f.game("e3", 102, false, "") // of g once it is on 102
	f.game("g", 0, false, "playstation-4")
	f.igdbGame(103)
	f.igdbGame(200)

	ctx := context.Background()
	holders := make([]bun.Tx, 3)
	pids := make([]int, 3)
	for i, id := range []string{"e1", "e2", "e3"} {
		tx, err := testDB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback() })
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM external_games WHERE id = ? FOR UPDATE`, id); err != nil {
			t.Fatal(err)
		}
		if err := tx.NewRaw(`SELECT pg_backend_pid()`).Scan(ctx, &pids[i]); err != nil {
			t.Fatal(err)
		}
		holders[i] = tx
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.rematch("g", 200, "") }()

	for i, tx := range holders {
		waitForBlockedBy(t, pids[i])
		if _, err := tx.ExecContext(ctx, `UPDATE external_games SET resolved_igdb_id = ? WHERE id = 'g'`, 101+i); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	rec := <-done
	wantCode(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "try again") {
		t.Errorf("body = %s, want the try-again message", rec.Body.String())
	}
	got := f.str(`SELECT string_agg(id || '=' || coalesce(resolved_igdb_id::text, '-'), ' ' ORDER BY id) FROM external_games`)
	if want := "e1=- e2=101 e3=102 g=103"; got != want {
		t.Errorf("games = %s, want %s (nothing written by the refused rematch)", got, want)
	}
}

// waitForBlockedBy waits until a backend is blocked on a lock held by the
// backend pid.
func waitForBlockedBy(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := testDB.NewRaw(`SELECT count(*) FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))`, pid).Scan(context.Background(), &n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no backend blocked by pid %d", pid)
}

// A title with quotes, a backslash, a placeholder character and non-ASCII
// text survives the editions lock's read of the game's state and its binding
// back into the lock: Skip and Rematch act on both editions, and not on a game
// whose title differs only by a trailing space.
func TestEditions_TitleWithQuotesAndSymbols_LockFindsTheSet(t *testing.T) {
	f := newRM(t, "rm-odd-title")
	title := `Marvel's "Spider-Man" \ ? 100% Pokémon™ 戦`
	f.titled("eg", title, 0, false, "playstation-4")
	f.titled("ed", title, 0, false, "playstation-5")
	f.titled("other", title+" ", 0, false, "")
	f.igdbGame(200)

	if n := len(f.list()); n != 2 {
		t.Errorf("listed entries = %d, want 2 (the set, and the other title)", n)
	}
	wantCode(t, f.skip("ed"), http.StatusNoContent)
	wantCode(t, f.rematch("eg", 200, ""), http.StatusNoContent)

	got := f.str(`SELECT string_agg(id || '=' || coalesce(resolved_igdb_id::text, '-') || '/' || is_skipped::text, ' ' ORDER BY id) FROM external_games`)
	if want := "ed=200/false eg=200/false other=-/false"; got != want {
		t.Errorf("games = %s, want %s", got, want)
	}
}
