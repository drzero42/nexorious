package tasks

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/logging"
)

// InsertSkippedChange records a skipped External game in a sync Job's diff:
// one 'skipped' row, unless the Job already has one for an edition of the game
// (#1443: a game is listed once, whatever editions it has). changes has no
// unique constraint, and the dispatch (which may run again), the write stage
// and a Skip can each record the same game in one Job. Two editions recorded at
// the same moment can still both write a row; that rare double line is
// accepted.
func InsertSkippedChange(ctx context.Context, db bun.IDB, jobID, userID, externalGameID, title string) error {
	if _, err := db.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 SELECT ?, ?, ?, ?, ?, ?, now()
		 WHERE NOT EXISTS (
		   SELECT 1
		     FROM changes c
		     JOIN external_games o ON o.id = c.external_game_id
		     JOIN external_games me ON me.id = ?
		    WHERE c.job_id = ? AND c.change_type = ? AND `+externalgame.SameEdition("o", "me")+`)`,
		uuid.NewString(), jobID, userID, externalGameID, enum.ChangeSkipped, title, externalGameID, jobID, enum.ChangeSkipped,
	).Exec(ctx); err != nil {
		return fmt.Errorf("insert sync_change (skipped): %w", err)
	}
	return nil
}

// InsertLibraryWriteChanges records a library write in a sync Job's diff:
// one status_changed row per ownership upgrade, otherwise one added row (when
// added) or one already_in_library row. The sync worker passes
// w.Result.Created; a rematch passes whether any write of the editions created
// the user game and this is the first item it completes in that Job.
func InsertLibraryWriteChanges(ctx context.Context, db bun.IDB, jobID, userID string, w externalgame.LibraryWrite, added bool) error {
	ugID := w.Result.UserGameID
	upgraded := false
	for _, pc := range w.Result.PlatformChanges {
		if !pc.OwnershipUpgraded {
			continue
		}
		upgraded = true
		if _, err := db.NewRaw(
			`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, old_status, new_status, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now())`,
			uuid.NewString(), jobID, userID, w.ExternalGameID, ugID, enum.ChangeStatusChanged, w.Title, pc.OldOwnership, pc.NewOwnership,
		).Exec(ctx); err != nil {
			return fmt.Errorf("insert change (status_changed): %w", err)
		}
	}
	var changeType enum.ChangeType
	switch {
	case added:
		changeType = enum.ChangeAdded
	case !upgraded:
		changeType = enum.ChangeAlreadyInLibrary
	}
	if changeType == "" {
		return nil
	}
	if _, err := db.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, now())`,
		uuid.NewString(), jobID, userID, w.ExternalGameID, ugID, changeType, w.Title,
	).Exec(ctx); err != nil {
		return fmt.Errorf("insert change (%s): %w", changeType, err)
	}
	return nil
}

// EnqueueImmediateMetadataFetch queues a fire-and-forget metadata_fetch for
// gameID when its games row has no description yet, so a newly written game
// gets cover art within seconds instead of at the next bulk refresh. It does
// not check whether IGDB is configured: MetadataFetchWorker does nothing then.
// Call it after commit; every failure is logged only (the bulk refresh is the
// safety net).
func EnqueueImmediateMetadataFetch(ctx context.Context, db bun.IDB, rc *river.Client[pgx.Tx], gameID int32) {
	var descriptionIsNull bool
	if err := db.NewRaw(
		`SELECT description IS NULL FROM games WHERE id = ?`, gameID,
	).Scan(ctx, &descriptionIsNull); err != nil {
		slog.WarnContext(ctx, "sync: check game description for immediate metadata fetch",
			logging.KeyErr, err, "game_id", gameID, logging.Cat(logging.CategoryDB))
		return
	}
	if !descriptionIsNull {
		return // already has metadata
	}
	if rc == nil {
		slog.WarnContext(ctx, "sync: river client unavailable, skipping immediate metadata fetch", "game_id", gameID)
		return
	}
	if _, err := rc.Insert(ctx, MetadataFetchArgs{GameID: gameID}, nil); err != nil {
		slog.WarnContext(ctx, "sync: enqueue immediate metadata fetch failed", logging.KeyErr, err, "game_id", gameID)
	}
}
