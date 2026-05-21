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

	"github.com/drzero42/nexorious/internal/db/models"
	epicsvc "github.com/drzero42/nexorious/internal/services/epic"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/matching"
	"github.com/drzero42/nexorious/internal/services/platformresolution"
	psnsvc "github.com/drzero42/nexorious/internal/services/psn"
	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
)

// SteamLibraryAdapter fetches the Steam game library.
type SteamLibraryAdapter interface {
	GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]steamsvc.OwnedGame, error)
	GetAppDetailsPlatforms(ctx context.Context, appID int) (steamsvc.Platforms, error)
}

// PSNLibraryAdapter fetches the PSN game library.
type PSNLibraryAdapter interface {
	GetLibrary(ctx context.Context, npssoToken string, batchSize int, onBatch func([]psnsvc.ExternalLibraryEntry) error) error
}

const psnLibraryBatchSize = 10

// EpicLibraryAdapter fetches the Epic Games Store library via Legendary.
type EpicLibraryAdapter interface {
	GetLibrary(
		ctx context.Context,
		userID string,
		onBatch func([]epicsvc.ExternalLibraryEntry) error,
	) error
}

// GOGLibraryAdapter fetches the GOG game library.
type GOGLibraryAdapter interface {
	GetLibrary(ctx context.Context, accessToken string, batchSize int,
		onBatch func([]gogsvc.ExternalLibraryEntry) error) error
	RefreshToken(ctx context.Context, refreshToken string) (*gogsvc.TokenResponse, error)
}

// epicSubprocessClient is the subset of *epicsvc.Client that EpicClientAdapter
// depends on. Declared as an interface so tests can substitute a fake without
// invoking the real legendary subprocess.
type epicSubprocessClient interface {
	Configured() bool
	RestoreSnapshot(userID string, snapshot map[string]string) error
	GetLibrary(ctx context.Context, userID string, onBatch func([]epicsvc.ExternalLibraryEntry) error) error
	CaptureSnapshot(userID string) (map[string]string, error)
}

// EpicClientAdapter implements EpicLibraryAdapter.
// It loads the legendary state snapshot from the DB, restores it to disk,
// calls the epic.Client, then captures and persists the updated snapshot.
type EpicClientAdapter struct {
	Client epicSubprocessClient
	DB     *bun.DB
}

