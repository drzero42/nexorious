package tasks_test

// Tests for editions in a sync (#1449, #1450): a game without a decision
// takes the decision of the oldest same-title game that has one (precedent),
// and a confident match settles same-title games waiting in Pending review.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

const edStore = "steam"

// edGame describes an external_games row for an edition test.
type edGame struct {
	extID      string
	title      string
	storefront string // "" = edStore
	skipped    bool
	available  bool
	resolved   int32 // 0 = unmatched
	age        time.Duration
}

func edInsertGame(t *testing.T, userID string, g edGame) string {
	t.Helper()
	ctx := context.Background()
	if g.title == "" {
		g.title = "Horizon"
	}
	id := uuid.NewString()
	if g.storefront == "" {
		g.storefront = edStore
	}
	var resolved any
	if g.resolved != 0 {
		edEnsureGame(t, g.resolved)
		resolved = g.resolved
	}
	created := time.Now().Add(-g.age - time.Hour)
	if _, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now())`,
		id, userID, g.storefront, g.extID, g.title, g.skipped, g.available, resolved, created,
	).Exec(ctx); err != nil {
		t.Fatalf("insert external game %s: %v", g.extID, err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), id,
	).Exec(ctx); err != nil {
		t.Fatalf("insert platform %s: %v", g.extID, err)
	}
	return id
}

func edEnsureGame(t *testing.T, igdbID int32) {
	t.Helper()
	if _, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert game %d: %v", igdbID, err)
	}
}

func edInsertJob(t *testing.T, userID string, status models.JobStatus, dispatchComplete bool) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete)
		 VALUES (?, ?, 'sync', ?, ?, 'normal', 0, ?)`,
		id, userID, edStore, status, dispatchComplete,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func edInsertItem(t *testing.T, jobID, userID, egID string, status models.JobItemStatus) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'Horizon', ?, '{}', ?, '{}', '[]', now())`,
		id, jobID, userID, uuid.NewString(), egID, status,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	return id
}

func edSetup(t *testing.T) string {
	t.Helper()
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, ?, 'daily')`,
		uuid.NewString(), userID, edStore,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert sync config: %v", err)
	}
	return userID
}

// edDispatch runs a dispatch for the job that fetches the given batches.
func edDispatch(t *testing.T, rc *river.Client[pgx.Tx], jobID, userID string, batches ...[]tasks.ExternalGameEntry) {
	t.Helper()
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{batches: batches}), LC: newTestLC(t)}
	if err := w.Work(context.Background(), syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: edStore})); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func edEntry(extID string) tasks.ExternalGameEntry {
	return tasks.ExternalGameEntry{ExternalID: extID, Title: "Horizon", Platforms: []string{"pc-windows"}}
}

func edMatch(t *testing.T, rc *river.Client[pgx.Tx], itemID string) error {
	t.Helper()
	w := &tasks.IGDBMatchWorker{DB: testDB, LC: newTestLC(t)}
	return w.Work(context.Background(), matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID}))
}

type edEG struct {
	IsSkipped bool   `bun:"is_skipped"`
	Resolved  *int32 `bun:"resolved_igdb_id"`
}

func edGet(t *testing.T, userID, extID string) (string, edEG) {
	t.Helper()
	var row struct {
		ID string `bun:"id"`
		edEG
	}
	if err := testDB.NewRaw(
		`SELECT id, is_skipped, resolved_igdb_id FROM external_games WHERE user_id = ? AND external_id = ?`, userID, extID,
	).Scan(context.Background(), &row); err != nil {
		t.Fatalf("load external game %s: %v", extID, err)
	}
	return row.ID, row.edEG
}

func edItemStatus(t *testing.T, itemID string) (status models.JobItemStatus, msg *string) {
	t.Helper()
	var row struct {
		Status models.JobItemStatus `bun:"status"`
		Msg    *string              `bun:"error_message"`
	}
	if err := testDB.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &row); err != nil {
		t.Fatalf("load item: %v", err)
	}
	return row.Status, row.Msg
}

