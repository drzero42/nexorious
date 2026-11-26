package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/logging"
)

// WriteMatchedToLibrary writes the matched External game id to the library,
// as Rematch, Unskip and the match worker's completion of same-title reviews
// do. A game with no platform rows has nothing to write: that is logged and
// reported as wrote=false without an error, since its match stands and its
// item can still complete (without diff rows).
func WriteMatchedToLibrary(ctx context.Context, db bun.IDB, userID, id string) (lw externalgame.LibraryWrite, wrote bool, err error) {
	lw, err = externalgame.WriteToLibrary(ctx, db, userID, id)
	if errors.Is(err, externalgame.ErrNoPlatforms) {
		slog.WarnContext(ctx, "sync: external game has no platform rows; match recorded, nothing written to the library",
			logging.KeyExternalGameID, id)
		return lw, false, nil
	}
	if err != nil {
		return lw, false, fmt.Errorf("write %s to the library: %w", id, err)
	}
	return lw, true, nil
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
			 VALUES (?, ?, ?, ?, ?, 'status_changed', ?, ?, ?, now())`,
			uuid.NewString(), jobID, userID, w.ExternalGameID, ugID, w.Title, pc.OldOwnership, pc.NewOwnership,
		).Exec(ctx); err != nil {
			return fmt.Errorf("insert change (status_changed): %w", err)
		}
	}
	changeType := ""
	switch {
	case added:
		changeType = "added"
	case !upgraded:
		changeType = "already_in_library"
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
