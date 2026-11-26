package externalgame

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/usergame"
)

// The decisions on a set of editions: Rematch, Skip and Unskip (#1412). Each
// acts on every edition of the game it is given or, with only-this-edition
// (#1452), on that game alone: the whole set is still locked, but only that
// game is changed. A decision is made on the handle LockEditions returns, so
// it always runs under the set's lock.

// ErrOrphanActionRequired: a Rematch would leave a user game with no
// storefront link, and the request names no orphan action (keep or remove).
// Callers answer it with 409.
var ErrOrphanActionRequired = errors.New("orphan_action required: game would lose its only storefront link")

// OrphanRemove is the orphan action that deletes the user games a Rematch
// leaves without a storefront link. Any other non-empty action keeps them.
const OrphanRemove = "remove"

// Editions is a locked set of editions: the game LockEditions was given and
// every edition of it, locked FOR UPDATE in the caller's transaction. It holds
// nothing but the rows read under the lock; use it only inside that
// transaction.
type Editions struct {
	tx     bun.Tx
	userID string
	game   models.ExternalGame
	set    []models.ExternalGame // ordered by id, game included
}

// Game is the game LockEditions was given, as read under the lock.
func (e *Editions) Game() models.ExternalGame { return e.game }

// IDs is the ids of the whole set, the game included, in id order.
func (e *Editions) IDs() []string { return GameIDs(e.set) }

// Changed is the External games a decision changes: the given game alone
// with onlyThis, else the whole set.
func (e *Editions) Changed(onlyThis bool) []string {
	changed, _ := e.scope(onlyThis)
	return changed
}

// scope is Changed, plus the editions that stay in the set.
func (e *Editions) scope(onlyThis bool) (changed, staying []string) {
	if !onlyThis {
		return e.IDs(), nil
	}
	for _, g := range e.set {
		if g.ID != e.game.ID {
			staying = append(staying, g.ID)
		}
	}
	return []string{e.game.ID}, staying
}

// RematchRequest is a Rematch: the new match, what to do with a user game the
// rematch leaves without a storefront link ("" refuses with
// ErrOrphanActionRequired, OrphanRemove deletes it, anything else keeps it),
// and whether to rematch the given game alone.
//
// IGDBID is the id as the request gave it. Only the games row takes it as
// int32; the match is written as given, so an id outside int32 is refused by
// the database instead of wrapping to another game.
//
// IGDBTitle is the IGDB game's title, for the games row when the IGDB game is
// new to the database (#1455). The caller looks it up in IGDB; "" falls back
// to the External game's title until the metadata fetch replaces it.
type RematchRequest struct {
	IGDBID          int
	IGDBTitle       string
	OrphanAction    string
	OnlyThisEdition bool
}

// Rematch re-resolves the changed games to the new match, unskips them and
// writes them to the library. It returns the library write of each changed
// game that was written (one with no platform rows is not, see WriteMatched).
//
// With OnlyThisEdition the editions that stay keep their library game: they
// take over the links they share with the leaving game first, so the orphan
// rule, applied to the leaving game alone, counts their links as links from
// outside.
func (e *Editions) Rematch(ctx context.Context, req RematchRequest) (map[string]LibraryWrite, error) {
	changed, staying := e.scope(req.OnlyThisEdition)
	if err := EnsureGameRow(ctx, e.tx, int32(req.IGDBID), req.IGDBTitle, e.game.Title); err != nil { //nolint:gosec // an id outside int32 is refused by the match UPDATE below, which takes it unconverted
		return nil, err
	}
	if err := e.handOverSharedLinks(ctx, staying); err != nil {
		return nil, err
	}
	if err := e.unlink(ctx, changed, req.IGDBID, req.OrphanAction); err != nil {
		return nil, err
	}
	if _, err := e.tx.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id IN (?)`,
		req.IGDBID, bun.List(changed),
	).Exec(ctx); err != nil {
		return nil, fmt.Errorf("set match: %w", err)
	}
	return e.write(ctx, changed)
}

// Skip skips the changed games. Nothing is unlinked, but with onlyThis the
// editions that stay first take over the links they share with the given
// game, so their entry keeps showing the library game.
func (e *Editions) Skip(ctx context.Context, onlyThis bool) error {
	changed, staying := e.scope(onlyThis)
	if err := e.handOverSharedLinks(ctx, staying); err != nil {
		return err
	}
	if _, err := e.tx.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id IN (?)`, bun.List(changed),
	).Exec(ctx); err != nil {
		return fmt.Errorf("skip: %w", err)
	}
	return nil
}