func edItemsFor(t *testing.T, jobID, egID string) []string {
	t.Helper()
	var ids []string
	if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ? AND external_game_id = ?`, jobID, egID).Scan(context.Background(), &ids); err != nil {
		t.Fatalf("load items: %v", err)
	}
	return ids
}

func edJobStatus(t *testing.T, jobID string) models.JobStatus {
	t.Helper()
	var s models.JobStatus
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(context.Background(), &s); err != nil {
		t.Fatalf("load job: %v", err)
	}
	return s
}

func edLiveRiverJobs(t *testing.T, itemID string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ? AND state NOT IN ('completed', 'cancelled', 'discarded')`, itemID,
	).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count river jobs: %v", err)
	}
	return n
}

func edCount(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(q, args...).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// ── The status guard ──────────────────────────────────────────────────────────

func TestSyncItemWorkers_ItemNotPending_WritesNothing(t *testing.T) {
	for _, status := range []models.JobItemStatus{"completed", "failed", "skipped", "cancelled", "pending_review"} {
		t.Run(string(status), func(t *testing.T) {
			userID := edSetup(t)
			rc := newTestRiverClient(t)
			jobID := edInsertJob(t, userID, "processing", true)
			matched := edInsertGame(t, userID, edGame{extID: "M", available: true, resolved: 501})
			unmatched := edInsertGame(t, userID, edGame{extID: "U", title: "Other", available: true})
			writeItem := edInsertItem(t, jobID, userID, matched, status)
			matchItem := edInsertItem(t, jobID, userID, unmatched, status)

			uw := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t), RiverClient: rc}
			if err := uw.Work(context.Background(), writeJob(t, tasks.UserGameArgs{JobItemID: writeItem})); err != nil {
				t.Fatalf("user_game_write: %v", err)
			}
			if err := edMatch(t, rc, matchItem); err != nil {
				t.Fatalf("igdb_match: %v", err)
			}

			if n := edCount(t, `SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID); n != 0 {
				t.Errorf("user_games: want 0, got %d", n)
			}
			for _, id := range []string{writeItem, matchItem} {
				if got, _ := edItemStatus(t, id); got != status {
					t.Errorf("item status: want %q, got %q", status, got)
				}
			}
			if n := edCount(t, `SELECT COUNT(*) FROM river_job`); n != 0 {
				t.Errorf("river jobs: want 0, got %d", n)
			}
		})
	}
}

// ── Precedent ─────────────────────────────────────────────────────────────────

// edDrain runs the sync pipeline without IGDB: a game that is searched goes to
// Pending review.
func edDrain(t *testing.T, lc *joblifecycle.Module) {
	t.Helper()
	drainSync(t, &tasks.IGDBMatchWorker{DB: testDB, LC: lc}, &tasks.UserGameWorker{DB: testDB, LC: lc})
}

// edIGDBWorker is a match worker whose IGDB answers every search with results.
func edIGDBWorker(t *testing.T, lc *joblifecycle.Module, results []map[string]any) *tasks.IGDBMatchWorker {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	t.Cleanup(tokenSrv.Close)
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(results)
	}))
	t.Cleanup(igdbSrv.Close)
	return &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL), LC: lc}
}

// confident is an IGDB answer that matches the title "Horizon" with confidence.
func confident(id int) []map[string]any {
	return []map[string]any{{"id": id, "name": "Horizon", "slug": "horizon"}}
}

// notConfident is an IGDB answer that matches nothing with confidence.
var notConfident = []map[string]any{{"id": 999, "name": "Something else", "slug": "x"}}

// A new edition of a skipped game takes the skip: its item ends skipped and
// nothing is written. A game that is skipped when the dispatch sees it gets
// no item at all.
func TestSync_NewEditionOfSkippedGame_TakesTheSkip(t *testing.T) {
	cases := []struct {
		name      string
		resolved  int32
		available bool
	}{
		{"matched", 601, true},
		{"unmatched", 0, true},
		{"left the library", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := edSetup(t)
			skippedID := edInsertGame(t, userID, edGame{extID: "P", skipped: true, available: tc.available, resolved: tc.resolved, age: time.Hour})
			jobID := edInsertJob(t, userID, "pending", true)

			entries := []tasks.ExternalGameEntry{edEntry("NEW")}
			if tc.available {
				entries = append(entries, edEntry("P"))
			}
			edDispatch(t, nil, jobID, userID, entries)
			if ids := edItemsFor(t, jobID, skippedID); len(ids) != 0 {
				t.Errorf("the skipped game must get no item, got %d", len(ids))
			}
			newID, _ := edGet(t, userID, "NEW")
			items := edItemsFor(t, jobID, newID)
			if len(items) != 1 {
				t.Fatalf("the new edition must get one item, got %d", len(items))
			}

			edDrain(t, newTestLC(t))

			_, eg := edGet(t, userID, "NEW")
			if !eg.IsSkipped || !sameInt32(eg.Resolved, tc.resolved) {
				t.Errorf("new edition = skipped %v, match %v; want skipped with match %d", eg.IsSkipped, eg.Resolved, tc.resolved)
			}
			if s, _ := edItemStatus(t, items[0]); s != models.JobItemStatusSkipped {
				t.Errorf("item = %q, want skipped", s)
			}
			if n := edCount(t, `SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID); n != 0 {
				t.Errorf("user_games: want 0, got %d", n)
			}
			if got := edJobStatus(t, jobID); got != models.JobStatusCompleted {
				t.Errorf("job = %q, want completed", got)
			}
		})
	}
}

