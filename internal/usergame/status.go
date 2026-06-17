package usergame

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
)

// UpdateFieldsParams holds the parameters for UpdateFields.
// Non-nil pointer fields are written; nil means "leave unchanged".
// ClearPersonalRating explicitly sets personal_rating to NULL (distinct from
// PersonalRating==nil which means "leave unchanged").
// ClearPersonalNotes explicitly sets personal_notes to NULL (distinct from
// PersonalNotes==nil which means "leave unchanged").
type UpdateFieldsParams struct {
	UserID              string
	UserGameID          string
	PlayStatus          *string
	PersonalNotes       *string
	ClearPersonalNotes  bool
	PersonalRating      *int32
	ClearPersonalRating bool
	IsLoved             *bool
}

// UpdateFields applies a partial update to a user_games row. Only non-nil
// fields are written. If PlayStatus is set, RemoveFromPoolsIfFinished is called
// inside the same transaction. Returns ErrNotFound if the row does not belong
// to the user, ErrValidation if play_status is invalid.
func UpdateFields(ctx context.Context, db *bun.DB, p UpdateFieldsParams) error {
	if p.PlayStatus != nil && !enum.PlayStatus(*p.PlayStatus).Valid() {
		return fmt.Errorf("invalid play_status %q: %w", *p.PlayStatus, ErrValidation)
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertOwned(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}

		setClauses := []string{"updated_at = NOW()"}
		args := []any{}

		if p.PlayStatus != nil {
			setClauses = append(setClauses, "play_status = ?")
			args = append(args, *p.PlayStatus)
		}
		switch {
		case p.ClearPersonalNotes:
			setClauses = append(setClauses, "personal_notes = ?")
			args = append(args, nil)
		case p.PersonalNotes != nil:
			setClauses = append(setClauses, "personal_notes = ?")
			args = append(args, *p.PersonalNotes)
		}
		switch {
		case p.ClearPersonalRating:
			setClauses = append(setClauses, "personal_rating = ?")
			args = append(args, nil)
		case p.PersonalRating != nil:
			setClauses = append(setClauses, "personal_rating = ?")
			args = append(args, *p.PersonalRating)
		}
		if p.IsLoved != nil {
			setClauses = append(setClauses, "is_loved = ?")
			args = append(args, *p.IsLoved)
		}

		if len(args) == 0 {
			// only updated_at — still a valid no-op, but return early
			return nil
		}

		args = append(args, p.UserGameID, p.UserID)
		query := fmt.Sprintf(
			`UPDATE user_games SET %s WHERE id = ? AND user_id = ?`,
			strings.Join(setClauses, ", "),
		)

		res, err := tx.NewRaw(query, args...).Exec(ctx)
		if err != nil {
			return fmt.Errorf("update user_game: %w", err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if rows == 0 {
			return ErrNotFound
		}

		if p.PlayStatus != nil {
			if err := RemoveFromPoolsIfFinished(ctx, tx, p.UserGameID); err != nil {
				return fmt.Errorf("remove from pools: %w", err)
			}
		}
		return nil
	})
}

// ProgressParams holds the parameters for RecordProgress.
type ProgressParams struct {
	UserID     string
	UserGameID string
	PlayStatus string
}

// RecordProgress verifies ownership, sets play_status on the user_games row,
// then calls PromoteToInProgressIfPlayed so any accrued platform hours are
// reflected in the status when transitioning from not_started.
// RowsAffected==0 → ErrNotFound. Returns ErrValidation for an invalid status.
func RecordProgress(ctx context.Context, db *bun.DB, p ProgressParams) error {
	if !enum.PlayStatus(p.PlayStatus).Valid() {
		return fmt.Errorf("invalid play_status %q: %w", p.PlayStatus, ErrValidation)
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertOwned(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}

		res, err := tx.NewRaw(
			`UPDATE user_games SET play_status = ?, updated_at = NOW() WHERE id = ? AND user_id = ?`,
			p.PlayStatus, p.UserGameID, p.UserID,
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("update play_status: %w", err)
		}
		rows, _ := res.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pgdriver; count is advisory
		if rows == 0 {
			return ErrNotFound
		}

		return PromoteToInProgressIfPlayed(ctx, tx, p.UserGameID)
	})
}

// BulkStatusParams holds the parameters for SetPlayStatusBulk.
type BulkStatusParams struct {
	UserID      string
	UserGameIDs []string
	PlayStatus  string
}

// SetPlayStatusBulk updates play_status on each supplied user_game row (owned
// by UserID) and calls RemoveFromPoolsIfFinished for each, all in one
// transaction. Returns the count of rows actually updated.
func SetPlayStatusBulk(ctx context.Context, db *bun.DB, p BulkStatusParams) (int, error) {
	if !enum.PlayStatus(p.PlayStatus).Valid() {
		return 0, fmt.Errorf("invalid play_status %q: %w", p.PlayStatus, ErrValidation)
	}

	var updated int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, ugID := range p.UserGameIDs {
			res, err := tx.NewRaw(
				`UPDATE user_games SET play_status = ?, updated_at = NOW() WHERE id = ? AND user_id = ?`,
				p.PlayStatus, ugID, p.UserID,
			).Exec(ctx)
			if err != nil {
				return fmt.Errorf("bulk update user_game %s: %w", ugID, err)
			}
			rows, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows affected: %w", err)
			}
			if rows > 0 {
				updated++
				if err := RemoveFromPoolsIfFinished(ctx, tx, ugID); err != nil {
					return fmt.Errorf("remove from pools: %w", err)
				}
			}
		}
		return nil
	})
	return updated, err
}
