package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/usergame"
)

// Errors from WriteExternalGameToLibrary for games it cannot write. Callers
// decide what each means: the sync worker fails (or skips) its item, a rematch
// or unskip records the match and writes nothing for that game.
var (
	// ErrExternalGameNotFound: no external game with that id for the user.
	ErrExternalGameNotFound = errors.New("external game not found")
	// ErrExternalGameSkipped: the game is skipped (read under the row lock).
	ErrExternalGameSkipped = errors.New("external game is skipped")
	// ErrExternalGameUnmatched: the game has no resolved_igdb_id (read under the row lock).
	ErrExternalGameUnmatched = errors.New("no resolved_igdb_id on external_game")
	// ErrExternalGameNoPlatforms: the game has no external_game_platforms rows.
	ErrExternalGameNoPlatforms = errors.New("external game has no platform rows")
)

// LibraryWrite is what WriteExternalGameToLibrary wrote.
type LibraryWrite struct {
	ExternalGameID string
	Title          string
	IGDBID         int32
	Result         usergame.Result
}

// WriteExternalGameToLibrary turns a resolved external game into library state
// (#1338): it ensures the games row, then acquires the user game on the
// game's platforms with usergame.ModeUpsert, and stamps library_written_at.
//
// It locks the external_games row FOR UPDATE first and reads the match and the
// skip flag under that lock, so a writer that loaded the game before a rematch
// committed writes the rematch's match, never the old one. Run it inside a
// transaction (the sync worker, rematch and unskip all do); every failed
// statement is returned, because inside a transaction it aborts the rest.
func WriteExternalGameToLibrary(ctx context.Context, db bun.IDB, userID, externalGameID string) (LibraryWrite, error) {
	var eg models.ExternalGame
	if err := db.NewSelect().Model(&eg).
		Where("id = ? AND user_id = ?", externalGameID, userID).
		For("UPDATE").
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LibraryWrite{}, ErrExternalGameNotFound
		}
		return LibraryWrite{}, fmt.Errorf("lock external game: %w", err)
	}
	out := LibraryWrite{ExternalGameID: eg.ID, Title: eg.Title}
	if eg.IsSkipped {
		return out, ErrExternalGameSkipped
	}
	if eg.ResolvedIGDBID == nil {
		return out, ErrExternalGameUnmatched
	}
	out.IGDBID = *eg.ResolvedIGDBID

	if err := EnsureGameRow(ctx, db, out.IGDBID, eg.Title); err != nil {
		return out, err
	}

	var egPlatforms []models.ExternalGamePlatform
	if err := db.NewSelect().Model(&egPlatforms).
		Where("external_game_id = ?", eg.ID).
		OrderExpr("platform").
		Scan(ctx); err != nil {
		return out, fmt.Errorf("load platforms: %w", err)
	}
	if len(egPlatforms) == 0 {
		return out, ErrExternalGameNoPlatforms
	}

	storefrontSlug := eg.Storefront
	ownership := "owned"
	if eg.OwnershipStatus != nil {
		ownership = *eg.OwnershipStatus
	} else if eg.IsSubscription {
		ownership = "subscription"
	}
	plats := make([]usergame.PlatformInput, 0, len(egPlatforms))
	for _, egp := range egPlatforms {
		egp := egp
		plats = append(plats, usergame.PlatformInput{
			Platform: &egp.Platform, Storefront: &storefrontSlug,
			HoursPlayed: &egp.HoursPlayed, OwnershipStatus: &ownership,
			IsAvailable: boolptr(true), ExternalGameID: &eg.ID,
			SyncFromSource:       true,
			AchievementsUnlocked: egp.AchievementsUnlocked,
			AchievementsTotal:    egp.AchievementsTotal,
		})
	}
	res, err := usergame.Acquire(ctx, db, usergame.AcquireParams{
		UserID: userID, GameID: out.IGDBID, Mode: usergame.ModeUpsert, Platforms: plats,
	})
	if err != nil {
		return out, fmt.Errorf("acquire: %w", err)
	}
	out.Result = res

	if _, err := db.NewRaw(
		`UPDATE external_games SET updated_at = now(), library_written_at = clock_timestamp() WHERE id = ?`, eg.ID,
	).Exec(ctx); err != nil {
		return out, fmt.Errorf("stamp library_written_at: %w", err)
	}
	return out, nil
}

// EnsureGameRow inserts a stub games row for igdbID when none exists; the
// metadata fetch fills it in later. external_games.resolved_igdb_id and
// user_games.game_id reference it.
func EnsureGameRow(ctx context.Context, db bun.IDB, igdbID int32, title string) error {
	if _, err := db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID, title,
	).Exec(ctx); err != nil {
		return fmt.Errorf("ensure game row %d: %w", igdbID, err)
	}
	return nil
}

// InsertLibraryWriteChanges records a library write in a sync Job's diff:
// one status_changed row per ownership upgrade, otherwise one added row (when
// added) or one already_in_library row. The sync worker passes
// w.Result.Created; a rematch passes whether any write in the family created
// the user game and this is the first item it completes in that Job.
func InsertLibraryWriteChanges(ctx context.Context, db bun.IDB, jobID, userID string, w LibraryWrite, added bool) error {
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
