package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/usergame"
)

// This file keeps a Child game's sync from waiting on a parent that will never
// be worked (#1339). A Child game (external_games.parent_id) is another edition
// of a game with the same title in the same storefront; the storefront list
// shows only parents, and a child's sync item waits for its parent's match.
//
// Family lock order, shared with rematch: the parent row first, then its
// children ordered by id.

// errChildRepointed is returned to River when a child was re-pointed to a new
// parent (promotion) while its igdb_match was deciding whether to wait; the
// retry decides again against the new parent.
var errChildRepointed = errors.New("igdb_match: child was re-pointed to another parent; retrying")

// childUnmatchedParentMsg is the item error for a child whose parent is
// unmatched and has nothing in this sync that could match it.
const childUnmatchedParentMsg = "parent game is unmatched and is not part of this sync"

// repairChildSkips gives every child of the user's storefront its parent's
// is_skipped, in both directions. A skip or unskip only cascades to the
// children that exist at that moment, and the unskip cascade can fail, so a
// family can drift apart.
//
// A child that this unskips and whose match differs from its parent's carries a
// stale match (rematch does not update skipped children). Its platform links
// are removed (the user game they were on is kept) and it takes the parent's
// match. A child whose match already equals the parent's keeps its links.
//
// One transaction: a child unskipped without its unlink would end up linked to
// both its old and its new library game.
func repairChildSkips(ctx context.Context, db *bun.DB, userID, storefront string) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Parent rows first (family lock order).
		var parentIDs []string
		if err := tx.NewRaw(`
			SELECT p.id FROM external_games p
			WHERE p.user_id = ? AND p.storefront = ? AND p.parent_id IS NULL
			  AND EXISTS (
			    SELECT 1 FROM external_games c
			    WHERE c.parent_id = p.id AND c.is_skipped <> p.is_skipped)
			ORDER BY p.id
			FOR SHARE`,
			userID, storefront,
		).Scan(ctx, &parentIDs); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lock parents: %w", err)
		}
		if len(parentIDs) == 0 {
			return nil
		}

		var stale []string
		if err := tx.NewRaw(`
			SELECT c.id FROM external_games c
			JOIN external_games p ON p.id = c.parent_id
			WHERE p.id IN (?)
			  AND c.is_skipped AND NOT p.is_skipped
			  AND c.resolved_igdb_id IS DISTINCT FROM p.resolved_igdb_id
			ORDER BY c.id`,
			bun.List(parentIDs),
		).Scan(ctx, &stale); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("find stale children: %w", err)
		}
		for _, childID := range stale {
			if err := usergame.UnlinkExternalGame(ctx, tx, userID, childID); err != nil {
				return err
			}
		}

		if _, err := tx.NewRaw(`
			UPDATE external_games c
			SET is_skipped = p.is_skipped,
			    resolved_igdb_id = CASE WHEN c.is_skipped AND NOT p.is_skipped
			                            THEN p.resolved_igdb_id ELSE c.resolved_igdb_id END,
			    updated_at = now()
			FROM external_games p
			WHERE p.id = c.parent_id
			  AND p.id IN (?)
			  AND c.is_skipped <> p.is_skipped`,
			bun.List(parentIDs),
		).Exec(ctx); err != nil {
			return fmt.Errorf("align child skips: %w", err)
		}
		return nil
	})
}

// promoteOrphanedChildren handles parents of the user's storefront that are
// unavailable, unmatched and not skipped, and that have an available child.
// Such a parent will never be matched by a sync, so its children would wait
// forever. The oldest available child becomes the parent, and the other
// children point to it. Errors are logged; the next dispatch tries again.
func promoteOrphanedChildren(ctx context.Context, db *bun.DB, userID, storefront string) {
	var parentIDs []string
	if err := db.NewRaw(`
		SELECT p.id FROM external_games p
		WHERE p.user_id = ? AND p.storefront = ? AND p.parent_id IS NULL
		  AND NOT p.is_available AND p.resolved_igdb_id IS NULL AND NOT p.is_skipped
		  AND EXISTS (SELECT 1 FROM external_games c WHERE c.parent_id = p.id AND c.is_available)
		ORDER BY p.id`,
		userID, storefront,
	).Scan(ctx, &parentIDs); err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.WarnContext(ctx, "dispatch_sync: find parents to promote from failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	for _, parentID := range parentIDs {
		promotedID, err := promoteChild(ctx, db, parentID)
		if err != nil {
			slog.WarnContext(ctx, "dispatch_sync: promote child failed", logging.KeyErr, err, "parent_id", parentID, logging.Cat(logging.CategoryDB))
			continue
		}
		if promotedID != "" {
			slog.InfoContext(ctx, "dispatch_sync: promoted child of a departed unmatched parent",
				"parent_id", parentID, logging.KeyExternalGameID, promotedID)
		}
	}
}

