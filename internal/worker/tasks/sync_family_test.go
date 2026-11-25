package tasks_test

// Tests for Child games in a sync (#1339): a child never waits on a parent
// that has nothing in the sync that could match it.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

const famStore = "steam"

// famGame describes an external_games row for a family test.
type famGame struct {
	extID     string
	title     string
	parent    string // "" = top-level
	skipped   bool
	available bool
	resolved  int32 // 0 = unmatched
	age       time.Duration
}

func famInsertGame(t *testing.T, userID string, g famGame) string {
	t.Helper()
	ctx := context.Background()
	if g.title == "" {
		g.title = "Horizon"
	}
	id := uuid.NewString()
	var parent, resolved any
	if g.parent != "" {
		parent = g.parent
	}
	if g.resolved != 0 {
		famEnsureGame(t, g.resolved)
		resolved = g.resolved
	}
	created := time.Now().Add(-g.age - time.Hour)
	if _, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, resolved_igdb_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, false, ?, ?, ?, now())`,
		id, userID, famStore, g.extID, g.title, g.skipped, g.available, parent, resolved, created,
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

func famEnsureGame(t *testing.T, igdbID int32) {
	t.Helper()
	if _, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert game %d: %v", igdbID, err)
	}
}

func famInsertJob(t *testing.T, userID, status string, dispatchComplete bool) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete)
		 VALUES (?, ?, 'sync', ?, ?, 'normal', 0, ?)`,
		id, userID, famStore, status, dispatchComplete,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func famInsertItem(t *testing.T, jobID, userID, egID, status string) string {
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

// famLink puts the external game in the library under igdbID and returns the
// user game's id.
func famLink(t *testing.T, userID, egID string, igdbID int32) string {
	t.Helper()
	ctx := context.Background()
	famEnsureGame(t, igdbID)
	ugID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`, ugID, userID, igdbID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user game: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, external_game_id, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', ?, true, 0, 'owned', true, ?, now(), now())`,
		uuid.NewString(), ugID, famStore, egID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user game platform: %v", err)
	}
	return ugID
}

func famSetup(t *testing.T) string {
	t.Helper()
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, ?, 'daily')`,
		uuid.NewString(), userID, famStore,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert sync config: %v", err)
	}
	return userID
}

// famDispatch runs a dispatch for the job that fetches the given batches.
func famDispatch(t *testing.T, rc *river.Client[pgx.Tx], jobID, userID string, batches ...[]tasks.ExternalGameEntry) {
	t.Helper()
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{batches: batches}), LC: newTestLC(t)}
	if err := w.Work(context.Background(), syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: famStore})); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func famEntry(extID string) tasks.ExternalGameEntry {
	return tasks.ExternalGameEntry{ExternalID: extID, Title: "Horizon", Platforms: []string{"pc-windows"}}
}

func famMatch(t *testing.T, rc *river.Client[pgx.Tx], itemID string) error {
	t.Helper()
	w := &tasks.IGDBMatchWorker{DB: testDB, LC: newTestLC(t)}
	return w.Work(context.Background(), matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID}))
}

type famEG struct {
	ParentID  *string `bun:"parent_id"`
	IsSkipped bool    `bun:"is_skipped"`
	Resolved  *int32  `bun:"resolved_igdb_id"`
}

func famGet(t *testing.T, userID, extID string) (string, famEG) {
	t.Helper()
	var row struct {
		ID string `bun:"id"`
		famEG
	}
	if err := testDB.NewRaw(
		`SELECT id, parent_id, is_skipped, resolved_igdb_id FROM external_games WHERE user_id = ? AND external_id = ?`, userID, extID,
	).Scan(context.Background(), &row); err != nil {
		t.Fatalf("load external game %s: %v", extID, err)
	}
	return row.ID, row.famEG
}

func famItemStatus(t *testing.T, itemID string) (status string, msg *string) {
	t.Helper()
	var row struct {
		Status string  `bun:"status"`
		Msg    *string `bun:"error_message"`
	}
	if err := testDB.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &row); err != nil {
		t.Fatalf("load item: %v", err)
	}
	return row.Status, row.Msg
}

func famItemsFor(t *testing.T, jobID, egID string) []string {
	t.Helper()
	var ids []string
	if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ? AND external_game_id = ?`, jobID, egID).Scan(context.Background(), &ids); err != nil {
		t.Fatalf("load items: %v", err)
	}
	return ids
}

