package migrate

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/riverqueue/river/rivermigrate"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/uptrace/bun"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

type AppState int32

const (
	AppStateDBUnavailable  AppState = iota
	AppStateNeedsMigration
	AppStateMigrating
	AppStateReady
	AppStateMigrationFailed
)

func (s AppState) String() string {
	switch s {
	case AppStateDBUnavailable:
		return "db_unavailable"
	case AppStateNeedsMigration:
		return "needs_migration"
	case AppStateMigrating:
		return "migrating"
	case AppStateReady:
		return "ready"
	case AppStateMigrationFailed:
		return "migration_failed"
	default:
		return "unknown"
	}
}

type Migrator struct {
	state             atomic.Int32
	prevState         atomic.Int32
	lastUnavailableAt atomic.Value
	needsSetup        bool
	mu                sync.RWMutex
	migrateMu         sync.Mutex
	probeInterval     time.Duration
	db                *bun.DB
	bunMig            *bunmigrate.Migrator
	logCh             chan string
	logWriter         io.Writer
	lastError         atomic.Value // string; "" or absent means no failure recorded
}

func NewMigrator(db *bun.DB) *Migrator {
	return &Migrator{db: db}
}

func (mg *Migrator) determineState() error {
	if mg.bunMig == nil {
		mg.bunMig = bunmigrate.NewMigrator(mg.db, migrations.Migrations)
		if err := mg.bunMig.Init(context.Background()); err != nil {
			return fmt.Errorf("determine state: init: %w", err)
		}
	}
	ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
	if err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	if len(ms.Unapplied()) > 0 {
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}

	riverPending, err := mg.riverPendingCount(context.Background())
	if err != nil {
		return fmt.Errorf("determine state: river: %w", err)
	}
	if riverPending > 0 {
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}

	mg.state.Store(int32(AppStateReady))
	return nil
}

func (mg *Migrator) riverPendingCount(ctx context.Context) (int, error) {
	riverMig, err := rivermigrate.New(riverdatabasesql.New(mg.db.DB), nil)
	if err != nil {
		return 0, err
	}
	existing, err := riverMig.ExistingVersions(ctx)
	if err != nil {
		return 0, err
	}
	return len(riverMig.AllVersions()) - len(existing), nil
}

func (mg *Migrator) DetermineState() error {
	return mg.determineState()
}

func (mg *Migrator) TransitionToReady() {
	mg.state.Store(int32(AppStateReady))
}

// TransitionToFailed records the error and switches state to AppStateMigrationFailed.
// The error is stored before the state flip so any observer that reads
// AppStateMigrationFailed is guaranteed to see a non-empty LastError —
// preserve this ordering when editing.
// The stored value is always a string; never store an error value here or
// atomic.Value will panic on subsequent loads of a different concrete type.
func (mg *Migrator) TransitionToFailed(err error) {
	mg.lastError.Store(err.Error())
	mg.state.Store(int32(AppStateMigrationFailed))
}

// LastError returns the most recent migration error message, or "" if none.
func (mg *Migrator) LastError() string {
	v := mg.lastError.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

func (mg *Migrator) State() AppState {
	return AppState(mg.state.Load())
}

func (mg *Migrator) PendingCount() (int, error) {
	if mg.bunMig == nil {
		if err := mg.determineState(); err != nil {
			return 0, fmt.Errorf("pending count: init: %w", err)
		}
	}
	ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
	if err != nil {
		return 0, fmt.Errorf("pending count: %w", err)
	}
	riverPending, err := mg.riverPendingCount(context.Background())
	if err != nil {
		return 0, fmt.Errorf("pending count: river: %w", err)
	}
	return len(ms.Unapplied()) + riverPending, nil
}

// Status returns the pending migration count and the name of the most-recently
// applied migration (or "none" if no migrations have been applied yet). It is
// intended for read-only CLI use such as `nexorious migrate status` and does
// not mutate Migrator state.
func (mg *Migrator) Status(ctx context.Context) (pending int, current string, err error) {
	if mg.bunMig == nil {
		mg.bunMig = bunmigrate.NewMigrator(mg.db, migrations.Migrations)
		if err := mg.bunMig.Init(ctx); err != nil {
			return 0, "", fmt.Errorf("status: init: %w", err)
		}
	}
	ms, err := mg.bunMig.MigrationsWithStatus(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("status: %w", err)
	}
	applied := ms.Applied()
	current = "none"
	if len(applied) > 0 {
		// Applied is sorted in descending order; index 0 is the most recent.
		current = applied[0].Name
	}
	return len(ms.Unapplied()), current, nil
}

func (mg *Migrator) LogCh() <-chan string {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()
	return mg.logCh
}

func (mg *Migrator) SetLogWriter(w io.Writer) {
	mg.logWriter = w
}

func (mg *Migrator) RunMigrations(ctx context.Context) error {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()

	if AppState(mg.state.Load()) == AppStateMigrating {
		return fmt.Errorf("migrations already in progress")
	}

	ch := make(chan string, 256)
	mg.logCh = ch
	mg.state.Store(int32(AppStateMigrating))

	if err := mg.bunMig.Lock(ctx); err != nil {
		mg.state.Store(int32(AppStateNeedsMigration))
		close(ch)
		return fmt.Errorf("migrate: acquire lock: %w", err)
	}
	defer mg.bunMig.Unlock(ctx) //nolint:errcheck

	group, err := mg.bunMig.Migrate(ctx)
	if err != nil {
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", err))
		mg.state.Store(int32(AppStateNeedsMigration))
		close(ch)
		return err
	}
	if group.IsZero() {
		mg.sendLog(ch, "No new migrations to run\n")
	} else {
		mg.sendLog(ch, fmt.Sprintf("Migrated to group %s\n", group))
	}

	riverMig, err := rivermigrate.New(riverdatabasesql.New(mg.db.DB), nil)
	if err != nil {
		mg.sendLog(ch, fmt.Sprintf("River migration setup failed: %v\n", err))
		mg.state.Store(int32(AppStateNeedsMigration))
		close(ch)
		return fmt.Errorf("migrate: River migrator: %w", err)
	}
	riverRes, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		mg.sendLog(ch, fmt.Sprintf("River migration failed: %v\n", err))
		mg.state.Store(int32(AppStateNeedsMigration))
		close(ch)
		return fmt.Errorf("migrate: River: %w", err)
	}
	if len(riverRes.Versions) == 0 {
		mg.sendLog(ch, "No new River migrations to run\n")
	} else {
		for _, v := range riverRes.Versions {
			mg.sendLog(ch, fmt.Sprintf("River migrated version %d\n", v.Version))
		}
	}
	close(ch)
	return nil
}