// promoteChild promotes the oldest available child of parentID, re-checking
// the conditions under the family lock (a rematch or skip may have committed
// since the candidates were read). It returns the promoted game's id, or ""
// when nothing was promoted.
func promoteChild(ctx context.Context, db *bun.DB, parentID string) (string, error) {
	var promotedID string
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var eligible bool
		if err := tx.NewRaw(`
			SELECT parent_id IS NULL AND NOT is_available AND resolved_igdb_id IS NULL AND NOT is_skipped
			FROM external_games WHERE id = ?
			FOR UPDATE`,
			parentID,
		).Scan(ctx, &eligible); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("lock parent: %w", err)
		}
		if !eligible {
			return nil
		}

		var children []familyChild
		if err := tx.NewRaw(`
			SELECT id, is_available, created_at FROM external_games
			WHERE parent_id = ?
			ORDER BY id
			FOR UPDATE`,
			parentID,
		).Scan(ctx, &children); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lock children: %w", err)
		}
		promotedID = oldestAvailable(children)
		if promotedID == "" {
			return nil
		}

		if _, err := tx.NewRaw(
			`UPDATE external_games SET parent_id = NULL, updated_at = now() WHERE id = ?`, promotedID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("clear promoted parent_id: %w", err)
		}
		if _, err := tx.NewRaw(
			`UPDATE external_games SET parent_id = ?, updated_at = now() WHERE parent_id = ?`, promotedID, parentID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("re-point siblings: %w", err)
		}
		// Unfinished items left in finished Jobs (a sync the user cancelled to
		// escape the hang) would hide the promoted game from the storefront list,
		// or keep the old parent showing as needing review.
		if _, err := tx.NewRaw(`
			UPDATE job_items ji SET status = 'cancelled', processed_at = now()
			FROM jobs j
			WHERE j.id = ji.job_id
			  AND j.status NOT IN ('pending', 'processing')
			  AND ji.status IN ('pending', 'pending_review')
			  AND ji.external_game_id IN (?, ?)`,
			promotedID, parentID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("cancel leftover items: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return promotedID, nil
}

// familyChild is a child row as promotion sees it.
type familyChild struct {
	ID          string    `bun:"id"`
	IsAvailable bool      `bun:"is_available"`
	CreatedAt   time.Time `bun:"created_at"`
}

// oldestAvailable returns the id of the oldest available child (created_at,
// then id), or "" when none is available.
func oldestAvailable(children []familyChild) string {
	var best *familyChild
	for i := range children {
		c := &children[i]
		if !c.IsAvailable {
			continue
		}
		if best == nil || c.CreatedAt.Before(best.CreatedAt) ||
			(c.CreatedAt.Equal(best.CreatedAt) && c.ID < best.ID) {
			best = c
		}
	}
	if best == nil {
		return ""
	}
	return best.ID
}

// childWaitAction is what igdb_match does with a child whose parent was
// unmatched when it looked.
type childWaitAction int

const (
	// childProceed: the facts changed (the game is skipped or matched, it is no
	// longer a child, or its parent is matched now); carry on with fresh state.
	childProceed childWaitAction = iota
	// childWait: leave the item pending; the parent's progress wakes it.
	childWait
	// childFailed: the item was failed inside the decision's transaction.
	childFailed
)

// childWaitResult carries the decision and the state it was taken on.
type childWaitResult struct {
	action childWaitAction
	// Fresh state of the game, valid for childProceed.
	isSkipped      bool
	resolvedIGDBID *int32
	parentID       *string
	// The locked parent's match, valid for childProceed when parentID is set.
	parentIGDBID *int32
}

// decideChildWait decides whether a child may wait for its unmatched parent,
// in one short transaction on facts that cannot change underneath it. A child
// waits only when the wait can end: its Job is active, the parent is top-level,
// and the parent has an unfinished item in the Job or the dispatch is still
// running (it may add one). Otherwise the item is failed.
//
// It runs on the worker's own ctx: a worker cut off by a shutdown must not
// commit a failure. Database errors are returned for River to retry.
func decideChildWait(ctx context.Context, db *bun.DB, itemID, jobID, gameID, parentID string) (childWaitResult, error) {
	var res childWaitResult
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res = childWaitResult{}
		// Parent first: rematch and promotion lock it first too, so this waits
		// for either to finish.
		var parent struct {
			ParentID       *string `bun:"parent_id"`
			ResolvedIGDBID *int32  `bun:"resolved_igdb_id"`
		}
		err := tx.NewRaw(
			`SELECT parent_id, resolved_igdb_id FROM external_games WHERE id = ? FOR SHARE`, parentID,
		).Scan(ctx, &parent)
		if errors.Is(err, sql.ErrNoRows) {
			return failChildItem(ctx, tx, itemID, "external game not found", &res)
		}
		if err != nil {
			return fmt.Errorf("lock parent: %w", err)
		}

		var game struct {
			ParentID       *string `bun:"parent_id"`
			ResolvedIGDBID *int32  `bun:"resolved_igdb_id"`
			IsSkipped      bool    `bun:"is_skipped"`
		}
		err = tx.NewRaw(
			`SELECT parent_id, resolved_igdb_id, is_skipped FROM external_games WHERE id = ?`, gameID,
		).Scan(ctx, &game)
		if errors.Is(err, sql.ErrNoRows) {
			return failChildItem(ctx, tx, itemID, "external game not found", &res)
		}
		if err != nil {
			return fmt.Errorf("reload game: %w", err)
		}
		if game.ParentID != nil && *game.ParentID != parentID {
			return errChildRepointed
		}

		res.isSkipped = game.IsSkipped
		res.resolvedIGDBID = game.ResolvedIGDBID
		res.parentID = game.ParentID
		res.parentIGDBID = parent.ResolvedIGDBID
		if game.IsSkipped || game.ResolvedIGDBID != nil || game.ParentID == nil || parent.ResolvedIGDBID != nil {
			res.action = childProceed
			return nil
		}

		var facts struct {
			JobActive       bool `bun:"job_active"`
			ParentHasItem   bool `bun:"parent_has_item"`
			DispatchRunning bool `bun:"dispatch_running"`
		}
		if err := tx.NewRaw(`
			SELECT
			  j.status IN ('pending', 'processing') AS job_active,
			  EXISTS (
			    SELECT 1 FROM job_items pi
			    WHERE pi.job_id = j.id AND pi.external_game_id = ?
			      AND pi.status IN ('pending', 'processing', 'pending_review')) AS parent_has_item,
			  (NOT j.dispatch_complete AND EXISTS (
			    SELECT 1 FROM river_job rj
			    WHERE rj.kind = ? AND rj.args->>'job_id' = j.id::text
			      AND rj.state NOT IN ('completed', 'cancelled', 'discarded'))) AS dispatch_running
			FROM jobs j WHERE j.id = ?`,
			parentID, DispatchSyncArgs{}.Kind(), jobID,
		).Scan(ctx, &facts); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return failChildItem(ctx, tx, itemID, childUnmatchedParentMsg, &res)
			}
			return fmt.Errorf("load job facts: %w", err)
		}

		if facts.JobActive && parent.ParentID == nil && (facts.ParentHasItem || facts.DispatchRunning) {
			res.action = childWait
			return nil
		}
		return failChildItem(ctx, tx, itemID, childUnmatchedParentMsg, &res)
	})
	return res, err
}

func failChildItem(ctx context.Context, tx bun.Tx, itemID, msg string, res *childWaitResult) error {
	if _, err := tx.NewRaw(
		`UPDATE job_items SET status = 'failed', error_message = ?, processed_at = now()
		 WHERE id = ? AND status = 'pending'`,
		msg, itemID,
	).Exec(ctx); err != nil {
		return fmt.Errorf("fail child item: %w", err)
	}
	res.action = childFailed
	return nil
}
