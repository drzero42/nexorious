// Package restoremarker records a backup restore in progress in the database
// itself (#1341), so that every process that opens the database (the server,
// `nexorious migrate` in an init container, the database probe) sees it.
//
// The marker lives in a schema of its own, nexorious_restore, which a restore
// does not drop: a restore replaces the public schema only. The table uses
// nothing from public (no types, foreign keys or extension functions), so
// DROP SCHEMA public CASCADE cannot reach it. It is created by the restore, not
// by a migration.
package restoremarker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/uptrace/bun"
)

// Phase is the stage a restore had reached. It is stored as text so a row
// written by another version still reads; see Marker.Restoring.
type Phase string

const (
	// PhaseCleanup: the database is whole, but work that was in flight has to
	// be cleared before River starts. Clear does that.
	PhaseCleanup Phase = "cleanup"
	// PhaseRestoring: the public schema is being replaced and may be
	// half-loaded. Nothing may migrate it or start River on it.
	PhaseRestoring Phase = "restoring"
)

// InterruptedMessage is the error message Clear gives every Job that was
// active when the restore began.
const InterruptedMessage = "interrupted by a restore"

const createSQL = `
CREATE SCHEMA IF NOT EXISTS nexorious_restore;
CREATE TABLE IF NOT EXISTS nexorious_restore.marker (
	id                    boolean PRIMARY KEY DEFAULT true CHECK (id),
	phase                 text NOT NULL,
	backup_id             text,
	pre_restore_backup_id text,
	updated_at            timestamptz NOT NULL DEFAULT now()
)`

// Marker is the one row of nexorious_restore.marker.
type Marker struct {
	Phase              Phase     `json:"phase"`
	BackupID           string    `json:"backup_id"`
	PreRestoreBackupID string    `json:"pre_restore_backup_id"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Restoring reports whether the marker forbids using the database: any phase
// other than cleanup. A phase this version does not know counts as restoring,
// so a row written by a newer version fails closed.
func (m *Marker) Restoring() bool {
	return m.Phase != PhaseCleanup
}

// Read returns the marker, or nil when there is none: no schema or table (a
// fresh install, or the first start after upgrading) or no row. Any error means
// the database could not be asked, never "no marker".
func Read(ctx context.Context, db bun.IDB) (*Marker, error) {
	var table sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('nexorious_restore.marker')::text`).Scan(&table); err != nil {
		return nil, fmt.Errorf("read restore marker: %w", err)
	}
	if !table.Valid {
		return nil, nil
	}
	// to_jsonb keeps the read tolerant across versions: columns this version
	// does not know are ignored, columns it misses stay zero.
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT to_jsonb(m) FROM nexorious_restore.marker m LIMIT 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read restore marker: %w", err)
	}
	var m Marker
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode restore marker: %w", err)
	}
	return &m, nil
}

// Set creates the marker schema and table if needed and writes the row.
func Set(ctx context.Context, db bun.IDB, m Marker) error {
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("create restore marker: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO nexorious_restore.marker (phase, backup_id, pre_restore_backup_id, updated_at)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), now())
		ON CONFLICT (id) DO UPDATE SET
			phase = EXCLUDED.phase,
			backup_id = EXCLUDED.backup_id,
			pre_restore_backup_id = EXCLUDED.pre_restore_backup_id,
			updated_at = EXCLUDED.updated_at`,
		string(m.Phase), m.BackupID, m.PreRestoreBackupID); err != nil {
		return fmt.Errorf("write restore marker: %w", err)
	}
	return nil
}

// ErrStillRestoring is returned by Clear when the marker says restoring.
var ErrStillRestoring = errors.New("restore marker says restoring")

// Clear removes what a restore left in flight and deletes the marker, in one
// transaction. It must run before the app state becomes Ready, while no request
// is served and River is stopped: run later, it could wipe a Job a client had
// just created.
//
//   - river_job and river_leader are emptied (without RESTART IDENTITY, so job
//     ids keep increasing);
//   - every active Job (pending, processing) is failed with InterruptedMessage
//     and its unfinished items are cancelled, by joblifecycle.FailAllAfterRestore.
//
// With no marker it does nothing and reports false. It reports true when it
// cleared. With the marker at restoring it changes nothing and returns
// ErrStillRestoring.
func Clear(ctx context.Context, db *bun.DB) (bool, error) {
	cleared := false
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var table sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT to_regclass('nexorious_restore.marker')::text`).Scan(&table); err != nil {
			return err
		}
		if !table.Valid {
			return nil
		}
		// Lock the row so two callers cannot both clear, and so a concurrent
		// change of phase is seen.
		var phase string
		err := tx.QueryRowContext(ctx, `SELECT phase FROM nexorious_restore.marker FOR UPDATE`).Scan(&phase)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if Phase(phase) != PhaseCleanup {
			return ErrStillRestoring
		}

		if _, err := tx.ExecContext(ctx, `TRUNCATE river_job, river_leader`); err != nil {
			return fmt.Errorf("empty the queue: %w", err)
		}
		// The Job lifecycle module is the one writer of Job and item status,
		// also here; it queues no finishing effects for a restore.
		if err := joblifecycle.FailAllAfterRestore(ctx, tx, InterruptedMessage); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM nexorious_restore.marker`); err != nil {
			return fmt.Errorf("delete restore marker: %w", err)
		}
		cleared = true
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrStillRestoring) {
			return false, err
		}
		return false, fmt.Errorf("clear after restore: %w", err)
	}
	return cleared, nil
}
