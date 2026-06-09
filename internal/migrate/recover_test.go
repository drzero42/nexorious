package migrate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)

// TestStartDBProbe_RecoveryFromMigrating exercises the AppStateMigrating arm of
// recoverFromUnavailable: when the probe recovers a DBUnavailable migrator whose
// prevState was Migrating, it re-runs determineState() to re-resolve the correct
// state. Staged deterministically (no bad-DB timing dependency / t.Skip) so the
// assertion always runs.
func TestStartDBProbe_RecoveryFromMigrating(t *testing.T) {
	db := setupTestDB(t)

	// Apply migrations so the DB is in a known good state.
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Stage the recovery entry point: prevState = Migrating (the arm under test),
	// state = DBUnavailable.
	m.SetPrevStateForTest(migrate.AppStateMigrating)
	m.SetStateForTest(migrate.AppStateDBUnavailable)
	m.SetProbeIntervalForTest(30 * time.Millisecond)

	// Probe against the GOOD db: state is DBUnavailable and the ping succeeds, so
	// the probe recovers via recoverFromUnavailable(prev=Migrating), the arm that
	// re-determines state from the DB.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.StartDBProbe(ctx, db, func(_ context.Context) error { return nil })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() != migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if m.State() == migrate.AppStateDBUnavailable {
		t.Fatal("expected recovery from DBUnavailable (Migrating arm), but still DBUnavailable")
	}
	// determineState() on a fully-migrated DB resolves to Ready.
	if m.State() != migrate.AppStateReady {
		t.Errorf("expected Ready after recovery, got %v", m.State())
	}
}

// TestPendingCount_WithoutPriorDetermineState exercises the
// "mg.bunMig == nil" lazy-init branch in PendingCount, and verifies that
// the lazy path produces the same count as the eager path.
func TestPendingCount_WithoutPriorDetermineState(t *testing.T) {
	db := setupTestDB(t)

	// Lazy path: PendingCount before DetermineState (bunMig == nil).
	lazy := migrate.NewMigrator(db)
	lazyCount, err := lazy.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount (lazy): %v", err)
	}

	// Eager path: same query after DetermineState (bunMig initialised).
	eager := migrate.NewMigrator(db)
	if err := eager.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	eagerCount, err := eager.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount (eager): %v", err)
	}

	if lazyCount != eagerCount {
		t.Errorf("lazy-init count (%d) differs from eager-init count (%d)", lazyCount, eagerCount)
	}
	if lazyCount <= 0 {
		t.Errorf("expected positive pending count on fresh DB, got %d", lazyCount)
	}
}

// TestLastUnavailableAt_ZeroValue verifies the zero-value path when
// lastUnavailableAt has never been set.
func TestLastUnavailableAt_ZeroValue(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	if !m.LastUnavailableAt().IsZero() {
		t.Error("expected zero time before any unavailability")
	}
}

// TestStartDBProbe_RecoveryFromFailed_ClearsLastError exercises the default
// branch of recoverFromUnavailable after a MigrationFailed → DBUnavailable
// transition. The recovery re-determines state from the (now reachable) DB and
// must clear the stale lastError inherited from the earlier failure.
func TestStartDBProbe_RecoveryFromFailed_ClearsLastError(t *testing.T) {
	db := setupTestDB(t)

	// Bring the DB to a known-good, fully-migrated state.
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Record a failure, then stage the recovery entry point deterministically:
	// prevState = MigrationFailed (the default-arm trigger), state = DBUnavailable.
	m.TransitionToFailed(errors.New("boom"))
	if m.LastError() != "boom" {
		t.Fatalf("precondition: expected LastError %q, got %q", "boom", m.LastError())
	}
	m.SetPrevStateForTest(migrate.AppStateMigrationFailed)
	m.SetStateForTest(migrate.AppStateDBUnavailable)

	// Probe against the GOOD db: state is DBUnavailable and the ping succeeds, so
	// the probe recovers via recoverFromUnavailable(prev=MigrationFailed) — the
	// default arm. No bad-DB timing dependency, so the assertion always runs.
	m.SetProbeIntervalForTest(30 * time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.StartDBProbe(ctx, db, func(_ context.Context) error { return nil })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() != migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if m.State() == migrate.AppStateDBUnavailable {
		t.Fatal("expected recovery from DBUnavailable (default branch), but still DBUnavailable")
	}
	if got := m.LastError(); got != "" {
		t.Errorf("expected LastError cleared after recovery, got %q", got)
	}
	// determineState() in the recovery arm (not RunMigrations) sets Ready directly,
	// so unlike the happy-path migration flow no TransitionToReady() is involved.
	if m.State() != migrate.AppStateReady {
		t.Errorf("expected Ready after recovery, got %v", m.State())
	}
}