// Unskip clears the skip on the changed games and, when the set is matched,
// writes them to the library. It returns the match it wrote, nil for an
// unmatched set: that one shows as unmatched until the user or the next sync
// matches it.
func (e *Editions) Unskip(ctx context.Context, onlyThis bool) (*int32, error) {
	changed := e.Changed(onlyThis)
	if _, err := e.tx.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id IN (?)`, bun.List(changed),
	).Exec(ctx); err != nil {
		return nil, fmt.Errorf("unskip: %w", err)
	}
	// Editions share one match, so the set is either matched or not.
	if e.game.ResolvedIGDBID == nil {
		return nil, nil
	}
	if _, err := e.write(ctx, changed); err != nil {
		return nil, err
	}
	return e.game.ResolvedIGDBID, nil
}

// handOverSharedLinks writes the editions that stay to the library again
// before the given game leaves its set, when the set is matched and not
// skipped and the game holds a link: a staying edition on the same platform
// and storefront then takes over the link row the game holds (the write sets
// its external_game_id), so unlinking the game does not delete it and the
// staying library game keeps it (#1452). A staying edition without platforms
// takes nothing.
func (e *Editions) handOverSharedLinks(ctx context.Context, staying []string) error {
	if len(staying) == 0 || e.game.ResolvedIGDBID == nil || e.game.IsSkipped {
		return nil
	}
	var holds bool
	if err := e.tx.NewRaw(
		`SELECT EXISTS (SELECT 1 FROM user_game_platforms WHERE external_game_id = ?)`, e.game.ID,
	).Scan(ctx, &holds); err != nil {
		return fmt.Errorf("find the game's links: %w", err)
	}
	if !holds {
		return nil
	}
	_, err := e.write(ctx, staying)
	return err
}

// unlink removes the platform links of the games ids and applies the orphan
// rule to every user game their links sit on: one left with no links from
// outside ids is orphaned, unless it is the user game of the new match
// (rematching onto it rewrites its links). Without orphanAction an orphan
// refuses with ErrOrphanActionRequired before anything is unlinked.
func (e *Editions) unlink(ctx context.Context, ids []string, newIGDBID int, orphanAction string) error {
	var ugs []struct {
		UserGameID string `bun:"user_game_id"`
		Others     int    `bun:"others"`
		IsNewMatch bool   `bun:"is_new_match"`
	}
	if err := e.tx.NewRaw(`
		SELECT ug.id AS user_game_id,
		       (SELECT COUNT(*) FROM user_game_platforms o
		        WHERE o.user_game_id = ug.id
		          AND (o.external_game_id IS NULL OR o.external_game_id NOT IN (?))) AS others,
		       ug.game_id = ? AS is_new_match
		FROM user_games ug
		WHERE ug.user_id = ?
		  AND ug.id IN (SELECT user_game_id FROM user_game_platforms WHERE external_game_id IN (?))
		ORDER BY ug.id`,
		bun.List(ids), newIGDBID, e.userID, bun.List(ids),
	).Scan(ctx, &ugs); err != nil {
		return fmt.Errorf("load linked user games: %w", err)
	}
	var orphaned []string
	for _, ug := range ugs {
		if ug.Others == 0 && !ug.IsNewMatch {
			orphaned = append(orphaned, ug.UserGameID)
		}
	}
	if len(orphaned) > 0 && orphanAction == "" {
		return ErrOrphanActionRequired
	}
	for _, id := range ids {
		if err := usergame.UnlinkExternalGame(ctx, e.tx, e.userID, id); err != nil {
			return err
		}
	}
	if orphanAction == OrphanRemove {
		for _, ugID := range orphaned {
			if err := usergame.Delete(ctx, e.tx, usergame.DeleteParams{UserID: e.userID, UserGameID: ugID}); err != nil {
				return fmt.Errorf("remove orphaned user game: %w", err)
			}
		}
	}
	return nil
}

// write writes each of the games ids to the library and returns the writes,
// leaving out a game with no platform rows (WriteMatched).
func (e *Editions) write(ctx context.Context, ids []string) (map[string]LibraryWrite, error) {
	writes := make(map[string]LibraryWrite, len(ids))
	for _, id := range ids {
		lw, wrote, err := WriteMatched(ctx, e.tx, e.userID, id)
		if err != nil {
			return nil, err
		}
		if wrote {
			writes[id] = lw
		}
	}
	return writes, nil
}

// WriteMatched writes the matched External game id to the library, as
// Rematch, Unskip and the match worker's completion of same-title reviews do.
// A game with no platform rows has nothing to write: that is logged and
// reported as wrote=false without an error, since its match stands and its
// item can still complete (without diff rows).
func WriteMatched(ctx context.Context, db bun.IDB, userID, id string) (lw LibraryWrite, wrote bool, err error) {
	lw, err = WriteToLibrary(ctx, db, userID, id)
	if errors.Is(err, ErrNoPlatforms) {
		slog.WarnContext(ctx, "sync: external game has no platform rows; match recorded, nothing written to the library",
			logging.KeyExternalGameID, id, logging.Cat(logging.CategoryValidation))
		return lw, false, nil
	}
	if err != nil {
		return lw, false, fmt.Errorf("write %s to the library: %w", id, err)
	}
	return lw, true, nil
}