func famJobStatus(t *testing.T, jobID string) string {
	t.Helper()
	var s string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(context.Background(), &s); err != nil {
		t.Fatalf("load job: %v", err)
	}
	return s
}

func famLiveRiverJobs(t *testing.T, itemID string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ? AND state NOT IN ('completed', 'cancelled', 'discarded')`, itemID,
	).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count river jobs: %v", err)
	}
	return n
}

func famCount(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(q, args...).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// ── Rule 1: the status guard ──────────────────────────────────────────────────

func TestSyncItemWorkers_ItemNotPending_WritesNothing(t *testing.T) {
	for _, status := range []string{"completed", "failed", "skipped", "cancelled", "pending_review"} {
		t.Run(status, func(t *testing.T) {
			userID := famSetup(t)
			rc := newTestRiverClient(t)
			jobID := famInsertJob(t, userID, "processing", true)
			matched := famInsertGame(t, userID, famGame{extID: "M", available: true, resolved: 501})
			unmatched := famInsertGame(t, userID, famGame{extID: "U", title: "Other", available: true})
			writeItem := famInsertItem(t, jobID, userID, matched, status)
			matchItem := famInsertItem(t, jobID, userID, unmatched, status)

			uw := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t), RiverClient: rc}
			if err := uw.Work(context.Background(), writeJob(t, tasks.UserGameArgs{JobItemID: writeItem})); err != nil {
				t.Fatalf("user_game_write: %v", err)
			}
			if err := famMatch(t, rc, matchItem); err != nil {
				t.Fatalf("igdb_match: %v", err)
			}

			if n := famCount(t, `SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID); n != 0 {
				t.Errorf("user_games: want 0, got %d", n)
			}
			for _, id := range []string{writeItem, matchItem} {
				if got, _ := famItemStatus(t, id); got != status {
					t.Errorf("item status: want %q, got %q", status, got)
				}
			}
			if n := famCount(t, `SELECT COUNT(*) FROM river_job`); n != 0 {
				t.Errorf("river jobs: want 0, got %d", n)
			}
		})
	}
}

// ── Rules 2 and 3: a new edition's parent and skip ───────────────────────────

