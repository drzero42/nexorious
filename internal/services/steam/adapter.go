package steam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// defaultRateLimitRetryWait is how long we sleep between retries after a 429 from
// the Steam Store API. Short enough to resume promptly once Steam's sliding-window
// limit clears, long enough that we are not spamming during the cooldown.
const defaultRateLimitRetryWait = 10 * time.Second

// Adapter wraps a Client with pre-configured credentials and implements storefrontadapter.Adapter.
type Adapter struct {
	client    *Client
	apiKey    string
	steamID   string
	retryWait time.Duration
}

// NewAdapter returns a storefrontadapter.Adapter for Steam.
func NewAdapter(client *Client, apiKey, steamID string) storefrontadapter.Adapter {
	return &Adapter{
		client:    client,
		apiKey:    apiKey,
		steamID:   steamID,
		retryWait: defaultRateLimitRetryWait,
	}
}

// NewAdapterForTests returns an Adapter with a custom rate-limit retry wait. Pass 0
// for instant retries to keep tests fast.
func NewAdapterForTests(client *Client, apiKey, steamID string, retryWait time.Duration) storefrontadapter.Adapter {
	return &Adapter{
		client:    client,
		apiKey:    apiKey,
		steamID:   steamID,
		retryWait: retryWait,
	}
}

// GetLibrary fetches the user's Steam library and streams results in batches of batchSize.
// PlaytimeHours in each ExternalGameEntry holds the total for the game; the worker assigns
// it to the first platform row only.
//
// On HTTP 429 from the Steam Store API, GetLibrary sleeps a fixed duration and retries
// the same appid indefinitely until success or context cancellation — we must not silently
// drop any entries.
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
	slog.DebugContext(ctx, "steam: GetOwnedGames returned", "total_games", len(owned), "steamid", a.steamID)

	processedCount := 0

	for start := 0; start < len(owned); start += batchSize {
		end := min(start+batchSize, len(owned))

		var entries []storefrontadapter.ExternalGameEntry
		for i, og := range owned[start:end] {
			gameIdx := start + i + 1

			slog.DebugContext(ctx, "steam: fetching appdetails",
				"game_index", gameIdx,
				"total_games", len(owned),
				"appid", og.AppID,
				"title", og.Title,
			)

			pl, err := a.fetchAppDetailsWithRetry(ctx, og, gameIdx, len(owned))
			if err != nil {
				return err
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
				slog.DebugContext(ctx, "steam: appdetails returned no platforms (delisted/removed), defaulting to pc-windows",
					"appid", og.AppID, "title", og.Title, "game_index", gameIdx)
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
				slog.DebugContext(ctx, "steam: sync progress",
					"processed", processedCount,
					"total", len(owned),
					"steamid", a.steamID,
				)
			}
		}

		if len(entries) > 0 {
			if err := onBatch(entries); err != nil {
				return err
			}
		}
	}

	slog.DebugContext(ctx, "steam: library fetch complete",
		"total_owned", len(owned),
		"processed", processedCount,
		"steamid", a.steamID,
	)
	return nil
}

// fetchAppDetailsWithRetry calls GetAppDetailsPlatforms for one appid, retrying
// indefinitely on ErrRateLimited with a fixed sleep between attempts. The first 429
// for a given appid logs at WARN; subsequent retries log at DEBUG so we don't spam
// the log while waiting out a sliding-window cooldown.
func (a *Adapter) fetchAppDetailsWithRetry(ctx context.Context, og OwnedGame, gameIdx, totalGames int) (Platforms, error) {
	for attempt := 1; ; attempt++ {
		pl, err := a.client.GetAppDetailsPlatforms(ctx, og.AppID)
		if err == nil {
			if attempt > 1 {
				slog.DebugContext(ctx, "steam: appdetails succeeded after retry",
					"appid", og.AppID,
					"title", og.Title,
					"game_index", gameIdx,
					"attempt", attempt,
				)
			}
			return pl, nil
		}
		if ctx.Err() != nil {
			return Platforms{}, ctx.Err()
		}
		if !errors.Is(err, ErrRateLimited) {
			return Platforms{}, fmt.Errorf("steam: appdetails failed for game %d/%d (%s, appid %d): %w",
				gameIdx, totalGames, og.Title, og.AppID, err)
		}
		if attempt == 1 {
			slog.WarnContext(ctx, "steam: rate limited, sleeping before retry",
				"wait", a.retryWait,
				"appid", og.AppID,
				"title", og.Title,
				"game_index", gameIdx,
				"attempt", attempt,
				logging.Cat(logging.CategoryExternalAPI),
			)
		} else {
			slog.DebugContext(ctx, "steam: rate limited, sleeping before retry",
				"wait", a.retryWait,
				"appid", og.AppID,
				"title", og.Title,
				"game_index", gameIdx,
				"attempt", attempt,
			)
		}
		if err := steamSleepCtx(ctx, a.retryWait); err != nil {
			return Platforms{}, err
		}
	}
}
