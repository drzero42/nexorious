package tasks_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// TestDispatchSync_SteamPartialOwnedGames_FailsWithoutRemovingGames runs a
// Steam reply whose game_count is higher than the number of games it lists
// through the real Steam adapter and the sync worker. Read as the full library,
// that reply would let the removal sweep mark the unlisted game removed; the
// empty-fetch guard (#1324) does not catch it because the fetch is not empty.
// The job must fail with a hint about the Game details setting, both games must
// stay available with no "removed" change recorded, and the credentials flag
// must stay clear (#1310).
func TestDispatchSync_SteamPartialOwnedGames_FailsWithoutRemovingGames(t *testing.T) {
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
	listedID := insertTestExternalGame(t, userID, "steam", "730", "Counter-Strike 2", "pc-windows")
	unlistedID := insertTestExternalGame(t, userID, "steam", "570", "Dota 2", "pc-windows")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "appdetails") {
			_, _ = w.Write([]byte(`{"730":{"success":true,"data":{"platforms":{"windows":true,"mac":false,"linux":false}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"response":{"game_count":2,"games":[{"appid":730,"name":"Counter-Strike 2","playtime_forever":60}]}}`))
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

	var jobRow struct {
		Status       string  `bun:"status"`
		ErrorMessage *string `bun:"error_message"`
	}
	if err := testDB.NewRaw(`SELECT status, error_message FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobRow); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if jobRow.Status != "failed" {
		t.Errorf("job status = %q, want failed", jobRow.Status)
	}
	if jobRow.ErrorMessage == nil || !strings.Contains(*jobRow.ErrorMessage, "Game details") {
		t.Errorf("job error_message = %v, want a hint about the Game details setting", jobRow.ErrorMessage)
	}

	for name, egID := range map[string]string{"listed": listedID, "unlisted": unlistedID} {
		var available bool
		if err := testDB.NewRaw(`SELECT is_available FROM external_games WHERE id = ?`, egID).Scan(ctx, &available); err != nil {
			t.Fatalf("read %s game availability: %v", name, err)
		}
		if !available {
			t.Errorf("%s game marked unavailable, want it left available", name)
		}
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

func TestDispatchSync_SteamIncompleteLibrary_NotifiesOnceUntilRecovery(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	); err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	for _, tc := range []struct {
		name   string
		reply  string
		status int
		events int
	}{
		{"first incomplete reply", `{"response":{}}`, http.StatusOK, 1},
		{"upstream network failure", `{}`, http.StatusBadGateway, 1},
		{"repeat incomplete reply", `{"response":{"games":[]}}`, http.StatusOK, 0},
		{"successful empty library", `{"response":{"game_count":0}}`, http.StatusOK, 0},
		{"first failure after recovery", `{"response":{}}`, http.StatusOK, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobID := uuid.NewString()
			if _, err := testDB.ExecContext(ctx,
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
				jobID, userID,
			); err != nil {
				t.Fatalf("insert job: %v", err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.reply))
			}))
			defer srv.Close()
			client := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return steam.NewAdapter(client, "key", "76561198000000001"), nil
			}}
			job := &river.Job[tasks.DispatchSyncArgs]{
				Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
			}
			if err := w.Work(ctx, job); err != nil {
				t.Fatalf("work: %v", err)
			}
			if got := eventCount(t, jobID+":sync.failed"); got != tc.events {
				t.Errorf("sync.failed events = %d, want %d", got, tc.events)
			}
		})
	}
}