// sameInt32 compares a nullable match with want, 0 meaning unmatched.
func sameInt32(got *int32, want int32) bool {
	if want == 0 {
		return got == nil
	}
	return got != nil && *got == want
}

// A new edition of a matched game takes the match without a search (no IGDB
// here: a search would send it to review) and is written, also when the
// matched game has left the library.
func TestSync_NewEditionOfMatchedGame_TakesTheMatch(t *testing.T) {
	for _, available := range []bool{true, false} {
		name := "in the library"
		if !available {
			name = "left the library"
		}
		t.Run(name, func(t *testing.T) {
			userID := edSetup(t)
			edInsertGame(t, userID, edGame{extID: "P", available: available, resolved: 701, age: time.Hour})
			jobID := edInsertJob(t, userID, "pending", true)
			entries := []tasks.ExternalGameEntry{edEntry("NEW")}
			if available {
				entries = append(entries, edEntry("P"))
			}
			edDispatch(t, nil, jobID, userID, entries)
			edDrain(t, newTestLC(t))

			newID, eg := edGet(t, userID, "NEW")
			if !sameInt32(eg.Resolved, 701) || eg.IsSkipped {
				t.Errorf("new edition = match %v skipped %v, want 701", eg.Resolved, eg.IsSkipped)
			}
			if s, _ := edItemStatus(t, edItemsFor(t, jobID, newID)[0]); s != models.JobItemStatusCompleted {
				t.Errorf("item = %q, want completed", s)
			}
			// Both editions are on the same platform, so the library has one
			// platform row, linked to whichever edition wrote it first.
			if n := edCount(t, `SELECT COUNT(*) FROM user_game_platforms ugp
				JOIN external_games eg ON eg.id = ugp.external_game_id WHERE eg.user_id = ?`, userID); n != 1 {
				t.Errorf("library links of the editions = %d, want 1", n)
			}
			if n := ugCountFor(t, userID, 701); n != 1 {
				t.Errorf("user games on 701 = %d, want 1", n)
			}
		})
	}
}

// Only a decision is a precedent: a same-title game without one, and a
// same-title game on another storefront, leave the new game to the search.
func TestSync_NoPrecedent_NewGameIsSearched(t *testing.T) {
	cases := []struct {
		name  string
		other edGame
	}{
		{"same-title game without a decision", edGame{extID: "U", available: false}},
		{"decided same-title game on another storefront", edGame{extID: "G", storefront: "gog", available: true, resolved: 100}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := edSetup(t)
			tc.other.age = time.Hour
			edInsertGame(t, userID, tc.other)
			jobID := edInsertJob(t, userID, "pending", true)
			edDispatch(t, nil, jobID, userID, []tasks.ExternalGameEntry{edEntry("NEW")})
			edDrain(t, newTestLC(t))

			newID, eg := edGet(t, userID, "NEW")
			if eg.Resolved != nil || eg.IsSkipped {
				t.Errorf("new game = match %v skipped %v, want no decision", eg.Resolved, eg.IsSkipped)
			}
			if s, _ := edItemStatus(t, edItemsFor(t, jobID, newID)[0]); s != models.JobItemStatusPendingReview {
				t.Errorf("item = %q, want pending_review (searched, no IGDB)", s)
			}
		})
	}
}

