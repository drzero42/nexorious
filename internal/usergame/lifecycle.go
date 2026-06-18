package usergame

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// DeleteParams is the parameter struct for Delete.
type DeleteParams struct {
	UserID     string
	UserGameID string
}

// Delete removes a single user_game row scoped to the caller's user_id.
// Returns ErrNotFound when the row does not exist or belongs to another user.
// Cascades handle user_game_platforms, user_game_tags, and pool_games.
func Delete(ctx context.Context, db *bun.DB, p DeleteParams) error {
	res, err := db.NewDelete().
		Model((*models.UserGame)(nil)).
		Where("user_game.id = ?", p.UserGameID).
		Where("user_game.user_id = ?", p.UserID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete user_game: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkDeleteParams is the parameter struct for DeleteBulk.
type BulkDeleteParams struct {
	UserID      string
	UserGameIDs []string
}

// DeleteBulk removes each user_game in UserGameIDs that is owned by UserID,
// silently skipping any ids that belong to other users or do not exist.
// Returns the number of rows actually deleted.
func DeleteBulk(ctx context.Context, db *bun.DB, p BulkDeleteParams) (int, error) {
	var deleted int64
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewDelete().
			Model((*models.UserGame)(nil)).
			Where("id IN (?)", bun.List(p.UserGameIDs)).
			Where("user_game.user_id = ?", p.UserID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("bulk delete user_games: %w", err)
		}
		deleted, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
}

// ClearLibrary deletes all user_games for the given user, along with their
// active River jobs and job records. Mirrors HandleClearLibrary exactly:
// cancels pending River job args referencing this user's job_items, deletes
// jobs (cascades job_items + changes), then deletes all user_games (cascades
// user_game_platforms + user_game_tags). Returns the number of user_game rows
// deleted.
func ClearLibrary(ctx context.Context, db *bun.DB, userID string) (int, error) {
	var deleted int64
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Cancel active River jobs whose items belong to this user.
		if _, err := tx.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = NOW()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE user_id = ?)`,
			userID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("cancel river jobs: %w", err)
		}
		// Delete jobs (cascades job_items + changes).
		if _, err := tx.NewDelete().Model((*models.Job)(nil)).
			Where("user_id = ?", userID).Exec(ctx); err != nil {
			return fmt.Errorf("delete jobs: %w", err)
		}
		// Delete user games (cascades user_game_platforms + user_game_tags).
		res, err := tx.NewDelete().Model((*models.UserGame)(nil)).
			Where("user_id = ?", userID).Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete user_games: %w", err)
		}
		deleted, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
}
