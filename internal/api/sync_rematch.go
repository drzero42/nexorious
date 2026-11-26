package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/usergame"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Rematch and unskip are edits to one External game and its editions, not
// sync work (#1338, ADR 0002): they write the library in the request's own
// transaction with the same library write the sync worker uses, and create no
// Job, no item and no River job. Rematch, Skip and Unskip act on every edition
// of the game they are given, through any edition's id (#1449).

// errSyncStillWorking is the 409 while a sync worker may be writing the game.
const errSyncStillWorking = "a sync is still working on this game"

// lockEditions locks the user's External game id and its editions, in id
// order (externalgame.LockEditions), and answers its refusals as HTTP errors:
// a missing game 404, a set that kept changing 409.
func lockEditions(ctx context.Context, tx bun.Tx, userID, id string) (models.ExternalGame, []models.ExternalGame, error) {
	eg, set, err := externalgame.LockEditions(ctx, tx, userID, id)
	switch {
	case errors.Is(err, externalgame.ErrNotFound):
		return eg, nil, echo.NewHTTPError(http.StatusNotFound, "not found")
	case errors.Is(err, externalgame.ErrEditionsChanged):
		return eg, nil, echo.NewHTTPError(http.StatusConflict, "the game changed while it was being updated; try again")
	case err != nil:
		return eg, nil, err
	}
	return eg, set, nil
}

// rematchUnlinkEditions removes the platform links of a set of editions and
// applies the orphan rule to every user game their links sit on: one left with
// no links from outside the set is orphaned, unless it is the user game of the
// new match (rematching onto it rewrites its links). Without orphanAction an
// orphan refuses the rematch with 409.
func rematchUnlinkEditions(ctx context.Context, tx bun.Tx, userID string, ids []string, newIGDBID int, orphanAction string) error {
	var ugs []struct {
		UserGameID string `bun:"user_game_id"`
		Others     int    `bun:"others"`
		IsNewMatch bool   `bun:"is_new_match"`
	}
	if err := tx.NewRaw(`
		SELECT ug.id AS user_game_id,
		       (SELECT COUNT(*) FROM user_game_platforms o
		        WHERE o.user_game_id = ug.id
		          AND (o.external_game_id IS NULL OR o.external_game_id NOT IN (?))) AS others,
		       ug.game_id = ? AS is_new_match
		FROM user_games ug
		WHERE ug.user_id = ?
		  AND ug.id IN (SELECT user_game_id FROM user_game_platforms WHERE external_game_id IN (?))
		ORDER BY ug.id`,
		bun.List(ids), newIGDBID, userID, bun.List(ids),
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
		return echo.NewHTTPError(http.StatusConflict, "orphan_action required: game would lose its only storefront link")
	}
	for _, id := range ids {
		if err := usergame.UnlinkExternalGame(ctx, tx, userID, id); err != nil {
			return err
		}
	}
	if orphanAction == "remove" {
		for _, ugID := range orphaned {
			if err := usergame.Delete(ctx, tx, usergame.DeleteParams{UserID: userID, UserGameID: ugID}); err != nil {
				return fmt.Errorf("remove orphaned user game: %w", err)
			}
		}
	}
	return nil
}

// writeEditions writes each edition to the library. One with no platform
// rows cannot be written; it is left out of the result
// (tasks.WriteMatchedToLibrary).
func writeEditions(ctx context.Context, tx bun.Tx, userID string, ids []string) (map[string]externalgame.LibraryWrite, error) {
	writes := make(map[string]externalgame.LibraryWrite, len(ids))
	for _, id := range ids {
		lw, wrote, err := tasks.WriteMatchedToLibrary(ctx, tx, userID, id)
		if err != nil {
			return nil, err
		}
		if wrote {
			writes[id] = lw
		}
	}
	return writes, nil
}

