package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
)

// QueueMetadataRefresh is the dedicated, low-concurrency River queue for
// metadata_refresh_item jobs. Keeping refresh items off the default queue stops
// the nightly batch from starving user-initiated syncs/imports; its low worker
// count (config METADATA_REFRESH_WORKERS, default 1) also caps how much of the
// shared IGDB rate budget the refresh consumes. Registered in cmd/nexorious/serve.go.
const QueueMetadataRefresh = "metadata_refresh"

// ─── Dispatch worker ─────────────────────────────────────────────────────────

// MetadataRefreshDispatchArgs is the River job args type for "metadata_refresh_dispatch".
// JobID, when set, names the Job an admin created (internal/joblifecycle, with
// this River job as its dispatch owner); empty means the scheduled path, where
// the dispatch runs before any Job exists and creates its Job with all items.
type MetadataRefreshDispatchArgs struct {
	JobID string `json:"job_id,omitempty"`
}

func (MetadataRefreshDispatchArgs) Kind() string { return "metadata_refresh_dispatch" }

func (MetadataRefreshDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

// MetadataRefreshDispatchWorker selects the games to refresh (optionally only
// those older than MinAge, oldest first) and creates one item per game.
type MetadataRefreshDispatchWorker struct {
	river.WorkerDefaults[MetadataRefreshDispatchArgs]
	DB         *bun.DB
	LC         *joblifecycle.Module
	IGDBClient *igdbsvc.Client
	// MinAge, when > 0, restricts dispatch to games whose last_updated is older
	// than the window. 0 means "all games" (back-compat for unset callers).
	MinAge time.Duration
}

// Timeout leaves room for selecting and inserting thousands of items.
func (w *MetadataRefreshDispatchWorker) Timeout(*river.Job[MetadataRefreshDispatchArgs]) time.Duration {
	return dispatchTimeout
}

// dispatchTimeout bounds a refresh dispatch, which inserts every item in one
// transaction.
const dispatchTimeout = 10 * time.Minute

func (w *MetadataRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshDispatchArgs]) error {
	if job.Args.JobID != "" {
		return w.dispatchJob(ctx, job)
	}
	return w.dispatchScheduled(ctx, job)
}

// dispatchJob fills a Job an admin created.
func (w *MetadataRefreshDispatchWorker) dispatchJob(ctx context.Context, rj *river.Job[MetadataRefreshDispatchArgs]) error {
	jobID := rj.Args.JobID
	ctx = logging.WithJobID(ctx, jobID)
	j, err := w.LC.StartDispatch(ctx, jobID, rj.ID)
	if err != nil {
		return exitOrRetry(ctx, err)
	}
	if !w.IGDBClient.Configured() {
		return failMaintDispatch(ctx, w.LC, jobID, rj.JobRow, "metadata_refresh_dispatch", "IGDB is not configured")
	}
	games, err := w.selectGames(ctx)
	if err != nil {
		if ctx.Err() == nil && lastAttempt(rj.JobRow) {
			return failMaintDispatch(ctx, w.LC, jobID, rj.JobRow, "metadata_refresh_dispatch", err.Error())
		}
		return exitOrRetry(ctx, err)
	}
	n := len(games)
	err = w.LC.MarkDispatched(ctx, jobID, rj.ID, joblifecycle.Dispatched{
		Items:      metadataItems(games, j.UserID),
		TotalItems: &n,
		Own: func(ctx context.Context, tx bun.Tx) error {
			if n == 0 {
				return nil // an empty refresh completes and tells nobody
			}
			return notify.EmitTx(ctx, tx, maintCompletedNotice(jobID, "metadata_refresh_dispatch", n))
		},
	})
	if err != nil {
		if ctx.Err() == nil && !joblifecycle.IsRefusal(err) && lastAttempt(rj.JobRow) {
			return failMaintDispatch(ctx, w.LC, jobID, rj.JobRow, "metadata_refresh_dispatch", err.Error())
		}
		return exitOrRetry(ctx, err)
	}
	slog.InfoContext(ctx, "metadata_refresh_dispatch: dispatched", "game_count", n)
	return nil
}

