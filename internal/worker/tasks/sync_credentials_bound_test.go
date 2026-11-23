package tasks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// seedBoundSync inserts a pending sync job and the store's config row with the
// given stored sign-in (nil: none), failing the test on any fixture error.
func seedBoundSync(t *testing.T, storefront string, stored *string) (userID, jobID string) {
	t.Helper()
	ctx := context.Background()
	userID, jobID = uuid.NewString(), uuid.NewString()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', ?, 'pending', 'low')`,
		jobID, userID, storefront,
	); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, ?, 'daily', ?)`,
		uuid.NewString(), userID, storefront, stored,
	).Exec(ctx); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return userID, jobID
}

// boundSyncOutcome reads what a sync left behind: the flag, the stored
// sign-in, the job status and how many events were written for the user.
func boundSyncOutcome(t *testing.T, userID, jobID, storefront string) (flag bool, stored *string, status string, events int) {
	t.Helper()
	ctx := context.Background()
	if err := testDB.NewRaw(`SELECT credentials_error, storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = ?`, userID, storefront).
		Scan(ctx, &flag, &stored); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if err := testDB.NewRaw(`SELECT count(*) FROM events WHERE actor_user_id = ?`, userID).Scan(ctx, &events); err != nil {
		t.Fatalf("read events: %v", err)
	}
	return flag, stored, status, events
}

// A credentials failure bound to the sign-in a sync loaded flags the store only
// while that sign-in is still stored; the check is in the flag's own UPDATE, so
// a reconnect committed at any point during the sync is never marked expired.
func TestDispatchSync_BoundCredentialsError_SparesReconnectedSignIn(t *testing.T) {
	for _, tc := range []struct {
		name           string
		reconnect      bool
		badCredentials bool
		wantFlag       bool
		wantStored     string
		wantEvents     int
		wantStatus     string
	}{
		{"same sign-in", false, false, true, "loaded-ct", 1, "failed"},
		{"reconnected during sync", true, false, false, "new-ct", 0, "failed"},
		// The flag write fails: Fail rolls back with it, and the error goes to
		// River, which runs the dispatch again.
		{"flag update failed", false, true, false, "loaded-ct", 0, "processing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			loaded := "loaded-ct"
			userID, jobID := seedBoundSync(t, "gog", &loaded)

			factory := func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return reconnectingAdapter{t: t, reconnect: tc.reconnect, badCredentials: tc.badCredentials, userID: userID}, nil
			}
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
			err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"}))
			if (err != nil) != tc.badCredentials {
				t.Fatalf("Work error = %v", err)
			}
			runSyncEffects(t, w.LC)
			flag, stored, status, events := boundSyncOutcome(t, userID, jobID, "gog")
			if flag != tc.wantFlag {
				t.Errorf("credentials_error = %v, want %v", flag, tc.wantFlag)
			}
			if stored == nil || *stored != tc.wantStored {
				t.Errorf("stored sign-in = %v, want %q", stored, tc.wantStored)
			}
			if status != tc.wantStatus {
				t.Errorf("job status = %q, want %s", status, tc.wantStatus)
			}
			if events != tc.wantEvents {
				t.Errorf("expiry notifications = %d, want %d", events, tc.wantEvents)
			}
		})
	}
}

// Building the adapter can fail before any write (no stored sign-in, or one
// that does not decrypt). That failure belongs to what the sync loaded, NULL
// included, so a sign-in connected meanwhile is not marked expired.
func TestDispatchSync_FactoryCredentialsError_SparesSignInConnectedMeanwhile(t *testing.T) {
	for _, tc := range []struct {
		name       string
		connect    bool
		wantFlag   bool
		wantEvents int
	}{
		{"still disconnected", false, true, 1},
		{"connected during sync", true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			userID, jobID := seedBoundSync(t, "steam", nil)
			factory := func(ctx context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				if tc.connect {
					if _, err := testDB.NewRaw(`UPDATE user_sync_configs SET storefront_credentials = 'fresh-ct' WHERE user_id = ?`, userID).Exec(ctx); err != nil {
						t.Fatalf("simulated reconnect: %v", err)
					}
				}
				return nil, tasks.ErrCredentials
			}
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
			if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			runSyncEffects(t, w.LC)
			flag, stored, status, events := boundSyncOutcome(t, userID, jobID, "steam")
			if flag != tc.wantFlag {
				t.Errorf("credentials_error = %v, want %v", flag, tc.wantFlag)
			}
			if tc.connect && (stored == nil || *stored != "fresh-ct") {
				t.Errorf("stored sign-in = %v, want the one connected meanwhile", stored)
			}
			if status != "failed" {
				t.Errorf("job status = %q, want failed", status)
			}
			if events != tc.wantEvents {
				t.Errorf("expiry notifications = %d, want %d", events, tc.wantEvents)
			}
		})
	}
}

// reconnectingAdapter fails with a credentials error bound to 'loaded-ct',
// first replacing the stored sign-in when reconnect is set.
type reconnectingAdapter struct {
	t              *testing.T
	reconnect      bool
	badCredentials bool
	userID         string
}

func (a reconnectingAdapter) GetLibrary(ctx context.Context, _ int, _ func([]tasks.ExternalGameEntry) error) error {
	if a.reconnect {
		if _, err := testDB.NewRaw(`UPDATE user_sync_configs SET storefront_credentials = 'new-ct' WHERE user_id = ?`, a.userID).Exec(ctx); err != nil {
			a.t.Fatalf("simulated reconnect: %v", err)
		}
	}
	loaded := "loaded-ct"
	if a.badCredentials {
		loaded = "a\x00b" // Postgres rejects a NUL in text: the flag UPDATE itself fails
	}
	return &tasks.CredentialsErrorFor{Credentials: &loaded, Err: tasks.ErrCredentials}
}

// The store page refreshes its connection once, when it sees the sync job
// end, so credentials_error must already be set when the job turns failed.
// A trigger records the flag at the moment the job's status changes.
func TestDispatchSync_CredentialsError_FlagSetBeforeJobFails(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE r25_flag_at_fail (flag boolean)`,
		`CREATE FUNCTION r25_record_flag() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN INSERT INTO r25_flag_at_fail SELECT credentials_error FROM user_sync_configs WHERE user_id = NEW.user_id AND storefront = NEW.source; RETURN NEW; END'`,
		`CREATE TRIGGER r25_record_flag AFTER UPDATE OF status ON jobs FOR EACH ROW WHEN (NEW.status = 'failed') EXECUTE FUNCTION r25_record_flag()`,
	} {
		if _, err := testDB.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS r25_record_flag ON jobs`)
		_, _ = testDB.ExecContext(ctx, `DROP FUNCTION IF EXISTS r25_record_flag()`)
		_, _ = testDB.ExecContext(ctx, `DROP TABLE IF EXISTS r25_flag_at_fail`)
	})

	loaded := "loaded-ct"
	userID, jobID := seedBoundSync(t, "gog", &loaded)
	factory := func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return reconnectingAdapter{t: t, userID: userID}, nil
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var seen []bool
	if err := testDB.NewRaw(`SELECT flag FROM r25_flag_at_fail`).Scan(ctx, &seen); err != nil {
		t.Fatalf("read trigger record: %v", err)
	}
	if len(seen) != 1 || !seen[0] {
		t.Errorf("credentials_error when the job turned failed = %v, want [true]", seen)
	}
}

// tryEffects runs the queued finishing effects once each and returns the
// first error; an effect that failed stays queued, as River would retry it.
func tryEffects(t *testing.T, lc *joblifecycle.Module) error {
	t.Helper()
	ctx := context.Background()
	var rows []struct {
		ID   int64           `bun:"id"`
		Args json.RawMessage `bun:"args"`
	}
	if err := testDB.NewRaw(`SELECT id, args FROM river_job WHERE kind = ? AND state = 'available' ORDER BY id`, joblifecycle.EffectsArgs{}.Kind()).Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	var first error
	for _, r := range rows {
		var args joblifecycle.EffectsArgs
		if err := json.Unmarshal(r.Args, &args); err != nil {
			t.Fatal(err)
		}
		err := (&joblifecycle.EffectsWorker{Module: lc}).Work(ctx, riverJob(r.ID, 1, args))
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if _, err := testDB.ExecContext(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = ?`, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	return first
}

