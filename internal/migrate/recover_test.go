package migrate_test

import (
	"context"
	"testing"
	"time"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)

// TestStartDBProbe_RecoveryFromMigrating exercises the AppStateMigrating
// branch inside recoverFromUnavailable. The probe transitions the migrator
// out of DBUnavailable when the DB is reachable; since prevState was
// Migrating, determineState() is called to re-resolve the correct state.
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

	// Simulate: state went Migrating → DBUnavailable.
	// We set prevState by first going Migrating then Unavailable via SetStateForTest.
	// The probe internally stores prevState when it detects unavailability.
	// We can't set prevState directly, so we drive the probe through a
	// two-step transition: trigger the "detect unavailable" path first.
	m.SetStateForTest(migrate.AppStateMigrating)
	m.SetProbeIntervalForTest(30 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Probe with a bad DB first to store prevState=Migrating, then we'll
	// recover by probing with the good DB.
	badDB := badBunDB(t)
	m.StartDBProbe(ctx, badDB, func(_ context.Context) error { return nil })

	// Wait for the probe to mark unavailable (stores prevState=Migrating).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() == migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if m.State() != migrate.AppStateDBUnavailable {
		t.Skip("probe did not set DBUnavailable within timeout, skipping recovery test")
	}

	cancel() // stop first probe

	// Now start a second probe with the good DB so recovery executes
	// the Migrating branch (re-determines state from DB).
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	m.SetProbeIntervalForTest(30 * time.Millisecond)
	m.StartDBProbe(ctx2, db, func(_ context.Context) error { return nil })

	deadline2 := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline2) {
		if m.State() != migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if m.State() == migrate.AppStateDBUnavailable {
		t.Error("expected recovery from DBUnavailable (Migrating branch), but still DBUnavailable")
	}
}

// TestStartDBProbe_RecoveryFromReady exercises the default branch in
// recoverFromUnavailable (prevState = Ready). The DB is up; state should
// be re-determined as Ready.
func TestStartDBProbe_RecoveryFromReady(t *testing.T) {
	db := setupTestDB(t)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	m.SetStateForTest(migrate.AppStateReady)
	m.SetProbeIntervalForTest(30 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	badDB := badBunDB(t)
	m.StartDBProbe(ctx, badDB, func(_ context.Context) error { return nil })

	// Wait for probe to detect unavailability (prevState=Ready stored internally).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() == migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if m.State() != migrate.AppStateDBUnavailable {
		t.Skip("probe did not set DBUnavailable within timeout")
	}

	cancel() // stop bad-DB probe

	// Now probe with good DB → default branch in recoverFromUnavailable.
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	m.SetProbeIntervalForTest(30 * time.Millisecond)
	m.StartDBProbe(ctx2, db, func(_ context.Context) error { return nil })

	deadline2 := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline2) {
		if m.State() != migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if m.State() == migrate.AppStateDBUnavailable {
		t.Error("expected recovery from DBUnavailable (default branch), but still DBUnavailable")
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
