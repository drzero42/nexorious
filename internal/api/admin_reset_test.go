package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestHandleAdminReset(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	// Seed admin and two regular users.
	adminID, adminTok := setupAdminUser(t, testDB, e, "reset")
	user1ID, _ := setupRegularUser(t, testDB, e, "reset-u1")
	user2ID, _ := setupRegularUser(t, testDB, e, "reset-u2")

	// Seed games (catalog — must survive reset).
	g1 := insertTestGame(t, testDB, "Catalog Game 1")
	g2 := insertTestGame(t, testDB, "Catalog Game 2")

	// Seed user games for all three users.
	insertTestUserGame(t, testDB, "ug-r-admin", adminID, int(g1))
	insertTestUserGame(t, testDB, "ug-r-u1", user1ID, int(g1))
	insertTestUserGame(t, testDB, "ug-r-u2", user2ID, int(g2))

	// Seed jobs + job items + river jobs.
	insertJob(t, testDB, "job-r-admin", adminID, "sync", "steam", "processing")
	insertJobItem(t, testDB, "ji-r-admin", "job-r-admin", adminID, "k1", "t1", "pending")
	riverID := insertRiverJob(t, testDB, "sync_item", "available", "ji-r-admin")

	insertJob(t, testDB, "job-r-u1", user1ID, "import", "csv", "completed")

	// Seed sync configs.
	for _, row := range []struct{ id, uid string }{
		{"sc-r-admin", adminID},
		{"sc-r-u1", user1ID},
	} {
		if _, err := testDB.ExecContext(context.Background(),
			`INSERT INTO user_sync_configs (id, user_id, storefront) VALUES (?, ?, 'steam')`,
			row.id, row.uid,
		); err != nil {
			t.Fatalf("seed sync_config: %v", err)
		}
	}

	// Seed tags.
	insertTag(t, testDB, "tag-r-admin", adminID, "Admin Tag", nil)
	insertTag(t, testDB, "tag-r-u1", user1ID, "User1 Tag", nil)

	t.Run("admin can reset", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/auth/admin/reset", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// 3 user_games were seeded (admin + user1 + user2).
		if resp["deleted"] != float64(3) {
			t.Errorf("deleted = %v, want 3", resp["deleted"])
		}
	})

	t.Run("non-admin users are deleted", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM users WHERE NOT is_admin`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("non-admin user count = %d, want 0", count)
		}
	})

	t.Run("admin account is preserved", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM users WHERE id = ?`, adminID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("admin count = %d, want 1", count)
		}
	})

	t.Run("all user_games are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("user_games count = %d, want 0", count)
		}
	})

	t.Run("all jobs are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("jobs count = %d, want 0", count)
		}
	})

	t.Run("all job_items are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM job_items`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("job_items count = %d, want 0", count)
		}
	})

	t.Run("all sync configs are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sync_configs`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("sync_configs count = %d, want 0", count)
		}
	})

	t.Run("all tags are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM tags`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("tags count = %d, want 0", count)
		}
	})

	t.Run("active river job is cancelled", func(t *testing.T) {
		var state string
		if err := testDB.NewRaw(`SELECT state FROM river_job WHERE id = ?`, riverID).
			Scan(context.Background(), &state); err != nil {
			t.Fatalf("river state: %v", err)
		}
		if state != "cancelled" {
			t.Errorf("river state = %q, want cancelled", state)
		}
	})

	t.Run("catalog games are preserved", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM games`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 2 {
			t.Errorf("games count = %d, want 2", count)
		}
	})

	t.Run("idempotent on empty state", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/auth/admin/reset", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["deleted"] != float64(0) {
			t.Errorf("deleted = %v, want 0", resp["deleted"])
		}
	})

	t.Run("non-admin gets 403", func(t *testing.T) {
		truncateAllTables(t)
		e2 := newTestEcho(t, testDB, cfg)
		_, regTok := setupRegularUser(t, testDB, e2, "reset-403")

		rec := postJSONAuth(t, e2, "/api/auth/admin/reset", nil, regTok)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})
}