// dispatchScheduled runs before any Job exists: it creates the Job with all
// its items, or nothing when there is nothing to do.
func (w *MetadataRefreshDispatchWorker) dispatchScheduled(ctx context.Context, rj *river.Job[MetadataRefreshDispatchArgs]) error {
	if !w.IGDBClient.Configured() {
		slog.WarnContext(ctx, "metadata_refresh_dispatch: IGDB not configured, skipping")
		return nil
	}
	var ownerID string
	err := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true ORDER BY created_at LIMIT 1`).Scan(ctx, &ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		slog.WarnContext(ctx, "metadata_refresh_dispatch: no admin user found, skipping")
		return nil
	}
	if err != nil {
		return maintDispatchError(ctx, w.DB, rj.JobRow, "metadata_refresh_dispatch", fmt.Errorf("query admin user: %w", err))
	}
	games, err := w.selectGames(ctx)
	if err != nil {
		return maintDispatchError(ctx, w.DB, rj.JobRow, "metadata_refresh_dispatch", err)
	}
	if len(games) == 0 {
		return nil
	}
	jobID := uuid.NewString()
	c, err := w.LC.CreateWithItems(ctx, joblifecycle.NewJob{
		ID: jobID, UserID: ownerID, JobType: models.JobTypeMetadataRefresh, Source: models.JobSourceSystem,
		Priority: models.JobPriorityLow, TotalItems: len(games),
	}, metadataItems(games, ownerID), func(ctx context.Context, tx bun.Tx) error {
		return notify.EmitTx(ctx, tx, maintCompletedNotice(jobID, "metadata_refresh_dispatch", len(games)))
	})
	if err != nil {
		return maintDispatchError(ctx, w.DB, rj.JobRow, "metadata_refresh_dispatch", err)
	}
	if !c.Created {
		slog.InfoContext(ctx, "metadata_refresh_dispatch: job already active, skipping", "existing_job_id", c.JobID)
		return nil
	}
	slog.InfoContext(ctx, "metadata_refresh_dispatch: job created", logging.KeyJobID, jobID, "game_count", len(games))
	return nil
}

type refreshGame struct {
	ID    int32  `bun:"id"`
	Title string `bun:"title"`
}

// selectGames returns the games to refresh, oldest-refreshed first, skipping
// games refreshed within MinAge. last_updated is NOT NULL (DEFAULT now()), so a
// never-refreshed game carries its creation time and ages into eligibility.
func (w *MetadataRefreshDispatchWorker) selectGames(ctx context.Context) ([]refreshGame, error) {
	var games []refreshGame
	var err error
	if w.MinAge > 0 {
		err = w.DB.NewRaw(
			`SELECT id, title FROM games
			 WHERE last_updated < now() - make_interval(secs => ?)
			 ORDER BY last_updated ASC`,
			w.MinAge.Seconds(),
		).Scan(ctx, &games)
	} else {
		err = w.DB.NewRaw(`SELECT id, title FROM games ORDER BY last_updated ASC`).Scan(ctx, &games)
	}
	if err != nil {
		return nil, fmt.Errorf("query games: %w", err)
	}
	return games, nil
}

func metadataItems(games []refreshGame, ownerID string) []joblifecycle.NewItem {
	items := make([]joblifecycle.NewItem, len(games))
	for i, g := range games {
		meta, _ := json.Marshal(metadataRefreshSourceMeta{GameID: g.ID}) //nolint:errcheck // marshaling a fixed struct cannot fail
		items[i] = joblifecycle.NewItem{
			UserID: ownerID, ItemKey: strconv.Itoa(int(g.ID)), SourceTitle: g.Title, SourceMetadata: meta,
		}
	}
	return items
}

func maintCompletedNotice(jobID, action string, count int) notify.EmitParams {
	return notify.EmitParams{
		Type: notify.TypeAdminMaintCompleted, Scope: notify.ScopeAdmin,
		Payload:  notify.MaintPayload{Action: action, Count: count},
		DedupKey: maintDedupKey(jobID, notify.TypeAdminMaintCompleted),
	}
}

func maintFailedNotice(riverJobID int64, action, msg string) notify.EmitParams {
	return notify.EmitParams{
		Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
		Payload:  notify.MaintPayload{Action: action, Error: logging.ScrubURLQueries(msg)},
		DedupKey: maintDedupKey(riverJobID, notify.TypeAdminMaintFailed),
	}
}

// failMaintDispatch fails a refresh Job whose dispatch cannot go on, telling
// the admin in the same transaction, so the Job is finished at once and the
// admin is told once.
func failMaintDispatch(ctx context.Context, lc *joblifecycle.Module, jobID string, rj *rivertype.JobRow, action, msg string) error {
	return exitOrRetry(ctx, lc.Fail(ctx, jobID, joblifecycle.Failure{
		Message:          msg,
		CallerRiverJobID: rj.ID,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			return nil, notify.EmitTx(ctx, tx, maintFailedNotice(rj.ID, action, msg))
		},
	}))
}

// maintDispatchError handles an error of a dispatch that has no Job yet: it
// is returned to River, and on the last attempt the admin is told.
func maintDispatchError(ctx context.Context, db *bun.DB, rj *rivertype.JobRow, action string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if lastAttempt(rj) {
		slog.ErrorContext(ctx, action+": giving up", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		notify.Emit(ctx, db, maintFailedNotice(rj.ID, action, err.Error()))
	}
	return err
}

// ─── Item worker ──────────────────────────────────────────────────────────────

// MetadataRefreshItemArgs is the River job args type for "metadata_refresh_item".
type MetadataRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (MetadataRefreshItemArgs) Kind() string { return "metadata_refresh_item" }

func (MetadataRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3, Queue: QueueMetadataRefresh}
}

// MetadataRefreshItemWorker is a River worker that fetches fresh IGDB
// metadata for a single game and updates the games row.
type MetadataRefreshItemWorker struct {
	river.WorkerDefaults[MetadataRefreshItemArgs]
	DB          *bun.DB
	LC          *joblifecycle.Module
	IGDBClient  *igdbsvc.Client
	StoragePath string
}

type metadataRefreshSourceMeta struct {
	GameID int32 `json:"game_id"`
}

// errGameGone means the game row no longer exists (the daily cleanup of
// unreferenced games can delete it while a refresh runs).
var errGameGone = errors.New("game no longer exists")

func (w *MetadataRefreshItemWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshItemArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, job.Args.JobItemID, job.ID)
	if done || err != nil {
		return err
	}
	// Correlate every line below (and the IGDB calls it makes) to the parent job.
	ctx = logging.WithJobID(ctx, item.JobID)
	fail := func(msg string) error { return failItem(ctx, w.LC, item.ID, job.ID, msg) }

	var sourceMeta metadataRefreshSourceMeta
	if err := json.Unmarshal(item.SourceMetadata, &sourceMeta); err != nil {
		return fail(fmt.Sprintf("parse source_metadata: %v", err))
	}
	if !w.IGDBClient.Configured() {
		return fail("igdb_not_configured")
	}

	md, err := fetchMetadata(ctx, w.DB, w.IGDBClient, w.StoragePath, sourceMeta.GameID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// A game IGDB does not know, a game that is gone, or credentials
		// IGDB rejected will not go away by trying again; anything else
		// (IGDB unreachable, the database) is retried, and fails the item on
		// the last attempt with its real message.
		if permanentMetadataError(err) || lastAttempt(job.JobRow) {
			return fail(err.Error())
		}
		return err
	}

	err = w.LC.FinishItem(ctx, item.ID, job.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusCompleted,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			n, err := writeGameMetadata(ctx, tx, sourceMeta.GameID, md)
			if err != nil {
				return nil, err
			}
			if n == 0 {
				return nil, joblifecycle.Abort(errGameGone)
			}
			return nil, nil
		},
	})
	if errors.Is(err, errGameGone) {
		return fail(errGameGone.Error())
	}
	return exitOrRetry(ctx, err)
}

func permanentMetadataError(err error) bool {
	return errors.Is(err, errGameGone) || errors.Is(err, igdbsvc.ErrGameNotFound) ||
		errors.Is(err, igdbsvc.ErrIGDBNotConfigured) || errors.Is(err, igdbsvc.ErrTwitchCredentialsRejected)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// fetchAndStoreMetadata fetches fresh IGDB metadata for a single game and writes
// it to the games row, including cover art (cover-art failure is non-fatal).
// It is the fire-and-forget MetadataFetchWorker's path; the refresh item worker
// calls fetchMetadata and writes the row inside its item's transition. The
// caller must verify IGDB is configured before calling.
func fetchAndStoreMetadata(ctx context.Context, db *bun.DB, igdbClient *igdbsvc.Client, storagePath string, gameID int32) error {
	md, err := fetchMetadata(ctx, db, igdbClient, storagePath, gameID)
	if err != nil {
		return err
	}
	if _, err := writeGameMetadata(ctx, db, gameID, md); err != nil {
		return err
	}
	return nil
}

// fetchMetadata loads the game, fetches its metadata from IGDB and saves its
// cover art (a progress write: safe to repeat, it does not record an
// outcome). A game row that does not exist is errGameGone.
func fetchMetadata(ctx context.Context, db *bun.DB, igdbClient *igdbsvc.Client, storagePath string, gameID int32) (*igdbsvc.GameMetadata, error) {
	// Load the current games row; cover_art_url drives the cover re-download decision.
	var game struct {
		ID          int32   `bun:"id"`
		Title       string  `bun:"title"`
		CoverArtUrl *string `bun:"cover_art_url"`
	}
	if err := db.NewRaw(
		`SELECT id, title, cover_art_url FROM games WHERE id = ?`, gameID,
	).Scan(ctx, &game); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errGameGone
		}
		return nil, fmt.Errorf("load game: %w", err)
	}

	md, err := igdbClient.FetchFullMetadata(ctx, int(gameID))
	if err != nil {
		return nil, err
	}

	// Cover art (non-fatal).
	if md.CoverImageID != "" {
		expectedURLPath := "/static/cover_art/" + md.CoverImageID + ".jpg"
		if game.CoverArtUrl == nil || *game.CoverArtUrl != expectedURLPath {
			coverURLPath, err := igdbClient.DownloadCoverArt(ctx, md.CoverImageID, storagePath)
			if err != nil {
				slog.WarnContext(ctx, "metadata fetch: cover art download failed",
					"game_id", game.ID, "image_id", md.CoverImageID, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
			} else if coverURLPath != "" {
				if _, err := db.NewRaw(
					`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
				).Exec(ctx); err != nil {
					slog.WarnContext(ctx, "metadata fetch: update cover_art_url failed", logging.KeyErr, err, "game_id", game.ID, logging.Cat(logging.CategoryDB))
				}
			}
		}
	}
	return md, nil
}

