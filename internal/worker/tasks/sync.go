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
		// NOT EXISTS: a dispatch run again sees the same skipped games;
		// changes has no unique constraint.
		if _, err := tx.NewRaw(
			`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
			 SELECT ?, ?, ?, ?, 'skipped', ?, now()
			 WHERE NOT EXISTS (
			   SELECT 1 FROM changes WHERE job_id = ? AND external_game_id = ? AND change_type = 'skipped')`,
			uuid.NewString(), p.JobID, p.UserID, egID, e.Title, p.JobID, egID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("insert sync_change (skipped): %w", err)
		}
		return nil
	})
	return egID, isSkipped, err
}

func upsertExternalGame(ctx context.Context, db bun.IDB, e ExternalGameEntry, p DispatchSyncArgs) (egID string, isSkipped bool, err error) {
	var row struct {
		ID        string `bun:"id"`
		IsSkipped bool   `bun:"is_skipped"`
		IsNew     bool   `bun:"is_new"`
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
		RETURNING id, is_skipped, (xmax = 0) AS is_new`,
		uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
		e.IsSubscription, e.OwnershipStatus, nullableJSON(sourceMetaJSON),
	).Scan(ctx, &row); err != nil {
		return "", false, fmt.Errorf("upsert external_game: %w", err)
	}

	if row.IsNew {
		// A new edition of a known game becomes its child and takes its skip
		// (#1339). Only a game a child can inherit from qualifies: top-level and
		// available, matched or skipped. An unmatched, unskipped game that has
		// left the library could never end a child's wait. The oldest wins. One
		// statement, with the parent locked, so a skip committing in between is
		// not missed.
		var inheritedSkip bool
		err := db.NewRaw(`
			UPDATE external_games c
			SET parent_id = p.id, is_skipped = p.is_skipped
			FROM (
			  SELECT id, is_skipped FROM external_games
			  WHERE user_id = ? AND storefront = ? AND title = ?
			    AND id != ? AND parent_id IS NULL
			    AND (is_available OR resolved_igdb_id IS NOT NULL OR is_skipped)
			  ORDER BY created_at, id
			  LIMIT 1
			  FOR SHARE
			) p
			WHERE c.id = ? AND c.parent_id IS NULL
			RETURNING c.is_skipped`,
			p.UserID, p.Storefront, e.Title, row.ID, row.ID,
		).Scan(ctx, &inheritedSkip)
		switch {
		case err == nil:
			row.IsSkipped = inheritedSkip
		case !errors.Is(err, sql.ErrNoRows):
			return "", false, fmt.Errorf("set parent_id: %w", err)
		}
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

	// Refused when the Job is finished, when this River job is not its
	// dispatch owner, or when the dispatch is already done: a dispatch
	// hard-killed after marking done must not fetch the library again.
	if _, err := w.LC.StartDispatch(ctx, p.JobID, rj.ID); err != nil {
		return exitOrRetry(ctx, err)
	}

	// Child games (#1339): give every child its parent's skip, then promote the
	// children of parents that left in an earlier sync, so a new edition in this
	// fetch finds the promoted game as its parent. Failures are logged: the next
	// dispatch repairs what this one missed.
	if err := repairChildSkips(ctx, w.DB, p.UserID, p.Storefront); err != nil && ctx.Err() == nil {
		slog.WarnContext(ctx, "dispatch_sync: repair child skips failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	promoteOrphanedChildren(ctx, w.DB, p.UserID, p.Storefront)
	if ctx.Err() != nil {
		return ctx.Err()
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

	// Parents that left in this sync: promote their children (#1339).
	promoteOrphanedChildren(ctx, w.DB, p.UserID, p.Storefront)

	// The sweeps ran on ctx: if it was cancelled meanwhile they may be
	// unfinished, so the dispatch must not be marked done.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Mark the dispatch done, with last_synced_at and the cleared
	// credentials flag as this sync's own writes, after sync.recovered when
	// this fetch ends an outage. The module then queues the items that can no
	// longer wait and evaluates completion (an empty library completes here).
	return exitOrRetry(ctx, w.LC.MarkDispatched(ctx, p.JobID, rj.ID, joblifecycle.Dispatched{
		Own: func(ctx context.Context, tx bun.Tx) error {
			if err := emitSyncRecovered(ctx, tx, p); err != nil {
				return err
			}
			_, err := tx.NewRaw(
				`UPDATE user_sync_configs SET last_synced_at = ?, credentials_error = false, updated_at = now() WHERE user_id = ? AND storefront = ?`,
				time.Now().UTC(), p.UserID, p.Storefront,
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
		// changed it, so a second run writes nothing twice.
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
				 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
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

// boolptr returns a pointer to b.
func boolptr(b bool) *bool { return &b }

// ── IGDBMatchWorker — Stage 2 ─────────────────────────────────────────────────

type IGDBMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (IGDBMatchArgs) Kind() string { return "igdb_match" }

func (IGDBMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

// IGDBMatchWorker matches a sync item's game to IGDB, or hands it straight to
// the write stage when it is already matched or skipped. It is also what the
// module queues by itself for a sync item (Retry, recovery, an item that
// stops waiting).
type IGDBMatchWorker struct {
	river.WorkerDefaults[IGDBMatchArgs]
	DB         *bun.DB
	LC         *joblifecycle.Module
	IGDBClient *igdbsvc.Client
}

// maxChildWaitTries bounds how often a child that was told "cannot wait"
// starts again from loading its game (it may have been re-pointed to another
// parent meanwhile); after that the error goes to River.
const maxChildWaitTries = 3

func (w *IGDBMatchWorker) Work(ctx context.Context, rj *river.Job[IGDBMatchArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, rj.Args.JobItemID, rj.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)
	if item.ExternalGameID == nil {
		return failItem(ctx, w.LC, item.ID, rj.ID, "external_game_id not set on job_item")
	}

	cannotWaitOn := ""
	for range maxChildWaitTries {
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
		if eg.ParentID == nil {
			return w.search(ctx, rj, item, &eg)
		}

		// A child of a parent that is matched now takes its match.
		err := w.toWrite(ctx, rj, item.ID, inheritParentMatch(&eg))
		if !errors.Is(err, errParentUnmatched) {
			return err
		}
		// Told "cannot wait" on this parent before and it is still
		// unmatched: the child cannot follow it.
		if *eg.ParentID == cannotWaitOn {
			slog.InfoContext(ctx, "igdb_match: child cannot wait for its unmatched parent; item failed",
				logging.KeyJobItemID, item.ID, "parent_id", *eg.ParentID)
			return failItem(ctx, w.LC, item.ID, rj.ID, childUnmatchedParentMsg)
		}
		waiting, err := w.LC.Wait(ctx, item.ID, rj.ID)
		if err != nil {
			return exitOrRetry(ctx, err)
		}
		if waiting {
			slog.DebugContext(ctx, "igdb_match: parent unmatched, waiting", logging.KeyJobItemID, item.ID, "parent_id", *eg.ParentID)
			return nil
		}
		// Cannot wait: the parent's item has finished, or it has none and the
		// dispatch is done, or the game is no longer a child. Decide on the
		// game as it is now.
		cannotWaitOn = *eg.ParentID
	}
	return fmt.Errorf("igdb_match: child game kept changing parent; retrying")
}

// errParentUnmatched rolls back an inherit whose parent has no match.
var errParentUnmatched = errors.New("igdb_match: parent game is unmatched")

// inheritParentMatch is the own write that gives a child its parent's match:
// the parent is read FOR SHARE in the transition, so a child never takes a
// match a rematch in progress is replacing, and the child only takes it while
// it has none (#1338).
func inheritParentMatch(eg *models.ExternalGame) joblifecycle.OwnWrite {
	return func(ctx context.Context, tx bun.Tx) error {
		var parentMatch *int32
		if err := tx.NewRaw(
			`SELECT resolved_igdb_id FROM external_games WHERE id = ? FOR SHARE`, *eg.ParentID,
		).Scan(ctx, &parentMatch); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lock parent: %w", err)
		}
		if parentMatch == nil {
			return joblifecycle.Abort(errParentUnmatched)
		}
		if err := EnsureGameRow(ctx, tx, *parentMatch, eg.Title); err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ? AND resolved_igdb_id IS NULL`,
			*parentMatch, eg.ID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("apply child inherit: %w", err)
		}
		return nil
	}
}

