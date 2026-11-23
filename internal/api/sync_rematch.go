package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/usergame"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Rematch and unskip are edits to one External game and its Child games, not
// sync work (#1338, ADR 0002): they write the library in the request's own
// transaction with the same library write the sync worker uses, and create no
// Job, no item and no River job.

// errSyncStillWorking is the 409 while a sync worker may be writing the game.
const errSyncStillWorking = "a sync is still working on this game"

// lockFamily locks an External game FOR UPDATE, then its Child games ordered
// by id: the family lock order #1339's promotion uses. A Child game is refused
// with 400; verb names the action in that message.
func lockFamily(ctx context.Context, tx bun.Tx, userID, id, verb string) (models.ExternalGame, []models.ExternalGame, error) {
	var eg models.ExternalGame
	err := tx.NewSelect().Model(&eg).
		Where("id = ? AND user_id = ?", id, userID).
		For("UPDATE").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return eg, nil, echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return eg, nil, fmt.Errorf("lock external game: %w", err)
	}
	if eg.ParentID != nil {
		return eg, nil, echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("this game is an edition of another game in the storefront library; %s that game instead", verb))
	}
	var children []models.ExternalGame
	if err := tx.NewSelect().Model(&children).
		Where("parent_id = ?", eg.ID).
		Order("id").
		For("UPDATE").
		Scan(ctx); err != nil {
		return eg, nil, fmt.Errorf("lock child external games: %w", err)
	}
	return eg, children, nil
}

func familyIDs(eg models.ExternalGame, children []models.ExternalGame) []string {
	ids := make([]string, 0, len(children)+1)
	ids = append(ids, eg.ID)
	for _, ch := range children {
		ids = append(ids, ch.ID)
	}
	return ids
}

