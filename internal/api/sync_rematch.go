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

// liveRiverJobPredicate matches the River job states in which a job can
// still run.
const liveRiverJobPredicate = `state NOT IN ('completed', 'cancelled', 'discarded')`

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

// familyItem is an unfinished item of a family member.
type familyItem struct {
	ID             string `bun:"id"`
	JobID          string `bun:"job_id"`
	ExternalGameID string `bun:"external_game_id"`
	Status         string `bun:"status"`
	// Worked: its Job is active, or a River job naming it can still run.
	Worked bool `bun:"worked"`
}

func loadFamilyItems(ctx context.Context, tx bun.Tx, ids []string) ([]familyItem, error) {
	var items []familyItem
	if err := tx.NewRaw(`
		SELECT ji.id, ji.job_id, ji.external_game_id, ji.status,
		       (j.status IN ('pending', 'processing') OR EXISTS (
		          SELECT 1 FROM river_job rj
		          WHERE rj.args->>'job_item_id' = ji.id AND rj.`+liveRiverJobPredicate+`)) AS worked
		FROM job_items ji
		JOIN jobs j ON j.id = ji.job_id
		WHERE ji.external_game_id IN (?)
		  AND ji.status IN ('pending', 'processing', 'pending_review')
		ORDER BY ji.id`,
		bun.List(ids),
	).Scan(ctx, &items); err != nil {
		return nil, fmt.Errorf("load family items: %w", err)
	}
	return items, nil
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

// completeWaitingItems completes the given items in active Jobs and writes
// their diff rows, as the sync worker would have. It returns the Jobs whose
// completion must be checked after commit. Items in a Job that is not active
// are left alone.
func completeWaitingItems(ctx context.Context, tx bun.Tx, userID string, items []familyItem, writes map[string]tasks.LibraryWrite) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	jobSet := map[string]bool{}
	var jobIDs []string
	for _, it := range items {
		if !jobSet[it.JobID] {
			jobSet[it.JobID] = true
			jobIDs = append(jobIDs, it.JobID)
		}
	}
	var jobs []struct {
		ID     string `bun:"id"`
		Active bool   `bun:"active"`
	}
	if err := tx.NewRaw(`
		SELECT id, status IN ('pending', 'processing') AS active
		FROM jobs WHERE id IN (?) ORDER BY id FOR UPDATE`, bun.List(jobIDs),
	).Scan(ctx, &jobs); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lock jobs: %w", err)
	}
	var active []string
	for _, j := range jobs {
		if j.Active {
			active = append(active, j.ID)
		}
	}
	isActive := map[string]bool{}
	for _, id := range active {
		isActive[id] = true
	}

	created := false
	for _, lw := range writes {
		if lw.Result.Created {
			created = true
		}
	}
	addedIn := map[string]bool{}
	for _, it := range items {
		if !isActive[it.JobID] {
			continue
		}
		// Conditional on the status read: a Skip may have got there first.
		res, err := tx.NewRaw(
			`UPDATE job_items SET status = 'completed', processed_at = now() WHERE id = ? AND status = ?`,
			it.ID, it.Status,
		).Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("complete item: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // pgdriver always reports RowsAffected
			continue
		}
		lw, ok := writes[it.ExternalGameID]
		if !ok {
			continue // not written (no platform rows): no diff row
		}
		added := created && !addedIn[it.JobID]
		if added {
			addedIn[it.JobID] = true
		}
		if err := tasks.InsertLibraryWriteChanges(ctx, tx, it.JobID, userID, lw, added); err != nil {
			return nil, err
		}
	}
	return active, nil
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

	var jobsToCheck []string
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		eg, children, err := lockFamily(ctx, tx, userID, id, "rematch")
		if err != nil {
			return err
		}
		ids := familyIDs(eg, children)

		// Refuse while a worker may be writing a family member, and note the
		// items this rematch settles: every item in Pending review, and a
		// waiting child's pending item. Both are decided now, before the match
		// is set: afterwards no family member is unmatched any more.
		unmatchedChild := map[string]bool{}
		if eg.ResolvedIGDBID == nil {
			for _, ch := range children {
				if ch.ResolvedIGDBID == nil {
					unmatchedChild[ch.ID] = true
				}
			}
		}
		items, err := loadFamilyItems(ctx, tx, ids)
		if err != nil {
			return err
		}
		var waiting []familyItem
		for _, it := range items {
			switch {
			case it.Status == models.JobItemStatusPendingReview:
				waiting = append(waiting, it)
			case !it.Worked:
				// Unfinished in a finished Job with nothing queued: left alone.
			case it.Status == models.JobItemStatusPending && unmatchedChild[it.ExternalGameID]:
				// No worker can write a child of an unmatched parent; it is
				// waiting for exactly this rematch.
				waiting = append(waiting, it)
			default:
				return echo.NewHTTPError(http.StatusConflict, errSyncStillWorking)
			}
		}

		if err := tasks.EnsureGameRow(ctx, tx, int32(body.IGDBID), eg.Title); err != nil { //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
			return err
		}
		if err := rematchUnlinkFamily(ctx, tx, userID, ids, body.IGDBID, body.OrphanAction); err != nil {
			return err
		}
		// The whole family takes the match and is unskipped: a child's skip
		// equals its parent's (#1339), and a rematch unskips the parent.
		if _, err := tx.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id IN (?)`,
			body.IGDBID, bun.List(ids),
		).Exec(ctx); err != nil {
			return fmt.Errorf("set match: %w", err)
		}
		writes, err := writeFamily(ctx, tx, userID, ids)
		if err != nil {
			return err
		}
		jobsToCheck, err = completeWaitingItems(ctx, tx, userID, waiting, writes)
		return err
	})
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		slog.ErrorContext(reqCtx, "sync: rematch failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rematch external game")
	}

	for _, jobID := range jobsToCheck {
		tasks.SyncCheckJobCompletion(ctx, h.db, h.riverClient, jobID)
	}
	tasks.EnqueueImmediateMetadataFetch(ctx, h.db, h.riverClient, int32(body.IGDBID)) //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
	return c.NoContent(http.StatusNoContent)
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
