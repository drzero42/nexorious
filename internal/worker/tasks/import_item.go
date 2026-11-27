package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/usergame"
)

// PoolsItemKey is the sentinel item_key of the synthetic job_item that carries
// the Play Planning pools payload for a Nexorious JSON import. It is created
// already completed and applied by the Job's ending effects on its first
// ending; it is not a game item and is excluded from per-job item listings.
const PoolsItemKey = "__pools__"

// importPoolData is one pool entry in a Nexorious JSON import's `pools` section.
type importPoolData struct {
	Name     string               `json:"name"`
	Color    *string              `json:"color"`
	Position int                  `json:"position"`
	Filter   json.RawMessage      `json:"filter"`
	Games    []importPoolGameData `json:"games"`
}

// importPoolGameData is a pool membership: position nil = Candidate, set = queued.
type importPoolGameData struct {
	IGDBID   int32 `json:"igdb_id"`
	Position *int  `json:"position"`
}

// ImportItemArgs is the River job args type for "import_item".
type ImportItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ImportItemArgs) Kind() string { return "import_item" }

// InsertOpts: several attempts. The library write commits with the item's
// completion, so running it again is safe.
func (ImportItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

// ImportItemWorker processes a single nexorious import job item.
type ImportItemWorker struct {
	river.WorkerDefaults[ImportItemArgs]
	DB          *bun.DB
	LC          *joblifecycle.Module
	IGDBClient  *igdb.Client
	StoragePath string
}

// importGameData is the parsed shape inside JobItem.SourceMetadata.data (v2.0).
type importGameData struct {
	IGDBID         int32                `json:"igdb_id"`
	Title          string               `json:"title"`
	PlayStatus     *string              `json:"play_status"`
	PersonalRating *int                 `json:"personal_rating"`
	IsLoved        bool                 `json:"is_loved"`
	IsWishlisted   bool                 `json:"is_wishlisted"`
	PersonalNotes  *string              `json:"personal_notes"`
	CreatedAt      *string              `json:"created_at"` // RFC3339
	UpdatedAt      *string              `json:"updated_at"` // RFC3339
	Platforms      []importPlatformData `json:"platforms"`
	Tags           []importTagData      `json:"tags"`
}

type importPlatformData struct {
	Platform        string   `json:"platform"`
	Storefront      string   `json:"storefront"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"` // date-only or RFC3339
	HoursPlayed     *float64 `json:"hours_played"`
	IsAvailable     *bool    `json:"is_available"`
}

// parseFlexibleDate accepts either a date-only string ("2006-01-02") or a full
// RFC3339 timestamp and returns a *time.Time, or nil if s is nil/unparseable.
func parseFlexibleDate(s *string) *time.Time {
	if s == nil {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t
		}
	}
	return nil
}

// parseRFC3339 returns the parsed *time.Time for an RFC3339 string, or nil if s
// is nil or unparseable. Used to seed imported created_at/updated_at; a nil
// result lets Acquire fall through to now().
func parseRFC3339(s *string) *time.Time {
	if s == nil {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, *s); err == nil {
		u := t.UTC()
		return &u
	}
	return nil
}

type importTagData struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// coercePlayStatus validates a play_status against enum.PlayStatus and returns
// nil for an invalid (or already-nil) value, so callers leave the column unset
// and let the user_games.play_status NOT NULL DEFAULT 'not_started' apply. All
// import paths use this to validate play_status uniformly.
func coercePlayStatus(ctx context.Context, s *string) *string {
	if s != nil && !enum.PlayStatus(*s).Valid() {
		slog.WarnContext(ctx, "import: invalid play_status, treating as unset", "value", *s, logging.Cat(logging.CategoryValidation))
		return nil
	}
	return s
}

