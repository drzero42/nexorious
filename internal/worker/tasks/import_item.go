package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
)

// importPayload is the PendingTask.Payload shape for "import_item".
type importPayload struct {
	JobItemID string `json:"job_item_id"`
}

// importGameData is the parsed shape inside JobItem.SourceMetadata.data.
type importGameData struct {
	IGDBID         int32                `json:"igdb_id"`
	Title          string               `json:"title"`
	Description    *string              `json:"description"`
	Genre          *string              `json:"genre"`
	Developer      *string              `json:"developer"`
	Publisher      *string              `json:"publisher"`
	ReleaseDate    *string              `json:"release_date"` // RFC3339
	CoverArtUrl    *string              `json:"cover_art_url"`
	RatingAverage  *float64             `json:"rating_average"`
	PlayStatus     *string              `json:"play_status"`
	PersonalRating *float64             `json:"personal_rating"` // float in export
	IsLoved        bool                 `json:"is_loved"`
	HoursPlayed    *float64             `json:"hours_played"`
	PersonalNotes  *string              `json:"personal_notes"`
	CreatedAt      *string              `json:"created_at"` // RFC3339
	UpdatedAt      *string              `json:"updated_at"` // RFC3339
	Platforms      []importPlatformData `json:"platforms"`
	Tags           []importTagData      `json:"tags"`
}

