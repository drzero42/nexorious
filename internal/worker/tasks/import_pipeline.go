package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/importmodel"
	"github.com/drzero42/nexorious/internal/services/matching"
	"github.com/drzero42/nexorious/internal/usergame"
)

// ── Stage 1: match ───────────────────────────────────────────────────────────

type ImportMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ImportMatchArgs) Kind() string { return "import_match" }

// InsertOpts: several attempts, as for every lifecycle item kind, so a hard
// kill does not fail the item at startup. An IGDB error is retried; on the
// last attempt the item goes to Pending review instead.
func (ImportMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type ImportMatchWorker struct {
	river.WorkerDefaults[ImportMatchArgs]
	DB         *bun.DB
	LC         *joblifecycle.Module
	IGDBClient *igdb.Client
}

// Work matches an import item's title against IGDB. A confident match, or a
// payload that already carries an IGDB id, hands the item to finalize; no
// match, no IGDB, or IGDB failing on the last attempt sends it to Pending
// review.
func (w *ImportMatchWorker) Work(ctx context.Context, job *river.Job[ImportMatchArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, job.Args.JobItemID, job.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)

	toFinalize := func(igdbID int, confidence float64) error {
		return exitOrRetry(ctx, w.LC.NextStage(ctx, item.ID, job.ID, joblifecycle.Stage{
			Args:           ImportFinalizeArgs{JobItemID: item.ID},
			ResolvedIGDBID: &igdbID, MatchConfidence: &confidence,
		}))
	}
	toReview := func(candidates json.RawMessage, confidence *float64) error {
		return exitOrRetry(ctx, w.LC.AwaitReview(ctx, item.ID, job.ID, joblifecycle.Review{
			Candidates: candidates, MatchConfidence: confidence,
		}))
	}

	// A row that already carries a real IGDB id (e.g. a re-imported Nexorious
	// CSV export) needs no title matching: finalize hydrates it through
	// ensureGameRow (a title-only stub if IGDB does not know the id). A
	// nil/<=0 id falls through to title matching.
	var payload importmodel.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err == nil &&
		payload.IGDBID != nil && *payload.IGDBID > 0 {
		return toFinalize(int(*payload.IGDBID), 1)
	}

	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		return toReview(nil, nil)
	}

	candidates, err := w.IGDBClient.SearchGames(ctx, item.SourceTitle, 10, nil)
	if err != nil {
		// Checked first: a search cut off by a shutdown or a Cancel is
		// neither an IGDB failure nor a reason to ask the user.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if lastAttempt(job.JobRow) {
			slog.WarnContext(ctx, "import_match: IGDB failed on final attempt, pending_review", logging.KeyJobItemID, item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
			return toReview(nil, nil)
		}
		return fmt.Errorf("import_match: search failed (will retry): %w", err)
	}

	cands := make([]matching.Candidate, len(candidates))
	for i, c := range candidates {
		cands[i] = matching.Candidate{ID: int32(c.IgdbID), Title: c.Title} //nolint:gosec // IGDB ids are positive, fit int32
	}
	decision := matching.Decide(item.SourceTitle, cands)
	if decision.Confident {
		return toFinalize(int(decision.ResolvedID), decision.BestScore)
	}

	candJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling candidates cannot fail
	bs := decision.BestScore
	return toReview(candJSON, &bs)
}

// ── Stage 2: finalize ────────────────────────────────────────────────────────

type ImportFinalizeArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ImportFinalizeArgs) Kind() string { return "import_finalize" }

