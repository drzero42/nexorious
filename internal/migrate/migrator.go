package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gmigrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	migsource "github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
)

// AppState represents the application migration state.
type AppState int32

const (
	AppStateDBUnavailable  AppState = iota // MUST be 0 — sentinel for prevState
	AppStateNeedsMigration
	AppStateMigrating
	AppStateReady
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
	default:
		return "unknown"
	}
}

// Migrator manages migration state.
type Migrator struct {
	state             atomic.Int32
	prevState         atomic.Int32 // state before DBUnavailable; zero == never operational
	lastUnavailableAt atomic.Value // stores time.Time
	needsSetup        bool
	mu                sync.RWMutex // guards needsSetup
	migrateMu         sync.Mutex   // guards mg.m; held by RunMigrations for its entire duration
	probeInterval     time.Duration // 0 = use default 5s; set via SetProbeIntervalForTest
	databaseURL       string
	src               migsource.Driver  // created in NewMigrator, reused in determineState
	m                 *gmigrate.Migrate // nil until determineState() first called
	logCh             chan string
	logWriter         io.Writer
}

// NewMigrator creates a Migrator ready to use.
// It does NOT connect to the database — state is DBUnavailable (zero value)
// until DetermineStateForTest() or determineState() is called.
func NewMigrator(databaseURL string) (*Migrator, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrator: create iofs source: %w", err)
	}
	return &Migrator{
		databaseURL: databaseURL,
		src:         src,
	}, nil
}

func (mg *Migrator) determineState() error {
	if mg.m == nil {
		migrateURL := strings.NewReplacer(
			"postgresql://", "pgx5://",
			"postgres://", "pgx5://",
		).Replace(mg.databaseURL)
		m, err := gmigrate.NewWithSourceInstance("iofs", mg.src, migrateURL)
		if err != nil {
			return fmt.Errorf("determine state: connect: %w", err)
		}
		mg.m = m
	}

	ver, dirty, err := mg.m.Version()
	if errors.Is(err, gmigrate.ErrNilVersion) {
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}
	if err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	if dirty {
		slog.Error("database is in dirty state",
			"version", ver,
			"hint", "manually resolve the migration and clear the dirty flag")
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}
	count, err := mg.PendingCount()
	if err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	if count > 0 {
		mg.state.Store(int32(AppStateNeedsMigration))
	} else {
		mg.state.Store(int32(AppStateReady))
	}
	return nil
}

// DetermineStateForTest calls determineState and is intended for tests only.
func (mg *Migrator) DetermineStateForTest() error {
	return mg.determineState()
}

// TransitionToReady atomically sets state to Ready. Called by the migration
// handler after InitNeedsSetup completes successfully.
func (mg *Migrator) TransitionToReady() {
	mg.state.Store(int32(AppStateReady))
}

// State returns the current AppState atomically.
func (mg *Migrator) State() AppState {
	return AppState(mg.state.Load())
}

// PendingCount returns the number of migrations not yet applied.
func (mg *Migrator) PendingCount() (int, error) {
	ver, _, err := mg.m.Version()
	if errors.Is(err, gmigrate.ErrNilVersion) {
		ver = 0
		err = nil
	}
	if err != nil {
		return 0, fmt.Errorf("pending count: %w", err)
	}

	src, srcErr := iofs.New(migrations.FS, ".")
	if srcErr != nil {
		return 0, fmt.Errorf("pending count source: %w", srcErr)
	}
	defer func() { _ = src.Close() }()

	count := 0
	if ver == 0 {
		// Fresh database: start from the first available migration.
		firstVer, firstErr := src.First()
		if firstErr != nil {
			// No migrations exist at all.
			return 0, nil
		}
		count = 1
		ver = firstVer
	}
	for {
		nextVer, openErr := src.Next(ver)
		if openErr != nil {
			break
		}
		count++
		ver = nextVer
	}
	return count, nil
}