func (a *EpicClientAdapter) GetLibrary(ctx context.Context, userID string, onBatch func([]epicsvc.ExternalLibraryEntry) error) error {
	if !a.Client.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}

	// 1. Load snapshot from DB.
	var snapshotJSON []byte
	if err := a.DB.NewRaw(
		`SELECT epic_legendary_state FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Scan(ctx, &snapshotJSON); err != nil || len(snapshotJSON) == 0 {
		return fmt.Errorf("epic: no legendary state found for user (not connected)")
	}
	var snapshot map[string]string
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return fmt.Errorf("epic: unmarshal legendary state: %w", err)
	}

	// 2. Restore snapshot to disk.
	if err := a.Client.RestoreSnapshot(userID, snapshot); err != nil {
		return fmt.Errorf("epic: restore snapshot: %w", err)
	}

	// 3. Fetch library.
	fetchErr := a.Client.GetLibrary(ctx, userID, onBatch)

	// 4. Capture updated snapshot regardless of fetch error.
	newSnapshot, captureErr := a.Client.CaptureSnapshot(userID)
	if captureErr != nil {
		slog.Error("epic: capture snapshot after GetLibrary failed", "user_id", userID, "err", captureErr)
	} else if len(newSnapshot) > 0 {
		newJSON, _ := json.Marshal(newSnapshot)
		if _, err := a.DB.NewRaw(
			`UPDATE user_sync_configs SET epic_legendary_state = ?, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
			string(newJSON), userID,
		).Exec(context.Background()); err != nil {
			slog.Error("epic: persist updated snapshot failed", "user_id", userID, "err", err)
		}
	}

	return fetchErr
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
	Epic        EpicLibraryAdapter
	GOG         GOGLibraryAdapter
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
	if cfg.StorefrontCredentials == nil && p.Storefront != "epic" {
		failSyncJob(ctx, w.DB, p.JobID, "credentials not configured")
		return nil
	}

	// fetchedIDs accumulates all external IDs seen in the fetch; used by step 5.
	fetchedIDs := make(map[string]struct{})

	// ── 3+4+6. Fetch library, upsert external_games, dispatch items ───────
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

		rawPlatformByExtID := make(map[string]string, len(raw))
		for _, e := range raw {
			extID := fmt.Sprintf("%d", e.AppID)
			fetchedIDs[extID] = struct{}{}
			rawPlatformByExtID[extID] = "pc-windows"
			ownership := "owned"
			upsertNow := time.Now().UTC()
			row := &models.ExternalGame{
				ID:              uuid.NewString(),
				UserID:          p.UserID,
				Storefront:      p.Storefront,
				ExternalID:      extID,
				Title:           e.Title,
				IsAvailable:     true,
				IsSubscription:  false,
				PlaytimeHours:   e.PlaytimeHours,
				OwnershipStatus: &ownership,
				RawPlatform:     "pc-windows",
				CreatedAt:       upsertNow,
				UpdatedAt:       upsertNow,
			}
			_, _ = w.DB.NewInsert().Model(row).
				On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
				Exec(ctx)
		}

		var toProcess []models.ExternalGame
		_ = w.DB.NewSelect().Model(&toProcess).
			Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
			Scan(ctx)
		for _, eg := range toProcess {
			metaJSON, _ := json.Marshal(map[string]any{
				"external_game_id": eg.ID,
				"raw_platform":     rawPlatformByExtID[eg.ExternalID],
				"playtime_hours":   eg.PlaytimeHours,
			})
			itemID := uuid.NewString()
			_, _ = w.DB.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				 ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
			).Exec(ctx)
			if w.RiverClient != nil {
				_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
			}
		}

	case "psn":
		var psnCreds struct {
			NpssoToken string `json:"npsso_token"`
			IsVerified bool   `json:"is_verified"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &psnCreds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
			return nil
		}
		if !psnCreds.IsVerified {
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}

		slog.Info("dispatch_sync: starting psn library fetch", "job_id", p.JobID, "user_id", p.UserID)
		if err := w.PSN.GetLibrary(ctx, psnCreds.NpssoToken, psnLibraryBatchSize,
			func(batch []psnsvc.ExternalLibraryEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: psn batch received", "job_id", p.JobID, "batch_size", len(batch))
				rawPlatformByExtID := make(map[string]string, len(batch))
				batchExtIDs := make([]string, 0, len(batch))
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					batchExtIDs = append(batchExtIDs, e.ExternalID)
					rawPlatformByExtID[e.ExternalID] = e.RawPlatform
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
						RawPlatform:     e.RawPlatform,
						CreatedAt:       upsertNow,
						UpdatedAt:       upsertNow,
					}
					if _, err := w.DB.NewInsert().Model(row).
						On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
						Exec(ctx); err != nil {
						slog.Error("dispatch_sync: psn upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}
				}

				// Re-query only this batch to get DB state (is_skipped, id).
				// is_skipped is preserved by the ON CONFLICT clause above.
				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: psn re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: psn batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess), "batch_size", len(batch))

				for _, eg := range toProcess {
					metaJSON, _ := json.Marshal(map[string]any{
						"external_game_id": eg.ID,
						"raw_platform":     rawPlatformByExtID[eg.ExternalID],
						"playtime_hours":   eg.PlaytimeHours,
					})
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(
						`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						 ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: psn insert job_item failed", "job_id", p.JobID, "external_id", eg.ExternalID, "err", err)
					}
					if w.RiverClient != nil {
						if _, err := w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
							slog.Error("dispatch_sync: psn river insert failed", "job_id", p.JobID, "item_id", itemID, "err", err)
						}
					}
				}
				return nil
			},
		); err != nil {
			if errors.Is(err, psnsvc.ErrInvalidNPSSOToken) {
				expiredAt := time.Now().UTC()
				newCreds := map[string]any{
					"npsso_token":      psnCreds.NpssoToken,
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
			} else {
				slog.Error("dispatch_sync: psn library fetch failed", "job_id", p.JobID, "err", err)
				failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("psn_fetch_error: %v", err))
			}
			return nil
		}

	case "epic":
		if w.Epic == nil {
			failSyncJob(ctx, w.DB, p.JobID, "Epic sync not configured (LEGENDARY_WORK_DIR unset)")
			return nil
		}
		slog.Info("dispatch_sync: starting epic library fetch", "job_id", p.JobID, "user_id", p.UserID)
		if err := w.Epic.GetLibrary(ctx, p.UserID,
			func(batch []epicsvc.ExternalLibraryEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: epic batch received", "job_id", p.JobID, "batch_size", len(batch))
				batchExtIDs := make([]string, 0, len(batch))
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					batchExtIDs = append(batchExtIDs, e.ExternalID)
					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()
					row := &models.ExternalGame{
						ID:              uuid.NewString(),
						UserID:          p.UserID,
						Storefront:      p.Storefront,
						ExternalID:      e.ExternalID,
						Title:           e.Title,
						IsAvailable:     true,
						IsSubscription:  false,
						OwnershipStatus: &ownership,
						RawPlatform:     "pc-windows",
						CreatedAt:       upsertNow,
						UpdatedAt:       upsertNow,
					}
					if _, err := w.DB.NewInsert().Model(row).
						On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
						Exec(ctx); err != nil {
						slog.Error("dispatch_sync: epic upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}
				}

				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: epic re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: epic batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess), "batch_size", len(batch))

				for _, eg := range toProcess {
					metaJSON, _ := json.Marshal(map[string]any{
						"external_game_id": eg.ID,
						"raw_platform":     "pc-windows",
					})
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(
						`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						 ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: epic insert job_item failed", "job_id", p.JobID, "external_id", eg.ExternalID, "err", err)
					}
					if w.RiverClient != nil {
						if _, err := w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
							slog.Error("dispatch_sync: epic river insert failed", "job_id", p.JobID, "item_id", itemID, "err", err)
						}
					}
				}
				return nil
			},
		); err != nil {
			slog.Error("dispatch_sync: epic library fetch failed", "job_id", p.JobID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("epic_fetch_error: %v", err))
			return nil
		}

	case "gog":
		if w.GOG == nil {
			failSyncJob(ctx, w.DB, p.JobID, "GOG sync not available")
			return nil
		}

		var creds struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			UserID       string `json:"user_id"`
			Username     string `json:"username"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid gog credentials")
			return nil
		}

		// Refresh the access token upfront; GOG tokens expire in ~1h.
		// Always persist the new tokens — refresh tokens may rotate.
		newTok, err := w.GOG.RefreshToken(ctx, creds.RefreshToken)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("gog token refresh failed: %v", err))
			return nil
		}
		creds.AccessToken = newTok.AccessToken
		creds.RefreshToken = newTok.RefreshToken
		if newCredsJSON, merr := json.Marshal(creds); merr == nil {
			_, _ = w.DB.NewRaw(
				`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
				string(newCredsJSON), p.UserID,
			).Exec(context.Background())
		}

		slog.Info("dispatch_sync: starting gog library fetch", "job_id", p.JobID, "user_id", p.UserID)
		const gogBatchSize = 50
		if err := w.GOG.GetLibrary(ctx, creds.AccessToken, gogBatchSize,
			func(batch []gogsvc.ExternalLibraryEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: gog batch received", "job_id", p.JobID, "batch_size", len(batch))

				batchExtIDs := make([]string, 0, len(batch))
				seen := make(map[string]struct{})
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					if _, ok := seen[e.ExternalID]; !ok {
						batchExtIDs = append(batchExtIDs, e.ExternalID)
						seen[e.ExternalID] = struct{}{}
					}

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
						RawPlatform:     e.RawPlatform,
						CreatedAt:       upsertNow,
						UpdatedAt:       upsertNow,
					}
					if _, err := w.DB.NewInsert().Model(row).
						On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
						Exec(ctx); err != nil {
						slog.Error("dispatch_sync: gog upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}
				}

				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: gog re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: gog batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess))

				for _, eg := range toProcess {
					// item_key includes raw_platform to ensure uniqueness for
					// dual-platform games (same ExternalID, different platform).
					itemKey := eg.ExternalID + ":" + eg.RawPlatform
					metaJSON, _ := json.Marshal(map[string]any{
						"external_game_id": eg.ID,
						"raw_platform":     eg.RawPlatform,
						"playtime_hours":   eg.PlaytimeHours,
					})
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(
						`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						 ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, itemKey, eg.Title, string(metaJSON),
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: gog insert job_item failed", "job_id", p.JobID, "external_id", eg.ExternalID, "err", err)
					}
					if w.RiverClient != nil {
						if _, err := w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
							slog.Error("dispatch_sync: gog river insert failed", "job_id", p.JobID, "item_id", itemID, "err", err)
						}
					}
				}
				return nil
			},
		); err != nil {
			slog.Error("dispatch_sync: gog library fetch failed", "job_id", p.JobID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("gog_fetch_error: %v", err))
			return nil
		}

	default:
		failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("unknown storefront: %s", p.Storefront))
		return nil
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
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *ProcessSyncItemWorker) Work(ctx context.Context, job *river.Job[ProcessSyncItemArgs]) error {
	p := job.Args

	// ── 1. Load job_item ──────────────────────────────────────────────────
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("process_sync_item: load job_item", "id", p.JobItemID, "err", err)
		return err
	}

	// ── 2. Parse source_metadata ──────────────────────────────────────────
	var meta struct {
		ExternalGameID string `json:"external_game_id"`
		RawPlatform    string `json:"raw_platform"`
		PlaytimeHours  int    `json:"playtime_hours"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// ── 3. Load external_game ─────────────────────────────────────────────
	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", meta.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// ── 3.5. Apply manual IGDB resolution ────────────────────────────────
	// HandleResolveItem stores the user's chosen IGDB ID on job_items but does
	// not update external_games (it doesn't know the game title). Apply it here
	// so the IGDB search step below is skipped on re-processing.
	if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil {
		igdbID := int32(*item.ResolvedIGDBID)
		eg.ResolvedIGDBID = &igdbID
		_, _ = w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx)
		_, _ = w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx)
	}

	// ── 3.6. Cross-SKU IGDB resolution ───────────────────────────────────
	// The same game can appear under multiple SKUs (e.g. CUSA/PS4 and PPSA/PS5).
	// If a sibling external_games row for the same user/storefront/title is
	// already resolved, inherit that IGDB ID so the new SKU skips IGDB search.
	if eg.ResolvedIGDBID == nil {
		var sibling models.ExternalGame
		if err := w.DB.NewSelect().Model(&sibling).
			Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NOT NULL",
				eg.UserID, eg.Storefront, eg.Title, eg.ID).
			Limit(1).
			Scan(ctx); err == nil && sibling.ResolvedIGDBID != nil {
			igdbID := *sibling.ResolvedIGDBID
			eg.ResolvedIGDBID = &igdbID
			_, _ = w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				igdbID, eg.Title,
			).Exec(ctx)
			_, _ = w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				igdbID, eg.ID,
			).Exec(ctx)
		}
	}

	// ── 4. Skipped games ──────────────────────────────────────────────────
	if eg.IsSkipped {
		syncMarkItemSkipped(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// ── 5. IGDB resolution ────────────────────────────────────────────────
	if eg.ResolvedIGDBID == nil && w.IGDBClient != nil && w.IGDBClient.Configured() {
		candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10)
		if err != nil {
			msg := fmt.Sprintf("igdb search failed: %v", err)
			syncMarkItemIGDBFailed(ctx, w.DB, &item, msg)
			syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
			return nil
		}
		normalizedQuery := matching.NormalizeTitle(eg.Title)
		var bestScore, secondBestScore float64
		var bestID int32
		for _, candidate := range candidates {
			score := matching.FuzzyConfidence(normalizedQuery, matching.NormalizeTitle(candidate.Title))
			if score > bestScore {
				secondBestScore = bestScore
				bestScore = score
				bestID = int32(candidate.IgdbID)
			} else if score > secondBestScore {
				secondBestScore = score
			}
		}
		// Require a clear winner: if two candidates score within 0.01 of each
		// other the match is ambiguous and manual review is safer.
		const autoResolveThreshold = 0.85
		const tieEpsilon = 0.01
		if bestScore >= autoResolveThreshold && (bestScore-secondBestScore) > tieEpsilon {
			// Auto-resolve: insert the games row first (FK constraint on
			// external_games.resolved_igdb_id references games.id), then link.
			eg.ResolvedIGDBID = &bestID
			_, _ = w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx)
			_, _ = w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx)
		} else {
			// Store candidates and wait for manual review.
			candidatesJSON, _ := json.Marshal(candidates)
			item.IGDBCandidates = candidatesJSON
			item.MatchConfidence = &bestScore
			syncMarkItemPendingReview(ctx, w.DB, &item)
			syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
			return nil
		}
	}

	// ── 6. Still no IGDB ID → pending_review ─────────────────────────────
	if eg.ResolvedIGDBID == nil {
		syncMarkItemPendingReview(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// ── 7. Resolve platform and storefront slugs ──────────────────────────
	platformSlug, platformOK := platformresolution.RawPlatformToSlug(meta.RawPlatform)
	storefrontSlug, storefrontOK := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !platformOK || !storefrontOK {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved platform=%s storefront=%s", meta.RawPlatform, eg.Storefront))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
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
			`INSERT INTO user_games (id, user_id, game_id, hours_played, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO NOTHING`,
			ugID, item.UserID, *eg.ResolvedIGDBID, float64(meta.PlaytimeHours), now, now,
		).Exec(ctx)
		// Re-fetch to get the winning row ID in case of race.
		if ferr := w.DB.NewRaw(
			`SELECT id FROM user_games WHERE user_id = ? AND game_id = ?`,
			item.UserID, *eg.ResolvedIGDBID,
		).Scan(ctx, &ugID); ferr != nil {
			syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("find/create user_game: %v", ferr))
			syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
			return nil
		}
	}
	if meta.PlaytimeHours > 0 {
		_, _ = w.DB.NewRaw(
			`UPDATE user_games SET hours_played = ?, updated_at = now() WHERE id = ? AND (hours_played IS NULL OR hours_played < ?)`,
			float64(meta.PlaytimeHours), ugID, float64(meta.PlaytimeHours),
		).Exec(ctx)
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
	syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
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

// syncMarkItemIGDBFailed sets a job_item to igdb_failed for IGDB API errors.
func syncMarkItemIGDBFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusIGDBFailed
	item.ErrorMessage = &msg
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "error_message", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemIGDBFailed", "id", item.ID, "err", err)
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
		Column("status", "igdb_candidates", "match_confidence").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemPendingReview", "id", item.ID, "err", err)
	}
}

// syncCheckJobCompletion checks whether active processing of job_items is complete and
// drives the job to its terminal state.
//
// "Active" means pending or processing — pending_review items require user action and
// do not block this check (they sit in the review queue indefinitely until resolved).
//
// Once no active items remain it checks for igdb_failed items:
//   - auto_retry_done=false: resets them to pending, re-enqueues, sets auto_retry_done=true.
//   - auto_retry_done=true: marks job completed_with_errors.
//
// If no igdb_failed items remain:
//   - pending_review items still exist: job stays processing (user must review).
//   - No pending_review items: marks job completed.
func syncCheckJobCompletion(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobID string) {
	var activeRemaining int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'processing')`,
		jobID,
	).Scan(ctx, &activeRemaining); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion count", "job_id", jobID, "err", err)
		return
	}
	if activeRemaining > 0 {
		return
	}

	var igdbFailedCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'igdb_failed'`,
		jobID,
	).Scan(ctx, &igdbFailedCount); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion igdb_failed count", "job_id", jobID, "err", err)
		return
	}

	if igdbFailedCount > 0 {
		var autoRetryDone bool
		if err := db.NewRaw(`SELECT auto_retry_done FROM jobs WHERE id = ?`, jobID).
			Scan(ctx, &autoRetryDone); err != nil {
			slog.Error("process_sync_item: syncCheckJobCompletion auto_retry_done", "job_id", jobID, "err", err)
			return
		}

		if !autoRetryDone {
			type itemRow struct {
				ID string `bun:"id"`
			}
			var resetItems []itemRow
			if err := db.NewRaw(
				`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL
				 WHERE job_id = ? AND status = 'igdb_failed'
				 RETURNING id`,
				jobID,
			).Scan(ctx, &resetItems); err != nil {
				slog.Error("process_sync_item: syncCheckJobCompletion reset igdb_failed", "job_id", jobID, "err", err)
				return
			}

			_, _ = db.NewRaw(`UPDATE jobs SET auto_retry_done = true WHERE id = ?`, jobID).Exec(ctx)

			// EnqueueOrFail keeps job_items.status='pending' and river_job in
			// lockstep: if the insert can't happen (nil client, River outage),
			// the item is moved to 'failed' so it doesn't get stranded.
			enqueueFailures := 0
			for _, item := range resetItems {
				if err := EnqueueOrFail(ctx, db, rc, item.ID, ProcessSyncItemArgs{JobItemID: item.ID}); err != nil {
					slog.Error("process_sync_item: auto-retry enqueue failed",
						"job_id", jobID, "item_id", item.ID, "err", err)
					enqueueFailures++
				}
			}
			// If every reset item just got marked failed, the parent job is now
			// settled and needs its completion check; without this it would sit
			// in 'processing' forever.
			if enqueueFailures == len(resetItems) && len(resetItems) > 0 {
				now := time.Now().UTC()
				_, _ = db.NewRaw(
					`UPDATE jobs SET status = 'completed_with_errors', completed_at = ?
					 WHERE id = ? AND status IN ('pending', 'processing')`,
					now, jobID,
				).Exec(ctx)
			}
			return
		}

		now := time.Now().UTC()
		_, _ = db.NewRaw(
			`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
			now, jobID,
		).Exec(ctx)
		return
	}

	// No igdb_failed items. If pending_review items still exist the job stays
	// processing — user must resolve them via the review queue.
	var pendingReviewCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'pending_review'`,
		jobID,
	).Scan(ctx, &pendingReviewCount); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion pending_review count", "job_id", jobID, "err", err)
		return
	}
	if pendingReviewCount > 0 {
		return
	}

	now := time.Now().UTC()
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
		now, jobID,
	).Exec(ctx)
}
