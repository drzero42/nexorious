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
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/usergame"
)

// PoolsItemKey is the sentinel item_key of the synthetic job_item that carries
// the Play Planning pools payload for a Nexorious JSON import. It is created
// pre-completed and applied once at the job-completion transition; it is not a
// game item and is excluded from per-job item listings.
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

func (ImportItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

// ImportItemWorker processes a single import job item.
type ImportItemWorker struct {
	river.WorkerDefaults[ImportItemArgs]
	DB          *bun.DB
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

// Work processes a single import job item. It never returns an error —
// failures are recorded on the JobItem itself.
func (w *ImportItemWorker) Work(ctx context.Context, job *river.Job[ImportItemArgs]) error {
	// Load JobItem
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.ErrorContext(ctx, "import_item: load job_item", "id", job.Args.JobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
	// Correlate every line below to the parent import job.
	ctx = logging.WithJobID(ctx, item.JobID)

	// Parse game data from source_metadata
	var wrapper struct {
		Data importGameData `json:"data"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &wrapper); err != nil {
		// Item writes use context.Background() so they succeed even if the River
		// job context was cancelled during graceful shutdown.
		markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err), "import_item: markItemFailed")
		checkJobCompletion(w.DB, item.JobID)
		return nil
	}
	gd := wrapper.Data

	// Validate igdb_id
	if gd.IGDBID == 0 {
		markItemFailed(context.Background(), w.DB, &item, "missing igdb_id", "import_item: markItemFailed")
		checkJobCompletion(w.DB, item.JobID)
		return nil
	}

	// Re-hydrate the game from IGDB by id (cover art, metadata). On any per-item
	// IGDB failure, ensureGameRow inserts a minimal id+title row so user data is
	// preserved; a later metadata refresh fills in the rest.
	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, gd.IGDBID, gd.Title); err != nil {
		markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("ensure game row: %v", err), "import_item: markItemFailed")
		checkJobCompletion(w.DB, item.JobID)
		return nil
	}

	// Parse the meta fields for the (possible) initial insert. Acquire(ModeImport)
	// writes these — and the caller-supplied timestamps — only when it creates the
	// user_game row; an existing row (including its updated_at) is left untouched.
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

	// Build platform inputs, verifying each platform/storefront against the DB
	// (platforms not in the seed data are silently skipped; storefronts not found
	// are recorded with a NULL storefront and a warning).
	var plats []usergame.PlatformInput
	for _, pd := range gd.Platforms {
		if pd.Platform == "" {
			continue
		}

		// Verify platform exists (must be seeded via seed data or migration).
		var platformName string
		if err := w.DB.QueryRowContext(ctx,
			"SELECT name FROM platforms WHERE name = ?", pd.Platform,
		).Scan(&platformName); err != nil {
			slog.WarnContext(ctx, "import_item: platform not found, skipping (load seed data first)", "platform", pd.Platform)
			continue
		}

		// Verify storefront exists (nullable — store NULL if blank or not yet seeded).
		var storefrontPtr *string
		if pd.Storefront != "" {
			var storefrontName string
			if err := w.DB.QueryRowContext(ctx,
				"SELECT name FROM storefronts WHERE name = ?", pd.Storefront,
			).Scan(&storefrontName); err == nil {
				storefrontPtr = &storefrontName
			} else {
				slog.WarnContext(ctx, "import_item: storefront not found, recording platform without storefront (load seed data first)", "storefront", pd.Storefront)
			}
		}

		ownership := pd.OwnershipStatus
		if ownership == nil || !enum.OwnershipStatus(*ownership).Valid() {
			if ownership != nil {
				slog.WarnContext(ctx, "import_item: invalid ownership_status, defaulting to owned", "value", *ownership)
			}
			owned := string(enum.OwnershipOwned)
			ownership = &owned
		}

		plats = append(plats, usergame.PlatformInput{
			Platform:        &platformName,
			Storefront:      storefrontPtr,
			IsAvailable:     pd.IsAvailable, // nil ⇒ Acquire defaults to true
			HoursPlayed:     pd.HoursPlayed,
			OwnershipStatus: ownership,
			AcquiredDate:    parseFlexibleDate(pd.AcquiredDate),
			// SyncFromSource intentionally omitted (false) — imports are not storefront syncs.
		})
	}

	// Build tag inputs.
	tags := make([]usergame.TagInput, 0, len(gd.Tags))
	for _, td := range gd.Tags {
		tags = append(tags, usergame.TagInput{Name: td.Name, Color: td.Color})
	}

	// Snapshot the tag count BEFORE Acquire so we can detect newly merged links.
	// (Acquire merges tags additively; counting after would make existingTagCount
	// equal totalTagCount and produce newTagCount = 0.) Counts 0 for a row that
	// does not exist yet, so the snapshot needs no prior existence check.
	existingTagCount := 0
	if len(tags) > 0 {
		if err := w.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_game_tags ugt"+
				" JOIN user_games ug ON ug.id = ugt.user_game_id"+
				" WHERE ug.user_id = ? AND ug.game_id = ?", item.UserID, gd.IGDBID,
		).Scan(&existingTagCount); err != nil {
			existingTagCount = 0
		}
	}

	// Acquire creates the user_game with the imported meta + timestamps (or leaves
	// an existing row, including its updated_at, untouched), merges platforms
	// (max-hours, ownership upgrade), clears wishlist on acquire, auto-promotes
	// play_status, and merges tags additively — all atomically.
	res, err := usergame.Acquire(ctx, w.DB, usergame.AcquireParams{
		UserID: item.UserID, GameID: gd.IGDBID, Mode: usergame.ModeImport,
		Platforms: plats, Tags: tags, TagMode: usergame.TagMerge,
		PlayStatus: playStatus, PersonalRating: personalRating,
		IsLoved: gd.IsLoved, IsWishlisted: gd.IsWishlisted, PersonalNotes: gd.PersonalNotes,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
	if err != nil {
		markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("acquire: %v", err), "import_item: markItemFailed")
		checkJobCompletion(w.DB, item.JobID)
		return nil
	}
	alreadyExists := !res.Created

	// Determine change type for the changes table.
	newPlatformCount := 0
	for _, ch := range res.PlatformChanges {
		if ch.Created {
			newPlatformCount++
		}
	}
	// Count total tags post-Acquire; diff against the pre-Acquire snapshot.
	var totalTagCount int
	if len(tags) > 0 {
		if err := w.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_game_tags WHERE user_game_id = ?", res.UserGameID,
		).Scan(&totalTagCount); err != nil {
			totalTagCount = 0
		}
	}
	newTagCount := totalTagCount - existingTagCount

	// Mark item completed
	// Record a per-item change row mirroring the sync worker's `changes` writes.
	changeType := "added"
	if alreadyExists {
		if newPlatformCount+newTagCount > 0 {
			changeType = "updated"
		} else {
			changeType = "already_in_library"
		}
	}
	if _, err := w.DB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, NULL, ?, ?, ?, now())`,
		uuid.NewString(), item.JobID, item.UserID, res.UserGameID, changeType, item.SourceTitle,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "import_item: insert change", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}

	result := map[string]any{
		"game_id":         gd.IGDBID,
		"user_game_id":    res.UserGameID,
		"is_new_addition": !alreadyExists,
		"already_exists":  alreadyExists,
	}
	markItemCompletedWithResult(context.Background(), w.DB, &item, result, "import_item: markItemCompleted")
	checkJobCompletion(w.DB, item.JobID)
	return nil
}

// applyImportedPools reads the synthetic pools job_item for a finished import job
// and applies its pools additively: find-or-create each pool by (user_id, name),
// then attach members resolved from igdb_id to the user's user_games. It is
// best-effort — a per-pool or per-member failure is logged and skipped, never
// failing the job. Safe to call only on the single job-completion transition.
func applyImportedPools(ctx context.Context, db *bun.DB, jobID, userID string) {
	var item models.JobItem
	err := db.NewSelect().Model(&item).
		Where("job_id = ? AND item_key = ?", jobID, PoolsItemKey).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "import: load pools item", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	var wrapper struct {
		Data []importPoolData `json:"data"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &wrapper); err != nil {
		slog.WarnContext(ctx, "import: parse pools payload", logging.KeyErr, err, logging.Cat(logging.CategoryValidation))
		return
	}
	for _, p := range wrapper.Data {
		poolID, err := findOrCreatePool(ctx, db, userID, p)
		if err != nil {
			slog.WarnContext(ctx, "import: find/create pool", logging.KeyErr, err, "pool", p.Name, logging.Cat(logging.CategoryDB))
			continue
		}
		for _, m := range p.Games {
			var ugID string
			if err := db.NewRaw(
				`SELECT id FROM user_games WHERE user_id = ? AND game_id = ?`, userID, m.IGDBID,
			).Scan(ctx, &ugID); err != nil {
				// Game absent (failed import) or lookup error: skip this member.
				continue
			}
			if _, err := db.NewRaw(
				`INSERT INTO pool_games (id, pool_id, user_game_id, position, created_at)
				 VALUES (?, ?, ?, ?, now())
				 ON CONFLICT (pool_id, user_game_id) DO NOTHING`,
				uuid.NewString(), poolID, ugID, m.Position,
			).Exec(ctx); err != nil {
				slog.WarnContext(ctx, "import: insert pool_game", logging.KeyErr, err, "pool", p.Name, logging.Cat(logging.CategoryDB))
			}
		}
	}
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

// checkJobCompletion counts remaining pending items and updates the parent Job if done.
// Uses context.Background() so the write succeeds even if the River job context
// was cancelled during graceful shutdown.
func checkJobCompletion(db *bun.DB, jobID string) {
	ctx := context.Background()

	pendingCount, ok := countJobItems(ctx, db, jobID, "status = 'pending'", "import_item: count pending items")
	if !ok || pendingCount > 0 {
		return
	}

	// No more pending — determine final job status.
	failedCount, ok := countJobItems(ctx, db, jobID, "status = 'failed'", "import_item: count failed items")
	if !ok {
		return
	}

	finalized := finalizeJobCompleted(ctx, db, jobID, "import_item: update job status", false)

	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
	if finalized {
		applyImportedPools(ctx, db, jobID, uid)
	}
	if failedCount > 0 {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeImportFailed, Scope: notify.ScopeUser, ActorUserID: uid,
			Payload:  notify.ImportFailedPayload{JobID: jobID, Failed: failedCount, Error: fmt.Sprintf("%d item(s) failed to import", failedCount)},
			DedupKey: jobID + ":" + notify.TypeImportFailed,
		})
		return
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeImportCompleted, Scope: notify.ScopeUser, ActorUserID: uid,
		Payload:  notify.ImportCompletedPayload{JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeImportCompleted,
	})
}
