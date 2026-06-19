package usergame

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	nexdb "github.com/drzero42/nexorious/internal/db"
)

// UpdatePlatformParams is the parameter struct for UpdatePlatform.
type UpdatePlatformParams struct {
	UserID     string
	UserGameID string
	PlatformID string
	Fields     PlatformInput
}

// RemovePlatformParams is the parameter struct for RemovePlatform.
type RemovePlatformParams struct {
	UserID     string
	UserGameID string
	PlatformID string
}

// BulkRemovePlatformParams is the parameter struct for RemovePlatformBulk.
type BulkRemovePlatformParams struct {
	UserID      string
	UserGameIDs []string
	Platform    string
	Storefront  string
}

// UpdatePlatform verifies ownership, applies a PATCH-style partial update to
// the named platform row, then runs promoteToInProgressIfPlayed (hours may
// have changed). RowsAffected==0 → ErrNotFound.
func UpdatePlatform(ctx context.Context, db *bun.DB, p UpdatePlatformParams) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertOwned(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}

		// Build a dynamic SET clause from the non-nil fields.
		q := tx.NewUpdate().TableExpr("user_game_platforms").
			Where("id = ?", p.PlatformID).
			Where("user_game_id = ?", p.UserGameID).
			Set("updated_at = now()")

		if p.Fields.Platform != nil {
			q = q.Set("platform = ?", *p.Fields.Platform)
		}
		if p.Fields.Storefront != nil {
			q = q.Set("storefront = ?", *p.Fields.Storefront)
		}
		if p.Fields.IsAvailable != nil {
			q = q.Set("is_available = ?", *p.Fields.IsAvailable)
		}
		if p.Fields.HoursPlayed != nil {
			q = q.Set("hours_played = ?", *p.Fields.HoursPlayed)
		}
		if p.Fields.OwnershipStatus != nil {
			q = q.Set("ownership_status = ?", *p.Fields.OwnershipStatus)
		}
		if p.Fields.ClearAcquiredDate {
			q = q.Set("acquired_date = NULL")
		} else if p.Fields.AcquiredDate != nil {
			q = q.Set("acquired_date = ?", *p.Fields.AcquiredDate)
		}
		if p.Fields.ExternalGameID != nil {
			q = q.Set("external_game_id = ?", *p.Fields.ExternalGameID)
		}
		// sync_from_source is intentionally never updated here.

		result, err := q.Exec(ctx)
		if err != nil {
			if nexdb.IsUniqueViolation(err) {
				return fmt.Errorf("platform/storefront combination already exists: %w", ErrConflict)
			}
			return fmt.Errorf("update platform: %w", err)
		}
		rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pgdriver; count is advisory
		if rows == 0 {
			return ErrNotFound
		}

		return promoteToInProgressIfPlayed(ctx, tx, p.UserGameID)
	})
}

// RemovePlatform verifies ownership and deletes the named platform row.
// RowsAffected==0 → ErrNotFound.
func RemovePlatform(ctx context.Context, db *bun.DB, p RemovePlatformParams) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertOwned(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}

		result, err := tx.NewDelete().TableExpr("user_game_platforms").
			Where("id = ?", p.PlatformID).
			Where("user_game_id = ?", p.UserGameID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete platform: %w", err)
		}
		rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pgdriver; count is advisory
		if rows == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// RemovePlatformBulk deletes the named platform/storefront combination from
// all owned user_games in one transaction and returns the number of rows
// deleted.
func RemovePlatformBulk(ctx context.Context, db *bun.DB, p BulkRemovePlatformParams) (int, error) {
	var removed int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewRaw(
			`DELETE FROM user_game_platforms
			 WHERE user_game_id IN (
			   SELECT id FROM user_games WHERE id IN (?) AND user_id = ?
			 )
			 AND platform = ? AND storefront = ?`,
			bun.List(p.UserGameIDs), p.UserID, p.Platform, p.Storefront,
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("bulk delete platforms: %w", err)
		}
		rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pgdriver; count is advisory
		removed = int(rows)
		return nil
	})
	return removed, err
}
