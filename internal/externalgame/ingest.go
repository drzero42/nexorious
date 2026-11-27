package externalgame

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Sighting is one game as a storefront listed it at a sync.
type Sighting struct {
	UserID          string
	Storefront      string
	ExternalID      string
	Title           string
	IsSubscription  bool
	OwnershipStatus string
	// SourceMetadata is kept as stored when empty.
	SourceMetadata map[string]string
	// Platforms are canonical platform slugs, already resolved by the caller.
	// The playtime and achievements go on the first one.
	Platforms            []string
	PlaytimeHours        float64
	AchievementsUnlocked *int
	AchievementsTotal    *int
}

// Seen writes a game a sync fetched, and its platform rows, and returns its
// id and whether it is skipped. A game that is new here is not given a
// decision: its match worker takes a same-title game's decision (precedent) or
// searches. A known game keeps its match and skip and is available again.
func Seen(ctx context.Context, db bun.IDB, s Sighting) (id string, skipped bool, err error) {
	var row struct {
		ID        string `bun:"id"`
		IsSkipped bool   `bun:"is_skipped"`
	}
	var sourceMetaJSON []byte
	if len(s.SourceMetadata) > 0 {
		sourceMetaJSON, _ = json.Marshal(s.SourceMetadata) //nolint:errcheck // marshaling a map[string]string cannot fail
	}
	if err := db.NewRaw(`
		INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, source_metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, now(), now())
		ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			is_subscription = EXCLUDED.is_subscription,
			ownership_status = EXCLUDED.ownership_status,
			source_metadata = COALESCE(EXCLUDED.source_metadata, external_games.source_metadata),
			is_available = true,
			updated_at = now()
		RETURNING id, is_skipped`,
		uuid.NewString(), s.UserID, s.Storefront, s.ExternalID, s.Title,
		s.IsSubscription, s.OwnershipStatus, nullableJSON(sourceMetaJSON),
	).Scan(ctx, &row); err != nil {
		return "", false, fmt.Errorf("upsert external_game: %w", err)
	}
	if err := upsertPlatforms(ctx, db, row.ID, s); err != nil {
		return "", false, err
	}
	return row.ID, row.IsSkipped, nil
}

// nullableJSON returns nil when b is empty so the column is written as SQL NULL.
// When non-empty it returns a json.RawMessage so pgdriver encodes it as JSONB.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

func upsertPlatforms(ctx context.Context, db bun.IDB, id string, s Sighting) error {
	for i, platform := range s.Platforms {
		hours := 0.0
		var unlocked, total *int
		if i == 0 {
			hours = s.PlaytimeHours
			unlocked = s.AchievementsUnlocked
			total = s.AchievementsTotal
		}
		if _, err := db.NewRaw(`
			INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, achievements_unlocked, achievements_total, created_at)
			VALUES (?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (external_game_id, platform) DO UPDATE SET
				hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played),
				achievements_unlocked = COALESCE(EXCLUDED.achievements_unlocked, external_game_platforms.achievements_unlocked),
				achievements_total = COALESCE(EXCLUDED.achievements_total, external_game_platforms.achievements_total)`,
			uuid.NewString(), id, platform, hours, unlocked, total,
		).Exec(ctx); err != nil {
			return fmt.Errorf("upsert platform %s: %w", platform, err)
		}
	}
	return nil
}

// PrunePlatforms deletes the platform rows of the External game id whose
// platform is not in keep: the platforms a sync no longer lists for it.
func PrunePlatforms(ctx context.Context, db bun.IDB, id string, keep []string) error {
	if _, err := db.NewRaw(`
		DELETE FROM external_game_platforms
		WHERE external_game_id = ? AND platform NOT IN (?)`,
		id, bun.List(keep),
	).Exec(ctx); err != nil {
		return fmt.Errorf("delete stale platforms: %w", err)
	}
	return nil
}

// MarkUnavailable marks the External game id unavailable: the storefront no
// longer lists it. It reports whether it changed the row, so a caller writes
// its 'removed' row once.
func MarkUnavailable(ctx context.Context, db bun.IDB, id string) (bool, error) {
	res, err := db.NewRaw(
		`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ? AND is_available = true`, id,
	).Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("mark game unavailable: %w", err)
	}
	n, _ := res.RowsAffected() //nolint:errcheck // pgdriver always reports RowsAffected
	return n > 0, nil
}

// AutoMatch is the write of a confident match found by the match worker's
// search: the External game id takes an edition's decision made during the
// search if there is one (precedent first), otherwise the search's match
// igdbID. It reports whether it wrote igdbID. The write is guarded so a
// rematch or Skip that committed during the search wins (#1338). A games row
// it creates takes igdbTitle, the IGDB title, not the storefront's (#1455);
// storefrontTitle is the title the caller read before the search.
func AutoMatch(ctx context.Context, db bun.IDB, id, storefrontTitle string, igdbID int32, igdbTitle string) (bool, error) {
	took, err := TakePrecedent(ctx, db, id)
	if err != nil || took {
		return false, err
	}
	if err := ensureGameRow(ctx, db, igdbID, igdbTitle, storefrontTitle); err != nil {
		return false, err
	}
	res, err := db.NewRaw(
		`UPDATE external_games eg SET resolved_igdb_id = ?, updated_at = now() WHERE eg.id = ? AND `+Undecided("eg"),
		igdbID, id,
	).Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("auto-match external_game: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("auto-match external_game: rows affected: %w", err)
	}
	return n > 0, nil
}

// MatchUnderLock reads the match of the External game id and share-locks its
// row, so a rematch in progress is waited for (#1338). A missing row is not an
// error: it has no match (nil).
func MatchUnderLock(ctx context.Context, tx bun.Tx, id string) (*int32, error) {
	var resolved *int32
	if err := tx.NewRaw(
		`SELECT resolved_igdb_id FROM external_games WHERE id = ? FOR SHARE`, id,
	).Scan(ctx, &resolved); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lock external game: %w", err)
	}
	return resolved, nil
}