func TestDispatchSync_NewEditionOfSkippedGame_GetsNoItem(t *testing.T) {
	cases := []struct {
		name      string
		resolved  int32
		available bool
	}{
		{"matched", 601, true},
		{"unmatched", 0, true},
		{"unavailable", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := famSetup(t)
			rc := newTestRiverClient(t)
			parentID := famInsertGame(t, userID, famGame{extID: "P", skipped: true, available: tc.available, resolved: tc.resolved})
			jobID := famInsertJob(t, userID, "pending", true)

			entries := []tasks.ExternalGameEntry{famEntry("NEW")}
			if tc.available {
				entries = append(entries, famEntry("P"))
			}
			famDispatch(t, rc, jobID, userID, entries)

			newID, eg := famGet(t, userID, "NEW")
			if eg.ParentID == nil || *eg.ParentID != parentID {
				t.Fatalf("new edition parent: want %s, got %v", parentID, eg.ParentID)
			}
			if !eg.IsSkipped {
				t.Error("new edition must inherit the parent's skip")
			}
			if ids := famItemsFor(t, jobID, newID); len(ids) != 0 {
				t.Errorf("new edition must get no item, got %d", len(ids))
			}
			if n := famCount(t, `SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID); n != 0 {
				t.Errorf("user_games: want 0, got %d", n)
			}
			if got := famJobStatus(t, jobID); got != "completed" {
				t.Errorf("job status: want completed, got %q", got)
			}
		})
	}
}

func TestDispatchSync_DepartedUnmatchedGameIsNotAParent(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	famInsertGame(t, userID, famGame{extID: "GONE", available: false})
	jobID := famInsertJob(t, userID, "pending", true)

	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{famEntry("NEW")})

	newID, eg := famGet(t, userID, "NEW")
	if eg.ParentID != nil {
		t.Fatalf("new edition must be top-level, got parent %s", *eg.ParentID)
	}
	items := famItemsFor(t, jobID, newID)
	if len(items) != 1 {
		t.Fatalf("want one item, got %d", len(items))
	}
	if err := famMatch(t, rc, items[0]); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	if got, _ := famItemStatus(t, items[0]); got != "pending_review" {
		t.Errorf("item status: want pending_review, got %q", got)
	}
}

func TestDispatchSync_OldestEligibleGameIsTheParent(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	famInsertGame(t, userID, famGame{extID: "B", available: true, age: time.Hour})
	older := famInsertGame(t, userID, famGame{extID: "A", available: true, age: 2 * time.Hour})
	jobID := famInsertJob(t, userID, "pending", true)

	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{famEntry("A"), famEntry("B"), famEntry("NEW")})

	_, eg := famGet(t, userID, "NEW")
	if eg.ParentID == nil || *eg.ParentID != older {
		t.Errorf("parent: want the oldest (%s), got %v", older, eg.ParentID)
	}
}

func TestDispatchSync_NewEditionOfMatchedDepartedGameInherits(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: false, resolved: 701})
	jobID := famInsertJob(t, userID, "pending", true)

	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{famEntry("NEW")})

	newID, eg := famGet(t, userID, "NEW")
	if eg.ParentID == nil || *eg.ParentID != parentID {
		t.Fatalf("parent: want %s, got %v", parentID, eg.ParentID)
	}
	items := famItemsFor(t, jobID, newID)
	if len(items) != 1 {
		t.Fatalf("want one item, got %d", len(items))
	}
	if err := famMatch(t, rc, items[0]); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	if _, eg := famGet(t, userID, "NEW"); eg.Resolved == nil || *eg.Resolved != 701 {
		t.Errorf("resolved: want 701 (inherited), got %v", eg.Resolved)
	}
}

// ── Rule 2: the skip repair ──────────────────────────────────────────────────

func TestDispatchSync_RepairsChildSkips(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	// A skipped parent with a child that missed the skip.
	p1 := famInsertGame(t, userID, famGame{extID: "P1", title: "One", skipped: true, available: true})
	famInsertGame(t, userID, famGame{extID: "C1", title: "One", parent: p1, available: true})
	// An unskipped (rematched) parent whose children stayed skipped: one with a
	// stale match of its own, one whose match already equals the parent's.
	p2 := famInsertGame(t, userID, famGame{extID: "P2", title: "Two", available: true, resolved: 200})
	c2 := famInsertGame(t, userID, famGame{extID: "C2", title: "Two", parent: p2, skipped: true, available: true, resolved: 300})
	c3 := famInsertGame(t, userID, famGame{extID: "C3", title: "Two", parent: p2, skipped: true, available: true, resolved: 200})
	staleUG := famLink(t, userID, c2, 300)
	famLink(t, userID, c3, 200)
	jobID := famInsertJob(t, userID, "pending", true)

	e := func(id, title string) tasks.ExternalGameEntry {
		return tasks.ExternalGameEntry{ExternalID: id, Title: title, Platforms: []string{"pc-windows"}}
	}
	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{
		e("P1", "One"), e("C1", "One"), e("P2", "Two"), e("C2", "Two"), e("C3", "Two"),
	})

	c1ID, c1 := famGet(t, userID, "C1")
	if !c1.IsSkipped {
		t.Error("C1 must take its parent's skip")
	}
	if ids := famItemsFor(t, jobID, c1ID); len(ids) != 0 {
		t.Errorf("C1 must get no item, got %d", len(ids))
	}

	_, got2 := famGet(t, userID, "C2")
	if got2.IsSkipped || got2.Resolved == nil || *got2.Resolved != 200 {
		t.Errorf("C2: want unskipped with the parent's match 200, got skipped=%v resolved=%v", got2.IsSkipped, got2.Resolved)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM user_game_platforms WHERE external_game_id = ?`, c2); n != 0 {
		t.Errorf("C2 must have no link left on its old game, got %d", n)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM user_games WHERE id = ?`, staleUG); n != 1 {
		t.Error("the user game C2 was linked to must be kept")
	}
	if ids := famItemsFor(t, jobID, c2); len(ids) != 1 {
		t.Errorf("C2 must get an item, got %d", len(ids))
	}

	_, got3 := famGet(t, userID, "C3")
	if got3.IsSkipped {
		t.Error("C3 must be unskipped")
	}
	if n := famCount(t, `SELECT COUNT(*) FROM user_game_platforms WHERE external_game_id = ?`, c3); n != 1 {
		t.Errorf("C3's match already equalled its parent's; its link must be kept, got %d", n)
	}
}

func TestRepairChildSkips_FailurePartWayChangesNothing(t *testing.T) {
	userID := famSetup(t)
	ctx := context.Background()
	p := famInsertGame(t, userID, famGame{extID: "P", available: true, resolved: 200})
	c := famInsertGame(t, userID, famGame{extID: "C", parent: p, skipped: true, available: true, resolved: 300})
	famLink(t, userID, c, 300)

	if _, err := testDB.ExecContext(ctx, `
		CREATE FUNCTION fam_block_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'blocked'; END $$;
		CREATE TRIGGER fam_block_delete BEFORE DELETE ON user_game_platforms
		FOR EACH ROW EXECUTE FUNCTION fam_block_delete();`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER fam_block_delete ON user_game_platforms; DROP FUNCTION fam_block_delete();`)
	})

	if err := tasks.RepairChildSkipsForTest(ctx, testDB, userID, famStore); err == nil {
		t.Fatal("want an error from the blocked unlink")
	}
	_, got := famGet(t, userID, "C")
	if !got.IsSkipped || got.Resolved == nil || *got.Resolved != 300 {
		t.Errorf("child must be unchanged, got skipped=%v resolved=%v", got.IsSkipped, got.Resolved)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM user_game_platforms WHERE external_game_id = ?`, c); n != 1 {
		t.Errorf("link must be kept, got %d", n)
	}
}

// ── Rule 4: promotion ─────────────────────────────────────────────────────────

func TestDispatchSync_PromotesChildOfParentThatLeftEarlier(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: false, age: 3 * time.Hour})
	c1 := famInsertGame(t, userID, famGame{extID: "C1", parent: parentID, available: true, age: 2 * time.Hour})
	famInsertGame(t, userID, famGame{extID: "C2", parent: parentID, available: false, age: time.Hour})
	jobID := famInsertJob(t, userID, "pending", true)

	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{famEntry("C1"), famEntry("NEW")})

	_, gotC1 := famGet(t, userID, "C1")
	if gotC1.ParentID != nil {
		t.Fatalf("C1 must be promoted, still has parent %s", *gotC1.ParentID)
	}
	if _, gotC2 := famGet(t, userID, "C2"); gotC2.ParentID == nil || *gotC2.ParentID != c1 {
		t.Errorf("C2 must point to the promoted game, got %v", gotC2.ParentID)
	}
	if _, gotNew := famGet(t, userID, "NEW"); gotNew.ParentID == nil || *gotNew.ParentID != c1 {
		t.Errorf("a new edition must become the promoted game's child, got %v", gotNew.ParentID)
	}
}

func TestDispatchSync_ParentLeavesThisSync_ChildIsMatchedInTheSameSync(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: true, age: 2 * time.Hour})
	famInsertGame(t, userID, famGame{extID: "C", parent: parentID, available: true, age: time.Hour})
	jobID := famInsertJob(t, userID, "pending", true)

	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{famEntry("C")})

	childID, got := famGet(t, userID, "C")
	if got.ParentID != nil {
		t.Fatalf("C must be promoted when its unmatched parent leaves, still has parent %s", *got.ParentID)
	}
	items := famItemsFor(t, jobID, childID)
	if len(items) != 1 {
		t.Fatalf("want one item, got %d", len(items))
	}
	if n := famLiveRiverJobs(t, items[0]); n != 1 {
		t.Errorf("item must have exactly one live River job, got %d", n)
	}
	if err := famMatch(t, rc, items[0]); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	if status, _ := famItemStatus(t, items[0]); status != "pending_review" {
		t.Errorf("promoted game must be searched (no IGDB → review), got %q", status)
	}
}

func TestDispatchSync_NoPromotion(t *testing.T) {
	cases := []struct {
		name      string
		parent    famGame
		childGone bool
	}{
		{"parent matched", famGame{extID: "P", available: true, resolved: 801}, false},
		{"parent skipped", famGame{extID: "P", available: true, skipped: true}, false},
		{"no available child", famGame{extID: "P", available: false}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := famSetup(t)
			rc := newTestRiverClient(t)
			tc.parent.age = 2 * time.Hour
			parentID := famInsertGame(t, userID, tc.parent)
			famInsertGame(t, userID, famGame{extID: "C", parent: parentID, skipped: tc.parent.skipped, available: !tc.childGone, age: time.Hour})
			famInsertGame(t, userID, famGame{extID: "G", title: "Other", available: true})
			jobID := famInsertJob(t, userID, "pending", true)

			entries := []tasks.ExternalGameEntry{{ExternalID: "G", Title: "Other", Platforms: []string{"pc-windows"}}}
			if !tc.childGone {
				entries = append(entries, famEntry("C"))
			}
			famDispatch(t, rc, jobID, userID, entries)

			if _, got := famGet(t, userID, "C"); got.ParentID == nil || *got.ParentID != parentID {
				t.Errorf("C must stay the parent's child, got %v", got.ParentID)
			}
		})
	}
}

// ── Rule 5: a child waits only when the wait can end ─────────────────────────

func TestIGDBMatchWorker_ChildWaitsOnlyWhenTheWaitCanEnd(t *testing.T) {
	cases := []struct {
		name             string
		dispatchComplete bool
		parentItem       string // "" = none
		want             string // "wait" or "failed"
	}{
		{"parent item pending", true, "pending", "wait"},
		{"parent in review", true, "pending_review", "wait"},
		{"no parent item yet, dispatch running", false, "", "wait"},
		{"no parent item, dispatch done", true, "", "failed"},
		{"parent item failed", true, "failed", "failed"},
		{"parent item completed while unmatched", false, "completed", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := famSetup(t)
			rc := newTestRiverClient(t)
			parentID := famInsertGame(t, userID, famGame{extID: "P", available: true})
			childID := famInsertGame(t, userID, famGame{extID: "C", parent: parentID, available: true})
			jobID := famInsertJob(t, userID, "processing", tc.dispatchComplete)
			if tc.parentItem != "" {
				famInsertItem(t, jobID, userID, parentID, tc.parentItem)
			}
			itemID := famInsertItem(t, jobID, userID, childID, "pending")

			if err := famMatch(t, rc, itemID); err != nil {
				t.Fatalf("igdb_match: %v", err)
			}

			status, msg := famItemStatus(t, itemID)
			switch tc.want {
			case "wait":
				if status != "pending" || famItemOwner(t, itemID) != nil {
					t.Errorf("want the child waiting (pending, no owner), got %q", status)
				}
			case "failed":
				if status != "failed" || msg == nil || *msg != "parent game is unmatched and is not part of this sync" {
					t.Errorf("want failed with the unmatched-parent message, got %q %v", status, msg)
				}
			}
		})
	}
}

func famItemOwner(t *testing.T, itemID string) *int64 {
	t.Helper()
	var id *int64
	if err := testDB.NewRaw(`SELECT river_job_id FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &id); err != nil {
		t.Fatalf("item owner: %v", err)
	}
	return id
}

