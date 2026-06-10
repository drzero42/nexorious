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

type importTagData struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// coercePlayStatus validates a play_status against enum.PlayStatus and returns
// nil for an invalid (or already-nil) value, so callers leave the column unset
// and let the user_games.play_status NOT NULL DEFAULT 'not_started' apply. All
// import paths use this to validate play_status uniformly.
func coercePlayStatus(s *string) *string {
	if s != nil && !enum.PlayStatus(*s).Valid() {
		slog.Warn("import: invalid play_status, treating as unset", "value", *s)
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

	// Check for existing UserGame
	var existingUG models.UserGame
	err := w.DB.NewSelect().Model(&existingUG).
		Where("user_id = ? AND game_id = ?", item.UserID, gd.IGDBID).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("check existing user_game: %v", err), "import_item: markItemFailed")
		checkJobCompletion(w.DB, item.JobID)
		return nil
	}
	alreadyExists := err == nil

	// Build and insert UserGame (skip if already exists)
	now := time.Now().UTC()
	var ug *models.UserGame
	if alreadyExists {
		ug = &existingUG
	} else {
		createdAt := now
		updatedAt := now
		if gd.CreatedAt != nil {
			if t, err := time.Parse(time.RFC3339, *gd.CreatedAt); err == nil {
				createdAt = t.UTC()
			}
		}
		if gd.UpdatedAt != nil {
			if t, err := time.Parse(time.RFC3339, *gd.UpdatedAt); err == nil {
				updatedAt = t.UTC()
			}
		}

		var personalRating *int32
		if gd.PersonalRating != nil && *gd.PersonalRating >= 1 && *gd.PersonalRating <= 5 {
			r := int32(*gd.PersonalRating) //nolint:gosec // bounded to 1..5 above
			personalRating = &r
		} else if gd.PersonalRating != nil {
			slog.WarnContext(ctx, "import_item: personal_rating out of range, treating as unrated", "value", *gd.PersonalRating)
		}

		playStatus := coercePlayStatus(gd.PlayStatus)

		ug = &models.UserGame{
			ID:             uuid.NewString(),
			UserID:         item.UserID,
			GameID:         gd.IGDBID,
			PlayStatus:     playStatus,
			PersonalRating: personalRating,
			IsLoved:        gd.IsLoved,
			PersonalNotes:  gd.PersonalNotes,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		}
		_, err = w.DB.NewInsert().Model(ug).Exec(ctx)
		if err != nil {
			markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("insert user_game: %v", err), "import_item: markItemFailed")
			checkJobCompletion(w.DB, item.JobID)
			return nil
		}
	}

	// Platforms
	// Build a set of existing (platform, storefront) pairs to avoid duplicates
	// when merging into an existing UserGame.
	type platformKey struct{ platform, storefront string }
	existingPlatforms := map[platformKey]bool{}
	if alreadyExists {
		var existingUGPs []models.UserGamePlatform
		if err := w.DB.NewSelect().Model(&existingUGPs).
			Where("user_game_id = ?", ug.ID).Scan(ctx); err == nil {
			for _, ugp := range existingUGPs {
				p := ""
				if ugp.Platform != nil {
					p = *ugp.Platform
				}
				s := ""
				if ugp.Storefront != nil {
					s = *ugp.Storefront
				}
				existingPlatforms[platformKey{p, s}] = true
			}
		}
	}

	newPlatformCount := 0
	newTagCount := 0
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

		// Skip if this (platform, storefront) pair is already recorded.
		sStr := ""
		if storefrontPtr != nil {
			sStr = *storefrontPtr
		}
		if existingPlatforms[platformKey{platformName, sStr}] {
			continue
		}

		ownership := pd.OwnershipStatus
		if ownership == nil || !enum.OwnershipStatus(*ownership).Valid() {
			if ownership != nil {
				slog.WarnContext(ctx, "import_item: invalid ownership_status, defaulting to owned", "value", *ownership)
			}
			owned := string(enum.OwnershipOwned)
			ownership = &owned
		}

		ugp := &models.UserGamePlatform{
			ID:              uuid.NewString(),
			UserGameID:      ug.ID,
			Platform:        &platformName,
			Storefront:      storefrontPtr,
			IsAvailable:     true, // imported rows default available; sync re-derives
			HoursPlayed:     pd.HoursPlayed,
			OwnershipStatus: ownership,
			AcquiredDate:    parseFlexibleDate(pd.AcquiredDate),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if _, err := w.DB.NewInsert().Model(ugp).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "import_item: insert user_game_platform", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		} else {
			newPlatformCount++
		}
	}
	if err := usergame.ClearWishlistOnAcquire(ctx, w.DB, ug.ID); err != nil {
		slog.WarnContext(ctx, "import_item: clear wishlist on acquire", logging.KeyErr, err, "user_game_id", ug.ID, logging.Cat(logging.CategoryDB))
	}

	// Tags
	// Build existing tag set to avoid duplicates when merging.
	existingTagIDs := map[string]bool{}
	if alreadyExists {
		var existingUGTs []models.UserGameTag
		if err := w.DB.NewSelect().Model(&existingUGTs).
			Where("user_game_id = ?", ug.ID).Scan(ctx); err == nil {
			for _, ugt := range existingUGTs {
				existingTagIDs[ugt.TagID] = true
			}
		}
	}

	for _, td := range gd.Tags {
		tagID, err := findOrCreateTag(ctx, w.DB, item.UserID, td.Name, td.Color)
		if err != nil {
			markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("find/create tag %q: %v", td.Name, err), "import_item: markItemFailed")
			checkJobCompletion(w.DB, item.JobID)
			return nil
		}
		if existingTagIDs[tagID] {
			continue
		}

		ugt := &models.UserGameTag{
			ID:         uuid.NewString(),
			UserGameID: ug.ID,
			TagID:      tagID,
			CreatedAt:  now,
		}
		if _, err := w.DB.NewInsert().Model(ugt).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "import_item: insert user_game_tag", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		} else {
			newTagCount++
		}
	}

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
		uuid.NewString(), item.JobID, item.UserID, ug.ID, changeType, item.SourceTitle,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "import_item: insert change", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}

	result := map[string]any{
		"game_id":         gd.IGDBID,
		"user_game_id":    ug.ID,
		"is_new_addition": !alreadyExists,
		"already_exists":  alreadyExists,
	}
	markItemCompletedWithResult(context.Background(), w.DB, &item, result, "import_item: markItemCompleted")
	checkJobCompletion(w.DB, item.JobID)
	return nil
}

// findOrCreateTag finds a tag by name (case-insensitive) for the user, or creates it.
func findOrCreateTag(ctx context.Context, db *bun.DB, userID, name string, color *string) (string, error) {
	var tag models.Tag
	err := db.NewSelect().Model(&tag).
		Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, name).
		Scan(ctx)
	if err == nil {
		return tag.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("select tag: %w", err)
	}

	now := time.Now().UTC()
	tag = models.Tag{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Color:     color,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.NewInsert().Model(&tag).Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("insert tag: %w", err)
	}
	return tag.ID, nil
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

	finalizeJobCompleted(ctx, db, jobID, "import_item: update job status", false)

	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
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
