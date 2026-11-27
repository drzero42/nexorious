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
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/matching"
	"github.com/drzero42/nexorious/internal/services/platformresolution"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
	"github.com/drzero42/nexorious/internal/usergame"
)

// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
// Defined as a type alias so service adapter packages can implement StorefrontAdapter
// without importing this (tasks) package and creating an import cycle.
type ExternalGameEntry = storefrontadapter.ExternalGameEntry

// StorefrontAdapter is the interface every storefront adapter must satisfy.
type StorefrontAdapter = storefrontadapter.Adapter

// ErrCredentials is returned by an adapter when credentials are invalid,
// expired, or cannot be decrypted. DispatchSyncWorker marks the job failed on this error.
var ErrCredentials = storefrontadapter.ErrCredentials

// CredentialsErrorFor is a credentials failure that belongs to one stored
// sign-in: Credentials is the user_sync_configs ciphertext the sync loaded.
// The worker flags credentials_error, and notifies, only while that sign-in is
// still the stored one, in the same UPDATE that sets the flag, so a reconnect
// committed while the sync ran is never marked expired by it.
type CredentialsErrorFor struct {
	Credentials *string // nil: the sync loaded no stored sign-in
	Err         error
}

func (e *CredentialsErrorFor) Error() string { return e.Err.Error() }
func (e *CredentialsErrorFor) Unwrap() error { return e.Err }

// ── DispatchSync ──────────────────────────────────────────────────────────────

// DispatchSyncArgs is the River job args type for "dispatch_sync".
type DispatchSyncArgs struct {
	JobID      string `json:"job_id"`
	UserID     string `json:"user_id"`
	Storefront string `json:"storefront"`
}

func (DispatchSyncArgs) Kind() string { return "dispatch_sync" }

func (DispatchSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 1}
}

// syncDispatchTimeout bounds one run of a sync dispatch. It is generous (a
// large library needs many sequential storefront calls) but finite, so a hung
// dispatch is cancelled, uses an attempt and runs again; after the last
// attempt recovery fails the sync (docs/job-lifecycle.md, "Recovery").
const syncDispatchTimeout = 2 * time.Hour

// Timeout is RunTimeout, or syncDispatchTimeout when that is zero.
func (w *DispatchSyncWorker) Timeout(*river.Job[DispatchSyncArgs]) time.Duration {
	if w.RunTimeout > 0 {
		return w.RunTimeout
	}
	return syncDispatchTimeout
}

// DispatchSyncWorker drives a full sync run for one (user, storefront) pair
// using a unified StorefrontAdapter. It is the dispatch owner of its Job: it
// starts the dispatch, adds the library's games in batches and marks the
// dispatch done through the Job lifecycle module.
type DispatchSyncWorker struct {
	river.WorkerDefaults[DispatchSyncArgs]
	DB      *bun.DB
	LC      *joblifecycle.Module
	Adapter func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (StorefrontAdapter, error)
	// RunTimeout overrides syncDispatchTimeout (tests).
	RunTimeout time.Duration
}

// nullableJSON returns nil when b is empty so the column is written as SQL NULL.
// When non-empty it returns a json.RawMessage so pgdriver encodes it as JSONB.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

// errSyncStopped means the sync's Job is no longer active (cancelled, failed,
// or its storefront data reset): the dispatch stops and exits cleanly.
var errSyncStopped = errors.New("dispatch_sync: the sync is no longer active")

// batchError is a database error while a batch's items were added. The
// dispatch returns it to River, which runs the dispatch again.
type batchError struct{ err error }

func (e *batchError) Error() string { return "dispatch_sync: add batch: " + e.err.Error() }
func (e *batchError) Unwrap() error { return e.err }

// shareLockActiveJob takes a share lock on the sync's Job row while it is
// active, or returns errSyncStopped. Writes that a Cancel or a reset must not
// be overtaken by (the batch's games, the sweeps' rows) take it first, which
// also keeps the module's lock order: the Job row, then everything else.
func shareLockActiveJob(ctx context.Context, tx bun.Tx, jobID string) error {
	var one []int
	if err := tx.NewRaw(
		`SELECT 1 FROM jobs WHERE id = ? AND status IN ('pending', 'processing') FOR KEY SHARE`, jobID,
	).Scan(ctx, &one); err != nil {
		return fmt.Errorf("lock job: %w", err)
	}
	if len(one) == 0 {
		return errSyncStopped
	}
	return nil
}

