package tasks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// insertImportItem inserts a darkadia job + one job_item with the given payload
// as source_metadata. Returns (jobID, itemID).
func insertImportItem(t *testing.T, userID string, payload map[string]any) (string, string) {
	t.Helper()
	ctx := context.Background()
	jobID := uuid.NewString()
	itemID := uuid.NewString()
	metaBytes, _ := json.Marshal(payload)
	meta := json.RawMessage(metaBytes)
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'import', 'darkadia', 'processing', 'high', 1, true, now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	title, _ := payload["title"].(string)
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'game_0', ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, title, meta,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return jobID, itemID
}

func TestImportMatch_NoIGDBClientMarksPendingReview(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-match"
	insertTestUser(t, testDB, userID)
	payload := map[string]any{"title": "Whatever", "play_status": "not_started", "platforms": []map[string]any{}}
	_, itemID := insertImportItem(t, userID, payload)

	w := &tasks.ImportMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.ImportMatchArgs]{Args: tasks.ImportMatchArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("match: %v", err)
	}
	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status != "pending_review" {
		t.Errorf("status = %q, want pending_review", status)
	}
}

func TestImportMatch_IGDBIDShortCircuits(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-match-igdbid"
	insertTestUser(t, testDB, userID)
	payload := map[string]any{
		"title":       "Anodyne",
		"igdb_id":     42,
		"play_status": "not_started",
		"platforms":   []map[string]any{},
	}
	_, itemID := insertImportItem(t, userID, payload)

	// Nil IGDB client: without the short-circuit this item would go
	// pending_review. With it, the id is trusted and resolved directly.
	w := &tasks.ImportMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.ImportMatchArgs]{Args: tasks.ImportMatchArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("match: %v", err)
	}

	var resolved sql.NullInt64
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &resolved); err != nil {
		t.Fatal(err)
	}
	if !resolved.Valid || resolved.Int64 != 42 {
		t.Fatalf("resolved_igdb_id = %v, want 42", resolved)
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status == "pending_review" {
		t.Errorf("status = pending_review; short-circuit should have skipped matching")
	}
}

func TestImportMatch_NoIGDBIDStillPendingReview(t *testing.T) {
	// A payload with no igdb_id and a nil client keeps the existing behaviour.
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-match-noid"
	insertTestUser(t, testDB, userID)
	payload := map[string]any{"title": "Whatever", "play_status": "not_started", "platforms": []map[string]any{}}
	_, itemID := insertImportItem(t, userID, payload)

	w := &tasks.ImportMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.ImportMatchArgs]{Args: tasks.ImportMatchArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("match: %v", err)
	}
	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status != "pending_review" {
		t.Errorf("status = %q, want pending_review", status)
	}
}

func TestImportFinalize_WritesUserGameAndPlatforms(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-fin1"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Anodyne', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Anodyne", "play_status": "completed", "is_loved": true,
		"personal_notes": "great",
		"platforms": []map[string]any{
			{"platform": "pc-windows", "storefront": "gog", "acquired_date": "2014-03-01"},
			{"platform": "mac"},
		},
	}
	jobID, itemID := insertImportItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = 42", userID).Scan(ctx); err != nil {
		t.Fatalf("user_game not written: %v", err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "completed" || !ug.IsLoved {
		t.Errorf("user_game fields wrong: %+v", ug)
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ug.ID).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("platforms = %d, want 2", count)
	}
	var jobStatus string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "completed" {
		t.Errorf("job status = %q, want completed", jobStatus)
	}
}

// An invalid play_status in the Darkadia payload must be coerced to unset
// (nil), mirroring the nexorious import worker. user_games.play_status is NOT
// NULL DEFAULT 'not_started', so the DB applies the default; the invalid value
// is never stored verbatim.
func TestImportFinalize_InvalidPlayStatusCoercedToNull(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-badstatus"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Bad Status', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Bad Status", "play_status": "not_a_real_status",
		"platforms": []map[string]any{},
	}
	_, itemID := insertImportItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = 42", userID).Scan(ctx); err != nil {
		t.Fatalf("user_game not written: %v", err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "not_started" {
		t.Errorf("play_status = %v, want 'not_started' (invalid coerced via NOT NULL default)", ug.PlayStatus)
	}
}