// InsertOpts: several attempts. The library write commits with the item's
// completion, so running it again is safe.
func (ImportFinalizeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type ImportFinalizeWorker struct {
	river.WorkerDefaults[ImportFinalizeArgs]
	DB          *bun.DB
	LC          *joblifecycle.Module
	IGDBClient  *igdb.Client
	StoragePath string
}

// Work writes a matched import item to the library. The games row and cover
// art are ensured first, outside the transition (safe to repeat, and slow);
// the library write, its changes row and the item's result then commit with
// the item's completion.
func (w *ImportFinalizeWorker) Work(ctx context.Context, job *river.Job[ImportFinalizeArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, job.Args.JobItemID, job.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)
	fail := func(msg string) error { return failItem(ctx, w.LC, item.ID, job.ID, msg) }

	if item.ResolvedIGDBID == nil {
		return fail("no resolved IGDB id")
	}
	igdbID := int32(*item.ResolvedIGDBID) //nolint:gosec // resolved id fits int32

	var payload importmodel.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err != nil {
		return fail(fmt.Sprintf("parse payload: %v", err))
	}

	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, igdbID, payload.Title); err != nil {
		return retryOrFail(ctx, w.LC, job.JobRow, item.ID, "ensure game", err)
	}

	// Acquire(ModeImport) writes the meta fields and the imported created_at
	// only when it creates the row; an existing row is left untouched. Its
	// ON CONFLICT DO UPDATE also covers two items resolving to the same game:
	// the second takes the merge path.
	ps := coercePlayStatus(ctx, &payload.PlayStatus)
	createdAt := parseDateOnly(payload.CreatedAt)

	// Game-level HoursPlayed goes on the first platform only.
	owned := "owned"
	plats := make([]usergame.PlatformInput, 0, len(payload.Platforms))
	for i, pl := range payload.Platforms {
		platform := pl.Platform
		pi := usergame.PlatformInput{
			Platform:        &platform,
			Storefront:      pl.Storefront,
			OwnershipStatus: &owned,
			AcquiredDate:    parseDateOnly(pl.AcquiredDate),
			// SyncFromSource intentionally omitted (false) — imports are not storefront syncs.
		}
		if i == 0 {
			pi.HoursPlayed = payload.HoursPlayed
		}
		plats = append(plats, pi)
	}
	tags := make([]usergame.TagInput, 0, len(payload.Tags))
	for _, name := range payload.Tags {
		tags = append(tags, usergame.TagInput{Name: name})
	}

	err = w.LC.EndItem(ctx, item.ID, job.ID, joblifecycle.ItemResult{
		Status: models.JobItemStatusCompleted,
		Own: func(ctx context.Context, tx bun.Tx) (json.RawMessage, error) {
			res, err := usergame.Acquire(ctx, tx, usergame.AcquireParams{
				UserID: item.UserID, GameID: igdbID, Mode: usergame.ModeImport,
				Platforms: plats, Tags: tags, TagMode: usergame.TagMerge,
				PlayStatus: ps, PersonalRating: payload.PersonalRating,
				IsLoved: payload.IsLoved, IsWishlisted: payload.IsWishlisted, PersonalNotes: payload.PersonalNotes,
				CreatedAt: createdAt,
			})
			if err != nil {
				return nil, acquireError(err)
			}
			if _, err := tx.NewRaw(
				`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, NULL, ?, ?, ?, now())`,
				uuid.NewString(), item.JobID, item.UserID, res.UserGameID, importChangeType(res), item.SourceTitle,
			).Exec(ctx); err != nil {
				return nil, fmt.Errorf("insert change: %w", err)
			}
			return json.Marshal(map[string]any{
				"game_id": igdbID, "user_game_id": res.UserGameID, "is_new_addition": res.Created,
			})
		},
	})
	return endImportItem(ctx, w.LC, job.JobRow, item.ID, err)
}

// importChangeType is the changes row's change_type for an Acquire result.
func importChangeType(res usergame.Result) enum.ChangeType {
	if res.Created {
		return enum.ChangeAdded
	}
	for _, ch := range res.PlatformChanges {
		if ch.Created {
			return enum.ChangeUpdated
		}
	}
	if res.TagsAdded > 0 {
		return enum.ChangeUpdated
	}
	return enum.ChangeAlreadyInLibrary
}

// errAcquire marks an Acquire error that will not go away by trying again.
var errAcquire = errors.New("acquire")

// acquireError turns an Acquire error into what the item's own write
// returns: a validation error (an unknown platform or storefront) rolls the
// transition back on purpose and fails the item; anything else is returned
// to River.
func acquireError(err error) error {
	if errors.Is(err, usergame.ErrUnprocessable) || errors.Is(err, usergame.ErrValidation) {
		return joblifecycle.Abort(fmt.Errorf("%w: %w", errAcquire, err))
	}
	return fmt.Errorf("acquire: %w", err)
}

// endImportItem turns the result of an import item's EndItem into what
// its worker returns: an Acquire validation error fails the item, in a new
// transaction; another error is retried, or fails the item on the last
// attempt.
func endImportItem(ctx context.Context, lc *joblifecycle.Module, rj *rivertype.JobRow, itemID string, err error) error {
	if err == nil || ctx.Err() != nil || joblifecycle.IsRefusal(err) {
		return exitOrRetry(ctx, err)
	}
	if errors.Is(err, errAcquire) {
		return failItem(ctx, lc, itemID, rj.ID, err.Error())
	}
	return retryOrFail(ctx, lc, rj, itemID, "end item", err)
}

// retryOrFail returns an error that may pass to River, or fails the item with
// it on the worker's last attempt. A cancelled context is returned as is.
func retryOrFail(ctx context.Context, lc *joblifecycle.Module, rj *rivertype.JobRow, itemID, what string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if lastAttempt(rj) {
		return failItem(ctx, lc, itemID, rj.ID, fmt.Sprintf("%s: %v", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func parseDateOnly(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	return nil
}

// ensureGameRow inserts the games row for igdbID if absent, fetching full IGDB
// metadata + cover when the client is configured, else a title-only stub.
func ensureGameRow(ctx context.Context, db *bun.DB, client *igdb.Client, storagePath string, igdbID int32, title string) error {
	var existing models.Game
	if db.NewSelect().Model(&existing).Where("id = ?", igdbID).Scan(ctx) == nil {
		return nil
	}
	if client != nil && client.Configured() {
		if md, err := client.FetchFullMetadata(ctx, int(igdbID)); err == nil {
			g := igdbMetadataToGame(md)
			if md.CoverImageID != "" {
				if localURL, derr := client.DownloadCoverArt(ctx, md.CoverImageID, storagePath); derr == nil {
					g.CoverArtUrl = &localURL
				}
			}
			if _, ierr := db.NewInsert().Model(g).On("CONFLICT (id) DO NOTHING").Exec(ctx); ierr != nil {
				return ierr
			}
			return nil
		}
	}
	_, err := db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID, title,
	).Exec(ctx)
	return err
}