// A child whose parent has no item in the sync ends failed once the dispatch
// is done, and the sync completes with errors instead of hanging.
func TestIGDBMatchWorker_ChildWithoutParentItem_SyncCompletes(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: true})
	childID := famInsertGame(t, userID, famGame{extID: "C", parent: parentID, available: true})
	jobID := famInsertJob(t, userID, "processing", true)
	itemID := famInsertItem(t, jobID, userID, childID, "pending")

	if err := famMatch(t, rc, itemID); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	if got := famJobStatus(t, jobID); got != "completed" {
		t.Errorf("job status: want completed (with errors), got %q", got)
	}
}

func TestIGDBMatchWorker_ChildOfFailedButMatchedParentIsWritten(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: true, resolved: 901})
	childID := famInsertGame(t, userID, famGame{extID: "C", parent: parentID, available: true})
	jobID := famInsertJob(t, userID, "processing", true)
	famInsertItem(t, jobID, userID, parentID, "failed")
	itemID := famInsertItem(t, jobID, userID, childID, "pending")

	if err := famMatch(t, rc, itemID); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	if status, _ := famItemStatus(t, itemID); status != "pending" {
		t.Errorf("item: want pending (handed to the write stage), got %q", status)
	}
	if _, got := famGet(t, userID, "C"); got.Resolved == nil || *got.Resolved != 901 {
		t.Errorf("child must inherit 901, got %v", got.Resolved)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM river_job WHERE kind = 'user_game_write' AND args->>'job_item_id' = ?`, itemID); n != 1 {
		t.Errorf("want one user_game_write, got %d", n)
	}
}

// A child whose game is promoted while its worker is deciding (it read the
// game as a child, then the promotion committed) is searched like any other
// game, not failed: told "cannot wait", it loads the game again.
func TestIGDBMatchWorker_PromotedWhileAskingToWait_IsSearched(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	ctx := context.Background()
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: false})
	childID := famInsertGame(t, userID, famGame{extID: "C", parent: parentID, available: true})
	jobID := famInsertJob(t, userID, "processing", true)
	itemID := famInsertItem(t, jobID, userID, childID, "pending")
	job := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID})

	// Hold the parent row: the worker's inherit reads it FOR SHARE and waits.
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM external_games WHERE id = ? FOR UPDATE`, parentID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		w := &tasks.IGDBMatchWorker{DB: testDB, LC: newTestLC(t)}
		done <- w.Work(ctx, job)
	}()
	waitForLockWait(t)
	// The promotion commits while the worker waits.
	if _, err := tx.ExecContext(ctx, `UPDATE external_games SET parent_id = NULL WHERE id = ?`, childID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	_ = rc
	if status, msg := famItemStatus(t, itemID); status != "pending_review" {
		t.Errorf("promoted game must be searched (no IGDB → review), got %q %v", status, msg)
	}
}

