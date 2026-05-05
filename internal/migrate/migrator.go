package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	gmigrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
)

// AppState represents the application migration state.
type AppState int32

const (
	AppStateNeedsMigration AppState = iota
	AppStateMigrating
	AppStateReady
)

func (s AppState) String() string {
	switch s {
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
	state       atomic.Int32
	databaseURL string
	m           *gmigrate.Migrate
	logCh       chan string
	logWriter   io.Writer
	mu          sync.Mutex
}

// NewMigrator creates a Migrator, connects to the database, and determines
// the current migration state.
func NewMigrator(ctx context.Context, databaseURL string) (*Migrator, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrator: create iofs source: %w", err)
	}

	m, err := gmigrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("migrator: new migrate instance: %w", err)
	}

	mg := &Migrator{
		databaseURL: databaseURL,
		m:           m,
	}

	if err := mg.determineState(); err != nil {
		m.Close()
		return nil, err
	}

	return mg, nil
}

func (mg *Migrator) determineState() error {
	_, dirty, err := mg.m.Version()
	if errors.Is(err, gmigrate.ErrNilVersion) {
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}
	if err != nil {
		return fmt.Errorf("determine state: %w", err)
	}

	if dirty {
		ver, _, _ := mg.m.Version()
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
	defer src.Close()

	count := 0
	next := ver
	for {
		nextVer, openErr := src.Next(next)
		if openErr != nil {
			break
		}
		count++
		next = nextVer
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
	mg.mu.Lock()
	defer mg.mu.Unlock()
	return mg.logCh
}

// SetLogWriter sets a writer to receive migration log output instead of logCh.
func (mg *Migrator) SetLogWriter(w io.Writer) {
	mg.logWriter = w
}

// RunMigrations applies all pending migrations and transitions state accordingly.
func (mg *Migrator) RunMigrations(ctx context.Context) error {
	mg.mu.Lock()
	if AppState(mg.state.Load()) == AppStateMigrating {
		mg.mu.Unlock()
		return fmt.Errorf("migrations already in progress")
	}

	ch := make(chan string, 256)
	mg.logCh = ch
	mg.state.Store(int32(AppStateMigrating))

	adapter := &logAdapter{ch: ch, writer: mg.logWriter}
	mg.m.Log = adapter
	mg.mu.Unlock()

	err := mg.m.Up()
	if err == nil || errors.Is(err, gmigrate.ErrNoChange) {
		mg.state.Store(int32(AppStateReady))
		close(ch)
		// Phase 3 extension point: call OnReady() here when workers/scheduler are added.
		return nil
	}

	adapter.Printf("migration failed: %v\n", err)
	mg.state.Store(int32(AppStateNeedsMigration))
	close(ch)
	return err
}

// Close releases resources held by the Migrator.
func (mg *Migrator) Close() error {
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

// logAdapter implements migrate.Logger.
type logAdapter struct {
	ch     chan string
	writer io.Writer
}

func (l *logAdapter) Printf(format string, v ...any) {
	line := fmt.Sprintf(format, v...)
	if l.writer != nil {
		fmt.Fprint(l.writer, line)
		return
	}
	select {
	case l.ch <- line:
	default: // drop if buffer full
	}
}

func (l *logAdapter) Verbose() bool { return false }