// Work imports one game of a nexorious export. The games row and cover art
// are ensured first, outside the transition; the library write, its changes
// row and the item's result then commit with the item's completion. Platforms
// and storefronts the database does not know are left out, as they always
// were for this format.
func (w *ImportItemWorker) Work(ctx context.Context, job *river.Job[ImportItemArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, job.Args.JobItemID, job.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)
	fail := func(msg string) error { return failItem(ctx, w.LC, item.ID, job.ID, msg) }

	var wrapper struct {
		Data importGameData `json:"data"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &wrapper); err != nil {
		return fail(fmt.Sprintf("parse source_metadata: %v", err))
	}
	gd := wrapper.Data
	if gd.IGDBID == 0 {
		return fail("missing igdb_id")
	}

	// Re-hydrate the game from IGDB by id. On an IGDB failure ensureGameRow
	// inserts a minimal id+title row so user data is kept; a later metadata
	// refresh fills in the rest.
	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, gd.IGDBID, gd.Title); err != nil {
		return retryOrFail(ctx, w.LC, job.JobRow, item.ID, "ensure game row", err)
	}

	// Acquire(ModeImport) writes the meta fields and the given timestamps only
	// when it creates the user_game row; an existing row is left untouched.
	var personalRating *int32
	if gd.PersonalRating != nil && *gd.PersonalRating >= 1 && *gd.PersonalRating <= 5 {
		r := int32(*gd.PersonalRating) //nolint:gosec // bounded to 1..5 above
		personalRating = &r
	} else if gd.PersonalRating != nil {
		slog.WarnContext(ctx, "import_item: personal_rating out of range, treating as unrated", "value", *gd.PersonalRating)
	}
	playStatus := coercePlayStatus(ctx, gd.PlayStatus)
	createdAt := parseRFC3339(gd.CreatedAt)
	updatedAt := parseRFC3339(gd.UpdatedAt)

	plats, err := w.platformInputs(ctx, gd.Platforms)
	if err != nil {
		return retryOrFail(ctx, w.LC, job.JobRow, item.ID, "look up platforms", err)
	}
	tags := make([]usergame.TagInput, 0, len(gd.Tags))
	for _, td := range gd.Tags {
		tags = append(tags, usergame.TagInput{Name: td.Name, Color: td.Color})
	}

	err = w.LC.EndItem(ctx, item.ID, job.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusCompleted,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			res, err := usergame.Acquire(ctx, tx, usergame.AcquireParams{
				UserID: item.UserID, GameID: gd.IGDBID, Mode: usergame.ModeImport,
				Platforms: plats, Tags: tags, TagMode: usergame.TagMerge,
				PlayStatus: playStatus, PersonalRating: personalRating,
				IsLoved: gd.IsLoved, IsWishlisted: gd.IsWishlisted, PersonalNotes: gd.PersonalNotes,
				CreatedAt: createdAt, UpdatedAt: updatedAt,
			})
			if err != nil {
				return nil, acquireError(err)
			}
			if _, err := tx.NewRaw(
				`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, NULL, ?, ?, ?, now())`,
				uuid.NewString(), item.JobID, item.UserID, res.UserGameID, importChangeType(res), item.SourceTitle,
			).Exec(ctx); err != nil {
				return nil, fmt.Errorf("insert change: %w", err)
			}
			return json.Marshal(map[string]any{
				"game_id":         gd.IGDBID,
				"user_game_id":    res.UserGameID,
				"is_new_addition": res.Created,
				"already_exists":  !res.Created,
			})
		},
	})
	return endImportItem(ctx, w.LC, job.JobRow, item.ID, err)
}

// platformInputs builds the Acquire platform inputs of a nexorious export
// game. A platform the database does not know is left out; a storefront it
// does not know is recorded as none. Only sql.ErrNoRows means "not known":
// any other lookup error is returned.
func (w *ImportItemWorker) platformInputs(ctx context.Context, pds []importPlatformData) ([]usergame.PlatformInput, error) {
	var plats []usergame.PlatformInput
	for _, pd := range pds {
		if pd.Platform == "" {
			continue
		}
		var platformName string
		if err := w.DB.QueryRowContext(ctx, "SELECT name FROM platforms WHERE name = ?", pd.Platform).Scan(&platformName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.WarnContext(ctx, "import_item: platform not found, skipping (load seed data first)", "platform", pd.Platform)
				continue
			}
			return nil, fmt.Errorf("platform %q: %w", pd.Platform, err)
		}

		var storefrontPtr *string
		if pd.Storefront != "" {
			var storefrontName string
			err := w.DB.QueryRowContext(ctx, "SELECT name FROM storefronts WHERE name = ?", pd.Storefront).Scan(&storefrontName)
			switch {
			case err == nil:
				storefrontPtr = &storefrontName
			case errors.Is(err, sql.ErrNoRows):
				slog.WarnContext(ctx, "import_item: storefront not found, recording platform without storefront (load seed data first)", "storefront", pd.Storefront)
			default:
				return nil, fmt.Errorf("storefront %q: %w", pd.Storefront, err)
			}
		}

		ownership := enum.OwnershipOwned
		if pd.OwnershipStatus != nil {
			if s := enum.OwnershipStatus(*pd.OwnershipStatus); s.Valid() {
				ownership = s
			} else {
				slog.WarnContext(ctx, "import_item: invalid ownership_status, defaulting to owned", "value", *pd.OwnershipStatus)
			}
		}

		plats = append(plats, usergame.PlatformInput{
			Platform:        &platformName,
			Storefront:      storefrontPtr,
			IsAvailable:     pd.IsAvailable, // nil ⇒ Acquire defaults to true
			HoursPlayed:     pd.HoursPlayed,
			OwnershipStatus: &ownership,
			AcquiredDate:    parseFlexibleDate(pd.AcquiredDate),
			// SyncFromSource intentionally omitted (false) — imports are not storefront syncs.
		})
	}
	return plats, nil
}

// applyImportedPools reads the pools item of an ended nexorious import and
// applies its pools additively: find-or-create each pool by (user_id, name),
// then attach the members whose games are in the user's library. A bad pools
// payload is logged and skipped; a database error is returned so the
// ending effects are retried (the SQL is idempotent). A member whose game
// was not imported is skipped.
func applyImportedPools(ctx context.Context, db *bun.DB, jobID, userID string) error {
	var item models.JobItem
	err := db.NewSelect().Model(&item).
		Where("job_id = ? AND item_key = ?", jobID, PoolsItemKey).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load pools item: %w", err)
	}
	var wrapper struct {
		Data []importPoolData `json:"data"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &wrapper); err != nil {
		slog.WarnContext(ctx, "import: parse pools payload, pools not applied", logging.KeyErr, err, logging.Cat(logging.CategoryValidation))
		return nil
	}
	for _, p := range wrapper.Data {
		poolID, err := findOrCreatePool(ctx, db, userID, p)
		if err != nil {
			return fmt.Errorf("find or create pool %q: %w", p.Name, err)
		}
		for _, m := range p.Games {
			var ugID string
			err := db.NewRaw(
				`SELECT id FROM user_games WHERE user_id = ? AND game_id = ?`, userID, m.IGDBID,
			).Scan(ctx, &ugID)
			if errors.Is(err, sql.ErrNoRows) {
				continue // the game was not imported
			}
			if err != nil {
				return fmt.Errorf("look up pool member: %w", err)
			}
			if _, err := db.NewRaw(
				`INSERT INTO pool_games (id, pool_id, user_game_id, position, created_at)
				 VALUES (?, ?, ?, ?, now())
				 ON CONFLICT (pool_id, user_game_id) DO NOTHING`,
				uuid.NewString(), poolID, ugID, m.Position,
			).Exec(ctx); err != nil {
				return fmt.Errorf("insert pool member of %q: %w", p.Name, err)
			}
		}
	}
	return nil
}

