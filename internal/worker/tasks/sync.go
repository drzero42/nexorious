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

	"github.com/drzero42/nexorious/internal/crypto"
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
	Client    epicSubprocessClient
	DB        *bun.DB
	Encrypter *crypto.Encrypter
}

func (a *EpicClientAdapter) GetLibrary(ctx context.Context, userID string, onBatch func([]epicsvc.ExternalLibraryEntry) error) error {
	if !a.Client.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}

	// 1. Load snapshot from DB.
	var ciphertextStr string
	if err := a.DB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Scan(ctx, &ciphertextStr); err != nil || ciphertextStr == "" {
		return fmt.Errorf("epic: no legendary state found for user (not connected)")
	}
	plainState, err := a.Encrypter.Decrypt(ciphertextStr)
	if err != nil {
		slog.Warn("epic: legendary state decrypt failed", "user_id", userID, "err", err)
		return fmt.Errorf("epic: legendary state decrypt failed")
	}
	var snapshot map[string]string
	if err := json.Unmarshal(plainState, &snapshot); err != nil {
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
		newPlainJSON, _ := json.Marshal(newSnapshot)
		newCiphertext, encErr := a.Encrypter.Encrypt(newPlainJSON)
		if encErr != nil {
			slog.Error("epic: encrypt updated snapshot failed", "user_id", userID, "err", encErr)
		} else {
			if _, err := a.DB.NewRaw(
				`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
				newCiphertext, userID,
			).Exec(context.Background()); err != nil {
				slog.Error("epic: persist updated snapshot failed", "user_id", userID, "err", err)
			}
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

// Timeout overrides River's 1-minute default so large libraries (hundreds of
// games needing sequential appdetails calls) can complete in a single run.
func (w *DispatchSyncWorker) Timeout(*river.Job[DispatchSyncArgs]) time.Duration { return -1 }

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
	Encrypter   *crypto.Encrypter
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
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
	}

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
		plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
		if err != nil {
			slog.Warn("dispatch_sync: steam credentials decrypt failed", "user_id", p.UserID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
			return nil
		}
		var creds struct {
			WebAPIKey string `json:"web_api_key"`
			SteamID   string `json:"steam_id"`
		}
		if err := json.Unmarshal(plainCreds, &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
			return nil
		}
		owned, err := w.Steam.GetOwnedGames(ctx, creds.WebAPIKey, creds.SteamID)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("fetch steam library: %v", err))
			return nil
		}
		slog.Debug("dispatch_sync: steam owned games fetched", "count", len(owned), "user_id", p.UserID)

		// Global backoff state shared across the game loop. When Steam rate-limits us,
		// we sleep once (lifting the limit for all subsequent games) rather than skipping.
		// Two attempts: 2 minutes, then 5 minutes. Skipping is the last resort.
		steamGlobalBackoffs := []time.Duration{2 * time.Minute, 5 * time.Minute}
		steamGlobalBackoffIdx := 0

		for _, og := range owned {
			appidStr := fmt.Sprintf("%d", og.AppID)
			fetchedIDs[appidStr] = struct{}{}

			ownership := "owned"
			upsertNow := time.Now().UTC()

			var egRow struct {
				ID        string `bun:"id"`
				IsSkipped bool   `bun:"is_skipped"`
			}
			if err := w.DB.NewRaw(`
				INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, true, false, ?, ?, ?)
				ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
					title = EXCLUDED.title,
					is_subscription = EXCLUDED.is_subscription,
					ownership_status = EXCLUDED.ownership_status,
					is_available = true,
					updated_at = now()
				RETURNING id, is_skipped`,
				uuid.NewString(), p.UserID, p.Storefront, appidStr, og.Title,
				&ownership, upsertNow, upsertNow,
			).Scan(ctx, &egRow); err != nil {
				slog.Error("dispatch_sync: steam upsert external_game failed", "err", err, "job_id", p.JobID, "external_id", appidStr)
				continue
			}
			egID := egRow.ID

			// Fetch platforms from appdetails to keep data current.
			// On ErrRateLimited the client already did one brief retry; we do a longer
			// global backoff here (shared across the loop) and retry the same game.
			// On a non-rate-limit error we skip platform updates for this game only.
			pl, detErr := w.Steam.GetAppDetailsPlatforms(ctx, og.AppID)
			if detErr != nil {
				if ctx.Err() != nil {
					slog.Warn("dispatch_sync: steam loop exiting early — context cancelled", "ctx_err", ctx.Err(), "appid", og.AppID, "appdetails_err", detErr, "job_id", p.JobID)
					failSyncJob(context.Background(), w.DB, p.JobID, fmt.Sprintf("sync cancelled: %v", ctx.Err()))
					return ctx.Err()
				}
				if errors.Is(detErr, steamsvc.ErrRateLimited) && steamGlobalBackoffIdx < len(steamGlobalBackoffs) {
					d := steamGlobalBackoffs[steamGlobalBackoffIdx]
					steamGlobalBackoffIdx++
					slog.Warn("dispatch_sync: steam rate limited, waiting for limit to lift", "wait", d, "appid", og.AppID, "job_id", p.JobID)
					timer := time.NewTimer(d)
					var sleepErr error
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						sleepErr = ctx.Err()
					}
					if sleepErr != nil {
						failSyncJob(context.Background(), w.DB, p.JobID, fmt.Sprintf("sync cancelled: %v", sleepErr))
						return sleepErr
					}
					pl, detErr = w.Steam.GetAppDetailsPlatforms(ctx, og.AppID)
				}
				if detErr != nil {
					if ctx.Err() != nil {
						slog.Warn("dispatch_sync: steam loop exiting early — context cancelled", "ctx_err", ctx.Err(), "appid", og.AppID, "appdetails_err", detErr, "job_id", p.JobID)
						failSyncJob(context.Background(), w.DB, p.JobID, fmt.Sprintf("sync cancelled: %v", ctx.Err()))
						return ctx.Err()
					}
					slog.Warn("dispatch_sync: steam appdetails failed after retry, skipping platform update for this game", "appid", og.AppID, "err", detErr, "job_id", p.JobID)
					continue
				}
			}

			var resolvedPlatforms []string
			if pl.Windows {
				resolvedPlatforms = append(resolvedPlatforms, "pc-windows")
			}
			if pl.Mac {
				resolvedPlatforms = append(resolvedPlatforms, "mac")
			}
			if pl.Linux {
				resolvedPlatforms = append(resolvedPlatforms, "pc-linux")
			}
			if len(resolvedPlatforms) == 0 {
				resolvedPlatforms = []string{"pc-windows"}
			}

			for i, platform := range resolvedPlatforms {
				platformHours := 0.0
				if i == 0 {
					platformHours = float64(og.PlaytimeHours)
				}
				if _, err := w.DB.NewRaw(`
					INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
					VALUES (?, ?, ?, ?, now())
					ON CONFLICT (external_game_id, platform) DO UPDATE SET
						hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
					uuid.NewString(), egID, platform, platformHours,
				).Exec(ctx); err != nil {
					slog.Error("dispatch_sync: steam upsert platform failed", "err", err, "job_id", p.JobID, "external_id", appidStr, "platform", platform)
				}
			}

			if _, err := w.DB.NewRaw(`
				DELETE FROM external_game_platforms
				WHERE external_game_id = ? AND platform NOT IN (?)`,
				egID, bun.List(resolvedPlatforms),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: steam delete stale platforms failed", "err", err, "job_id", p.JobID, "external_id", appidStr)
			}

			// Insert the job_item as 'pending' as soon as platform data is written so
			// it is visible in the DB (and in the UI) without waiting for the full loop.
			// The River enqueue is deferred to after the loop so that no worker can call
			// syncCheckJobCompletion before all items have been inserted as 'pending' —
			// otherwise a fast early match could prematurely complete the job.
			// Skip user-skipped games (is_skipped survives the ON CONFLICT DO UPDATE).
			if egRow.IsSkipped {
				continue
			}
			itemID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
				VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
				ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, appidStr, og.Title, egID,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: steam insert job_item failed", "err", err, "job_id", p.JobID, "external_id", appidStr)
			}
		}

		// All job_items are now in the DB as 'pending'. Enqueue them to River in one
		// pass so workers only start after every item exists — this prevents a fast
		// early match from seeing activeRemaining=0 and prematurely completing the job.
		var steamPending []struct {
			ID string `bun:"id"`
		}
		if err := w.DB.NewRaw(
			`SELECT id FROM job_items WHERE job_id = ? AND status = 'pending'`,
			p.JobID,
		).Scan(ctx, &steamPending); err != nil {
			slog.Error("dispatch_sync: steam query pending items failed", "err", err, "job_id", p.JobID)
		}
		slog.Debug("dispatch_sync: steam enqueuing items", "count", len(steamPending), "job_id", p.JobID)
		for _, item := range steamPending {
			if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, ProcessSyncItemArgs{JobItemID: item.ID}); err != nil {
				slog.Error("dispatch_sync: steam enqueue failed", "err", err, "job_id", p.JobID, "item_id", item.ID)
			}
		}

	case "psn":
		plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
		if err != nil {
			slog.Warn("dispatch_sync: psn credentials decrypt failed", "user_id", p.UserID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
			return nil
		}
		var psnCreds struct {
			NpssoToken string `json:"npsso_token"`
			IsVerified bool   `json:"is_verified"`
		}
		if err := json.Unmarshal(plainCreds, &psnCreds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
			return nil
		}
		if !psnCreds.IsVerified {
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}

		slog.Info("dispatch_sync: starting psn library fetch", "job_id", p.JobID, "user_id", p.UserID)

		// seenPSNPlatforms tracks canonical platform slugs seen per external_game_id
		// across all batches, for end-of-stream reconciliation.
		seenPSNPlatforms := make(map[string][]string)

		if err := w.PSN.GetLibrary(ctx, psnCreds.NpssoToken, psnLibraryBatchSize,
			func(batch []psnsvc.ExternalLibraryEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: psn batch received", "job_id", p.JobID, "batch_size", len(batch))
				batchExtIDs := make([]string, 0, len(batch))
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					batchExtIDs = append(batchExtIDs, e.ExternalID)

					platform, ok := platformresolution.RawPlatformToSlug(e.RawPlatform)
					if !ok {
						slog.Error("dispatch_sync: psn unknown platform, using default", "storefront_platform", e.RawPlatform, "external_id", e.ExternalID)
						platform = "playstation-4"
					}

					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()

					var egID string
					if err := w.DB.NewRaw(`
						INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						e.IsSubscription, &ownership, upsertNow, upsertNow,
					).Scan(ctx, &egID); err != nil {
						slog.Error("dispatch_sync: psn upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
						continue
					}

					seenPSNPlatforms[egID] = append(seenPSNPlatforms[egID], platform)

					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
						VALUES (?, ?, ?, ?, now())
						ON CONFLICT (external_game_id, platform) DO UPDATE SET
							hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
						uuid.NewString(), egID, platform, e.PlaytimeHours,
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: psn upsert platform failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}
				}

				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: psn re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: psn batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess), "batch_size", len(batch))

				for _, eg := range toProcess {
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(`
						INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
						VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
						ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, eg.ID,
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
					enc, encErr := w.Encrypter.Encrypt(b)
					if encErr != nil {
						slog.Error("dispatch_sync: encrypt expired psn token failed", "err", encErr, "job_id", p.JobID)
					} else if _, err := w.DB.NewRaw(
						`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
						enc, p.UserID,
					).Exec(context.Background()); err != nil {
						slog.Error("dispatch_sync: persist expired psn token failed", "err", err, "job_id", p.JobID)
					}
				}
				failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			} else {
				slog.Error("dispatch_sync: psn library fetch failed", "job_id", p.JobID, "err", err)
				failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("psn_fetch_error: %v", err))
			}
			return nil
		}

		// Reconcile: delete platform rows no longer present in the upstream library.
		for egID, platforms := range seenPSNPlatforms {
			if _, err := w.DB.NewRaw(`
				DELETE FROM external_game_platforms
				WHERE external_game_id = ? AND platform NOT IN (?)`,
				egID, bun.List(platforms),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: psn delete stale platforms failed", "err", err, "job_id", p.JobID, "external_game_id", egID)
			}
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

					var egID string
					if err := w.DB.NewRaw(`
						INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, false, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						&ownership, upsertNow, upsertNow,
					).Scan(ctx, &egID); err != nil {
						slog.Error("dispatch_sync: epic upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
						continue
					}

					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
						VALUES (?, ?, 'pc-windows', 0, now())
						ON CONFLICT (external_game_id, platform) DO UPDATE SET
							hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
						uuid.NewString(), egID,
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: epic upsert platform failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
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
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(`
						INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
						VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
						ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, eg.ID,
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

		plainGOGCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
		if err != nil {
			slog.Warn("dispatch_sync: gog credentials decrypt failed", "user_id", p.UserID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
			return nil
		}
		var creds struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			UserID       string `json:"user_id"`
			Username     string `json:"username"`
		}
		if err := json.Unmarshal(plainGOGCreds, &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid gog credentials")
			return nil
		}

		newTok, err := w.GOG.RefreshToken(ctx, creds.RefreshToken)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("gog token refresh failed: %v", err))
			return nil
		}
		creds.AccessToken = newTok.AccessToken
		creds.RefreshToken = newTok.RefreshToken
		if newCredsJSON, merr := json.Marshal(creds); merr == nil {
			enc, encErr := w.Encrypter.Encrypt(newCredsJSON)
			if encErr != nil {
				slog.Error("dispatch_sync: encrypt refreshed gog token failed", "err", encErr, "job_id", p.JobID)
			} else {
				if _, err := w.DB.NewRaw(
					`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
					enc, p.UserID,
				).Exec(context.Background()); err != nil {
					slog.Error("dispatch_sync: persist refreshed gog token failed", "err", err, "job_id", p.JobID)
				}
			}
		}

		// seenEGPlatforms tracks which canonical platform slugs were seen per
		// external_game_id across all batches, for end-of-stream reconciliation.
		seenEGPlatforms := make(map[string][]string)

		slog.Info("dispatch_sync: starting gog library fetch", "job_id", p.JobID, "user_id", p.UserID)
		const gogBatchSize = 50
		if err := w.GOG.GetLibrary(ctx, creds.AccessToken, gogBatchSize,
			func(batch []gogsvc.ExternalLibraryEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: gog batch received", "job_id", p.JobID, "batch_size", len(batch))

				// dispatchedInBatch deduplicates job_item dispatch within this batch.
				dispatchedInBatch := make(map[string]struct{})

				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}

					platform, ok := platformresolution.RawPlatformToSlug(e.RawPlatform)
					if !ok {
						slog.Error("dispatch_sync: gog unknown platform, using default", "storefront_platform", e.RawPlatform, "external_id", e.ExternalID)
						platform = "pc-windows"
					}

					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()

					var egID string
					if err := w.DB.NewRaw(`
						INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						e.IsSubscription, &ownership, upsertNow, upsertNow,
					).Scan(ctx, &egID); err != nil {
						slog.Error("dispatch_sync: gog upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
						continue
					}

					seenEGPlatforms[egID] = append(seenEGPlatforms[egID], platform)

					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
						VALUES (?, ?, ?, 0, now())
						ON CONFLICT (external_game_id, platform) DO UPDATE SET
							hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
						uuid.NewString(), egID, platform,
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: gog upsert platform failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}

					if _, alreadyDispatched := dispatchedInBatch[e.ExternalID]; alreadyDispatched {
						continue
					}
					dispatchedInBatch[e.ExternalID] = struct{}{}
				}

				// Re-query this batch's unique external_ids to get DB state (is_skipped, id).
				batchExtIDs := make([]string, 0, len(dispatchedInBatch))
				for extID := range dispatchedInBatch {
					batchExtIDs = append(batchExtIDs, extID)
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
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(`
						INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
						VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
						ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, eg.ID,
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

		// Reconcile: delete platform rows no longer present in the upstream library.
		for egID, platforms := range seenEGPlatforms {
			if _, err := w.DB.NewRaw(`
				DELETE FROM external_game_platforms
				WHERE external_game_id = ? AND platform NOT IN (?)`,
				egID, bun.List(platforms),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: gog delete stale platforms failed", "err", err, "job_id", p.JobID, "external_game_id", egID)
			}
		}

	default:
		failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("unknown storefront: %s", p.Storefront))
		return nil
	}

	// ── 5. Mark removed games as unavailable ──────────────────────────────
	var available []models.ExternalGame
	if err := w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		slog.Error("dispatch_sync: query available games failed", "err", err, "job_id", p.JobID)
	}
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: mark game unavailable failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
			if _, err := w.DB.NewRaw(
				`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: insert sync_change (removed) failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
		}
	}

	// ── 7. Update last_synced_at ──────────────────────────────────────────
	syncedNow := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background()); err != nil {
		slog.Error("dispatch_sync: update last_synced_at failed", "err", err, "job_id", p.JobID)
	}

	return nil
}

// failSyncJob marks a job as failed with the given error message and cancels
// any pending job_items for that job so they are not left orphaned.
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	now := time.Now().UTC()
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`,
		msg, now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: fail job update failed", "err", err, "job_id", jobID)
	}
	if _, err := db.NewRaw(
		`UPDATE job_items SET status = 'cancelled' WHERE job_id = ? AND status = 'pending'`,
		jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: cancel pending items failed", "err", err, "job_id", jobID)
	}
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

// ── IGDBMatchWorker — Stage 2 ─────────────────────────────────────────────────

type IGDBMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (IGDBMatchArgs) Kind() string { return "igdb_match" }

func (IGDBMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

type IGDBMatchWorker struct {
	river.WorkerDefaults[IGDBMatchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *IGDBMatchWorker) Work(ctx context.Context, job *river.Job[IGDBMatchArgs]) error {
	p := job.Args

	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("igdb_match: load job_item", "id", p.JobItemID, "err", err)
		return err
	}

	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Fast-path: skipped games go straight to UserGameWorker.
	if eg.IsSkipped {
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Fast-path: already resolved (manual or prior run).
	if eg.ResolvedIGDBID != nil {
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Sibling check: same user/storefront/title already resolved by another SKU.
	var sibling models.ExternalGame
	if err := w.DB.NewSelect().Model(&sibling).
		Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NOT NULL",
			eg.UserID, eg.Storefront, eg.Title, eg.ID).
		Limit(1).
		Scan(ctx); err == nil && sibling.ResolvedIGDBID != nil {
		igdbID := *sibling.ResolvedIGDBID
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("igdb_match: insert game row (sibling)", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("igdb_match: apply sibling resolution", "err", err, "external_game_id", eg.ID)
		}
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// IGDB search.
	if w.IGDBClient != nil && w.IGDBClient.Configured() {
		candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10)
		if err != nil {
			if job.Attempt >= job.MaxAttempts {
				slog.Warn("igdb_match: IGDB failed on final attempt, marking pending_review",
					"item_id", p.JobItemID, "err", err)
				syncMarkItemPendingReview(ctx, w.DB, &item)
				syncCheckJobCompletion(ctx, w.DB, item.JobID)
				return nil
			}
			return fmt.Errorf("igdb_match: search failed (will retry): %w", err)
		}

		normalizedQuery := matching.NormalizeTitle(eg.Title)
		var bestScore, secondBestScore float64
		var bestID int32
		for _, c := range candidates {
			score := matching.FuzzyConfidence(normalizedQuery, matching.NormalizeTitle(c.Title))
			if score > bestScore {
				secondBestScore = bestScore
				bestScore = score
				bestID = int32(c.IgdbID)
			} else if score > secondBestScore {
				secondBestScore = score
			}
		}

		const autoResolveThreshold = 0.85
		const tieEpsilon = 0.01
		if bestScore >= autoResolveThreshold && (bestScore-secondBestScore) > tieEpsilon {
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: insert game row (auto-resolve)", "err", err, "igdb_id", bestID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: auto-resolve external_game", "err", err, "external_game_id", eg.ID)
			}
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}

		// Low confidence — store candidates, mark pending_review.
		candidatesJSON, _ := json.Marshal(candidates)
		item.IGDBCandidates = candidatesJSON
		item.MatchConfidence = &bestScore
		syncMarkItemPendingReview(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// No IGDB client configured — mark pending_review.
	syncMarkItemPendingReview(ctx, w.DB, &item)
	syncCheckJobCompletion(ctx, w.DB, item.JobID)
	return nil
}

func (w *IGDBMatchWorker) enqueueUserGame(ctx context.Context, jobItemID, jobID string) error {
	if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, jobItemID, UserGameArgs{JobItemID: jobItemID}); err != nil {
		slog.Error("igdb_match: enqueue user_game_write failed", "item_id", jobItemID, "err", err)
		syncCheckJobCompletion(ctx, w.DB, jobID)
	}
	return nil
}

// ── UserGameWorker — Stage 3 ──────────────────────────────────────────────────

type UserGameArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (UserGameArgs) Kind() string { return "user_game_write" }

func (UserGameArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
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

	// ── 2. Load external_game via direct column ───────────────────────────
	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 3. Load external_game ─────────────────────────────────────────────
	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 3.5. Apply manual IGDB resolution ────────────────────────────────
	// HandleResolveItem stores the user's chosen IGDB ID on job_items but does
	// not update external_games (it doesn't know the game title). Apply it here
	// so the IGDB search step below is skipped on re-processing.
	if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil {
		igdbID := int32(*item.ResolvedIGDBID)
		eg.ResolvedIGDBID = &igdbID
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: insert game row (step 3.5) failed", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: apply manual resolution failed", "err", err, "external_game_id", eg.ID)
		}
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
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				igdbID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: insert game row (step 3.6) failed", "err", err, "igdb_id", igdbID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				igdbID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: cross-sku resolution failed", "err", err, "external_game_id", eg.ID)
			}
		}
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
			msg := fmt.Sprintf("igdb search failed: %v", err)
			syncMarkItemFailed(ctx, w.DB, &item, msg)
			syncCheckJobCompletion(ctx, w.DB, item.JobID)
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
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: insert game row (auto-resolve) failed", "err", err, "igdb_id", bestID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: auto-resolve external_game failed", "err", err, "external_game_id", eg.ID)
			}
		} else {
			// Store candidates and wait for manual review.
			candidatesJSON, _ := json.Marshal(candidates)
			item.IGDBCandidates = candidatesJSON
			item.MatchConfidence = &bestScore
			syncMarkItemPendingReview(ctx, w.DB, &item)
			syncCheckJobCompletion(ctx, w.DB, item.JobID)
			return nil
		}
	}

	// ── 6. Still no IGDB ID → pending_review ─────────────────────────────
	if eg.ResolvedIGDBID == nil {
		syncMarkItemPendingReview(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// ── 7. Load platform rows ─────────────────────────────────────────────
	var egPlatforms []models.ExternalGamePlatform
	if err := w.DB.NewSelect().Model(&egPlatforms).
		Where("external_game_id = ?", eg.ID).
		Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("load platforms: %v", err))
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}
	if len(egPlatforms) == 0 {
		syncMarkItemFailed(ctx, w.DB, &item, "external game has no platform rows")
		syncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	storefrontSlug, storefrontOK := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !storefrontOK {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved storefront=%s", eg.Storefront))
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
		if _, err := w.DB.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO NOTHING`,
			ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: insert user_game failed", "err", err, "job_item_id", p.JobItemID)
		}
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

	// ── 9. Find or create user_game_platform for each platform ───────────
	ownership := ""
	if eg.OwnershipStatus != nil {
		ownership = *eg.OwnershipStatus
	} else if eg.IsSubscription {
		ownership = "subscription"
	} else {
		ownership = "owned"
	}

	for _, egp := range egPlatforms {
		hoursPlayed := egp.HoursPlayed
		var existingUGPID string
		var existingOwnership *string
		ugpErr := w.DB.NewRaw(
			`SELECT id, ownership_status FROM user_game_platforms WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
			ugID, egp.Platform, storefrontSlug,
		).Scan(ctx, &existingUGPID, &existingOwnership)

		if errors.Is(ugpErr, sql.ErrNoRows) || ugpErr != nil { // treat any scan error as not-found; ON CONFLICT DO NOTHING makes the insert safe
			ugpID := uuid.NewString()
			// original_platform_name holds the canonical slug post-normalisation; raw upstream names are resolved at dispatch time.
			if _, err := w.DB.NewRaw(`
				INSERT INTO user_game_platforms
				(id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
				 original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
				VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
				ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
				ugpID, ugID, egp.Platform, storefrontSlug, hoursPlayed, ownership,
				egp.Platform, eg.Storefront, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: insert user_game_platform failed", "err", err, "job_item_id", p.JobItemID, "platform", egp.Platform)
			}
		} else {
			existingRank := 0
			if existingOwnership != nil {
				existingRank = ownershipRank(*existingOwnership)
			}
			if ownershipRank(ownership) > existingRank {
				if _, err := w.DB.NewRaw(
					`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, updated_at = now() WHERE id = ?`,
					ownership, hoursPlayed, existingUGPID,
				).Exec(ctx); err != nil {
					slog.Error("process_sync_item: update ugp ownership failed", "err", err, "job_item_id", p.JobItemID)
				}
			} else {
				if _, err := w.DB.NewRaw(
					`UPDATE user_game_platforms SET hours_played = ?, updated_at = now() WHERE id = ?`,
					hoursPlayed, existingUGPID,
				).Exec(ctx); err != nil {
					slog.Error("process_sync_item: update ugp playtime failed", "err", err, "job_item_id", p.JobItemID)
				}
			}
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
// "Active" means pending or processing. pending_review items require user action and
// do not count as active, but they DO block job termination — the job stays in
// 'processing' until every item has been resolved by the user, auto-matched, or failed.
//
// Once no active items remain:
//   - pending_review items still exist: job stays processing (user must review).
//   - Any failed items exist and no pending_review: marks job completed_with_errors.
//   - No pending_review, no failed: marks job completed.
func syncCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
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

	// If pending_review items still exist the job stays processing — user must resolve them.
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

	var failedCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'failed'`,
		jobID,
	).Scan(ctx, &failedCount); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion failed count", "job_id", jobID, "err", err)
		return
	}

	now := time.Now().UTC()
	finalStatus := "completed"
	if failedCount > 0 {
		finalStatus = "completed_with_errors"
	}
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
		finalStatus, now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("process_sync_item: finalize job failed", "err", err, "job_id", jobID, "final_status", finalStatus)
	}
}