// upsertGame writes one fetched game, in a short transaction that first
// share-locks the active Job: the external game, its platforms, and for a
// skipped game its 'skipped' row. These are progress writes, safe to repeat.
func upsertGame(ctx context.Context, db *bun.DB, e ExternalGameEntry, p DispatchSyncArgs, platforms []string) (egID string, isSkipped bool, err error) {
	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := shareLockActiveJob(ctx, tx, p.JobID); err != nil {
			return err
		}
		var err error
		if egID, isSkipped, err = upsertExternalGame(ctx, tx, e, p); err != nil {
			return err
		}
		if err := upsertPlatforms(ctx, tx, egID, platforms, e.PlaytimeHours, e.AchievementsUnlocked, e.AchievementsTotal); err != nil {
			return err
		}
		if !isSkipped {
			return nil
		}
		// A dispatch run again sees the same skipped games: one row per Job.
		return InsertSkippedChange(ctx, tx, p.JobID, p.UserID, egID, e.Title)
	})
	return egID, isSkipped, err
}

// upsertExternalGame writes the fetched game and returns its id and whether
// it is skipped. A game that is new here is not given a decision: its match
// worker takes a same-title game's decision (precedent) or searches.
func upsertExternalGame(ctx context.Context, db bun.IDB, e ExternalGameEntry, p DispatchSyncArgs) (egID string, isSkipped bool, err error) {
	var row struct {
		ID        string `bun:"id"`
		IsSkipped bool   `bun:"is_skipped"`
	}
	var sourceMetaJSON []byte
	if len(e.SourceMetadata) > 0 {
		sourceMetaJSON, _ = json.Marshal(e.SourceMetadata) //nolint:errcheck // marshaling a map[string]string cannot fail
	}
	if err := db.NewRaw(`
		INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, source_metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, now(), now())
		ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			is_subscription = EXCLUDED.is_subscription,
			ownership_status = EXCLUDED.ownership_status,
			source_metadata = COALESCE(EXCLUDED.source_metadata, external_games.source_metadata),
			is_available = true,
			updated_at = now()
		RETURNING id, is_skipped`,
		uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
		e.IsSubscription, e.OwnershipStatus, nullableJSON(sourceMetaJSON),
	).Scan(ctx, &row); err != nil {
		return "", false, fmt.Errorf("upsert external_game: %w", err)
	}
	return row.ID, row.IsSkipped, nil
}

func upsertPlatforms(ctx context.Context, db bun.IDB, egID string, platforms []string, playtimeHours float64, achievementsUnlocked, achievementsTotal *int) error {
	for i, platform := range platforms {
		hours := 0.0
		var unlocked, total *int
		if i == 0 {
			hours = playtimeHours
			unlocked = achievementsUnlocked
			total = achievementsTotal
		}
		if _, err := db.NewRaw(`
			INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, achievements_unlocked, achievements_total, created_at)
			VALUES (?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (external_game_id, platform) DO UPDATE SET
				hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played),
				achievements_unlocked = COALESCE(EXCLUDED.achievements_unlocked, external_game_platforms.achievements_unlocked),
				achievements_total = COALESCE(EXCLUDED.achievements_total, external_game_platforms.achievements_total)`,
			uuid.NewString(), egID, platform, hours, unlocked, total,
		).Exec(ctx); err != nil {
			return fmt.Errorf("upsert platform %s: %w", platform, err)
		}
	}
	return nil
}