// rematchUnlinkFamily removes the platform links of the game and its Child
// games and applies the orphan rule to every user game the family's links sit
// on: one left with no links from outside the family is orphaned, unless it is
// the user game of the new match (rematching onto it rewrites its links).
// Without orphanAction an orphan refuses the rematch with 409.
func rematchUnlinkFamily(ctx context.Context, tx bun.Tx, userID string, ids []string, newIGDBID int, orphanAction string) error {
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

// writeFamily writes each family member to the library. A member with no
// platform rows cannot be written; it is logged and left out of the result.
func writeFamily(ctx context.Context, tx bun.Tx, userID string, ids []string) (map[string]tasks.LibraryWrite, error) {
	writes := make(map[string]tasks.LibraryWrite, len(ids))
	for _, id := range ids {
		lw, err := tasks.WriteExternalGameToLibrary(ctx, tx, userID, id)
		if errors.Is(err, tasks.ErrExternalGameNoPlatforms) {
			slog.WarnContext(ctx, "sync: external game has no platform rows; match recorded, nothing written to the library",
				logging.KeyExternalGameID, id)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("write %s to the library: %w", id, err)
		}
		writes[id] = lw
	}
	return writes, nil
}

// HandleRematchExternalGame handles POST /api/sync/external-games/:id/rematch:
// it re-resolves a top-level external game and its Child games to a new IGDB
// id and writes them to the library, all in one transaction (#1338).
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

	// A plain read, no lock: the active Job in which the game or a child has
	// an unfinished item, preferring the game's own. Then everything runs in
	// the module's transaction for that Job: the Job row lock first, then the
	// family, as a sync worker does, so the two cannot deadlock.
	var jobID string
	if err := h.db.NewRaw(`
		SELECT ji.job_id FROM job_items ji
		JOIN jobs j ON j.id = ji.job_id
		JOIN external_games eg ON eg.id = ji.external_game_id
		WHERE (eg.id = ? OR eg.parent_id = ?) AND eg.user_id = ?
		  AND ji.status IN ('pending', 'pending_review')
		  AND j.status IN ('pending', 'processing')
		ORDER BY (eg.id = ?) DESC, j.created_at DESC
		LIMIT 1`,
		id, id, userID, id,
	).Scan(ctx, &jobID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.ErrorContext(reqCtx, "sync: rematch: find items", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rematch external game")
	}

	var err error
	if jobID != "" {
		_, err = h.lc.CompleteByRematch(ctx, jobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
			return rematchFamily(ctx, tx, userID, id, body.IGDBID, body.OrphanAction, job.ID)
		})
		if errors.Is(err, joblifecycle.ErrJobNotActive) || errors.Is(err, joblifecycle.ErrGone) {
			jobID, err = "", nil // finished meanwhile: no items to touch
		}
	}
	if jobID == "" && err == nil {
		err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			_, err := rematchFamily(ctx, tx, userID, id, body.IGDBID, body.OrphanAction, "")
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

// rematchFamily re-resolves a top-level game and its Child games to igdbID
// and writes them to the library. With jobID set it runs in the module's
// transaction for that Job and returns the items to complete there, with
// their diff rows written: the game's item when it is in Pending review, and
// every unfinished item of its children, waiting or not. It refuses (409)
// while the game's own item is pending: the sync has not finished with it.
func rematchFamily(ctx context.Context, tx bun.Tx, userID, id string, igdbID int, orphanAction, jobID string) ([]string, error) {
	eg, children, err := lockFamily(ctx, tx, userID, id, "rematch")
	if err != nil {
		return nil, abortHTTP(err)
	}
	ids := familyIDs(eg, children)

	var items []struct {
		ID             string `bun:"id"`
		ExternalGameID string `bun:"external_game_id"`
		Status         string `bun:"status"`
	}
	if jobID != "" {
		if err := tx.NewRaw(`
			SELECT id, external_game_id, status FROM job_items
			WHERE job_id = ? AND external_game_id IN (?) AND status IN ('pending', 'pending_review')
			ORDER BY id`,
			jobID, bun.List(ids),
		).Scan(ctx, &items); err != nil {
			return nil, fmt.Errorf("load family items: %w", err)
		}
	}
	for _, it := range items {
		if it.ExternalGameID == eg.ID && it.Status == models.JobItemStatusPending {
			return nil, abortHTTP(echo.NewHTTPError(http.StatusConflict, errSyncStillWorking))
		}
	}

	if err := tasks.EnsureGameRow(ctx, tx, int32(igdbID), eg.Title); err != nil { //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		return nil, err
	}
	if err := rematchUnlinkFamily(ctx, tx, userID, ids, igdbID, orphanAction); err != nil {
		return nil, abortHTTP(err)
	}
	// The whole family takes the match and is unskipped: a child's skip
	// equals its parent's (#1339), and a rematch unskips the parent.
	if _, err := tx.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id IN (?)`,
		igdbID, bun.List(ids),
	).Exec(ctx); err != nil {
		return nil, fmt.Errorf("set match: %w", err)
	}
	writes, err := writeFamily(ctx, tx, userID, ids)
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
// the skip on a top-level game and its Child games; a matched game is written
// to the library at once, with its children on its match. An unmatched game is
// only unskipped: it shows as unmatched until the user matches it or the next
// sync does (#1338). All of it is one transaction.
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
		eg, children, err := lockFamily(ctx, tx, userID, id, "unskip")
		if err != nil {
			return err
		}
		ids := familyIDs(eg, children)
		if _, err := tx.NewRaw(
			`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id IN (?)`, bun.List(ids),
		).Exec(ctx); err != nil {
			return fmt.Errorf("unskip: %w", err)
		}
		if eg.ResolvedIGDBID == nil {
			return nil
		}
		// A child skipped with its parent may carry an older match, and one
		// created while the parent was skipped has none: it takes the parent's,
		// leaving its old user game (unskip has no orphan_action; it is kept).
		for _, ch := range children {
			if ch.ResolvedIGDBID != nil && *ch.ResolvedIGDBID == *eg.ResolvedIGDBID {
				continue
			}
			if err := usergame.UnlinkExternalGame(ctx, tx, userID, ch.ID); err != nil {
				return err
			}
			if _, err := tx.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ? WHERE id = ?`, *eg.ResolvedIGDBID, ch.ID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("re-point child: %w", err)
			}
		}
		if _, err := writeFamily(ctx, tx, userID, ids); err != nil {
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
