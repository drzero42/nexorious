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
			slog.WarnContext(ctx, "import_match: set resolved id from payload", logging.KeyErr, err, "item_id", item.ID, logging.Cat(logging.CategoryDB))
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, ImportFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.WarnContext(ctx, "import_match: enqueue finalize (igdb-id short-circuit)", logging.KeyErr, err, "item_id", item.ID, logging.Cat(logging.CategoryDB))
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
			slog.WarnContext(ctx, "import_match: IGDB failed on final attempt, pending_review", "item_id", item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
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
			slog.WarnContext(ctx, "import_match: set resolved id", logging.KeyErr, err, "item_id", item.ID, logging.Cat(logging.CategoryDB))
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, ImportFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.WarnContext(ctx, "import_match: enqueue finalize", logging.KeyErr, err, "item_id", item.ID, logging.Cat(logging.CategoryDB))
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

	var ug models.UserGame
	err := w.DB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", item.UserID, igdbID).Scan(ctx)
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
		ug = models.UserGame{
			ID: uuid.NewString(), UserID: item.UserID, GameID: igdbID,
			PlayStatus: ps, PersonalRating: payload.PersonalRating, IsLoved: payload.IsLoved,
			IsWishlisted:  payload.IsWishlisted,
			PersonalNotes: payload.PersonalNotes, CreatedAt: created, UpdatedAt: now,
		}
		// ON CONFLICT DO NOTHING guards against a concurrent finalize of another
		// item that resolved to the same game (duplicate titles in one import):
		// the loser re-selects the winner's row and proceeds as the merge path,
		// rather than failing on the user_games (user_id, game_id) unique index.
		res, ierr := w.DB.NewInsert().Model(&ug).On("CONFLICT (user_id, game_id) DO NOTHING").Exec(ctx)
		if ierr != nil {
			markItemFailed(bg, w.DB, &item, fmt.Sprintf("insert user_game: %v", ierr), "import_finalize: markItemFailed")
			ImportCheckJobCompletion(w.DB, item.JobID)
			return nil
		}
		if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // advisory RowsAffected
			if serr := w.DB.NewSelect().Model(&ug).
				Where("user_id = ? AND game_id = ?", item.UserID, igdbID).Scan(ctx); serr != nil {
				markItemFailed(bg, w.DB, &item, fmt.Sprintf("load conflicting user_game: %v", serr), "import_finalize: markItemFailed")
				ImportCheckJobCompletion(w.DB, item.JobID)
				return nil
			}
			alreadyExists = true
		}
	}

	existing := map[[2]string]bool{}
	if alreadyExists {
		var ugps []models.UserGamePlatform
		if err := w.DB.NewSelect().Model(&ugps).Where("user_game_id = ?", ug.ID).Scan(ctx); err == nil {
			for _, p := range ugps {
				existing[[2]string{deref(p.Platform), deref(p.Storefront)}] = true
			}
		}
	}
	owned := "owned"
	newPlatforms := 0
	for i, pl := range payload.Platforms {
		sf := pl.Storefront
		if existing[[2]string{pl.Platform, deref(sf)}] {
			continue
		}
		platform := pl.Platform
		ugp := models.UserGamePlatform{
			ID: uuid.NewString(), UserGameID: ug.ID, Platform: &platform, Storefront: sf,
			OwnershipStatus: &owned, AcquiredDate: parseDateOnly(pl.AcquiredDate),
			CreatedAt: now, UpdatedAt: now,
		}
		// Game-level total playtime lands on the first consolidated entry only,
		// and only when that entry is newly inserted (additive merge). If the
		// first entry already exists (or there are no platforms), the playtime
		// has no home and is intentionally dropped rather than overwritten.
		if i == 0 {
			ugp.HoursPlayed = payload.HoursPlayed
		}
		if _, ierr := w.DB.NewInsert().Model(&ugp).Exec(ctx); ierr != nil {
			slog.WarnContext(ctx, "import_finalize: insert platform", logging.KeyErr, ierr, logging.Cat(logging.CategoryDB))
		} else {
			newPlatforms++
		}
	}
	if err := usergame.ClearWishlistOnAcquire(ctx, w.DB, ug.ID); err != nil {
		slog.WarnContext(ctx, "import_finalize: clear wishlist on acquire", logging.KeyErr, err, "user_game_id", ug.ID, logging.Cat(logging.CategoryDB))
	}

	existingTagIDs := map[string]bool{}
	if alreadyExists {
		var existingUGTs []models.UserGameTag
		if err := w.DB.NewSelect().Model(&existingUGTs).Where("user_game_id = ?", ug.ID).Scan(ctx); err == nil {
			for _, ugt := range existingUGTs {
				existingTagIDs[ugt.TagID] = true
			}
		}
	}
	newTags := 0
	for _, name := range payload.Tags {
		tagID, terr := findOrCreateTag(ctx, w.DB, item.UserID, name, nil)
		if terr != nil {
			// Unlike the JSON importer (which fails the item), a tag error here
			// is logged and skipped: this is a one-off migration where dropping
			// one tag link is preferable to failing the whole game import.
			slog.WarnContext(ctx, "import_finalize: find/create tag", logging.KeyErr, terr, "name", name, logging.Cat(logging.CategoryDB))
			continue
		}
		if existingTagIDs[tagID] {
			continue
		}
		ugt := &models.UserGameTag{ID: uuid.NewString(), UserGameID: ug.ID, TagID: tagID, CreatedAt: now}
		if _, ierr := w.DB.NewInsert().Model(ugt).Exec(ctx); ierr != nil {
			slog.WarnContext(ctx, "import_finalize: insert user_game_tag", logging.KeyErr, ierr, logging.Cat(logging.CategoryDB))
		} else {
			newTags++
		}
	}

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
		"game_id": igdbID, "user_game_id": ug.ID, "is_new_addition": !alreadyExists,
	}, "import_finalize: markItemCompleted")
	ImportCheckJobCompletion(w.DB, item.JobID)
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
