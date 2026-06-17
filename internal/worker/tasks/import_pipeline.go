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
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
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
func (ImportMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 3}
}

type ImportMatchWorker struct {
	river.WorkerDefaults[ImportMatchArgs]
	DB          *bun.DB
	IGDBClient  *igdb.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *ImportMatchWorker) Work(ctx context.Context, job *river.Job[ImportMatchArgs]) error {
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.ErrorContext(ctx, "import_match: load job_item", "id", job.Args.JobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
	// Correlate every line below to the parent import job.
	ctx = logging.WithJobID(ctx, item.JobID)

	// Short-circuit: a row that already carries a real IGDB id (e.g. a
	// re-imported Nexorious CSV export) needs no title matching. Trust it and
	// hand straight to finalize, which hydrates via ensureGameRow (falling back
	// to a title-only stub if IGDB doesn't recognize the id). Inert for sources
	// that never set IGDBID. A nil/<=0 id falls through to title matching below.
	var payload importmodel.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err == nil &&
		payload.IGDBID != nil && *payload.IGDBID > 0 {
		if _, err := w.DB.NewRaw(
			`UPDATE job_items SET resolved_igdb_id = ?, match_confidence = 1 WHERE id = ?`,
			*payload.IGDBID, item.ID,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "import_match: set resolved id from payload", logging.KeyErr, err, logging.KeyJobItemID, item.ID, logging.Cat(logging.CategoryDB))
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, ImportFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.WarnContext(ctx, "import_match: enqueue finalize (igdb-id short-circuit)", logging.KeyErr, err, logging.KeyJobItemID, item.ID, logging.Cat(logging.CategoryDB))
			ImportCheckJobCompletion(w.DB, item.JobID)
		}
		return nil
	}

	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		importMarkPendingReview(ctx, w.DB, &item, nil, nil)
		ImportCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	candidates, err := w.IGDBClient.SearchGames(ctx, item.SourceTitle, 10, nil)
	if err != nil {
		if job.Attempt >= job.MaxAttempts {
			slog.WarnContext(ctx, "import_match: IGDB failed on final attempt, pending_review", logging.KeyJobItemID, item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
			importMarkPendingReview(ctx, w.DB, &item, nil, nil)
			ImportCheckJobCompletion(w.DB, item.JobID)
			return nil
		}
		return fmt.Errorf("import_match: search failed (will retry): %w", err)
	}

	cands := make([]matching.Candidate, len(candidates))
	for i, c := range candidates {
		cands[i] = matching.Candidate{ID: int32(c.IgdbID), Title: c.Title} //nolint:gosec // IGDB ids are positive, fit int32
	}
	decision := matching.Decide(item.SourceTitle, cands)

	if decision.Confident {
		if _, err := w.DB.NewRaw(
			`UPDATE job_items SET resolved_igdb_id = ?, match_confidence = ? WHERE id = ?`,
			decision.ResolvedID, decision.BestScore, item.ID,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "import_match: set resolved id", logging.KeyErr, err, logging.KeyJobItemID, item.ID, logging.Cat(logging.CategoryDB))
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, ImportFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.WarnContext(ctx, "import_match: enqueue finalize", logging.KeyErr, err, logging.KeyJobItemID, item.ID, logging.Cat(logging.CategoryDB))
			ImportCheckJobCompletion(w.DB, item.JobID)
		}
		return nil
	}

	candJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling candidates cannot fail
	bs := decision.BestScore
	importMarkPendingReview(ctx, w.DB, &item, candJSON, &bs)
	ImportCheckJobCompletion(w.DB, item.JobID)
	return nil
}

func importMarkPendingReview(ctx context.Context, db *bun.DB, item *models.JobItem, candidates json.RawMessage, confidence *float64) {
	item.Status = models.JobItemStatusPendingReview
	if candidates != nil {
		item.IGDBCandidates = candidates
	}
	item.MatchConfidence = confidence
	if _, err := db.NewUpdate().Model(item).
		Column("status", "igdb_candidates", "match_confidence").
		Where("id = ?", item.ID).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "import_match: mark pending_review", "id", item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}