// findOrCreatePool returns the id of the user's pool named p.Name, creating it
// (with the imported color/filter and next position) if absent. An existing
// pool's curation is never overwritten — only its id is returned.
func findOrCreatePool(ctx context.Context, db *bun.DB, userID string, p importPoolData) (string, error) {
	var filterArg any
	if len(p.Filter) > 0 {
		filterArg = string(p.Filter)
	}
	if _, err := db.NewRaw(
		`INSERT INTO pools (id, user_id, name, color, position, filter, created_at, updated_at)
		 VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM pools WHERE user_id = ?), 0), ?, now(), now())
		 ON CONFLICT (user_id, name) DO NOTHING`,
		uuid.NewString(), userID, p.Name, p.Color, userID, filterArg,
	).Exec(ctx); err != nil {
		return "", err
	}
	var poolID string
	if err := db.NewRaw(
		`SELECT id FROM pools WHERE user_id = ? AND name = ?`, userID, p.Name,
	).Scan(ctx, &poolID); err != nil {
		return "", err
	}
	return poolID, nil
}

// igdbMetadataToGame maps an IGDB GameMetadata response to a models.Game ready for insert.
func igdbMetadataToGame(md *igdb.GameMetadata) *models.Game {
	now := time.Now().UTC()
	game := &models.Game{
		ID:                         int32(md.IgdbID), //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		Title:                      md.Title,
		Description:                md.Description,
		Genre:                      md.Genre,
		Developer:                  md.Developer,
		Publisher:                  md.Publisher,
		CoverArtUrl:                md.CoverArtURL,
		RatingAverage:              md.RatingAverage,
		RatingCount:                md.RatingCount,
		HowlongtobeatMain:          md.HowlongtobeatMain,
		HowlongtobeatExtra:         md.HowlongtobeatExtra,
		HowlongtobeatCompletionist: md.HowlongtobeatCompletionist,
		IgdbSlug:                   &md.IgdbSlug,
		GameModes:                  md.GameModes,
		Themes:                     md.Themes,
		PlayerPerspectives:         md.PlayerPerspectives,
		LastUpdated:                now,
		CreatedAt:                  now,
	}
	if md.ReleaseDate != nil {
		if t, err := time.Parse("2006-01-02", *md.ReleaseDate); err == nil {
			game.ReleaseDate = &t
		}
	}
	if len(md.PlatformIDs) > 0 {
		b, _ := json.Marshal(md.PlatformIDs) //nolint:errcheck // marshaling a fixed slice cannot fail
		s := string(b)
		game.IgdbPlatformIds = &s
	}
	if len(md.PlatformNames) > 0 {
		b, _ := json.Marshal(md.PlatformNames) //nolint:errcheck // marshaling a fixed slice cannot fail
		s := string(b)
		game.IgdbPlatformNames = &s
	}
	return game
}
