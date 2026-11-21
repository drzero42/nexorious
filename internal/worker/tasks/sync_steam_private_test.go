package tasks_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// TestDispatchSync_SteamPrivateGameDetails_FailsWithoutRemovingGames runs a
// Steam reply with neither games nor game_count (what Steam sends when the
// account's game details are private) through the real Steam adapter and the
// sync worker. The job must fail, and the game already in the library must stay
// available with no "removed" change recorded and the credentials flag clear
// (#1310).
func TestDispatchSync_SteamPrivateGameDetails_FailsWithoutRemovingGames(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	); err != nil {
		t.Fatalf("insert sync config: %v", err)
	}
	egID := insertTestExternalGame(t, userID, "steam", "730", "Counter-Strike 2", "pc-windows")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{}}`))
	}))
	defer srv.Close()
	client := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	factory := func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return steam.NewAdapter(client, "key", "76561198000000001"), nil
	}

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("read job status: %v", err)
	}
	if status != "failed" {
		t.Errorf("job status = %q, want failed", status)
	}

	var available bool
	if err := testDB.NewRaw(`SELECT is_available FROM external_games WHERE id = ?`, egID).Scan(ctx, &available); err != nil {
		t.Fatalf("read game availability: %v", err)
	}
	if !available {
		t.Error("game marked unavailable, want it left available")
	}

	var removed int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM changes WHERE user_id = ? AND change_type = 'removed'`, userID,
	).Scan(ctx, &removed); err != nil {
		t.Fatalf("count removed changes: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed changes = %d, want 0", removed)
	}

	var credErr bool
	if err := testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`, userID,
	).Scan(ctx, &credErr); err != nil {
		t.Fatalf("read credentials_error: %v", err)
	}
	if credErr {
		t.Error("credentials_error = true, want false (the key works; game details are private)")
	}
}