// Same-title games that disagree: a new edition takes the oldest decision, and
// the others are left alone.
func TestSync_SameTitleGamesDisagree_OldestDecisionWins(t *testing.T) {
	cases := []struct {
		name        string
		oldest      edGame
		wantSkipped bool
		wantMatch   int32
	}{
		{"oldest is a match", edGame{extID: "A", resolved: 100}, false, 100},
		{"oldest is a skip", edGame{extID: "A", skipped: true}, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := edSetup(t)
			tc.oldest.age = 3 * time.Hour
			edInsertGame(t, userID, tc.oldest)
			edInsertGame(t, userID, edGame{extID: "B", resolved: 200, age: 2 * time.Hour})
			jobID := edInsertJob(t, userID, "pending", true)
			edDispatch(t, nil, jobID, userID, []tasks.ExternalGameEntry{edEntry("NEW")})
			edDrain(t, newTestLC(t))

			_, eg := edGet(t, userID, "NEW")
			if eg.IsSkipped != tc.wantSkipped || !sameInt32(eg.Resolved, tc.wantMatch) {
				t.Errorf("new edition = skipped %v match %v, want skipped %v match %d", eg.IsSkipped, eg.Resolved, tc.wantSkipped, tc.wantMatch)
			}
			if _, a := edGet(t, userID, "A"); a.IsSkipped != tc.oldest.skipped || !sameInt32(a.Resolved, tc.oldest.resolved) {
				t.Errorf("A changed: %+v", a)
			}
			if _, b := edGet(t, userID, "B"); b.IsSkipped || !sameInt32(b.Resolved, 200) {
				t.Errorf("B changed: %+v", b)
			}
		})
	}
}

// A game without a decision beside a decided same-title game takes it at the
// next sync, whether it is new or was already known.
func TestSync_UndecidedGameBesideADecidedOne_TakesItAtTheNextSync(t *testing.T) {
	userID := edSetup(t)
	edInsertGame(t, userID, edGame{extID: "D", available: true, resolved: 300, age: 3 * time.Hour})
	edInsertGame(t, userID, edGame{extID: "U", available: true, age: 2 * time.Hour})
	jobID := edInsertJob(t, userID, "pending", true)
	edDispatch(t, nil, jobID, userID, []tasks.ExternalGameEntry{edEntry("D"), edEntry("U"), edEntry("NEW")})
	edDrain(t, newTestLC(t))

	for _, ext := range []string{"U", "NEW"} {
		id, eg := edGet(t, userID, ext)
		if !sameInt32(eg.Resolved, 300) {
			t.Errorf("%s: match %v, want 300", ext, eg.Resolved)
		}
		if s, _ := edItemStatus(t, edItemsFor(t, jobID, id)[0]); s != models.JobItemStatusCompleted {
			t.Errorf("%s: item %q, want completed", ext, s)
		}
	}
	if n := ugCountFor(t, userID, 300); n != 1 {
		t.Errorf("user games on 300 = %d, want 1", n)
	}
	if got := edJobStatus(t, jobID); got != models.JobStatusCompleted {
		t.Errorf("job = %q, want completed", got)
	}
}