// A child only takes the match of the game that is its parent when it writes
// (#1437). The worker loaded the child with parent P; before its inherit reads
// P, the promotion re-points the child and the user Rematches P. The child
// must not copy P's new match: it decides again on the game as it is now.
func TestIGDBMatchWorker_ParentChangedBeforeInherit_DoesNotTakeOldParentsMatch(t *testing.T) {
	cases := []struct {
		name string
		// newParent re-points the child to another unmatched game; false
		// promotes it to top-level.
		newParent  bool
		wantStatus string
		wantMsg    string
	}{
		// The new parent has no item and the dispatch is done: the child
		// cannot wait for it and fails.
		{name: "re-pointed", newParent: true, wantStatus: "failed", wantMsg: "parent game is unmatched and is not part of this sync"},
		// Top-level now: searched (no IGDB client → Pending review).
		{name: "promoted", newParent: false, wantStatus: "pending_review"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := famSetup(t)
			ctx := context.Background()
			oldParentID := famInsertGame(t, userID, famGame{extID: "P", available: false, age: 2 * time.Hour})
			childID := famInsertGame(t, userID, famGame{extID: "C", parent: oldParentID, available: true})
			var newParentID any
			if tc.newParent {
				newParentID = famInsertGame(t, userID, famGame{extID: "Q", available: true, age: time.Hour})
			}
			famEnsureGame(t, 902)
			jobID := famInsertJob(t, userID, "processing", true)
			itemID := famInsertItem(t, jobID, userID, childID, "pending")
			job := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID})

			// Hold the old parent: the worker's inherit reads it FOR SHARE and waits.
			tx, err := testDB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.ExecContext(ctx, `SELECT 1 FROM external_games WHERE id = ? FOR UPDATE`, oldParentID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				w := &tasks.IGDBMatchWorker{DB: testDB, LC: newTestLC(t)}
				done <- w.Work(ctx, job)
			}()
			waitForLockWait(t)
			// The promotion re-points the child, and a Rematch matches the old parent.
			if _, err := tx.ExecContext(ctx, `UPDATE external_games SET parent_id = ? WHERE id = ?`, newParentID, childID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE external_games SET resolved_igdb_id = 902 WHERE id = ?`, oldParentID); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("igdb_match: %v", err)
			}

			if _, got := famGet(t, userID, "C"); got.Resolved != nil {
				t.Errorf("child must not take the old parent's match, got %d", *got.Resolved)
			}
			status, msg := famItemStatus(t, itemID)
			if status != tc.wantStatus {
				t.Errorf("item status: want %q, got %q %v", tc.wantStatus, status, msg)
			}
			if tc.wantMsg != "" && (msg == nil || *msg != tc.wantMsg) {
				t.Errorf("item message: want %q, got %v", tc.wantMsg, msg)
			}
		})
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

// ── Rule 6: the re-queue at the end of the dispatch ──────────────────────────

// A child that waited during the dispatch and whose parent left the library
// in this sync is promoted by the sweep; marking the dispatch done queues it,
// so it is matched in the same sync.
func TestDispatchSync_WaitingChildPromotedInTheSweep_IsQueued(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	parentID := famInsertGame(t, userID, famGame{extID: "P", available: true, age: 2 * time.Hour})
	childID := famInsertGame(t, userID, famGame{extID: "C", parent: parentID, available: true, age: time.Hour})
	jobID := famInsertJob(t, userID, "processing", false)
	// A child that waited while the dispatch ran: pending, nothing running it.
	waiting := famInsertItemKey(t, jobID, userID, childID, "C", "pending")

	famDispatch(t, rc, jobID, userID, []tasks.ExternalGameEntry{famEntry("C")})

	if _, got := famGet(t, userID, "C"); got.ParentID != nil {
		t.Fatalf("C must be promoted")
	}
	if n := famLiveRiverJobs(t, waiting); n != 1 {
		t.Errorf("waiting child: want one live River job, got %d", n)
	}
	if err := famMatch(t, rc, waiting); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
	if status, _ := famItemStatus(t, waiting); status != "pending_review" {
		t.Errorf("promoted child must be searched, got %q", status)
	}
}

func famInsertItemKey(t *testing.T, jobID, userID, egID, key, status string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'Horizon', ?, '{}', ?, '{}', '[]', now())`,
		id, jobID, userID, key, egID, status,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	return id
}

func TestDispatchSync_RunAgain_GivesNoItemASecondRiverJob(t *testing.T) {
	userID := famSetup(t)
	rc := newTestRiverClient(t)
	jobID := famInsertJob(t, userID, "pending", true)
	entries := []tasks.ExternalGameEntry{famEntry("A"), {ExternalID: "B", Title: "Other", Platforms: []string{"pc-windows"}}}

	famDispatch(t, rc, jobID, userID, entries)
	// As if the first run was cut off before it marked the dispatch done.
	if _, err := testDB.ExecContext(context.Background(), `UPDATE jobs SET dispatch_complete = false WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	famDispatch(t, rc, jobID, userID, entries)

	var items []string
	if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ?`, jobID).Scan(context.Background(), &items); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	for _, id := range items {
		if n := famCount(t, `SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ?`, id); n != 1 {
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
	if err := onBatch([]tasks.ExternalGameEntry{famEntry("A")}); err != nil {
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
	userID := famSetup(t)
	lc := newTestLC(t)
	famInsertGame(t, userID, famGame{extID: "X", title: "Elsewhere", available: true})
	jobID := famInsertJob(t, userID, "pending", true)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&cancellingAdapter{jobID: jobID, lc: lc}), LC: lc}
	if err := w.Work(context.Background(), syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: famStore})); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if got := famJobStatus(t, jobID); got != "cancelled" {
		t.Fatalf("job: want cancelled, got %q", got)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM external_games WHERE user_id = ? AND external_id = 'B'`, userID); n != 0 {
		t.Errorf("the dispatch went on after the cancel: B was written")
	}
	if n := famCount(t, `SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'pending_review')`, jobID); n != 0 {
		t.Errorf("%d open items left", n)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM river_job WHERE state IN ('available', 'scheduled', 'retryable', 'pending')`); n != 0 {
		t.Errorf("%d River jobs left queued", n)
	}
	if _, x := famGet(t, userID, "X"); x.ParentID != nil {
		t.Fatal("unexpected parent")
	}
	if n := famCount(t, `SELECT COUNT(*) FROM external_games WHERE user_id = ? AND external_id = 'X' AND is_available`, userID); n != 1 {
		t.Errorf("the removed-game sweep ran after the cancel")
	}
}
