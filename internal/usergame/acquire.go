package usergame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	nexdb "github.com/drzero42/nexorious/internal/db"
)

// Acquire ensures the user owns the game on the supplied platforms, running the
// full acquire invariant set (clear-wishlist, promote-if-played, optional tag
// reconcile) atomically. See docs/superpowers/specs/2026-06-17-issue-1056-*.
func Acquire(ctx context.Context, db *bun.DB, p AcquireParams) (Result, error) {
	var res Result
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := validatePlatformRefs(ctx, tx, p.Platforms); err != nil {
			return err
		}
		ugID, created, err := upsertUserGame(ctx, tx, p)
		if err != nil {
			return err
		}
		res.UserGameID = ugID
		res.Created = created

		changes, err := mergePlatforms(ctx, tx, ugID, p.Platforms)
		if err != nil {
			return err
		}
		res.PlatformChanges = changes

		if err := clearWishlistOnAcquire(ctx, tx, ugID); err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		if err := promoteToInProgressIfPlayed(ctx, tx, ugID); err != nil {
			return fmt.Errorf("promote if played: %w", err)
		}
		if len(p.Tags) > 0 {
			if err := reconcileTags(ctx, tx, ugID, p.UserID, p.Tags, p.TagMode); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

func upsertUserGame(ctx context.Context, tx bun.IDB, p AcquireParams) (string, bool, error) {
	now := time.Now().UTC()
	id := uuid.NewString()
	if p.Mode == ModeUpsert {
		var row struct {
			ID    string `bun:"id"`
			IsNew bool   `bun:"is_new"`
		}
		err := tx.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
			 RETURNING id, (xmax = 0) AS is_new`,
			id, p.UserID, p.GameID, now, now,
		).Scan(ctx, &row)
		if err != nil {
			return "", false, fmt.Errorf("upsert user_game: %w", err)
		}
		return row.ID, row.IsNew, nil
	}
	if p.Mode == ModeImport {
		// Persist all caller-supplied meta + timestamps on a fresh insert; leave an
		// existing row (including its updated_at) fully intact on conflict. The
		// DO UPDATE self-assignment is a no-op value-wise but still emits a row, so
		// RETURNING surfaces the existing id and (xmax = 0) reports created-vs-merged.
		// play_status NOT NULL DEFAULT 'not_started': COALESCE so a nil pointer
		// lands on 'not_started' (which the promote-if-played guard keys off).
		createdAt := now
		if p.CreatedAt != nil {
			createdAt = p.CreatedAt.UTC()
		}
		updatedAt := now
		if p.UpdatedAt != nil {
			updatedAt = p.UpdatedAt.UTC()
		}
		var row struct {
			ID    string `bun:"id"`
			IsNew bool   `bun:"is_new"`
		}
		err := tx.NewRaw(
			`INSERT INTO user_games
			 (id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, is_wishlisted, created_at, updated_at)
			 VALUES (?, ?, ?, COALESCE(?, 'not_started'), ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = user_games.updated_at
			 RETURNING id, (xmax = 0) AS is_new`,
			id, p.UserID, p.GameID, p.PlayStatus, p.PersonalRating, p.IsLoved, p.PersonalNotes, p.IsWishlisted, createdAt, updatedAt,
		).Scan(ctx, &row)
		if err != nil {
			return "", false, fmt.Errorf("import upsert user_game: %w", err)
		}
		return row.ID, row.IsNew, nil
	}
	// ModeCreate — persist all caller-supplied meta fields in the initial row.
	// play_status has a NOT NULL DEFAULT 'not_started'; use COALESCE so a nil
	// pointer falls through to the column default rather than violating the constraint.
	_, err := tx.NewRaw(
		`INSERT INTO user_games
		 (id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, is_wishlisted, created_at, updated_at)
		 VALUES (?, ?, ?, COALESCE(?, 'not_started'), ?, ?, ?, ?, ?, ?)`,
		id, p.UserID, p.GameID, p.PlayStatus, p.PersonalRating, p.IsLoved, p.PersonalNotes, p.IsWishlisted, now, now,
	).Exec(ctx)
	if err != nil {
		if nexdb.IsUniqueViolation(err) {
			return "", false, fmt.Errorf("game already in collection: %w", ErrConflict)
		}
		return "", false, fmt.Errorf("insert user_game: %w", err)
	}
	return id, true, nil
}

func mergePlatforms(ctx context.Context, tx bun.IDB, ugID string, ins []PlatformInput) ([]PlatformChange, error) {
	var changes []PlatformChange
	for _, in := range ins {
		ch, err := mergeOnePlatform(ctx, tx, ugID, in)
		if err != nil {
			return nil, err
		}
		changes = append(changes, ch)
	}
	return changes, nil
}

func mergeOnePlatform(ctx context.Context, tx bun.IDB, ugID string, in PlatformInput) (PlatformChange, error) {
	ownership := "owned"
	if in.OwnershipStatus != nil && *in.OwnershipStatus != "" {
		ownership = *in.OwnershipStatus
	}
	available := true
	if in.IsAvailable != nil {
		available = *in.IsAvailable
	}
	var hours float64
	if in.HoursPlayed != nil {
		hours = *in.HoursPlayed
	}
	ch := PlatformChange{Platform: deref(in.Platform), Storefront: deref(in.Storefront)}

	var existingID string
	var existingOwnership *string
	var existingHours *float64
	err := tx.NewRaw(
		`SELECT id, ownership_status, hours_played FROM user_game_platforms
		 WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
		ugID, in.Platform, in.Storefront,
	).Scan(ctx, &existingID, &existingOwnership, &existingHours)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		ch.Created = true
		// Bind in.HoursPlayed (*float64) directly so nil → SQL NULL.
		// Do NOT use the coerced `hours` local here (nil → 0 is wrong for INSERT).
		// Bind in.AchievementsUnlocked / in.AchievementsTotal (*int) directly so nil → SQL NULL.
		_, err := tx.NewRaw(
			`INSERT INTO user_game_platforms
			 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, external_game_id, acquired_date, sync_from_source, achievements_unlocked, achievements_total, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now(), now())
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			uuid.NewString(), ugID, in.Platform, in.Storefront, available, in.HoursPlayed, ownership, in.ExternalGameID, in.AcquiredDate, in.SyncFromSource, in.AchievementsUnlocked, in.AchievementsTotal,
		).Exec(ctx)
		if err != nil {
			return ch, fmt.Errorf("insert platform: %w", err)
		}
	case err != nil:
		return ch, fmt.Errorf("select existing platform: %w", err)
	default:
		finalOwnership := ownership
		if existingOwnership != nil {
			finalOwnership = *existingOwnership
		}
		if ownershipRank(ownership) > ownershipRankPtr(existingOwnership) {
			ch.OwnershipUpgraded = true
			ch.OldOwnership = existingOwnership
			o := ownership
			ch.NewOwnership = &o
			finalOwnership = ownership
		}
		// UPDATE branch: use coerced `hours` (0 if nil) for max comparison.
		// This is correct — the UPDATE always writes a concrete value.
		finalHours := hours
		if existingHours != nil && *existingHours > finalHours {
			finalHours = *existingHours
		}
		_, err := tx.NewRaw(
			`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, external_game_id = COALESCE(?, external_game_id), achievements_unlocked = COALESCE(?, achievements_unlocked), achievements_total = COALESCE(?, achievements_total), updated_at = now() WHERE id = ?`,
			finalOwnership, finalHours, in.ExternalGameID, in.AchievementsUnlocked, in.AchievementsTotal, existingID,
		).Exec(ctx)
		if err != nil {
			return ch, fmt.Errorf("update platform: %w", err)
		}
	}
	return ch, nil
}

// AddPlatformParams is the parameter struct for AddPlatform.
type AddPlatformParams struct {
	UserID     string
	UserGameID string
	Platform   PlatformInput
}

// BulkAddPlatformParams is the parameter struct for AddPlatformBulk.
type BulkAddPlatformParams struct {
	UserID      string
	UserGameIDs []string
	Platform    PlatformInput
}

// MoveParams is the parameter struct for MoveToLibrary.
type MoveParams struct {
	UserID     string
	UserGameID string
	Platforms  []PlatformInput
}

// AddPlatform verifies ownership, inserts the platform strictly (duplicate →
// ErrConflict), clears the wishlist flag, and promotes play status, all
// atomically.
func AddPlatform(ctx context.Context, db *bun.DB, p AddPlatformParams) (Result, error) {
	var res Result
	res.UserGameID = p.UserGameID
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertOwned(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}
		if err := validatePlatformRef(ctx, tx, p.Platform.Platform, p.Platform.Storefront); err != nil {
			return err
		}
		platID, err := insertPlatformStrict(ctx, tx, p.UserGameID, p.Platform)
		if err != nil {
			return err
		}
		res.PlatformID = platID
		if err := clearWishlistOnAcquire(ctx, tx, p.UserGameID); err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		return promoteToInProgressIfPlayed(ctx, tx, p.UserGameID)
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// AddPlatformBulk inserts a platform onto each owned user_game in one
// transaction, skipping rows not owned by UserID and rows where the platform
// already exists. Returns the number of newly inserted rows.
func AddPlatformBulk(ctx context.Context, db *bun.DB, p BulkAddPlatformParams) (int, error) {
	var added int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := validatePlatformRef(ctx, tx, p.Platform.Platform, p.Platform.Storefront); err != nil {
			return err
		}
		for _, ugID := range p.UserGameIDs {
			if err := assertOwned(ctx, tx, ugID, p.UserID); err != nil {
				if errors.Is(err, ErrNotFound) {
					continue // skip rows not owned
				}
				return err
			}
			result, insertErr := tx.NewRaw(
				`INSERT INTO user_game_platforms
				 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, external_game_id, acquired_date, sync_from_source, created_at, updated_at)
				 VALUES (?, ?, ?, ?, true, NULL, 'owned', NULL, NULL, false, now(), now())
				 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
				uuid.NewString(), ugID, p.Platform.Platform, p.Platform.Storefront,
			).Exec(ctx)
			if insertErr != nil {
				return fmt.Errorf("bulk insert platform: %w", insertErr)
			}
			rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pgdriver; count is advisory
			if rows > 0 {
				added++
				if err := clearWishlistOnAcquire(ctx, tx, ugID); err != nil {
					return fmt.Errorf("clear wishlist: %w", err)
				}
			}
		}
		return nil
	})
	return added, err
}