// writeGameMetadata writes md to the games row and returns how many rows it
// changed (0 when the game is gone).
func writeGameMetadata(ctx context.Context, db bun.IDB, gameID int32, md *igdbsvc.GameMetadata) (int64, error) {
	var releaseDate *time.Time
	if md.ReleaseDate != nil {
		if t, err := time.Parse("2006-01-02", *md.ReleaseDate); err == nil {
			releaseDate = &t
		}
	}

	var igdbSlug *string
	if md.IgdbSlug != "" {
		igdbSlug = &md.IgdbSlug
	}

	var igdbPlatformIds *string
	if len(md.PlatformIDs) > 0 {
		b, _ := json.Marshal(md.PlatformIDs) //nolint:errcheck // marshaling a fixed slice cannot fail
		s := string(b)
		igdbPlatformIds = &s
	}

	var igdbPlatformNames *string
	if len(md.PlatformNames) > 0 {
		b, _ := json.Marshal(md.PlatformNames) //nolint:errcheck // marshaling a fixed slice cannot fail
		s := string(b)
		igdbPlatformNames = &s
	}

	res, err := db.NewRaw(
		`UPDATE games SET
			title = ?,
			description = ?,
			genre = ?,
			developer = ?,
			publisher = ?,
			release_date = ?,
			rating_average = ?,
			rating_count = ?,
			howlongtobeat_main = ?,
			howlongtobeat_extra = ?,
			howlongtobeat_completionist = ?,
			igdb_slug = ?,
			igdb_platform_ids = ?,
			igdb_platform_names = ?,
			game_modes = ?,
			themes = ?,
			player_perspectives = ?,
			last_updated = now()
		WHERE id = ?`,
		md.Title,
		md.Description,
		md.Genre,
		md.Developer,
		md.Publisher,
		releaseDate,
		md.RatingAverage,
		md.RatingCount,
		md.HowlongtobeatMain,
		md.HowlongtobeatExtra,
		md.HowlongtobeatCompletionist,
		igdbSlug,
		igdbPlatformIds,
		igdbPlatformNames,
		md.GameModes,
		md.Themes,
		md.PlayerPerspectives,
		gameID,
	).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("update games: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("update games: %w", err)
	}
	return n, nil
}
