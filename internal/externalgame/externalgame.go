// Package externalgame owns the storefront side of a sync: External games
// (external_games) and their platform rows (external_game_platforms), and the
// rules about a game's editions (#1449, ADR 0008): the definition of a set of
// editions (SameEdition, EditionKey), locking a game's editions
// (LockEditions), and taking a same-title game's decision (TakePrecedent). It
// sits between internal/joblifecycle, which owns the sync Job, and
// internal/usergame, which owns the library.
//
// It also holds the library write (WriteToLibrary); the other writes to these
// tables move in over the following steps of #1412.
package externalgame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/usergame"
)

// Errors from WriteToLibrary for games it cannot write. Callers decide what
// each means: the sync worker fails (or skips) its item, a rematch or unskip
// records the match and writes nothing for that game.
var (
	// ErrNotFound: no external game with that id for the user.
	ErrNotFound = errors.New("external game not found")
	// ErrSkipped: the game is skipped (read under the row lock).
	ErrSkipped = errors.New("external game is skipped")
	// ErrUnmatched: the game has no resolved_igdb_id (read under the row lock).
	ErrUnmatched = errors.New("no resolved_igdb_id on external_game")
	// ErrNoPlatforms: the game has no external_game_platforms rows.
	ErrNoPlatforms = errors.New("external game has no platform rows")
)

// LibraryWrite is what WriteToLibrary wrote.
type LibraryWrite struct {
	ExternalGameID string
	Title          string
	IGDBID         int32
	Result         usergame.Result
}

// WriteToLibrary turns a resolved external game into library state (#1338):
// it ensures the games row, then acquires the user game on the game's
// platforms with usergame.ModeUpsert, and stamps library_written_at.
//
// It locks the external_games row FOR UPDATE first and reads the match and the
// skip flag under that lock, so a writer that loaded the game before a rematch
// committed writes the rematch's match, never the old one. Run it inside a
// transaction (the sync worker, rematch and unskip all do); every failed
// statement is returned, because inside a transaction it aborts the rest.
func WriteToLibrary(ctx context.Context, db bun.IDB, userID, externalGameID string) (LibraryWrite, error) {
	var eg models.ExternalGame
	if err := db.NewSelect().Model(&eg).
		Where("id = ? AND user_id = ?", externalGameID, userID).
		For("UPDATE").
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LibraryWrite{}, ErrNotFound
		}
		return LibraryWrite{}, fmt.Errorf("lock external game: %w", err)
	}
	out := LibraryWrite{ExternalGameID: eg.ID, Title: eg.Title}
	if eg.IsSkipped {
		return out, ErrSkipped
	}
	if eg.ResolvedIGDBID == nil {
		return out, ErrUnmatched
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
		return out, ErrNoPlatforms
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
		available := true
		plats = append(plats, usergame.PlatformInput{
			Platform: &egp.Platform, Storefront: &storefrontSlug,
			HoursPlayed: &egp.HoursPlayed, OwnershipStatus: &ownership,
			IsAvailable: &available, ExternalGameID: &eg.ID,
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
