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
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
	igdbsvc "github.com/drzero42/nexorious-go/internal/services/igdb"
	"github.com/drzero42/nexorious-go/internal/services/matching"
	"github.com/drzero42/nexorious-go/internal/services/platformresolution"
	psnsvc "github.com/drzero42/nexorious-go/internal/services/psn"
	steamsvc "github.com/drzero42/nexorious-go/internal/services/steam"
)

// syncLibraryEntry is the tasks-package normalised game entry.
// Avoids depending on service-package internal types in task logic.
type syncLibraryEntry struct {
	ExternalID      string
	Title           string
	RawPlatform     string
	PlaytimeHours   int
	OwnershipStatus string
	IsSubscription  bool
}

// SteamLibraryAdapter fetches the Steam game library.
type SteamLibraryAdapter interface {
	GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]steamsvc.ExternalLibraryEntry, error)
}

// PSNLibraryAdapter fetches the PSN game library.
type PSNLibraryAdapter interface {
	GetLibrary(ctx context.Context, npssoToken string) ([]psnsvc.ExternalLibraryEntry, error)
}

// ── DispatchSync ──────────────────────────────────────────────────────────────

// DispatchSyncArgs is the River job args type for "dispatch_sync".
type DispatchSyncArgs struct {
	JobID      string `json:"job_id"`
	UserID     string `json:"user_id"`
	Storefront string `json:"storefront"`
}

func (DispatchSyncArgs) Kind() string { return "dispatch_sync" }

func (DispatchSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 1}
}

// DispatchSyncWorker is a River worker that:
// 1. Marks the job as processing
// 2. Reads credentials from user_sync_configs
// 3. Fetches the library from Steam or PSN
// 4. Upserts external_games rows
// 5. Marks removed games as unavailable
// 6. Dispatches ProcessSyncItem jobs for each non-skipped game
// 7. Updates last_synced_at
type DispatchSyncWorker struct {
	river.WorkerDefaults[DispatchSyncArgs]
	DB          *bun.DB
	Steam       SteamLibraryAdapter
	PSN         PSNLibraryAdapter
	RiverClient *river.Client[pgx.Tx]
}

func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
	p := job.Args

	// ── 1. Mark job as processing ─────────────────────────────────────────
	now := time.Now().UTC()
	_, _ = w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx)

	// ── 2. Read sync credentials ──────────────────────────────────────────
	var cfg models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&cfg).
		Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		failSyncJob(ctx, w.DB, p.JobID, "no sync config found")
		return nil
	}
	if cfg.StorefrontCredentials == nil {
		failSyncJob(ctx, w.DB, p.JobID, "credentials not configured")
		return nil
	}

	// ── 3. Fetch library ──────────────────────────────────────────────────
	var entries []syncLibraryEntry
	switch p.Storefront {
	case "steam":
		var creds struct {
			WebAPIKey string `json:"web_api_key"`
			SteamID   string `json:"steam_id"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
			return nil
		}
		raw, err := w.Steam.GetOwnedGames(ctx, creds.WebAPIKey, creds.SteamID)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("fetch steam library: %v", err))
			return nil
		}
		for _, e := range raw {
			entries = append(entries, syncLibraryEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				RawPlatform:     e.RawPlatform,
				PlaytimeHours:   e.PlaytimeHours,
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  e.IsSubscription,
			})
		}

	case "psn":
		var creds struct {
			NpssoToken string `json:"npsso_token"`
			IsVerified bool   `json:"is_verified"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
			return nil
		}
		if !creds.IsVerified {
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}
		raw, err := w.PSN.GetLibrary(ctx, creds.NpssoToken)
		if err != nil {
			// Mark token as expired in user_sync_configs before failing the job.
			expiredAt := time.Now().UTC()
			newCreds := map[string]any{
				"npsso_token":      creds.NpssoToken,
				"is_verified":      false,
				"token_expired_at": expiredAt,
			}
			if b, merr := json.Marshal(newCreds); merr == nil {
				s := string(b)
				_, _ = w.DB.NewRaw(
					`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
					s, p.UserID,
				).Exec(context.Background())
			}
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}
		for _, e := range raw {
			entries = append(entries, syncLibraryEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				RawPlatform:     e.RawPlatform,
				PlaytimeHours:   e.PlaytimeHours,
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  e.IsSubscription,
			})
		}

	default:
		failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("unknown storefront: %s", p.Storefront))
		return nil
	}

	// ── 4. Upsert external_games ──────────────────────────────────────────
	fetchedIDs := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		fetchedIDs[e.ExternalID] = struct{}{}
		ownership := e.OwnershipStatus
		upsertNow := time.Now().UTC()
		row := &models.ExternalGame{
			ID:              uuid.NewString(),
			UserID:          p.UserID,
			Storefront:      p.Storefront,
			ExternalID:      e.ExternalID,
			Title:           e.Title,
			IsAvailable:     true,
			IsSubscription:  e.IsSubscription,
			PlaytimeHours:   e.PlaytimeHours,
			OwnershipStatus: &ownership,
			CreatedAt:       upsertNow,
			UpdatedAt:       upsertNow,
		}
		_, _ = w.DB.NewInsert().Model(row).
			On("CONFLICT (user_id, storefront, external_id) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
			Exec(ctx)
	}

	// ── 5. Mark removed games as unavailable ──────────────────────────────
	var available []models.ExternalGame
	_ = w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx)
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			_, _ = w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx)
		}
	}

	// ── 6. Dispatch ProcessSyncItem jobs for non-skipped games ────────────
	rawPlatformByExtID := make(map[string]string, len(entries))
	for _, e := range entries {
		rawPlatformByExtID[e.ExternalID] = e.RawPlatform
	}

	var toProcess []models.ExternalGame
	_ = w.DB.NewSelect().Model(&toProcess).
		Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
		Scan(ctx)

	for _, eg := range toProcess {
		rawPlatform := rawPlatformByExtID[eg.ExternalID]
		meta := map[string]string{
			"external_game_id": eg.ID,
			"raw_platform":     rawPlatform,
		}
		metaJSON, _ := json.Marshal(meta)

		itemID := uuid.NewString()

		// Insert job_item — columns match job_items schema exactly.
		_, _ = w.DB.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
			 ON CONFLICT (job_id, item_key) DO NOTHING`,
			itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
		).Exec(ctx)

		// Enqueue River job for ProcessSyncItem.
		_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
	}

	// ── 7. Update last_synced_at ──────────────────────────────────────────
	syncedNow := time.Now().UTC()
	_, _ = w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background())

	return nil
}