// Two items in one import that resolve to the SAME game (duplicate titles) must
// not fail on the user_games (user_id, game_id) unique index; the loser of the
// race re-selects the existing row and merges its platforms in.
func TestImportFinalize_ConcurrentDuplicateGameNoFailure(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-dup"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Dup Game', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'import', 'darkadia', 'processing', 'high', 2, true, now())`, jobID, userID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"title": "Dup Game", "play_status": "not_started", "platforms": []map[string]any{{"platform": "pc-windows"}}}
	meta, _ := json.Marshal(payload)
	itemIDs := []string{uuid.NewString(), uuid.NewString()}
	for i, id := range itemIDs {
		if _, err := testDB.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, resolved_igdb_id, created_at)
			 VALUES (?, ?, ?, ?, 'Dup Game', ?, 'processing', '{}', '[]', 42, now())`,
			id, jobID, userID, fmt.Sprintf("game_%d", i), json.RawMessage(meta)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	var wg sync.WaitGroup
	for _, id := range itemIDs {
		wg.Add(1)
		go func(itemID string) {
			defer wg.Done()
			_ = w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: itemID}})
		}(id)
	}
	wg.Wait()

	var failed int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'failed'`, jobID).Scan(ctx, &failed); err != nil {
		t.Fatal(err)
	}
	if failed != 0 {
		t.Errorf("failed items = %d, want 0 (duplicate game must not fail)", failed)
	}
	var ugCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = 42`, userID).Scan(ctx, &ugCount); err != nil {
		t.Fatal(err)
	}
	if ugCount != 1 {
		t.Errorf("user_games = %d, want 1", ugCount)
	}
}

// A finalized item must NOT complete the job while dispatch is still in flight
// (dispatch_complete=false) — this is the guard against the upload handler
// finalizing the job before it has inserted every item.
func TestImportCheckJobCompletion_BlockedUntilDispatchComplete(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-dispatch"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'G', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'import', 'darkadia', 'processing', 'high', 1, false, now())`, jobID, userID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"title": "G", "play_status": "not_started", "platforms": []map[string]any{}}
	meta, _ := json.Marshal(payload)
	itemID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, resolved_igdb_id, created_at)
		 VALUES (?, ?, ?, 'game_0', 'G', ?, 'processing', '{}', '[]', 42, now())`,
		itemID, jobID, userID, json.RawMessage(meta)).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("job finalized while dispatch in flight: status=%s", status)
	}

	// Dispatch finishes → the completion check now finalizes the job.
	if _, err := testDB.NewRaw(`UPDATE jobs SET dispatch_complete = true WHERE id = ?`, jobID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	tasks.ImportCheckJobCompletion(testDB, jobID)
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("job not completed after dispatch complete: status=%s", status)
	}
}

func TestImportFinalize_AdditiveMergeDoesNotOverwrite(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-fin2"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Anodyne', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	ugID := uuid.NewString()
	if _, err := testDB.NewRaw(`INSERT INTO user_games (id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, created_at, updated_at)
		VALUES (?, ?, 42, 'mastered', 5, true, 'mine', now(), now())`, ugID, userID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Anodyne", "play_status": "not_started", "is_loved": false,
		"personal_rating": 2, "personal_notes": "imported",
		"platforms": []map[string]any{{"platform": "mac"}},
	}
	_, itemID := insertImportItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("id = ?", ugID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "mastered" || ug.PersonalRating == nil || *ug.PersonalRating != 5 || ug.PersonalNotes == nil || *ug.PersonalNotes != "mine" || !ug.IsLoved {
		t.Errorf("curation overwritten: %+v", ug)
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("platforms = %d, want 1 (mac merged in)", count)
	}
}

func TestImportFinalize_TagsAndPlaytimeOnFirstPlatform(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-tags"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (77, 'Tagged', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Tagged", "play_status": "completed",
		"tags":         []string{"Co-op", "VR"},
		"hours_played": 148.0,
		"platforms": []map[string]any{
			{"platform": "pc-windows", "storefront": "steam"},
			{"platform": "mac"},
		},
	}
	_, itemID := insertImportItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 77 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = 77", userID).Scan(ctx); err != nil {
		t.Fatal(err)
	}

	var tagCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_tags WHERE user_game_id = ?`, ug.ID).Scan(ctx, &tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 2 {
		t.Errorf("tags = %d, want 2", tagCount)
	}

	var pcHours, macHours sql.NullFloat64
	if err := testDB.NewRaw(`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = 'pc-windows'`, ug.ID).Scan(ctx, &pcHours); err != nil {
		t.Fatal(err)
	}
	if err := testDB.NewRaw(`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = 'mac'`, ug.ID).Scan(ctx, &macHours); err != nil {
		t.Fatal(err)
	}
	if !pcHours.Valid || pcHours.Float64 != 148 {
		t.Errorf("pc hours = %+v, want 148", pcHours)
	}
	if macHours.Valid {
		t.Errorf("mac hours = %+v, want NULL", macHours)
	}
}

// A wishlisted, platform-less game keeps is_wishlisted=true after finalize
// (ClearWishlistOnAcquire only clears when a platform exists). A wishlisted game
// that also has a platform is cleared (acquisition wins).
func TestImportFinalize_WishlistFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-gv-wish"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (314246, 'Borderlands 4', now(), now()), (71, 'Portal', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	// Wishlist-only (no platforms) -> stays wishlisted.
	wishPayload := map[string]any{
		"title": "Borderlands 4", "play_status": "not_started",
		"is_wishlisted": true, "platforms": []map[string]any{},
	}
	_, wishItem := insertImportItem(t, userID, wishPayload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 314246 WHERE id = ?`, wishItem).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	// Wishlisted + owned (has a platform) -> cleared on acquire.
	ownedPayload := map[string]any{
		"title": "Portal", "play_status": "completed",
		"is_wishlisted": true,
		"platforms":     []map[string]any{{"platform": "pc-windows"}},
	}
	_, ownedItem := insertImportItem(t, userID, ownedPayload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 71 WHERE id = ?`, ownedItem).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	for _, id := range []string{wishItem, ownedItem} {
		if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: id}}); err != nil {
			t.Fatalf("finalize %s: %v", id, err)
		}
	}

	var wish models.UserGame
	if err := testDB.NewSelect().Model(&wish).Where("user_id = ? AND game_id = 314246", userID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if !wish.IsWishlisted {
		t.Errorf("platform-less wishlist game: is_wishlisted = false, want true")
	}
	var owned models.UserGame
	if err := testDB.NewSelect().Model(&owned).Where("user_id = ? AND game_id = 71", userID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if owned.IsWishlisted {
		t.Errorf("owned game: is_wishlisted = true, want false (cleared on acquire)")
	}
}