// ── Stage 2: finalize ────────────────────────────────────────────────────────

type ImportFinalizeArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ImportFinalizeArgs) Kind() string { return "import_finalize" }
func (ImportFinalizeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type ImportFinalizeWorker struct {
	river.WorkerDefaults[ImportFinalizeArgs]
	DB          *bun.DB
	IGDBClient  *igdb.Client
	StoragePath string
}

func (w *ImportFinalizeWorker) Work(ctx context.Context, job *river.Job[ImportFinalizeArgs]) error {
	bg := context.Background()
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.ErrorContext(ctx, "import_finalize: load job_item", "id", job.Args.JobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
	// Correlate every line below to the parent import job.
	ctx = logging.WithJobID(ctx, item.JobID)
	if item.ResolvedIGDBID == nil {
		markItemFailed(bg, w.DB, &item, "no resolved IGDB id", "import_finalize: markItemFailed")
		ImportCheckJobCompletion(w.DB, item.JobID)
		return nil
	}
	igdbID := int32(*item.ResolvedIGDBID) //nolint:gosec // resolved id fits int32

	var payload importmodel.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("parse payload: %v", err), "import_finalize: markItemFailed")
		ImportCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, igdbID, payload.Title); err != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("ensure game: %v", err), "import_finalize: markItemFailed")
		ImportCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	err := w.DB.NewSelect().Model(&models.UserGame{}).
		Where("user_id = ? AND game_id = ?", item.UserID, igdbID).Scan(ctx)
	alreadyExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("load user_game: %v", err), "import_finalize: markItemFailed")
		ImportCheckJobCompletion(w.DB, item.JobID)
		return nil
	}
	now := time.Now().UTC()
	if !alreadyExists {
		created := now
		if payload.CreatedAt != "" {
			if t, perr := time.Parse("2006-01-02", payload.CreatedAt); perr == nil {
				created = t.UTC()
			}
		}
		ps := coercePlayStatus(ctx, &payload.PlayStatus)
		ug := models.UserGame{
			ID: uuid.NewString(), UserID: item.UserID, GameID: igdbID,
			PlayStatus: ps, PersonalRating: payload.PersonalRating, IsLoved: payload.IsLoved,
			IsWishlisted:  payload.IsWishlisted,
			PersonalNotes: payload.PersonalNotes, CreatedAt: created, UpdatedAt: now,
		}
		// ON CONFLICT DO NOTHING guards against a concurrent finalize of another
		// item that resolved to the same game (duplicate titles in one import):
		// the loser re-selects the winner's row and proceeds as the merge path,
		// rather than failing on the user_games (user_id, game_id) unique index.
		insertRes, ierr := w.DB.NewInsert().Model(&ug).On("CONFLICT (user_id, game_id) DO NOTHING").Exec(ctx)
		if ierr != nil {
			markItemFailed(bg, w.DB, &item, fmt.Sprintf("insert user_game: %v", ierr), "import_finalize: markItemFailed")
			ImportCheckJobCompletion(w.DB, item.JobID)
			return nil
		}
		if n, _ := insertRes.RowsAffected(); n == 0 { //nolint:errcheck // advisory RowsAffected
			// Concurrent finalize won the race; treat this as a merge.
			alreadyExists = true
		}
	}

	// Build platform inputs. Game-level HoursPlayed goes on the first entry only,
	// matching the pre-refactor behaviour (playtime has no home if the first entry
	// already exists, so Acquire's max-hours merge naturally handles that case).
	owned := "owned"
	plats := make([]usergame.PlatformInput, 0, len(payload.Platforms))
	for i, pl := range payload.Platforms {
		platform := pl.Platform
		acquiredDate := parseDateOnly(pl.AcquiredDate)
		pi := usergame.PlatformInput{
			Platform:        &platform,
			Storefront:      pl.Storefront,
			OwnershipStatus: &owned,
			AcquiredDate:    acquiredDate,
			// SyncFromSource intentionally omitted (false) — imports are not storefront syncs.
		}
		// Game-level total playtime on the first entry only.
		if i == 0 {
			pi.HoursPlayed = payload.HoursPlayed
		}
		plats = append(plats, pi)
	}

	// Build tag inputs (tags are plain strings in the pipeline payload).
	tags := make([]usergame.TagInput, 0, len(payload.Tags))
	for _, name := range payload.Tags {
		tags = append(tags, usergame.TagInput{Name: name})
	}

	// Snapshot the tag count BEFORE Acquire so we can detect newly merged links.
	// (Acquire merges tags additively; counting after would make existingTagCount
	// equal totalTagCount and produce newTags = 0.)
	existingTagCount := 0
	if alreadyExists && len(tags) > 0 {
		if qerr := w.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_game_tags ugt"+
				" JOIN user_games ug ON ug.id = ugt.user_game_id"+
				" WHERE ug.user_id = ? AND ug.game_id = ?", item.UserID, igdbID,
		).Scan(&existingTagCount); qerr != nil {
			existingTagCount = 0
		}
	}

	// Acquire upserts the user_game (no-op since the row exists), merges platforms
	// (max-hours, ownership upgrade), clears wishlist on acquire, auto-promotes
	// play_status, and merges tags additively.
	acqRes, acqErr := usergame.Acquire(ctx, w.DB, usergame.AcquireParams{
		UserID: item.UserID, GameID: igdbID, Mode: usergame.ModeUpsert,
		Platforms: plats, Tags: tags, TagMode: usergame.TagMerge,
	})
	if acqErr != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("acquire: %v", acqErr), "import_finalize: markItemFailed")
		ImportCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	// Determine change type from Acquire results.
	newPlatforms := 0
	for _, ch := range acqRes.PlatformChanges {
		if ch.Created {
			newPlatforms++
		}
	}
	// Count total tags post-Acquire; diff against the pre-Acquire snapshot.
	var totalTagCount int
	if len(tags) > 0 {
		if qerr := w.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_game_tags WHERE user_game_id = ?", acqRes.UserGameID,
		).Scan(&totalTagCount); qerr != nil {
			totalTagCount = 0
		}
	}
	newTags := totalTagCount - existingTagCount

	changeType := "added"
	if alreadyExists {
		if newPlatforms > 0 || newTags > 0 {
			changeType = "updated"
		} else {
			changeType = "already_in_library"
		}
	}
	if _, err := w.DB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, NULL, ?, ?, now())`,
		uuid.NewString(), item.JobID, item.UserID, changeType, item.SourceTitle,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "import_finalize: insert change", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}

	markItemCompletedWithResult(bg, w.DB, &item, map[string]any{
		"game_id": igdbID, "user_game_id": acqRes.UserGameID, "is_new_addition": !alreadyExists,
	}, "import_finalize: markItemCompleted")
	ImportCheckJobCompletion(w.DB, item.JobID)
	return nil
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

// ImportCheckJobCompletion finalizes an import job once no active or
// pending_review items remain. pending_review blocks termination (the job stays
// processing until the user resolves/skips every such item).
func ImportCheckJobCompletion(db *bun.DB, jobID string) {
	ctx := context.Background()
	active, ok := countJobItems(ctx, db, jobID, "status IN ('pending', 'processing')", "import: count active")
	if !ok || active > 0 {
		return
	}
	review, ok := countJobItems(ctx, db, jobID, "status = 'pending_review'", "import: count pending_review")
	if !ok || review > 0 {
		return
	}
	if !finalizeJobCompleted(ctx, db, jobID, "import: finalize job", true) {
		return
	}
	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
	failed, ok := countJobItems(ctx, db, jobID, "status = 'failed'", "import: count failed")
	if !ok {
		return
	}
	if failed > 0 {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeImportFailed, Scope: notify.ScopeUser, ActorUserID: uid,
			Payload:  notify.ImportFailedPayload{JobID: jobID, Failed: failed, Error: fmt.Sprintf("%d item(s) failed to import", failed)},
			DedupKey: jobID + ":" + notify.TypeImportFailed,
		})
		return
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeImportCompleted, Scope: notify.ScopeUser, ActorUserID: uid,
		Payload:  notify.ImportCompletedPayload{JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeImportCompleted,
	})
}