// CurrentVersion returns the current migration version, dirty flag, and error.
// Returns (0, false, nil) when no migrations have been applied yet.
func (mg *Migrator) CurrentVersion() (uint, bool, error) {
	ver, dirty, err := mg.m.Version()
	if errors.Is(err, gmigrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return ver, dirty, nil
}

// LogCh returns the current log channel (nil if RunMigrations has not been called).
func (mg *Migrator) LogCh() <-chan string {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()
	return mg.logCh
}

// SetLogWriter sets a writer to receive migration log output instead of logCh.
func (mg *Migrator) SetLogWriter(w io.Writer) {
	mg.logWriter = w
}

// RunMigrations applies all pending migrations and transitions state accordingly.
func (mg *Migrator) RunMigrations(ctx context.Context) error {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()

	if AppState(mg.state.Load()) == AppStateMigrating {
		return fmt.Errorf("migrations already in progress")
	}

	ch := make(chan string, 256)
	mg.logCh = ch
	mg.state.Store(int32(AppStateMigrating))

	adapter := &logAdapter{ch: ch, writer: mg.logWriter}
	mg.m.Log = adapter

	err := mg.m.Up()
	if err == nil || errors.Is(err, gmigrate.ErrNoChange) {
		close(ch)
		return nil
	}

	adapter.Printf("migration failed: %v\n", err)
	mg.state.Store(int32(AppStateNeedsMigration))
	close(ch)
	return err
}

// Close releases resources held by the Migrator.
func (mg *Migrator) Close() error {
	if mg.m == nil {
		return nil
	}
	srcErr, dbErr := mg.m.Close()
	if srcErr != nil {
		return srcErr
	}
	return dbErr
}

// SetStateForTest sets the state atomically (for tests only).
func (mg *Migrator) SetStateForTest(s AppState) {
	mg.state.Store(int32(s))
}

// NewMigratorForTest creates a Migrator with the given state for testing middleware.
// The underlying golang-migrate instance is nil — do not call RunMigrations or PendingCount.
func NewMigratorForTest(s AppState) *Migrator {
	mg := &Migrator{}
	mg.state.Store(int32(s))
	return mg
}

// NeedsSetup returns true if no admin user has been created yet.
func (mg *Migrator) NeedsSetup() bool {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	return mg.needsSetup
}

// SetNeedsSetup sets the needsSetup flag.
func (mg *Migrator) SetNeedsSetup(v bool) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	mg.needsSetup = v
}

// InitNeedsSetup queries the users table and sets needsSetup = (count == 0).
func (mg *Migrator) InitNeedsSetup(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return fmt.Errorf("InitNeedsSetup: %w", err)
	}
	mg.SetNeedsSetup(count == 0)
	return nil
}

// LastUnavailableAt returns the time the DB was last detected as unavailable.
// Returns the zero time.Time if the DB has never been unavailable.
func (mg *Migrator) LastUnavailableAt() time.Time {
	v := mg.lastUnavailableAt.Load()
	if v == nil {
		return time.Time{}
	}
	return v.(time.Time)
}

// SetProbeIntervalForTest overrides the probe ticker interval for unit tests.
// Must be called before StartDBProbe.
func (mg *Migrator) SetProbeIntervalForTest(d time.Duration) {
	mg.probeInterval = d
}

// StartDBProbe polls pool.Ping() on a configurable interval and manages the
// DBUnavailable state. onRecovery is called (with the probe's context) when
// the DB first comes back from unavailable.
func (mg *Migrator) StartDBProbe(ctx context.Context, pool *pgxpool.Pool, onRecovery func(context.Context) error) {
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
			err := pool.Ping(pingCtx)
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
					if err := mg.recoverFromUnavailable(ctx, pool, prev, onRecovery); err != nil {
						slog.Error("db probe: recovery failed, remaining in DBUnavailable", "err", err)
					}
				}
			}
		}
	}()
}

func (mg *Migrator) recoverFromUnavailable(ctx context.Context, pool *pgxpool.Pool, prev AppState, onRecovery func(context.Context) error) error {
	switch prev {
	case AppStateDBUnavailable:
		// Never had an operational state — run full init.
		if err := onRecovery(ctx); err != nil {
			return err
		}
		slog.Info("db probe: recovery complete (first init)")

	case AppStateMigrating:
		// Migration goroutine died — re-consult DB for actual state.
		if err := mg.determineState(); err != nil {
			return err
		}
		slog.Info("db probe: recovery complete (re-determined state after migrating)", "state", mg.State())

	default:
		// NeedsMigration or Ready — re-determine state with a fresh connection.
		// mg.m may hold a stale connection (e.g. the DB socket was removed and
		// recreated on restart), so close it and let determineState reconnect.
		if mg.m != nil {
			_, _ = mg.m.Close()
			mg.m = nil
		}
		if err := mg.determineState(); err != nil {
			return err
		}
		if prev == AppStateReady && mg.NeedsSetup() {
			if err := mg.InitNeedsSetup(ctx, pool); err != nil {
				mg.state.Store(int32(AppStateDBUnavailable))
				return fmt.Errorf("re-check needsSetup: %w", err)
			}
		}
		slog.Info("db probe: recovery complete (re-determined state)", "state", mg.State())
	}
	return nil
}

// logAdapter implements migrate.Logger.
type logAdapter struct {
	ch     chan string
	writer io.Writer
}

func (l *logAdapter) Printf(format string, v ...any) {
	line := fmt.Sprintf(format, v...)
	if l.writer != nil {
		_, _ = fmt.Fprint(l.writer, line)
		return
	}
	select {
	case l.ch <- line:
	default: // drop if buffer full
	}
}

func (l *logAdapter) Verbose() bool { return false }