func (w *DispatchSyncWorker) Work(ctx context.Context, rj *river.Job[DispatchSyncArgs]) error {
	p := rj.Args
	// Seed the application job id so every in-job log line (and the helpers below
	// that receive this ctx) is correlated to the user-facing jobs.id.
	ctx = logging.WithJobID(ctx, p.JobID)

	// Refused when the Job has ended, when this River job is not its
	// dispatch owner, or when the dispatch is already done: a dispatch
	// hard-killed after marking done must not fetch the library again.
	if _, err := w.LC.StartDispatch(ctx, p.JobID, rj.ID); err != nil {
		return exitOrRetry(ctx, err)
	}

	var cfg models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&cfg).
		Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return w.fail(ctx, rj, "no sync config found", nil)
		}
		return fmt.Errorf("dispatch_sync: load sync config: %w", err)
	}

	// Build adapter (credential loading, decryption, and token refresh happen inside).
	adapter, err := w.Adapter(ctx, p.Storefront, cfg)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrCredentials) {
		// Nothing has written the stored sign-in yet, so the failure belongs
		// to exactly the one loaded above (none, when it is NULL).
		return w.failCredentials(ctx, rj, cfg.CredentialsError, &CredentialsErrorFor{Credentials: cfg.StorefrontCredentials, Err: err})
	}
	if err != nil {
		return w.fail(ctx, rj, err.Error(), nil)
	}

	fetchedIDs := make(map[string]struct{})
	seenPlatforms := make(map[string][]string)

	slog.InfoContext(ctx, "dispatch_sync: starting library fetch", logging.KeyUserID, p.UserID, logging.KeySource, p.Storefront)
	totalProcessed := 0
	fetchErr := adapter.GetLibrary(ctx, 10, func(batch []ExternalGameEntry) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		newItems := make([]joblifecycle.NewItem, 0, len(batch))
		skippedInBatch := 0
		for _, e := range batch {
			fetchedIDs[e.ExternalID] = struct{}{}
			// Every external game must have at least one platform; default to PC
			// (Windows) when the adapter reports none. Platform-slug validity is
			// enforced by the external_game_platforms -> platforms(name) foreign key,
			// so adapters must emit canonical platforms.name slugs.
			platforms := e.Platforms
			if len(platforms) == 0 {
				platforms = []string{"pc-windows"}
			}
			egID, isSkipped, err := upsertGame(ctx, w.DB, e, p, platforms)
			switch {
			case ctx.Err() != nil:
				return ctx.Err()
			case errors.Is(err, errSyncStopped):
				return errSyncStopped
			case err != nil:
				// Best effort, as for every progress write: the next sync
				// writes the game again.
				slog.WarnContext(ctx, "dispatch_sync: write game failed", logging.KeyErr, err, "external_id", e.ExternalID, logging.Cat(logging.CategoryDB))
				continue
			}
			seenPlatforms[egID] = append(seenPlatforms[egID], platforms...)
			if isSkipped {
				skippedInBatch++
				continue
			}
			id := egID
			newItems = append(newItems, joblifecycle.NewItem{
				UserID: p.UserID, ItemKey: e.ExternalID, SourceTitle: e.Title, ExternalGameID: &id,
			})
		}
		added, err := w.LC.AddNewItems(ctx, p.JobID, rj.ID, newItems)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case joblifecycle.IsRefusal(err):
			return errSyncStopped
		case err != nil:
			return &batchError{err: err}
		}
		totalProcessed += len(batch)
		slog.DebugContext(ctx, "dispatch_sync: batch complete",
			"batch_size", len(batch),
			"enqueued_for_matching", added,
			"skipped_by_user", skippedInBatch,
			"total_processed_so_far", totalProcessed,
		)
		return nil
	})
	// Checked first, whatever the fetch returned: a fetch can return without
	// an error under a cancelled context with a partial library, and an error
	// it returns then (an authentication error from a cancelled request, for
	// one) is not the storefront's answer. Nothing is recorded.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if fetchErr != nil {
		var be *batchError
		switch {
		case errors.Is(fetchErr, errSyncStopped):
			slog.InfoContext(ctx, "dispatch_sync: the sync is no longer active; stopping", logging.KeySource, p.Storefront)
			return nil
		case errors.As(fetchErr, &be):
			return be
		case errors.Is(fetchErr, ErrCredentials):
			return w.failCredentials(ctx, rj, cfg.CredentialsError, fetchErr)
		}
		if incomplete := new(*storefrontadapter.IncompleteLibraryError); errors.As(fetchErr, incomplete) {
			slog.WarnContext(ctx, "dispatch_sync: incomplete library", logging.KeySource, p.Storefront, logging.KeyErr, fetchErr, logging.Cat(logging.CategoryExternalAPI))
		} else {
			slog.ErrorContext(ctx, "dispatch_sync: library fetch failed", logging.KeySource, p.Storefront, logging.KeyErr, fetchErr, logging.Cat(logging.CategoryExternalAPI))
		}
		return w.fail(ctx, rj, fetchErr.Error(), nil)
	}

	// Empty-fetch guard (#1324): a fetch that returns nothing while games are
	// known for this storefront is far more likely a privacy setting, an
	// upstream glitch or a changed response shape than a library that really
	// went to zero. Running the sweep would mark every game removed and
	// announce it through sync.diff, so fail the job instead. There is no
	// adapter bypass on purpose: in those failure modes the adapter believes it
	// saw a real, empty library.
	if len(fetchedIDs) == 0 {
		known, err := w.DB.NewSelect().Model((*models.ExternalGame)(nil)).
			Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
			Count(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("dispatch_sync: count available games: %w", err)
		}
		if known > 0 {
			slog.ErrorContext(ctx, "dispatch_sync: empty library fetch with known games; refusing to mark them removed",
				logging.KeySource, p.Storefront, "known_games", known, logging.Cat(logging.CategoryExternalAPI))
			return w.fail(ctx, rj, fmt.Sprintf(
				"the storefront returned an empty library while %d games are known; nothing was marked removed. "+
					"If the library really is empty, reset this storefront's sync data or disconnect it.", known), nil)
		}
	}

	w.sweep(ctx, p, fetchedIDs, seenPlatforms)

	// The sweeps ran on ctx: if it was cancelled meanwhile they may not
	// have run to the end, so the dispatch must not be marked done.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Mark the dispatch done, with last_synced_at, last_fetched_at and the
	// cleared credentials flag as this sync's own writes, after sync.recovered when
	// this fetch ends an outage. The module then evaluates completion (an empty
	// library completes here).
	return exitOrRetry(ctx, w.LC.MarkDispatched(ctx, p.JobID, rj.ID, joblifecycle.Dispatched{
		Own: func(ctx context.Context, tx bun.Tx) error {
			if err := emitSyncRecovered(ctx, tx, p); err != nil {
				return err
			}
			now := time.Now().UTC()
			_, err := tx.NewRaw(
				`UPDATE user_sync_configs SET last_synced_at = ?, last_fetched_at = ?, credentials_error = false, updated_at = now() WHERE user_id = ? AND storefront = ?`,
				now, now, p.UserID, p.Storefront,
			).Exec(ctx)
			return err
		},
	}))
}

