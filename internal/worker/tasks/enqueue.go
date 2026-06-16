package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/importsource"
)

// ErrNilRiverClient is returned by EnqueueOrFail when called with a nil River
// client. The corresponding job_item is also marked 'failed' so it does not sit
// stranded in 'pending' without a backing River job.
var ErrNilRiverClient = errors.New("nil river client")

// EnqueueOrFail inserts a River job for the given job_item. If the insert
// cannot be performed — because the River client is nil or River returns an
// error — the job_item is moved to status='failed' with a diagnostic
// error_message so it does not get stuck in 'pending' forever.
//
// Why: job_items.status='pending' and the existence of a non-terminal river_job
// row must stay in lockstep. Every silent rc.Insert failure breaks that
// invariant and produces a job_item the workers will never touch again. The
// scheduled RescueOrphanedPendingItems job is a slow safety net (≥30 min,
// ≥1 h item age, parent job must still be 'processing'); EnqueueOrFail closes
// the gap synchronously by failing loudly at the call site instead.
//
// The job_item is marked failed only on Insert failure — on success the caller
// keeps the item in its current state (typically 'pending').
func EnqueueOrFail(
	ctx context.Context,
	db *bun.DB,
	rc *river.Client[pgx.Tx],
	jobItemID string,
	args river.JobArgs,
) error {
	if rc == nil {
		markEnqueueFailed(ctx, db, jobItemID, "river client unavailable")
		return ErrNilRiverClient
	}
	if _, err := rc.Insert(ctx, args, nil); err != nil {
		markEnqueueFailed(ctx, db, jobItemID, fmt.Sprintf("river enqueue failed: %v", err))
		return fmt.Errorf("river insert %s: %w", args.Kind(), err)
	}
	return nil
}

// UsesGenericImportPipeline reports whether a job source runs the shared
// ImportMatch → pending_review → ImportFinalize chain. Registry mapper sources
// qualify, and so does the CSV source: it runs the same pipeline via the
// import handler's enqueueImportJob without being a registry Mapper (it has no
// Parse(raw); it is the dialog-driven import card). Sync sources and the legacy
// single-stage nexorious import do NOT use this chain.
//
// This is the single source of truth for "is this source on the generic import
// pipeline?" — every match/finalize/resolve routing decision must consult it so
// a new pipeline source cannot silently miss one site.
func UsesGenericImportPipeline(source string) bool {
	return importsource.IsRegistered(source) || source == models.JobSourceCSV
}

// ArgsForJobType returns the appropriate River JobArgs for the given job_type,
// source, and job_item_id. Used by callers (the HTTP retry handlers, the orphan
// rescuer) that switch on job_type at runtime. The source disambiguates imports
// that share the "import" job_type but need a different task chain (registry
// sources use the match→finalize chain).
func ArgsForJobType(jobType, source, jobItemID string) (river.JobArgs, error) {
	// Generic-pipeline imports run a match→finalize chain; a retried item
	// re-enters at the match stage regardless of the (shared) "import" job_type.
	if UsesGenericImportPipeline(source) {
		return ImportMatchArgs{JobItemID: jobItemID}, nil
	}
	switch jobType {
	case models.JobTypeSync:
		return IGDBMatchArgs{JobItemID: jobItemID}, nil
	case models.JobTypeImport:
		return ImportItemArgs{JobItemID: jobItemID}, nil
	case models.JobTypeMetadataRefresh:
		return MetadataRefreshItemArgs{JobItemID: jobItemID}, nil
	default:
		return nil, fmt.Errorf("unknown job_type %q", jobType)
	}
}

// FinalizeArgsForSource returns the finalize-stage River args for an import
// source whose job_items are resolved interactively (the manual-match flow).
// Only generic-pipeline sources (registry mappers + CSV) are supported.
// Used by the generic job-item resolve endpoint so the finalize task is routed
// by source rather than hard-coded.
func FinalizeArgsForSource(source, jobItemID string) (river.JobArgs, error) {
	if UsesGenericImportPipeline(source) {
		return ImportFinalizeArgs{JobItemID: jobItemID}, nil
	}
	return nil, fmt.Errorf("source %q has no interactive finalize stage", source)
}

func markEnqueueFailed(ctx context.Context, db *bun.DB, jobItemID, msg string) {
	msg = logging.ScrubURLQueries(msg) // never persist URL query credentials (#937)
	now := time.Now().UTC()
	if _, err := db.NewRaw(
		`UPDATE job_items SET status = ?, error_message = ?, processed_at = ?
		 WHERE id = ? AND status = ?`,
		models.JobItemStatusFailed, msg, now, jobItemID, models.JobItemStatusPending,
	).Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "EnqueueOrFail: mark item failed",
			logging.KeyJobItemID, jobItemID, "msg", msg, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}