func (mg *Migrator) sendLog(ch chan string, line string) {
	if mg.logWriter != nil {
		_, _ = fmt.Fprint(mg.logWriter, line)
		return
	}
	select {
	case ch <- line:
	default:
	}
}

func (mg *Migrator) Close() error { return nil }

func (mg *Migrator) SetStateForTest(s AppState) {
	mg.state.Store(int32(s))
}

func NewMigratorForTest(s AppState) *Migrator {
	mg := &Migrator{}
	mg.state.Store(int32(s))
	return mg
}

func (mg *Migrator) NeedsSetup() bool {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	return mg.needsSetup
}

func (mg *Migrator) SetNeedsSetup(v bool) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	mg.needsSetup = v
}

func (mg *Migrator) InitNeedsSetup(ctx context.Context, db *bun.DB) error {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return fmt.Errorf("InitNeedsSetup: %w", err)
	}
	mg.SetNeedsSetup(count == 0)
	return nil
}

func (mg *Migrator) LastUnavailableAt() time.Time {
	v := mg.lastUnavailableAt.Load()
	if v == nil {
		return time.Time{}
	}
	return v.(time.Time)
}

func (mg *Migrator) SetProbeIntervalForTest(d time.Duration) {
	mg.probeInterval = d
}

func (mg *Migrator) StartDBProbe(ctx context.Context, db *bun.DB, onRecovery func(context.Context) error) {
	interval := mg.probeInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := db.PingContext(pingCtx)
			cancel()

			if err != nil {
				if AppState(mg.state.Load()) != AppStateDBUnavailable {
					mg.prevState.Store(mg.state.Load())
					mg.state.Store(int32(AppStateDBUnavailable))
					mg.lastUnavailableAt.Store(time.Now())
					slog.Warn("database unavailable", "err", err)
				}
			} else {
				if AppState(mg.state.Load()) == AppStateDBUnavailable {
					prev := AppState(mg.prevState.Load())
					if err := mg.recoverFromUnavailable(ctx, db, prev, onRecovery); err != nil {
						slog.Error("db probe: recovery failed, remaining in DBUnavailable", "err", err)
					}
				}
			}
		}
	}()
}

func (mg *Migrator) recoverFromUnavailable(ctx context.Context, db *bun.DB, prev AppState, onRecovery func(context.Context) error) error {
	switch prev {
	case AppStateDBUnavailable:
		if err := onRecovery(ctx); err != nil {
			return err
		}
		slog.Info("db probe: recovery complete (first init)")

	case AppStateMigrating:
		if err := mg.determineState(); err != nil {
			return err
		}
		slog.Info("db probe: recovery complete (re-determined state after migrating)", "state", mg.State())

	default:
		if mg.bunMig != nil {
			mg.bunMig = nil
		}
		if err := mg.determineState(); err != nil {
			return err
		}
		if prev == AppStateReady && mg.NeedsSetup() {
			if err := mg.InitNeedsSetup(ctx, db); err != nil {
				mg.state.Store(int32(AppStateDBUnavailable))
				return fmt.Errorf("re-check needsSetup: %w", err)
			}
		}
		slog.Info("db probe: recovery complete (re-determined state)", "state", mg.State())
	}
	return nil
}