type importPlatformData struct {
	PlatformID      string   `json:"platform_id"`
	StorefrontID    string   `json:"storefront_id"`
	StoreGameID     *string  `json:"store_game_id"`
	StoreUrl        *string  `json:"store_url"`
	IsAvailable     bool     `json:"is_available"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"` // date-only or RFC3339
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

// NewImportItemHandler returns a TaskHandler that processes a single import job item.
// It never returns an error — failures are recorded on the JobItem itself.
func NewImportItemHandler(db *bun.DB, igdbClient *igdb.Client, storagePath string) func(ctx context.Context, task *models.PendingTask) error {
	return func(ctx context.Context, task *models.PendingTask) error {
		// ── 1. Parse payload ──────────────────────────────────────────────────
		var payload importPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			slog.Error("import_item: unmarshal payload", "err", err)
			return nil
		}

		// ── 2. Load JobItem ───────────────────────────────────────────────────
		var item models.JobItem
		if err := db.NewSelect().Model(&item).Where("id = ?", payload.JobItemID).Scan(ctx); err != nil {
			slog.Error("import_item: load job_item", "id", payload.JobItemID, "err", err)
			return nil
		}

		// ── 3. Parse game data from source_metadata ───────────────────────────
		var wrapper struct {
			Data importGameData `json:"data"`
		}
		if err := json.Unmarshal(item.SourceMetadata, &wrapper); err != nil {
			markItemFailed(ctx, db, &item, fmt.Sprintf("parse source_metadata: %v", err))
			checkJobCompletion(ctx, db, item.JobID)
			return nil
		}
		gd := wrapper.Data

		// ── 4. Validate igdb_id ───────────────────────────────────────────────
		if gd.IGDBID == 0 {
			markItemFailed(ctx, db, &item, "missing igdb_id")
			checkJobCompletion(ctx, db, item.JobID)
			return nil
		}

		// ── 5. Upsert Game — fetch from IGDB if not already in DB ────────────
		var existingGame models.Game
		gameExists := db.NewSelect().Model(&existingGame).Where("id = ?", gd.IGDBID).Scan(ctx) == nil

		var game *models.Game
		if !gameExists && igdbClient.Configured() {
			md, igdbErr := igdbClient.FetchFullMetadata(ctx, int(gd.IGDBID))
			if igdbErr != nil {
				slog.Warn("import_item: IGDB fetch failed, falling back to JSON data", "igdb_id", gd.IGDBID, "err", igdbErr)
			} else {
				game = igdbMetadataToGame(md)
				if md.CoverArtURL != nil {
					imageID := igdbExtractImageID(*md.CoverArtURL)
					if imageID != "" {
						localURL, dlErr := igdbClient.DownloadCoverArt(ctx, imageID, storagePath)
						if dlErr != nil {
							slog.Warn("import_item: cover art download failed", "igdb_id", gd.IGDBID, "err", dlErr)
						} else {
							game.CoverArtUrl = &localURL
						}
					}
				}
			}
		}

		if game == nil {
			// Fall back to JSON export data (IGDB unconfigured or fetch failed)
			now := time.Now().UTC()
			game = &models.Game{
				ID:            gd.IGDBID,
				Title:         gd.Title,
				Description:   gd.Description,
				Genre:         gd.Genre,
				Developer:     gd.Developer,
				Publisher:     gd.Publisher,
				CoverArtUrl:   gd.CoverArtUrl,
				RatingAverage: gd.RatingAverage,
				LastUpdated:   now,
				CreatedAt:     now,
			}
			if gd.ReleaseDate != nil {
				if t, err := time.Parse(time.RFC3339, *gd.ReleaseDate); err == nil {
					game.ReleaseDate = &t
				}
			}
		}

		var err error
		if !gameExists {
			_, err = db.NewInsert().Model(game).Exec(ctx)
			if err != nil {
				markItemFailed(ctx, db, &item, fmt.Sprintf("insert game: %v", err))
				checkJobCompletion(ctx, db, item.JobID)
				return nil
			}
		}

		// ── 6. Check for existing UserGame ────────────────────────────────────
		var existingUG models.UserGame
		err = db.NewSelect().Model(&existingUG).
			Where("user_id = ? AND game_id = ?", item.UserID, gd.IGDBID).
			Scan(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			markItemFailed(ctx, db, &item, fmt.Sprintf("check existing user_game: %v", err))
			checkJobCompletion(ctx, db, item.JobID)
			return nil
		}
		alreadyExists := err == nil

		// ── 7. Build and insert UserGame (skip if already exists) ────────────
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
			if gd.PersonalRating != nil {
				r := int32(*gd.PersonalRating)
				personalRating = &r
			}

			ug = &models.UserGame{
				ID:             uuid.NewString(),
				UserID:         item.UserID,
				GameID:         gd.IGDBID,
				PlayStatus:     gd.PlayStatus,
				PersonalRating: personalRating,
				IsLoved:        gd.IsLoved,
				HoursPlayed:    gd.HoursPlayed,
				PersonalNotes:  gd.PersonalNotes,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			}
			_, err = db.NewInsert().Model(ug).Exec(ctx)
			if err != nil {
				markItemFailed(ctx, db, &item, fmt.Sprintf("insert user_game: %v", err))
				checkJobCompletion(ctx, db, item.JobID)
				return nil
			}
		}

		// ── 8. Platforms ──────────────────────────────────────────────────────
		// Build a set of existing (platform, storefront) pairs to avoid duplicates
		// when merging into an existing UserGame.
		type platformKey struct{ platform, storefront string }
		existingPlatforms := map[platformKey]bool{}
		if alreadyExists {
			var existingUGPs []models.UserGamePlatform
			if err := db.NewSelect().Model(&existingUGPs).
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

		for _, pd := range gd.Platforms {
			if pd.PlatformID == "" {
				continue
			}

			// Verify platform exists (must be seeded via seed data or migration).
			var platformName string
			if err := db.QueryRowContext(ctx,
				"SELECT name FROM platforms WHERE name = ?", pd.PlatformID,
			).Scan(&platformName); err != nil {
				slog.Warn("import_item: platform not found, skipping (load seed data first)", "platform", pd.PlatformID)
				continue
			}

			// Verify storefront exists (nullable — store NULL if blank or not yet seeded).
			var storefrontPtr *string
			if pd.StorefrontID != "" {
				var storefrontName string
				if err := db.QueryRowContext(ctx,
					"SELECT name FROM storefronts WHERE name = ?", pd.StorefrontID,
				).Scan(&storefrontName); err == nil {
					storefrontPtr = &storefrontName
				} else {
					slog.Warn("import_item: storefront not found, recording platform without storefront (load seed data first)", "storefront", pd.StorefrontID)
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

			ugp := &models.UserGamePlatform{
				ID:              uuid.NewString(),
				UserGameID:      ug.ID,
				Platform:        &platformName,
				Storefront:      storefrontPtr,
				StoreGameID:     pd.StoreGameID,
				StoreUrl:        pd.StoreUrl,
				IsAvailable:     pd.IsAvailable,
				HoursPlayed:     pd.HoursPlayed,
				OwnershipStatus: pd.OwnershipStatus,
				AcquiredDate:    parseFlexibleDate(pd.AcquiredDate),
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if _, err := db.NewInsert().Model(ugp).Exec(ctx); err != nil {
				slog.Error("import_item: insert user_game_platform", "err", err)
			}
		}

		// ── 9. Tags ───────────────────────────────────────────────────────────
		// Build existing tag set to avoid duplicates when merging.
		existingTagIDs := map[string]bool{}
		if alreadyExists {
			var existingUGTs []models.UserGameTag
			if err := db.NewSelect().Model(&existingUGTs).
				Where("user_game_id = ?", ug.ID).Scan(ctx); err == nil {
				for _, ugt := range existingUGTs {
					existingTagIDs[ugt.TagID] = true
				}
			}
		}

		for _, td := range gd.Tags {
			tagID, err := findOrCreateTag(ctx, db, item.UserID, td.Name, td.Color)
			if err != nil {
				markItemFailed(ctx, db, &item, fmt.Sprintf("find/create tag %q: %v", td.Name, err))
				checkJobCompletion(ctx, db, item.JobID)
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
			if _, err := db.NewInsert().Model(ugt).Exec(ctx); err != nil {
				slog.Error("import_item: insert user_game_tag", "err", err)
			}
		}

		// ── 10. Mark item completed ───────────────────────────────────────────
		result := map[string]any{
			"game_id":         gd.IGDBID,
			"user_game_id":    ug.ID,
			"is_new_addition": !alreadyExists,
			"already_exists":  alreadyExists,
		}
		markItemCompleted(ctx, db, &item, result)
		checkJobCompletion(ctx, db, item.JobID)
		return nil
	}
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

// markItemFailed updates the JobItem to failed status with an error message.
func markItemFailed(ctx context.Context, db *bun.DB, item *models.JobItem, errMsg string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusFailed
	item.ErrorMessage = &errMsg
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "error_message", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("import_item: markItemFailed", "id", item.ID, "err", err)
	}
}

// markItemCompleted updates the JobItem to completed status with a result.
func markItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem, result any) {
	now := time.Now().UTC()
	resultJSON, _ := json.Marshal(result)
	item.Status = models.JobItemStatusCompleted
	item.Result = resultJSON
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "result", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("import_item: markItemCompleted", "id", item.ID, "err", err)
	}
}

// igdbMetadataToGame maps an IGDB GameMetadata response to a models.Game ready for insert.
func igdbMetadataToGame(md *igdb.GameMetadata) *models.Game {
	now := time.Now().UTC()
	game := &models.Game{
		ID:                         int32(md.IgdbID),
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
		ids := make([]string, len(md.PlatformIDs))
		for i, id := range md.PlatformIDs {
			ids[i] = strconv.Itoa(id)
		}
		s := strings.Join(ids, ",")
		game.IgdbPlatformIds = &s
	}
	if len(md.PlatformNames) > 0 {
		s := strings.Join(md.PlatformNames, ",")
		game.IgdbPlatformNames = &s
	}
	return game
}

// igdbExtractImageID extracts the bare image ID from an IGDB cover art URL.
func igdbExtractImageID(coverURL string) string {
	parts := strings.Split(coverURL, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSuffix(parts[len(parts)-1], ".jpg")
}

// checkJobCompletion counts remaining pending items and updates the parent Job if done.
func checkJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	var pendingCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = ?",
		jobID, models.JobItemStatusPending,
	).Scan(&pendingCount); err != nil {
		slog.Error("import_item: count pending items", "job_id", jobID, "err", err)
		return
	}

	if pendingCount > 0 {
		return
	}

	// No more pending — determine final job status.
	var failedCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = ?",
		jobID, models.JobItemStatusFailed,
	).Scan(&failedCount); err != nil {
		slog.Error("import_item: count failed items", "job_id", jobID, "err", err)
		return
	}

	finalStatus := models.JobStatusCompleted
	if failedCount > 0 {
		finalStatus = "completed_with_errors"
	}

	now := time.Now().UTC()
	_, err := db.NewRaw(
		"UPDATE jobs SET status = ?, completed_at = ? WHERE id = ?",
		finalStatus, now, jobID,
	).Exec(ctx)
	if err != nil {
		slog.Error("import_item: update job status", "job_id", jobID, "err", err)
	}
}