// The flag commits with the failed Job; its notice is the failed effect,
// queued in that same transaction. A notice that cannot be written is
// retried, not lost: a flag without its notice would silence every later
// sync (they see the store already flagged).
func TestDispatchSync_CredentialsError_NoticeRetriedUntilWritten(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE FUNCTION r25_refuse_event() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''forced event failure''; END'`,
		`CREATE TRIGGER r25_refuse_event BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION r25_refuse_event()`,
	} {
		if _, err := testDB.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	dropTrigger := func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS r25_refuse_event ON events`)
		_, _ = testDB.ExecContext(ctx, `DROP FUNCTION IF EXISTS r25_refuse_event()`)
	}
	t.Cleanup(dropTrigger)

	loaded := "loaded-ct"
	userID, jobID := seedBoundSync(t, "gog", &loaded)
	factory := func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return reconnectingAdapter{t: t, userID: userID}, nil
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tryEffects(t, w.LC); err == nil {
		t.Fatal("the effect must fail while events cannot be written")
	}
	flag, _, status, events := boundSyncOutcome(t, userID, jobID, "gog")
	if !flag || status != "failed" || events != 0 {
		t.Errorf("flag=%v status=%q events=%d, want flag set, job failed, no notice yet", flag, status, events)
	}

	dropTrigger()
	if err := tryEffects(t, w.LC); err != nil {
		t.Fatalf("retried effect: %v", err)
	}
	if _, _, _, events := boundSyncOutcome(t, userID, jobID, "gog"); events != 1 {
		t.Errorf("events after the retry = %d, want 1", events)
	}
}

// A subscription read that fails makes the effect fail and be retried; it
// never drops the notice or the flag.
func TestDispatchSync_CredentialsError_SubscriptionReadFailureRetriesNotice(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `ALTER TABLE notification_subscriptions RENAME TO notification_subscriptions_r25`); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		_, _ = testDB.ExecContext(ctx, `ALTER TABLE IF EXISTS notification_subscriptions_r25 RENAME TO notification_subscriptions`)
	}
	t.Cleanup(restore)

	loaded := "loaded-ct"
	userID, jobID := seedBoundSync(t, "gog", &loaded)
	factory := func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return reconnectingAdapter{t: t, userID: userID}, nil
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tryEffects(t, w.LC); err == nil {
		t.Fatal("the effect must fail while subscriptions cannot be read")
	}
	restore()
	if err := tryEffects(t, w.LC); err != nil {
		t.Fatalf("retried effect: %v", err)
	}
	flag, _, status, events := boundSyncOutcome(t, userID, jobID, "gog")
	if !flag || events != 1 || status != "failed" {
		t.Errorf("flag=%v events=%d status=%q, want flag set, one sync.failed notice, job failed", flag, events, status)
	}
	var eventType string
	if err := testDB.NewRaw(`SELECT type FROM events WHERE actor_user_id = ?`, userID).Scan(ctx, &eventType); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if eventType != "sync.failed" {
		t.Errorf("notice type = %q, want sync.failed (the user is not subscribed to sync.auth_expired)", eventType)
	}
}

// A flag that cannot be written takes the failure with it: the error goes
// to River, which runs the dispatch again; nothing is recorded meanwhile.
// (After the last attempt recovery fails the sync, which notifies.)
func TestDispatchSync_CredentialsError_FlagFailureRetriesDispatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	loaded := "loaded-ct"
	userID, jobID := seedBoundSync(t, "epic-games-store", &loaded)
	for _, stmt := range []string{
		`CREATE FUNCTION r25_refuse_config_update() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''flag refused''; END'`,
		`CREATE TRIGGER r25_refuse_config_update BEFORE UPDATE ON user_sync_configs FOR EACH ROW EXECUTE FUNCTION r25_refuse_config_update()`,
	} {
		if _, err := testDB.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS r25_refuse_config_update ON user_sync_configs`)
		_, _ = testDB.ExecContext(ctx, `DROP FUNCTION IF EXISTS r25_refuse_config_update()`)
	})
	factory := func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return unboundFailingAdapter{}, nil
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "epic-games-store"})); err == nil {
		t.Fatal("want the flag error returned to River")
	}
	runSyncEffects(t, w.LC)
	flag, _, status, events := boundSyncOutcome(t, userID, jobID, "epic-games-store")
	if flag || events != 0 || status != "processing" {
		t.Errorf("flag=%v events=%d status=%q, want no flag, no notice, job still processing", flag, events, status)
	}
}

// unboundFailingAdapter fails with a plain credentials error.
type unboundFailingAdapter struct{}

func (unboundFailingAdapter) GetLibrary(context.Context, int, func([]tasks.ExternalGameEntry) error) error {
	return tasks.ErrCredentials
}

// A refused Fail (the sync was cancelled meanwhile) leaves the flag alone.
func TestDispatchSync_CancelledJobDoesNotFlagCredentials(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	lc := newTestLC(t)
	loaded := "loaded-ct"
	userID, jobID := seedBoundSync(t, "gog", &loaded)
	factory := func(ctx context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		if err := lc.Cancel(ctx, jobID); err != nil {
			t.Fatalf("cancel job: %v", err)
		}
		return nil, tasks.ErrCredentials
	}
	worker := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: lc}
	if err := worker.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runSyncEffects(t, lc)
	flagged, _, status, events := boundSyncOutcome(t, userID, jobID, "gog")
	if flagged || status != "cancelled" || events != 0 {
		t.Errorf("cancelled sync left credentials_error=%v, status=%q, notices=%d; want false, cancelled, 0", flagged, status, events)
	}
}