// failSyncJob marks a job as failed with the given error message.
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	now := time.Now().UTC()
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`,
		msg, now, jobID,
	).Exec(ctx)
}

// ownershipRank returns a numeric rank for an ownership status string.
// Higher = better. Used to avoid downgrading ownership on update.
func ownershipRank(status string) int {
	switch status {
	case "owned":
		return 4
	case "borrowed", "rented":
		return 3
	case "subscription":
		return 2
	case "no_longer_owned":
		return 1
	default:
		return 0
	}
}

// ── ProcessSyncItem ───────────────────────────────────────────────────────────

// ProcessSyncItemArgs is the River job args type for "process_sync_item".
type ProcessSyncItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ProcessSyncItemArgs) Kind() string { return "process_sync_item" }

func (ProcessSyncItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

// ProcessSyncItemWorker resolves a single sync job item:
// IGDB matching → user_game find-or-create → user_game_platform
// find-or-create with ownership-rank guard → marks the item completed.
type ProcessSyncItemWorker struct {
	river.WorkerDefaults[ProcessSyncItemArgs]
	DB         *bun.DB
	IGDBClient *igdbsvc.Client
}

func (w *ProcessSyncItemWorker) Work(ctx context.Context, job *river.Job[ProcessSyncItemArgs]) error {
	p := job.Args

	// ── 1. Load job_item ──────────────────────────────────────────────────
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("process_sync_item: load job_item", "id", p.JobItemID, "err", err)
		return nil
	}

	// ── 2. Parse source_metadata ──────────────────────────────────────────
	var meta struct {
		ExternalGameID string `json:"external_game_id"`
		RawPlatform    string `json:"raw_platform"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err))
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 3. Load external_game ─────────────────────────────────────────────
	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", meta.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 4. Skipped games ──────────────────────────────────────────────────
	if eg.IsSkipped {
		syncMarkItemSkipped(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 5. IGDB resolution ────────────────────────────────────────────────
	if eg.ResolvedIGDBID == nil && w.IGDBClient != nil && w.IGDBClient.Configured() {
		candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10)
		if err != nil {
			slog.Warn("process_sync_item: igdb search failed", "title", eg.Title, "err", err)
		} else {
			normalizedQuery := matching.NormalizeTitle(eg.Title)
			var bestScore float64
			var bestID int32
			for _, candidate := range candidates {
				score := matching.FuzzyConfidence(normalizedQuery, matching.NormalizeTitle(candidate.Title))
				if score > bestScore {
					bestScore = score
					bestID = int32(candidate.IgdbID)
				}
			}
			if bestScore >= 0.85 {
				// Auto-resolve: persist on external_game and ensure games row exists.
				eg.ResolvedIGDBID = &bestID
				_, _ = w.DB.NewRaw(
					`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
					bestID, eg.ID,
				).Exec(ctx)
				_, _ = w.DB.NewRaw(
					`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
					bestID, eg.Title,
				).Exec(ctx)
			} else {
				// Store candidates and wait for manual review.
				candidatesJSON, _ := json.Marshal(candidates)
				item.IGDBCandidates = candidatesJSON
				syncMarkItemPendingReview(ctx, w.DB, &item)
				syncCheckJobCompletion(ctx, w.DB, item.JobID)
				return nil
			}
		}
	}

	// ── 6. Still no IGDB ID → pending_review ─────────────────────────────
	if eg.ResolvedIGDBID == nil {
		syncMarkItemPendingReview(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 7. Resolve platform and storefront slugs ──────────────────────────
	platformSlug, platformOK := platformresolution.RawPlatformToSlug(meta.RawPlatform)
	storefrontSlug, storefrontOK := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !platformOK || !storefrontOK {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved platform=%s storefront=%s", meta.RawPlatform, eg.Storefront))
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 8. Find or create user_game ───────────────────────────────────────
	var ugID string
	err := w.DB.NewRaw(
		`SELECT id FROM user_games WHERE user_id = ? AND game_id = ?`,
		item.UserID, *eg.ResolvedIGDBID,
	).Scan(ctx, &ugID)
	if err != nil {
		// Not found (or error) — insert; use ON CONFLICT to handle races.
		ugID = uuid.NewString()
		now := time.Now().UTC()
		_, _ = w.DB.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO NOTHING`,
			ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
		).Exec(ctx)
		// Re-fetch to get the winning row ID in case of race.
		if ferr := w.DB.NewRaw(
			`SELECT id FROM user_games WHERE user_id = ? AND game_id = ?`,
			item.UserID, *eg.ResolvedIGDBID,
		).Scan(ctx, &ugID); ferr != nil {
			syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("find/create user_game: %v", ferr))
			syncCheckJobCompletion(ctx, w.DB, item.JobID)
			return nil
		}
	}

	// ── 9. Find or create user_game_platform ─────────────────────────────
	ownership := ""
	if eg.OwnershipStatus != nil {
		ownership = *eg.OwnershipStatus
	} else if eg.IsSubscription {
		ownership = "subscription"
	} else {
		ownership = "owned"
	}

	hoursPlayed := float64(eg.PlaytimeHours)

	var existingUGPID string
	var existingOwnership *string
	ugpErr := w.DB.NewRaw(
		`SELECT id, ownership_status FROM user_game_platforms WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
		ugID, platformSlug, storefrontSlug,
	).Scan(ctx, &existingUGPID, &existingOwnership)

	if errors.Is(ugpErr, sql.ErrNoRows) || ugpErr != nil {
		// Insert new platform row.
		ugpID := uuid.NewString()
		extGameID := eg.ID
		_, _ = w.DB.NewRaw(
			`INSERT INTO user_game_platforms
			 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
			  original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
			 VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			ugpID, ugID, platformSlug, storefrontSlug, hoursPlayed, ownership,
			meta.RawPlatform, eg.Storefront, extGameID,
		).Exec(ctx)
	} else {
		// Update if new ownership rank is higher than existing.
		existingRank := 0
		if existingOwnership != nil {
			existingRank = ownershipRank(*existingOwnership)
		}
		if ownershipRank(ownership) > existingRank {
			_, _ = w.DB.NewRaw(
				`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, updated_at = now() WHERE id = ?`,
				ownership, hoursPlayed, existingUGPID,
			).Exec(ctx)
		} else {
			// Still update hours_played even if ownership rank doesn't improve.
			_, _ = w.DB.NewRaw(
				`UPDATE user_game_platforms SET hours_played = ?, updated_at = now() WHERE id = ?`,
				hoursPlayed, existingUGPID,
			).Exec(ctx)
		}
	}

	// ── 10. Mark item completed ───────────────────────────────────────────
	syncMarkItemCompleted(ctx, w.DB, &item)
	syncCheckJobCompletion(ctx, w.DB, item.JobID)
	return nil
}

// syncMarkItemFailed sets a job_item to failed with an error message.
func syncMarkItemFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusFailed
	item.ErrorMessage = &msg
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "error_message", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemFailed", "id", item.ID, "err", err)
	}
}

// syncMarkItemCompleted sets a job_item to completed.
func syncMarkItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusCompleted
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemCompleted", "id", item.ID, "err", err)
	}
}

// syncMarkItemSkipped sets a job_item to skipped.
func syncMarkItemSkipped(ctx context.Context, db *bun.DB, item *models.JobItem) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusSkipped
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemSkipped", "id", item.ID, "err", err)
	}
}

// syncMarkItemPendingReview sets a job_item to pending_review and persists any IGDB candidates.
func syncMarkItemPendingReview(ctx context.Context, db *bun.DB, item *models.JobItem) {
	item.Status = models.JobItemStatusPendingReview
	_, err := db.NewUpdate().Model(item).
		Column("status", "igdb_candidates").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemPendingReview", "id", item.ID, "err", err)
	}
}

// syncCheckJobCompletion counts job_items still in a non-terminal state for the job.
// If none remain (pending_review blocks completion), it marks the job completed.
func syncCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	var remaining int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'processing', 'pending_review')`,
		jobID,
	).Scan(ctx, &remaining); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion count", "job_id", jobID, "err", err)
		return
	}
	if remaining > 0 {
		return
	}
	now := time.Now().UTC()
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ?`,
		now, jobID,
	).Exec(ctx)
}