// MoveToLibrary asserts the row exists and is_wishlisted, inserts each
// platform strictly (duplicate → ErrConflict), clears the wishlist flag, and
// promotes play status, all atomically.
func MoveToLibrary(ctx context.Context, db *bun.DB, p MoveParams) (Result, error) {
	var res Result
	res.UserGameID = p.UserGameID
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertWishlisted(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}
		if err := validatePlatformRefs(ctx, tx, p.Platforms); err != nil {
			return err
		}
		for _, plat := range p.Platforms {
			if _, err := insertPlatformStrict(ctx, tx, p.UserGameID, plat); err != nil {
				return err
			}
		}
		if err := clearWishlistOnAcquire(ctx, tx, p.UserGameID); err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		return promoteToInProgressIfPlayed(ctx, tx, p.UserGameID)
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// assertOwned returns ErrNotFound if the user_game row does not exist or is
// not owned by userID.
func assertOwned(ctx context.Context, tx bun.IDB, ugID, userID string) error {
	var x int
	err := tx.NewRaw(
		`SELECT 1 FROM user_games WHERE id = ? AND user_id = ?`, ugID, userID,
	).Scan(ctx, &x)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// assertWishlisted returns ErrNotFound if the row does not exist, ErrValidation
// if it exists but is not on the wishlist.
func assertWishlisted(ctx context.Context, tx bun.IDB, ugID, userID string) error {
	var wishlisted bool
	err := tx.NewRaw(
		`SELECT is_wishlisted FROM user_games WHERE id = ? AND user_id = ?`, ugID, userID,
	).Scan(ctx, &wishlisted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !wishlisted {
		return fmt.Errorf("not on wishlist: %w", ErrValidation)
	}
	return nil
}

// insertPlatformStrict inserts a platform row without ON CONFLICT DO NOTHING,
// so a duplicate (user_game_id, platform, storefront) returns ErrConflict.
// Returns the new platform row ID.
func insertPlatformStrict(ctx context.Context, tx bun.IDB, ugID string, in PlatformInput) (string, error) {
	ownership := "owned"
	if in.OwnershipStatus != nil && *in.OwnershipStatus != "" {
		ownership = *in.OwnershipStatus
	}
	available := true
	if in.IsAvailable != nil {
		available = *in.IsAvailable
	}
	id := uuid.NewString()
	_, err := tx.NewRaw(
		`INSERT INTO user_game_platforms
		 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, external_game_id, acquired_date, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, false, now(), now())`,
		id, ugID, in.Platform, in.Storefront, available, in.HoursPlayed, ownership, in.ExternalGameID, in.AcquiredDate,
	).Exec(ctx)
	if err != nil {
		if nexdb.IsUniqueViolation(err) {
			return "", fmt.Errorf("platform already exists: %w", ErrConflict)
		}
		return "", fmt.Errorf("insert platform: %w", err)
	}
	return id, nil
}

func ownershipRankPtr(s *string) int {
	if s == nil {
		return 0
	}
	return ownershipRank(*s)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func reconcileTags(ctx context.Context, tx bun.IDB, ugID, userID string, tags []TagInput, mode TagMode) error {
	if mode == TagReplace {
		names := make([]string, 0, len(tags))
		for _, t := range tags {
			names = append(names, t.Name)
		}
		return ReplaceTags(ctx, tx, ugID, userID, names)
	}
	// TagMerge: resolve/create each tag and insert the link if absent.
	for _, t := range tags {
		tagID, err := ResolveOrCreateTag(ctx, tx, userID, t.Name, t.Color)
		if err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`INSERT INTO user_game_tags (id, user_game_id, tag_id, created_at)
			 VALUES (?, ?, ?, now())
			 ON CONFLICT (user_game_id, tag_id) DO NOTHING`,
			uuid.NewString(), ugID, tagID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("merge tag link: %w", err)
		}
	}
	return nil
}