// toWrite hands the item to the write stage, with own as the transition's
// own write. errParentUnmatched is returned as is, for the caller to decide.
func (w *IGDBMatchWorker) toWrite(ctx context.Context, rj *river.Job[IGDBMatchArgs], itemID string, own joblifecycle.OwnWrite) error {
	err := w.LC.NextStage(ctx, itemID, rj.ID, joblifecycle.Stage{Args: UserGameArgs{JobItemID: itemID}, Own: own})
	if errors.Is(err, errParentUnmatched) {
		return err
	}
	return exitOrRetry(ctx, err)
}

// search matches a top-level game through IGDB: a confident match goes to
// the write stage, anything else to Pending review.
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
		bestID := decision.ResolvedID
		// The games row and the match are this transition's own writes. IS
		// NULL: a rematch that committed during the search wins (#1338);
		// either way the write stage writes whatever match the game has.
		return w.toWrite(ctx, rj, item.ID, func(ctx context.Context, tx bun.Tx) error {
			if err := EnsureGameRow(ctx, tx, bestID, eg.Title); err != nil {
				return err
			}
			if _, err := tx.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ? AND resolved_igdb_id IS NULL`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("auto-resolve external_game: %w", err)
			}
			return nil
		})
	}

	candidatesJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling the candidates slice cannot fail
	bs := decision.BestScore
	return w.toReview(ctx, rj, item.ID, eg.ID, candidatesJSON, &bs)
}

// toReview puts the item in Pending review, unless the game was matched
// meanwhile (a rematch committed during the search): then it goes on to the
// write stage. The game is read FOR SHARE in the transition, so a rematch in
// progress is waited for (#1338).
func (w *IGDBMatchWorker) toReview(ctx context.Context, rj *river.Job[IGDBMatchArgs], itemID, externalGameID string, candidates json.RawMessage, confidence *float64) error {
	return exitOrRetry(ctx, w.LC.AwaitReview(ctx, itemID, rj.ID, joblifecycle.Review{
		Candidates: candidates, MatchConfidence: confidence,
		Divert: func(ctx context.Context, tx bun.Tx) (river.JobArgs, error) {
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
	return errors.Is(err, ErrExternalGameNotFound) || errors.Is(err, ErrExternalGameUnmatched) ||
		errors.Is(err, ErrExternalGameNoPlatforms) || errors.Is(err, usergame.ErrValidation) ||
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
		return w.finishSkipped(ctx, rj, item, &eg)
	}

	// The write runs after the module checked ownership in the same
	// transaction, so a second River job for the item, or this one run again
	// after its commit, is refused before it writes anything. The write
	// re-reads the match and skip flag under the game's row lock, so a
	// rematch that committed after eg was loaded wins.
	var lw LibraryWrite
	err = w.LC.FinishItem(ctx, item.ID, rj.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusCompleted,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			var err error
			if lw, err = WriteExternalGameToLibrary(ctx, tx, item.UserID, eg.ID); err != nil {
				if errors.Is(err, ErrExternalGameSkipped) || cannotWrite(err) {
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
	case errors.Is(err, ErrExternalGameSkipped):
		return w.finishSkipped(ctx, rj, item, &eg)
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

// finishSkipped finishes the item of a skipped game as skipped, with its
// 'skipped' diff row as the transition's own write.
func (w *UserGameWorker) finishSkipped(ctx context.Context, rj *river.Job[UserGameArgs], item *models.JobItem, eg *models.ExternalGame) error {
	return exitOrRetry(ctx, w.LC.FinishItem(ctx, item.ID, rj.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusSkipped,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			// NOT EXISTS: the dispatch may have written it for this Job.
			if _, err := tx.NewRaw(
				`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 SELECT ?, ?, ?, ?, 'skipped', ?, now()
				 WHERE NOT EXISTS (
				   SELECT 1 FROM changes WHERE job_id = ? AND external_game_id = ? AND change_type = 'skipped')`,
				uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title, item.JobID, eg.ID,
			).Exec(ctx); err != nil {
				return nil, fmt.Errorf("insert sync_change (skipped): %w", err)
			}
			return nil, nil
		},
	}))
}