// An edition decided while the other is searched: the searched one takes that
// decision, whatever its search found, and does not go to review.
func TestIGDBMatchWorker_EditionDecidedDuringTheSearch_IsTaken(t *testing.T) {
	cases := []struct {
		name    string
		results []map[string]any
	}{
		{"search not confident", notConfident},
		{"search confident on another game", confident(500)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := edSetup(t)
			aID := edInsertGame(t, userID, edGame{extID: "A", available: true, age: 2 * time.Hour})
			bID := edInsertGame(t, userID, edGame{extID: "B", available: true, age: time.Hour})
			jobID := edInsertJob(t, userID, "processing", true)
			itemID := edInsertItem(t, jobID, userID, bID, "pending")

			igdbMatchWithSearchHook(t, itemID, tc.results, func() { setMatch(t, aID, 200) })

			if _, b := edGet(t, userID, "B"); !sameInt32(b.Resolved, 200) {
				t.Errorf("B match = %v, want A's 200", b.Resolved)
			}
			if s, _ := edItemStatus(t, itemID); s != models.JobItemStatusPending {
				t.Errorf("item = %q, want pending (on to the write stage)", s)
			}
			if n := edCount(t, `SELECT COUNT(*) FROM river_job WHERE kind = 'user_game_write' AND args->>'job_item_id' = ?`, itemID); n != 1 {
				t.Errorf("user_game_write jobs = %d, want 1", n)
			}
		})
	}
}

// ── Settle ────────────────────────────────────────────────────────────────────

// One edition matches with confidence and the other went to review: settle
// gives the other the match and completes its item, so neither stays in review,
// and the library gets one user game with one 'added' row.
func TestSync_OneEditionMatchesConfidently_SettlesTheOther(t *testing.T) {
	ctx := context.Background()
	userID := edSetup(t)
	lc := newTestLC(t)
	aID := edInsertGame(t, userID, edGame{extID: "A", available: true, age: 2 * time.Hour})
	bID := edInsertGame(t, userID, edGame{extID: "B", available: true, age: time.Hour})
	jobID := edInsertJob(t, userID, "processing", true)
	itemA := edInsertItem(t, jobID, userID, aID, "pending")
	itemB := edInsertItem(t, jobID, userID, bID, "pending")

	if err := (&tasks.IGDBMatchWorker{DB: testDB, LC: lc}).Work(ctx, matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemB})); err != nil {
		t.Fatal(err)
	}
	if s, _ := edItemStatus(t, itemB); s != models.JobItemStatusPendingReview {
		t.Fatalf("B's item = %q, want pending_review", s)
	}
	if err := edIGDBWorker(t, lc, confident(500)).Work(ctx, matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemA})); err != nil {
		t.Fatal(err)
	}
	if s, _ := edItemStatus(t, itemB); s != models.JobItemStatusCompleted {
		t.Errorf("B's item = %q, want completed by settle", s)
	}
	if _, b := edGet(t, userID, "B"); !sameInt32(b.Resolved, 500) {
		t.Errorf("B match = %v, want 500", b.Resolved)
	}
	if n := edCount(t, `SELECT COUNT(*) FROM user_game_platforms WHERE external_game_id = ?`, bID); n != 1 {
		t.Errorf("B's library links = %d, want 1", n)
	}

	edDrain(t, lc)
	if s, _ := edItemStatus(t, itemA); s != models.JobItemStatusCompleted {
		t.Errorf("A's item = %q, want completed", s)
	}
	if n := ugCountFor(t, userID, 500); n != 1 {
		t.Errorf("user games on 500 = %d, want 1", n)
	}
	if n := edCount(t, `SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'added'`, jobID); n != 1 {
		t.Errorf("'added' rows = %d, want 1", n)
	}
	if got := edJobStatus(t, jobID); got != models.JobStatusCompleted {
		t.Errorf("job = %q, want completed", got)
	}
}

