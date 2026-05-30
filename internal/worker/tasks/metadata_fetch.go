package tasks

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
)

// MetadataFetchArgs is the River job args type for "metadata_fetch": an
// immediate, fire-and-forget IGDB metadata fetch for a single newly added game.
type MetadataFetchArgs struct {
	GameID int32 `json:"game_id"`
}

func (MetadataFetchArgs) Kind() string { return "metadata_fetch" }

// InsertOpts sets priority 2 (between sync workers at 1 and the bulk metadata
// refresh at 3) and 3 attempts. The fetch runs promptly after the triggering
// sync without delaying in-flight sync jobs, and transient IGDB failures are
// retried with backoff before giving up.
func (MetadataFetchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 2}
}

// MetadataFetchWorker performs a single-game IGDB metadata fetch with no
// jobs/job_items tracking layer. It is triggered at the end of sync Stage 3 for
// games that have no metadata yet. Fire-and-forget: success logs at debug,
// exhausted retries log at error, and the periodic bulk refresh remains the
// safety net for anything this misses.
type MetadataFetchWorker struct {
	river.WorkerDefaults[MetadataFetchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	StoragePath string
}

func (w *MetadataFetchWorker) Work(ctx context.Context, job *river.Job[MetadataFetchArgs]) error {
	gameID := job.Args.GameID

	// IGDB guard: nothing to do if not configured. Return nil so River does not retry.
	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		slog.Debug("metadata_fetch: IGDB not configured, skipping", "game_id", gameID)
		return nil
	}

	if err := fetchAndStoreMetadata(ctx, w.DB, w.IGDBClient, w.StoragePath, gameID); err != nil {
		// Exhausted retries: log at error and stop (return nil). The periodic
		// bulk refresh will still pick the game up eventually.
		if job.Attempt >= job.MaxAttempts {
			slog.Error("metadata_fetch: exhausted retries", "game_id", gameID, "err", err)
			return nil
		}
		// Otherwise return the error so River retries with backoff.
		return fmt.Errorf("metadata_fetch game %d: %w", gameID, err)
	}

	slog.Debug("metadata_fetch: completed", "game_id", gameID)
	return nil
}