// sweep runs after a full, uninterrupted fetch: it removes platform rows no
// longer present upstream, and marks the games that left the library
// unavailable with their 'removed' rows. Every error is logged and the
// dispatch carries on; a sweep that finds the Job no longer active stops.
func (w *DispatchSyncWorker) sweep(ctx context.Context, p DispatchSyncArgs, fetchedIDs map[string]struct{}, seenPlatforms map[string][]string) {
	for egID, platforms := range seenPlatforms {
		if _, err := w.DB.NewRaw(`
			DELETE FROM external_game_platforms
			WHERE external_game_id = ? AND platform NOT IN (?)`,
			egID, bun.List(platforms),
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "dispatch_sync: delete stale platforms failed", logging.KeyErr, err, logging.KeyExternalGameID, egID, logging.Cat(logging.CategoryDB))
		}
	}

	var available []models.ExternalGame
	if err := w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: query available games failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; found {
			continue
		}
		// One transaction per game (#1329): the share lock on the Job row
		// first (the changes insert would take one through its foreign key
		// anyway), then the game, and the 'removed' row only when this run
		// changed it, so a second run writes nothing twice. A game is listed
		// as removed once, by the sync that takes its last available edition
		// away (#1443): no row while another game that counts as the same game
		// is still available.
		err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if err := shareLockActiveJob(ctx, tx, p.JobID); err != nil {
				return err
			}
			res, err := tx.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ? AND is_available = true`, eg.ID,
			).Exec(ctx)
			if err != nil {
				return fmt.Errorf("mark game unavailable: %w", err)
			}
			if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // pgdriver always reports RowsAffected
				return nil
			}
			if _, err := tx.NewRaw(
				`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 SELECT ?, ?, ?, eg.id, 'removed', eg.title, now()
				   FROM external_games eg
				  WHERE eg.id = ?
				    AND NOT EXISTS (
				      SELECT 1 FROM external_games o
				       WHERE o.id <> eg.id AND o.is_available AND `+externalgame.SameGame("o", "eg")+`)`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("insert sync_change (removed): %w", err)
			}
			return nil
		})
		if errors.Is(err, errSyncStopped) {
			return
		}
		if err != nil {
			slog.WarnContext(ctx, "dispatch_sync: mark game removed failed", logging.KeyErr, err, logging.KeyExternalGameID, eg.ID, logging.Cat(logging.CategoryDB))
		}
	}
}

// fail fails the sync for its dispatch owner. own, when set, is the
// caller's own write in Fail's transaction and returns the kind data.
func (w *DispatchSyncWorker) fail(ctx context.Context, rj *river.Job[DispatchSyncArgs], msg string, own func(ctx context.Context, tx bun.Tx) (json.RawMessage, error)) error {
	return exitOrRetry(ctx, w.LC.Fail(ctx, rj.Args.JobID, joblifecycle.Failure{
		Message: msg, CallerRiverJobID: rj.ID, Own: own,
	}))
}

// failCredentials fails the sync for a credentials failure. The
// credentials_error flag is set in Fail's transaction, so a refused Fail
// leaves it alone. A failure bound to one stored sign-in (#1347) sets it only
// while that sign-in is still stored; the effect notifies only when this
// failure set the flag on a store that was healthy (priorErr false).
func (w *DispatchSyncWorker) failCredentials(ctx context.Context, rj *river.Job[DispatchSyncArgs], priorErr bool, cause error) error {
	var bound *CredentialsErrorFor
	errors.As(cause, &bound)
	return w.fail(ctx, rj, credentialsErrorMessage, func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
		flagged, err := flagCredentialsError(ctx, tx, rj.Args, bound)
		if err != nil {
			return nil, fmt.Errorf("flag credentials_error: %w", err)
		}
		return json.Marshal(syncFailure{Credentials: true, Notify: flagged && !priorErr})
	})
}

// credentialsErrorMessage is the error_message of a sync Job failed by a
// credentials failure.
const credentialsErrorMessage = "credentials error"

// flagCredentialsError sets credentials_error for the sync's store. A failure
// bound to the sign-in the sync loaded flags the store only while that
// sign-in is still stored, checked in the same UPDATE; flagged is false when
// it is not (the user reconnected during the sync).
func flagCredentialsError(ctx context.Context, db bun.IDB, p DispatchSyncArgs, bound *CredentialsErrorFor) (flagged bool, err error) {
	query := `UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`
	args := []any{p.UserID, p.Storefront}
	if bound != nil {
		if bound.Credentials == nil {
			query += ` AND storefront_credentials IS NULL`
		} else {
			query += ` AND storefront_credentials = ?`
			args = append(args, *bound.Credentials)
		}
	}
	res, err := db.NewRaw(query, args...).Exec(ctx)
	if err != nil {
		return false, err
	}
	if bound != nil {
		if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // RowsAffected never errors for this driver; 0 also reads as "replaced"
			return false, nil
		}
	}
	return true, nil
}

// ── IGDBMatchWorker — Stage 2 ─────────────────────────────────────────────────

type IGDBMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (IGDBMatchArgs) Kind() string { return "igdb_match" }

func (IGDBMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

// IGDBMatchWorker matches a sync item's game to IGDB, or hands it straight to
// the write stage when it is already matched or skipped, or when a same-title
// game has a decision it can take (precedent, #1449). It is also what the
// module queues by itself for a sync item (Retry, recovery).
type IGDBMatchWorker struct {
	river.WorkerDefaults[IGDBMatchArgs]
	DB         *bun.DB
	LC         *joblifecycle.Module
	IGDBClient *igdbsvc.Client
}

// errNoPrecedent rolls back a transition whose precedent was gone by the time
// it ran (the other game's decision changed in between).
var errNoPrecedent = errors.New("igdb_match: no same-title game has a decision any more")

// errNoMatchToTake rolls back the completion of one same-title review: the
// game's precedent is not a match (it is a skip, or the match is gone), so the
// game keeps its item in review.
var errNoMatchToTake = errors.New("igdb_match: no same-title match to take")

func (w *IGDBMatchWorker) Work(ctx context.Context, rj *river.Job[IGDBMatchArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, rj.Args.JobItemID, rj.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)
	if item.ExternalGameID == nil {
		return failItem(ctx, w.LC, item.ID, rj.ID, "external_game_id not set on job_item")
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return failItem(ctx, w.LC, item.ID, rj.ID, "external game not found")
		}
		return fmt.Errorf("igdb_match: load external game: %w", err)
	}

	// Fast path: a skipped or matched game goes straight to the write stage.
	if eg.IsSkipped || eg.ResolvedIGDBID != nil {
		return w.toWrite(ctx, rj, item.ID, nil)
	}

	// Precedent: a same-title game has a decision, so this game takes it and
	// is not searched. Checked without a lock first; the take decides.
	has, err := externalgame.HasPrecedent(ctx, w.DB, eg.ID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("igdb_match: %w", err)
	}
	if has {
		// NextStage directly, not toWrite: a precedent gone by now is not a
		// refusal but a reason to search instead.
		err := w.LC.NextStage(ctx, item.ID, rj.ID, joblifecycle.Stage{
			Args: UserGameArgs{JobItemID: item.ID},
			Own: func(ctx context.Context, tx bun.Tx) error {
				took, err := externalgame.TakePrecedent(ctx, tx, eg.ID)
				if err != nil {
					return err
				}
				if !took {
					return joblifecycle.Abort(errNoPrecedent)
				}
				return nil
			},
		})
		if !errors.Is(err, errNoPrecedent) {
			return exitOrRetry(ctx, err)
		}
		slog.DebugContext(ctx, "igdb_match: precedent gone before it was taken, searching", logging.KeyJobItemID, item.ID)
	}
	return w.search(ctx, rj, item, &eg)
}

// toWrite hands the item to the write stage, with own as the transition's
// own write.
func (w *IGDBMatchWorker) toWrite(ctx context.Context, rj *river.Job[IGDBMatchArgs], itemID string, own joblifecycle.OwnWrite) error {
	return exitOrRetry(ctx, w.LC.NextStage(ctx, itemID, rj.ID, joblifecycle.Stage{Args: UserGameArgs{JobItemID: itemID}, Own: own}))
}

// search matches a game through IGDB: a confident match goes to the write
// stage, anything else to Pending review.
func (w *IGDBMatchWorker) search(ctx context.Context, rj *river.Job[IGDBMatchArgs], item *models.JobItem, eg *models.ExternalGame) error {
	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		slog.DebugContext(ctx, "igdb_match: no IGDB client configured, marking pending_review", logging.KeyJobItemID, item.ID, "title", eg.Title)
		return w.toReview(ctx, rj, item.ID, eg.ID, nil, nil)
	}
	platformIDs, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, w.DB, eg.ID)
	if perErr != nil {
		slog.DebugContext(ctx, "igdb_match: platform resolution failed, falling back to unfiltered",
			logging.KeyJobItemID, item.ID, logging.KeyExternalGameID, eg.ID, logging.KeyErr, perErr)
		platformIDs = nil
	}
	candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10, platformIDs)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if lastAttempt(rj.JobRow) {
			slog.WarnContext(ctx, "igdb_match: IGDB failed on final attempt, marking pending_review",
				logging.KeyJobItemID, item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
			return w.toReview(ctx, rj, item.ID, eg.ID, nil, nil)
		}
		return fmt.Errorf("igdb_match: search failed (will retry): %w", err)
	}

	cands := make([]matching.Candidate, len(candidates))
	for i, c := range candidates {
		cands[i] = matching.Candidate{ID: int32(c.IgdbID), Title: c.Title} //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
	}
	decision := matching.Decide(eg.Title, cands)
	slog.DebugContext(ctx, "igdb_match: search results",
		logging.KeyJobItemID, item.ID,
		"title", eg.Title,
		"candidate_count", len(candidates),
		"best_score", decision.BestScore,
		"second_best_score", decision.SecondBest,
		"best_igdb_id", decision.ResolvedID,
	)

	if decision.Confident {
		best := matching.Candidate{ID: decision.ResolvedID}
		for _, c := range cands {
			if c.ID == best.ID {
				best = c
				break
			}
		}
		// The write stage writes whatever the game has after the own write.
		var wroteMatch bool
		err := w.toWrite(ctx, rj, item.ID, func(ctx context.Context, tx bun.Tx) (err error) {
			wroteMatch, err = writeConfidentMatch(ctx, tx, eg, best)
			return err
		})
		if err != nil {
			return err
		}
		// wroteMatch is true only when the write committed: the module refuses
		// before the own write runs, and any error after it is returned above.
		if wroteMatch {
			w.completeSameTitleReviews(ctx, item, eg)
		}
		return nil
	}

	candidatesJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling the candidates slice cannot fail
	bs := decision.BestScore
	return w.toReview(ctx, rj, item.ID, eg.ID, candidatesJSON, &bs)
}

// writeConfidentMatch is the own write of a confident match: the game takes
// an edition's decision made during the search if there is one (precedent
// first), otherwise the search's match best. It reports whether it wrote
// best. The write is guarded so a rematch or Skip that committed during the
// search wins (#1338). A games row it creates takes best's IGDB title, not the
// storefront's (#1455).
func writeConfidentMatch(ctx context.Context, tx bun.Tx, eg *models.ExternalGame, best matching.Candidate) (bool, error) {
	took, err := externalgame.TakePrecedent(ctx, tx, eg.ID)
	if err != nil || took {
		return false, err
	}
	if err := externalgame.EnsureGameRow(ctx, tx, best.ID, best.Title, eg.Title); err != nil {
		return false, err
	}
	res, err := tx.NewRaw(
		`UPDATE external_games eg SET resolved_igdb_id = ?, updated_at = now() WHERE eg.id = ? AND `+externalgame.Undecided("eg"),
		best.ID, eg.ID,
	).Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("auto-resolve external_game: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("auto-resolve external_game: rows affected: %w", err)
	}
	return n > 0, nil
}

// completeSameTitleReviews completes the Pending review items of this game's same-title games
// in this Job, now that a search matched it with confidence (the settle rule of
// ADR 0008):
// two editions are searched with different platform filters, so one can match
// while the other went to review. Inside the module's transaction for the Job,
// in id order, each such game without a decision takes the match by
// precedent, is written to the library and its diff rows written, and its item
// is returned for completion. A game whose precedent is a skip is left alone,
// with its item in review. Best effort: it runs after the match committed, and
// a failure is logged and leaves the items in review.
func (w *IGDBMatchWorker) completeSameTitleReviews(ctx context.Context, item *models.JobItem, eg *models.ExternalGame) {
	n, err := w.LC.CompleteItems(ctx, item.JobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
		waiting, err := externalgame.UndecidedSameTitleInReview(ctx, tx, job.ID, eg.ID)
		if err != nil {
			return nil, err
		}
		done := make([]string, 0, len(waiting))
		for _, wg := range waiting {
			// A savepoint per game: one that cannot be written stays in review
			// without taking the others with it.
			err := tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
				took, err := externalgame.TakeMatchPrecedent(ctx, sp, wg.GameID)
				if err != nil {
					return err
				}
				if !took {
					return errNoMatchToTake
				}
				lw, wrote, err := externalgame.WriteMatched(ctx, sp, job.UserID, wg.GameID)
				if err != nil || !wrote {
					return err // not written (no platform rows): completed without diff rows
				}
				return InsertLibraryWriteChanges(ctx, sp, job.ID, job.UserID, lw, lw.Result.Created)
			})
			switch {
			case err == nil:
				done = append(done, wg.ItemID)
			case errors.Is(err, errNoMatchToTake):
				// Its oldest decision is a skip, or the match is gone: it stays
				// in review for the user, which is not a failure.
			default:
				// The library write refuses a game it cannot write (validation);
				// anything else is a database failure.
				cat := logging.CategoryDB
				if cannotWrite(err) {
					cat = logging.CategoryValidation
				}
				slog.WarnContext(ctx, "igdb_match: completing a same-title review failed; its item stays in review",
					logging.KeyExternalGameID, wg.GameID, logging.KeyErr, err, logging.Cat(cat))
			}
		}
		return done, nil
	})
	switch {
	case err == nil:
		if n > 0 {
			slog.DebugContext(ctx, "igdb_match: completed same-title reviews", logging.KeyJobItemID, item.ID, "count", n)
		}
	case errors.Is(err, joblifecycle.ErrJobNotActive), errors.Is(err, joblifecycle.ErrGone):
		// The Job ended or was deleted meanwhile: no items left to complete.
	default:
		slog.WarnContext(ctx, "igdb_match: completing same-title reviews failed; they stay in review",
			logging.KeyJobItemID, item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}

// toReview puts the item in Pending review, unless the game has a decision by
// now: a rematch committed during the search, or a same-title game was
// decided and this game takes it (precedent). Then it goes on to the write
// stage. The game's row is locked in the transition, so a rematch in progress
// is waited for (#1338).
func (w *IGDBMatchWorker) toReview(ctx context.Context, rj *river.Job[IGDBMatchArgs], itemID, externalGameID string, candidates json.RawMessage, confidence *float64) error {
	return exitOrRetry(ctx, w.LC.AwaitReview(ctx, itemID, rj.ID, joblifecycle.Review{
		Candidates: candidates, MatchConfidence: confidence,
		Divert: func(ctx context.Context, tx bun.Tx) (river.JobArgs, error) {
			took, err := externalgame.TakePrecedent(ctx, tx, externalGameID)
			if err != nil {
				return nil, err
			}
			if took {
				return UserGameArgs{JobItemID: itemID}, nil
			}
			var resolved *int32
			if err := tx.NewRaw(
				`SELECT resolved_igdb_id FROM external_games WHERE id = ? FOR SHARE`, externalGameID,
			).Scan(ctx, &resolved); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("lock external game: %w", err)
			}
			if resolved != nil {
				return UserGameArgs{JobItemID: itemID}, nil
			}
			return nil, nil
		},
	}))
}

// ── UserGameWorker — Stage 3 ──────────────────────────────────────────────────

type UserGameArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (UserGameArgs) Kind() string { return "user_game_write" }

func (UserGameArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

// UserGameWorker writes the library state of a sync item's game. The library
// write, its diff rows and the item's completion commit together (#1329).
type UserGameWorker struct {
	river.WorkerDefaults[UserGameArgs]
	DB         *bun.DB
	LC         *joblifecycle.Module
	IGDBClient *igdbsvc.Client
	// RiverClient inserts the immediate metadata fetch, which is not Job work.
	RiverClient *river.Client[pgx.Tx]
}

// cannotWrite reports whether err means the game cannot be written, which
// fails the item, as opposed to an error that might go away by trying again.
func cannotWrite(err error) bool {
	return errors.Is(err, externalgame.ErrNotFound) || errors.Is(err, externalgame.ErrUnmatched) ||
		errors.Is(err, externalgame.ErrNoPlatforms) || errors.Is(err, usergame.ErrValidation) ||
		errors.Is(err, usergame.ErrNotFound) || errors.Is(err, usergame.ErrConflict) ||
		errors.Is(err, usergame.ErrUnprocessable)
}

func (w *UserGameWorker) Work(ctx context.Context, rj *river.Job[UserGameArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, rj.Args.JobItemID, rj.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)
	if item.ExternalGameID == nil {
		return failItem(ctx, w.LC, item.ID, rj.ID, "external_game_id not set on job_item")
	}
	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return failItem(ctx, w.LC, item.ID, rj.ID, "external game not found")
		}
		return fmt.Errorf("user_game_write: load external game: %w", err)
	}
	if eg.IsSkipped {
		return w.endSkipped(ctx, rj, item, &eg)
	}

	// The write runs after the module checked ownership in the same
	// transaction, so a second River job for the item, or this one run again
	// after its commit, is refused before it writes anything. The write
	// re-reads the match and skip flag under the game's row lock, so a
	// rematch that committed after eg was loaded wins.
	var lw externalgame.LibraryWrite
	err = w.LC.EndItem(ctx, item.ID, rj.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusCompleted,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			var err error
			if lw, err = externalgame.WriteToLibrary(ctx, tx, item.UserID, eg.ID); err != nil {
				if errors.Is(err, externalgame.ErrSkipped) || cannotWrite(err) {
					return nil, joblifecycle.Abort(err)
				}
				return nil, err
			}
			return nil, InsertLibraryWriteChanges(ctx, tx, item.JobID, item.UserID, lw, lw.Result.Created)
		},
	})
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return ctx.Err()
	case joblifecycle.IsRefusal(err):
		return exitOrRetry(ctx, err)
	case errors.Is(err, externalgame.ErrSkipped):
		return w.endSkipped(ctx, rj, item, &eg)
	case cannotWrite(err), lastAttempt(rj.JobRow):
		return failItem(ctx, w.LC, item.ID, rj.ID, err.Error())
	default:
		return err
	}

	// Immediate metadata fetch for a game with no description yet: not Job
	// work, inserted after the commit. Checked here so an instance without
	// IGDB does not queue a no-op per game.
	if w.IGDBClient != nil && w.IGDBClient.Configured() {
		EnqueueImmediateMetadataFetch(ctx, w.DB, w.RiverClient, lw.IGDBID)
	}
	return nil
}

// endSkipped ends the item of a skipped game as skipped, with its
// 'skipped' diff row as the transition's own write.
func (w *UserGameWorker) endSkipped(ctx context.Context, rj *river.Job[UserGameArgs], item *models.JobItem, eg *models.ExternalGame) error {
	return exitOrRetry(ctx, w.LC.EndItem(ctx, item.ID, rj.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusSkipped,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			// The dispatch may have written it for this Job: one row per Job.
			return nil, InsertSkippedChange(ctx, tx, item.JobID, item.UserID, eg.ID, eg.Title)
		},
	}))
}