// Settle only gives a match: when the oldest decision of the title is a skip
// (it can only have appeared in a race), the game waiting in review is left as
// it is and its item stays in review.
func TestSettle_OldestDecisionIsASkip_LeavesTheGameInReview(t *testing.T) {
	ctx := context.Background()
	userID := edSetup(t)
	lc := newTestLC(t)
	edInsertGame(t, userID, edGame{extID: "A", available: true, skipped: true, age: 3 * time.Hour})
	cID := edInsertGame(t, userID, edGame{extID: "C", available: true, resolved: 500, age: 2 * time.Hour})
	bID := edInsertGame(t, userID, edGame{extID: "B", available: true, age: time.Hour})
	jobID := edInsertJob(t, userID, "processing", true)
	itemC := edInsertItem(t, jobID, userID, cID, "pending")
	itemB := edInsertItem(t, jobID, userID, bID, "pending_review")

	if err := tasks.CompleteSameTitleReviewsForTest(ctx, &tasks.IGDBMatchWorker{DB: testDB, LC: lc}, itemC, cID); err != nil {
		t.Fatal(err)
	}
	if _, b := edGet(t, userID, "B"); b.Resolved != nil || b.IsSkipped {
		t.Errorf("B = match %v skipped %v, want unchanged", b.Resolved, b.IsSkipped)
	}
	if s, _ := edItemStatus(t, itemB); s != models.JobItemStatusPendingReview {
		t.Errorf("B's item = %q, want pending_review", s)
	}
}

// ── Recovery ──────────────────────────────────────────────────────────────────