// HandleRematchExternalGame handles POST /api/sync/external-games/:id/rematch:
// it re-resolves an External game and its editions to a new IGDB id and writes
// them to the library, all in one transaction (#1338).
func (h *SyncHandler) HandleRematchExternalGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()
	reqCtx := c.Request().Context()

	var body struct {
		IGDBID       int    `json:"igdb_id"`
		OrphanAction string `json:"orphan_action"`
	}
	if err := c.Bind(&body); err != nil || body.IGDBID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "igdb_id is required")
	}

	// A plain read, no lock: the active Job in which the game or one of its
	// editions has an open item, preferring the game's own. Then everything
	// runs in the module's transaction for that Job: the Job row lock first,
	// then the editions, as a sync worker does, so the two cannot deadlock.
	jobIDs, err := externalgame.EditionsOpenItemJobs(ctx, h.db, userID, id)
	if err != nil {
		slog.ErrorContext(reqCtx, "sync: rematch: find items", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rematch external game")
	}
	jobID := ""
	if len(jobIDs) > 0 {
		jobID = jobIDs[0]
	}

	if jobID != "" {
		_, err = h.lc.CompleteByRematch(ctx, jobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
			return rematchEditions(ctx, tx, userID, id, body.IGDBID, body.OrphanAction, job.ID)
		})
		if errors.Is(err, joblifecycle.ErrJobNotActive) || errors.Is(err, joblifecycle.ErrGone) {
			jobID, err = "", nil // ended meanwhile: no items to touch
		}
	}
	if jobID == "" && err == nil {
		err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			_, err := rematchEditions(ctx, tx, userID, id, body.IGDBID, body.OrphanAction, "")
			return err
		})
	}
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		slog.ErrorContext(reqCtx, "sync: rematch failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rematch external game")
	}

	tasks.EnqueueImmediateMetadataFetch(ctx, h.db, h.riverClient, int32(body.IGDBID)) //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
	return c.NoContent(http.StatusNoContent)
}

// abortHTTP marks an HTTP error so a module transaction rolls back without
// logging it as a failed transition; errors.As still finds it.
func abortHTTP(err error) error {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return joblifecycle.Abort(err)
	}
	return err
}

// rematchEditions re-resolves an External game and its editions to igdbID and
// writes them to the library. With jobID set it runs in the module's
// transaction for that Job and returns the items to complete there, with their
// diff rows written: every open item of the set in that Job. It refuses (409)
// while the given game's own item is pending: the sync is not done with it.
func rematchEditions(ctx context.Context, tx bun.Tx, userID, id string, igdbID int, orphanAction, jobID string) ([]string, error) {
	eg, set, err := lockEditions(ctx, tx, userID, id)
	if err != nil {
		return nil, abortHTTP(err)
	}
	ids := externalgame.GameIDs(set)

	var items []externalgame.OpenItem
	if jobID != "" {
		if items, err = externalgame.EditionsOpenItems(ctx, tx, jobID, ids); err != nil {
			return nil, err
		}
	}
	for _, it := range items {
		if it.ExternalGameID == eg.ID && it.Status == models.JobItemStatusPending {
			return nil, abortHTTP(echo.NewHTTPError(http.StatusConflict, errSyncStillWorking))
		}
	}

	if err := externalgame.EnsureGameRow(ctx, tx, int32(igdbID), eg.Title); err != nil { //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		return nil, err
	}
	if err := rematchUnlinkEditions(ctx, tx, userID, ids, igdbID, orphanAction); err != nil {
		return nil, abortHTTP(err)
	}
	// Every edition takes the match and is unskipped.
	if _, err := tx.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id IN (?)`,
		igdbID, bun.List(ids),
	).Exec(ctx); err != nil {
		return nil, fmt.Errorf("set match: %w", err)
	}
	writes, err := writeEditions(ctx, tx, userID, ids)
	if err != nil {
		return nil, err
	}

	// Diff rows for the items this completes, as the sync worker would have
	// written them: 'added' once per Job when any write created the user game.
	created := false
	for _, lw := range writes {
		if lw.Result.Created {
			created = true
		}
	}
	done := make([]string, 0, len(items))
	for _, it := range items {
		done = append(done, it.ID)
		lw, ok := writes[it.ExternalGameID]
		if !ok {
			continue // not written (no platform rows): no diff row
		}
		if err := tasks.InsertLibraryWriteChanges(ctx, tx, jobID, userID, lw, created); err != nil {
			return nil, err
		}
		created = false
	}
	return done, nil
}

// HandleUnskipGame handles POST /api/sync/external-games/:id/unskip. It clears
// the skip on an External game and its editions; matched editions are written
// to the library at once. Unmatched ones are not written, and show as
// unmatched until the user matches them or the next sync does (#1338). All of
// it is one transaction.
func (h *SyncHandler) HandleUnskipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()
	reqCtx := c.Request().Context()

	var wrote *int32
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		eg, set, err := lockEditions(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		ids := externalgame.GameIDs(set)
		if _, err := tx.NewRaw(
			`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id IN (?)`, bun.List(ids),
		).Exec(ctx); err != nil {
			return fmt.Errorf("unskip: %w", err)
		}
		// Editions share one match, so the set is either matched or not.
		if eg.ResolvedIGDBID == nil {
			return nil
		}
		if _, err := writeEditions(ctx, tx, userID, ids); err != nil {
			return err
		}
		wrote = eg.ResolvedIGDBID
		return nil
	})
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		slog.ErrorContext(reqCtx, "sync: unskip failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unskip game")
	}
	if wrote != nil {
		tasks.EnqueueImmediateMetadataFetch(ctx, h.db, h.riverClient, *wrote)
	}
	return c.NoContent(http.StatusNoContent)
}
