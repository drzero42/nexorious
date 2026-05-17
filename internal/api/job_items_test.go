package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// ─── TestGetJobItem ────────────────────────────────────────────────────────────

func TestGetJobItem(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-get")

	insertJob(t, testDB, "job-ji-get", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-get-1", "job-ji-get", userID, "key1", "My Game", "pending_review")

	rec := getAuth(t, e, "/api/job-items/ji-get-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["id"] != "ji-get-1" {
		t.Fatalf("expected id=ji-get-1, got %v", resp["id"])
	}
	if resp["source_title"] != "My Game" {
		t.Fatalf("expected source_title=My Game, got %v", resp["source_title"])
	}
	if resp["status"] != "pending_review" {
		t.Fatalf("expected status=pending_review, got %v", resp["status"])
	}
}

// ─── TestGetJobItem_WrongOwner ────────────────────────────────────────────────

func TestGetJobItem_WrongOwner(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	ownerID := "u-ji-owner"
	insertAuthTestUser(t, testDB, ownerID, "jiowner", "pass123", true, false)

	insertJob(t, testDB, "job-ji-wrong", ownerID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-wrong-1", "job-ji-wrong", ownerID, "key2", "Other Game", "pending_review")

	_, token2 := setupTagUser(t, testDB, e, "ji-other")

	rec := getAuth(t, e, "/api/job-items/ji-wrong-1", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestResolveItem ──────────────────────────────────────────────────────────

func TestResolveItem(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-resolve")

	insertJob(t, testDB, "job-ji-resolve", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-resolve-1", "job-ji-resolve", userID, "key3", "Resolve Game", "pending_review")

	body := map[string]any{"igdb_id": 99999}
	rec := postJSONAuth(t, e, "/api/job-items/ji-resolve-1/resolve", body, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	var resolvedIGDBID *int
	err := testDB.QueryRowContext(context.Background(),
		"SELECT status, resolved_igdb_id FROM job_items WHERE id = 'ji-resolve-1'",
	).Scan(&status, &resolvedIGDBID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending, got %s", status)
	}
	if resolvedIGDBID == nil || *resolvedIGDBID != 99999 {
		t.Fatalf("expected resolved_igdb_id=99999, got %v", resolvedIGDBID)
	}
}

// ─── TestResolveItem_PropagatesResolutionToSiblings ───────────────────────────

func TestResolveItem_PropagatesResolutionToSiblings(t *testing.T) {
	// When the user resolves a pending_review job_item for one SKU (e.g. PPSA/PS5),
	// HandleResolveItem must also:
	//   - write resolved_igdb_id to external_games for the resolved SKU
	//   - write resolved_igdb_id to external_games for all same-title sibling SKUs
	//   - reset sibling pending_review job_items to pending (so the worker re-runs them)
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-sib")

	insertJob(t, testDB, "job-ji-sib", userID, "sync", "psn", "processing")

	// games row for IGDB ID 266683 (will be created by the handler).
	// external_games — PS5 (PPSA) and PS4 (CUSA), same title, both unresolved.
	egPSPAID := "eg-sib-ppsa"
	egCUSAID := "eg-sib-cusa"
	for _, row := range []struct{ id, extID, platform string }{
		{egPSPAID, "PPSA16902_00", "playstation-5"},
		{egCUSAID, "CUSA43774_00", "playstation-4"},
	} {
		_, err := testDB.ExecContext(context.Background(),
			`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
			 VALUES (?, ?, 'psn', ?, 'Tomb Raider I-III Remastered', false, true, false, 0)`,
			row.id, userID, row.extID,
		)
		if err != nil {
			t.Fatalf("insert external_game %s: %v", row.id, err)
		}
		_ = row.platform
	}

	// job_items for each SKU — both pending_review, source_metadata links to external_game.
	ppsMeta, _ := json.Marshal(map[string]string{"external_game_id": egPSPAID, "raw_platform": "playstation-5"})
	cusaMeta, _ := json.Marshal(map[string]string{"external_game_id": egCUSAID, "raw_platform": "playstation-4"})
	for _, row := range []struct {
		id, extID string
		meta      []byte
	}{
		{"ji-sib-ppsa", "PPSA16902_00", ppsMeta},
		{"ji-sib-cusa", "CUSA43774_00", cusaMeta},
	} {
		_, err := testDB.ExecContext(context.Background(),
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
			 VALUES (?, ?, ?, ?, 'Tomb Raider I-III Remastered', ?, 'pending_review', '{}', '[]')`,
			row.id, "job-ji-sib", userID, row.extID, string(row.meta),
		)
		if err != nil {
			t.Fatalf("insert job_item %s: %v", row.id, err)
		}
	}

	// Resolve the PPSA item.
	body := map[string]any{"igdb_id": 266683}
	rec := postJSONAuth(t, e, "/api/job-items/ji-sib-ppsa/resolve", body, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// PPSA external_game must be resolved.
	var ppsaResolved *int
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egPSPAID).Scan(ctx, &ppsaResolved)
	if ppsaResolved == nil || *ppsaResolved != 266683 {
		t.Errorf("expected external_games(PPSA).resolved_igdb_id=266683, got %v", ppsaResolved)
	}

	// CUSA sibling external_game must also be resolved.
	var cusaResolved *int
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egCUSAID).Scan(ctx, &cusaResolved)
	if cusaResolved == nil || *cusaResolved != 266683 {
		t.Errorf("expected external_games(CUSA).resolved_igdb_id=266683, got %v", cusaResolved)
	}

	// CUSA job_item must be re-queued (status reset to pending so the worker can complete it).
	var cusaStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, "ji-sib-cusa").Scan(ctx, &cusaStatus)
	if cusaStatus != "pending" {
		t.Errorf("expected CUSA job_item status=pending after sibling resolve, got %q", cusaStatus)
	}
}

// ─── TestResolveItem_NotPendingReview ─────────────────────────────────────────

func TestResolveItem_NotPendingReview(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-resolve-409")

	insertJob(t, testDB, "job-ji-resolve-409", userID, "import", "steam", "completed")
	insertJobItem(t, testDB, "ji-resolve-409-1", "job-ji-resolve-409", userID, "key4", "Done Game", "completed")

	body := map[string]any{"igdb_id": 12345}
	rec := postJSONAuth(t, e, "/api/job-items/ji-resolve-409-1/resolve", body, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestSkipItem ─────────────────────────────────────────────────────────────

func TestSkipItem(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-skip")

	insertJob(t, testDB, "job-ji-skip", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-skip-1", "job-ji-skip", userID, "key5", "Skip Game", "pending_review")

	rec := postJSONAuth(t, e, "/api/job-items/ji-skip-1/skip", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	err := testDB.QueryRowContext(context.Background(),
		"SELECT status FROM job_items WHERE id = 'ji-skip-1'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "skipped" {
		t.Fatalf("expected status=skipped, got %s", status)
	}
}

// ─── TestRetryItem ────────────────────────────────────────────────────────────

func TestRetryItem(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-retry")

	insertJob(t, testDB, "job-ji-retry", userID, "import", "steam", "failed")
	insertJobItem(t, testDB, "ji-retry-1", "job-ji-retry", userID, "key6", "Retry Game", "failed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry-1/retry", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	err := testDB.QueryRowContext(context.Background(),
		"SELECT status FROM job_items WHERE id = 'ji-retry-1'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending, got %s", status)
	}
}

// ─── TestRetryItem_NotFailed ──────────────────────────────────────────────────

func TestRetryItem_NotFailed(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-retry-409")

	insertJob(t, testDB, "job-ji-retry-409", userID, "import", "steam", "completed")
	insertJobItem(t, testDB, "ji-retry-409-1", "job-ji-retry-409", userID, "key7", "Complete Game", "completed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry-409-1/retry", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