// An item that waited on another item when the upgrade happened (pending, no
// owner, in a Job whose dispatch is done) is queued by recovery alone, and its
// game takes the same-title game's match.
func TestSyncRecovery_ItemThatWaitedBeforeTheUpgrade_IsQueued(t *testing.T) {
	ctx := context.Background()
	userID := edSetup(t)
	lc := newTestLC(t)
	edInsertGame(t, userID, edGame{extID: "P", available: true, resolved: 900, age: 2 * time.Hour})
	cID := edInsertGame(t, userID, edGame{extID: "C", available: true, age: time.Hour})
	jobID := edInsertJob(t, userID, "processing", true)
	itemID := edInsertItem(t, jobID, userID, cID, "pending")

	if _, err := lc.Recover(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if n := edLiveRiverJobs(t, itemID); n != 1 {
		t.Fatalf("live River jobs for the item = %d, want 1", n)
	}
	edDrain(t, lc)
	if _, c := edGet(t, userID, "C"); !sameInt32(c.Resolved, 900) {
		t.Errorf("C match = %v, want 900", c.Resolved)
	}
	if s, _ := edItemStatus(t, itemID); s != models.JobItemStatusCompleted {
		t.Errorf("item = %q, want completed", s)
	}
	if got := edJobStatus(t, jobID); got != models.JobStatusCompleted {
		t.Errorf("job = %q, want completed", got)
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

// The match jobs of two editions run at the same time, many rounds: whichever
// order precedent, review and settle end up in, the two end on one match,
// neither stays in review, and the library gets one user game.
func TestIGDBMatchWorker_TwoEditionsAtOnce_EndOnOneMatch(t *testing.T) {
	cases := []struct {
		name     string
		resultsA []map[string]any
		resultsB []map[string]any
	}{
		{"both confident on different games", confident(500), confident(600)},
		{"one confident, one not", confident(500), notConfident},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lc := newTestLC(t)
			wA := edIGDBWorker(t, lc, tc.resultsA)
			wB := edIGDBWorker(t, lc, tc.resultsB)
			for round := range 15 {
				ctx := context.Background()
				userID := edSetup(t)
				aID := edInsertGame(t, userID, edGame{extID: "A", available: true, age: 2 * time.Hour})
				bID := edInsertGame(t, userID, edGame{extID: "B", available: true, age: time.Hour})
				jobID := edInsertJob(t, userID, "processing", true)
				itemA := edInsertItem(t, jobID, userID, aID, "pending")
				itemB := edInsertItem(t, jobID, userID, bID, "pending")
				jobA := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemA})
				jobB := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemB})

				var wg sync.WaitGroup
				var start sync.WaitGroup
				start.Add(1)
				var failed atomic.Value
				run := func(w *tasks.IGDBMatchWorker, j *river.Job[tasks.IGDBMatchArgs]) {
					defer wg.Done()
					start.Wait()
					if err := w.Work(ctx, j); err != nil {
						failed.Store(err)
					}
				}
				wg.Add(2)
				go run(wA, jobA)
				go run(wB, jobB)
				start.Done()
				wg.Wait()
				if err, _ := failed.Load().(error); err != nil {
					t.Fatalf("round %d: igdb_match: %v", round, err)
				}
				edDrain(t, lc)

				_, a := edGet(t, userID, "A")
				_, b := edGet(t, userID, "B")
				if a.Resolved == nil || b.Resolved == nil || *a.Resolved != *b.Resolved || a.IsSkipped || b.IsSkipped {
					t.Fatalf("round %d: A = %+v, B = %+v, want one match", round, a, b)
				}
				for _, it := range []string{itemA, itemB} {
					if s, msg := edItemStatus(t, it); s != models.JobItemStatusCompleted {
						t.Fatalf("round %d: item %q %v, want completed", round, s, msg)
					}
				}
				if n := edCount(t, `SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID); n != 1 {
					t.Fatalf("round %d: user games = %d, want 1", round, n)
				}
				if got := edJobStatus(t, jobID); got != models.JobStatusCompleted {
					t.Fatalf("round %d: job = %q, want completed", round, got)
				}
			}
		})
	}
}

// ── Dispatch ──────────────────────────────────────────────────────────────────

func TestDispatchSync_RunAgain_GivesNoItemASecondRiverJob(t *testing.T) {
	userID := edSetup(t)
	rc := newTestRiverClient(t)
	jobID := edInsertJob(t, userID, "pending", true)
	entries := []tasks.ExternalGameEntry{edEntry("A"), {ExternalID: "B", Title: "Other", Platforms: []string{"pc-windows"}}}

	edDispatch(t, rc, jobID, userID, entries)
	// As if the first run was cut off before it marked the dispatch done.
	if _, err := testDB.ExecContext(context.Background(), `UPDATE jobs SET dispatch_complete = false WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	edDispatch(t, rc, jobID, userID, entries)

	var items []string
	if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ?`, jobID).Scan(context.Background(), &items); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	for _, id := range items {
		if n := edCount(t, `SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ?`, id); n != 1 {
			t.Errorf("item %s: want one River job, got %d", id, n)
		}
	}
}

// cancellingAdapter cancels the sync through the module (as HandleCancelJob
// does) after its first batch, then sends a second one.
type cancellingAdapter struct {
	jobID string
	lc    *joblifecycle.Module
}

func (a *cancellingAdapter) GetLibrary(ctx context.Context, _ int, onBatch func([]tasks.ExternalGameEntry) error) error {
	if err := onBatch([]tasks.ExternalGameEntry{edEntry("A")}); err != nil {
		return err
	}
	if err := a.lc.Cancel(ctx, a.jobID); err != nil {
		return err
	}
	return onBatch([]tasks.ExternalGameEntry{{ExternalID: "B", Title: "Other", Platforms: []string{"pc-windows"}}})
}

// Cancel during a running dispatch: the dispatch adds no further items, the
// sweeps do not run, and nothing is left open or queued.
func TestDispatchSync_CancelledDuringDispatch(t *testing.T) {
	userID := edSetup(t)
	lc := newTestLC(t)
	edInsertGame(t, userID, edGame{extID: "X", title: "Elsewhere", available: true})
	jobID := edInsertJob(t, userID, "pending", true)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&cancellingAdapter{jobID: jobID, lc: lc}), LC: lc}
	if err := w.Work(context.Background(), syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: edStore})); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if got := edJobStatus(t, jobID); got != "cancelled" {
		t.Fatalf("job: want cancelled, got %q", got)
	}
	if n := edCount(t, `SELECT COUNT(*) FROM external_games WHERE user_id = ? AND external_id = 'B'`, userID); n != 0 {
		t.Errorf("the dispatch went on after the cancel: B was written")
	}
	if n := edCount(t, `SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'pending_review')`, jobID); n != 0 {
		t.Errorf("%d open items left", n)
	}
	if n := edCount(t, `SELECT COUNT(*) FROM river_job WHERE state IN ('available', 'scheduled', 'retryable', 'pending')`); n != 0 {
		t.Errorf("%d River jobs left queued", n)
	}
	if n := edCount(t, `SELECT COUNT(*) FROM external_games WHERE user_id = ? AND external_id = 'X' AND is_available`, userID); n != 1 {
		t.Errorf("the removed-game sweep ran after the cancel")
	}
}
