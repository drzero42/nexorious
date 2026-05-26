package steam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// Adapter wraps a Client with pre-configured credentials and implements storefrontadapter.Adapter.
type Adapter struct {
	client  *Client
	apiKey  string
	steamID string
	backoffs []time.Duration
}

// NewAdapter returns a storefrontadapter.Adapter for Steam.
func NewAdapter(client *Client, apiKey, steamID string) storefrontadapter.Adapter {
	return &Adapter{
		client:   client,
		apiKey:   apiKey,
		steamID:  steamID,
		backoffs: []time.Duration{2 * time.Minute, 5 * time.Minute},
	}
}

// NewAdapterForTests returns an Adapter with custom backoff durations. Only for use in tests.
func NewAdapterForTests(client *Client, apiKey, steamID string, backoffs []time.Duration) storefrontadapter.Adapter {
	return &Adapter{client: client, apiKey: apiKey, steamID: steamID, backoffs: backoffs}
}

// GetLibrary fetches the user's Steam library and streams results in batches of batchSize.
// PlaytimeHours in each ExternalGameEntry holds the total for the game; the worker assigns
// it to the first platform row only.
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if batchSize <= 0 {
		batchSize = 10
	}
	owned, err := a.client.GetOwnedGames(ctx, a.apiKey, a.steamID)
	if errors.Is(err, ErrAPIKeyRejected) {
		return fmt.Errorf("%w: steam API key rejected", storefrontadapter.ErrCredentials)
	}
	if err != nil {
		return fmt.Errorf("steam: fetch owned games: %w", err)
	}

	slog.Debug("steam: GetOwnedGames returned", "total_games", len(owned), "steam_id", a.steamID)

	// Global backoff state shared across the game loop.
	backoffIdx := 0
	processedCount := 0

	for start := 0; start < len(owned); start += batchSize {
		end := min(start+batchSize, len(owned))

		var entries []storefrontadapter.ExternalGameEntry
		for i, og := range owned[start:end] {
			gameIdx := start + i + 1 // 1-based position across all owned games

			slog.Debug("steam: fetching appdetails",
				"game_index", gameIdx,
				"total_games", len(owned),
				"appid", og.AppID,
				"title", og.Title,
			)

			pl, detErr := a.client.GetAppDetailsPlatforms(ctx, og.AppID)
			if detErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if errors.Is(detErr, ErrRateLimited) && backoffIdx < len(a.backoffs) {
					d := a.backoffs[backoffIdx]
					backoffIdx++
					slog.Warn("steam: rate limited, backing off",
						"wait", d,
						"appid", og.AppID,
						"title", og.Title,
						"game_index", gameIdx,
						"backoff_slot", backoffIdx,
					)
					timer := time.NewTimer(d)
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						return ctx.Err()
					}
					slog.Debug("steam: backoff complete, retrying appdetails",
						"appid", og.AppID,
						"title", og.Title,
						"game_index", gameIdx,
					)
					pl, detErr = a.client.GetAppDetailsPlatforms(ctx, og.AppID)
				}
				if detErr != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if !errors.Is(detErr, ErrRateLimited) {
						return fmt.Errorf("steam: appdetails failed for game %d/%d (%s, appid %d): %w",
							gameIdx, len(owned), og.Title, og.AppID, detErr)
					}
					// Still rate-limited after the initial backoff (or budget already exhausted).
					// Retry indefinitely with the last backoff duration until success or
					// context cancellation — we must not silently drop library entries.
					for {
						d := a.backoffs[len(a.backoffs)-1]
						slog.Warn("steam: rate limited (budget exhausted), retrying",
							"wait", d,
							"appid", og.AppID,
							"title", og.Title,
							"game_index", gameIdx,
						)
						if err := steamSleepCtx(ctx, d); err != nil {
							return err
						}
						slog.Debug("steam: retry after rate-limit backoff",
							"appid", og.AppID,
							"title", og.Title,
							"game_index", gameIdx,
						)
						pl, detErr = a.client.GetAppDetailsPlatforms(ctx, og.AppID)
						if detErr == nil {
							break
						}
						if ctx.Err() != nil {
							return ctx.Err()
						}
						if !errors.Is(detErr, ErrRateLimited) {
							return fmt.Errorf("steam: appdetails failed for game %d/%d (%s, appid %d): %w",
								gameIdx, len(owned), og.Title, og.AppID, detErr)
						}
					}
				}
			}

			var platforms []string
			if pl.Windows {
				platforms = append(platforms, "pc-windows")
			}
			if pl.Mac {
				platforms = append(platforms, "mac")
			}
			if pl.Linux {
				platforms = append(platforms, "pc-linux")
			}
			if len(platforms) == 0 {
				slog.Debug("steam: appdetails returned no platforms (delisted/removed), defaulting to pc-windows",
					"appid", og.AppID,
					"title", og.Title,
					"game_index", gameIdx,
				)
				platforms = []string{"pc-windows"}
			}

			entries = append(entries, storefrontadapter.ExternalGameEntry{
				ExternalID:      fmt.Sprintf("%d", og.AppID),
				Title:           og.Title,
				PlaytimeHours:   og.PlaytimeHours,
				Platforms:       platforms,
				OwnershipStatus: "owned",
				IsSubscription:  false,
			})
			processedCount++
			if processedCount%50 == 0 {
				slog.Debug("steam: sync progress",
					"processed", processedCount,
					"total", len(owned),
					"backoff_slots_used", backoffIdx,
					"steam_id", a.steamID,
				)
			}
		}

		if len(entries) > 0 {
			if err := onBatch(entries); err != nil {
				return err
			}
		}
	}

	slog.Debug("steam: library fetch complete",
		"total_owned", len(owned),
		"processed", processedCount,
		"steam_id", a.steamID,
	)
	return nil
}
