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
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/darkadia"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/matching"
)

// ── Stage 1: match ───────────────────────────────────────────────────────────

type DarkadiaMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (DarkadiaMatchArgs) Kind() string { return "darkadia_match" }
func (DarkadiaMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 3}
}

type DarkadiaMatchWorker struct {
	river.WorkerDefaults[DarkadiaMatchArgs]
	DB          *bun.DB
	IGDBClient  *igdb.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *DarkadiaMatchWorker) Work(ctx context.Context, job *river.Job[DarkadiaMatchArgs]) error {
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.Error("darkadia_match: load job_item", "id", job.Args.JobItemID, "err", err)
		return nil
	}

	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		darkadiaMarkPendingReview(ctx, w.DB, &item, nil, nil)
		DarkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	candidates, err := w.IGDBClient.SearchGames(ctx, item.SourceTitle, 10, nil)
	if err != nil {
		if job.Attempt >= job.MaxAttempts {
			slog.Warn("darkadia_match: IGDB failed on final attempt, pending_review", "item_id", item.ID, "err", err)
			darkadiaMarkPendingReview(ctx, w.DB, &item, nil, nil)
			DarkadiaCheckJobCompletion(w.DB, item.JobID)
			return nil
		}
		return fmt.Errorf("darkadia_match: search failed (will retry): %w", err)
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
			slog.Error("darkadia_match: set resolved id", "err", err, "item_id", item.ID)
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, DarkadiaFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.Error("darkadia_match: enqueue finalize", "err", err, "item_id", item.ID)
			DarkadiaCheckJobCompletion(w.DB, item.JobID)
		}
		return nil
	}

	candJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling candidates cannot fail
	bs := decision.BestScore
	darkadiaMarkPendingReview(ctx, w.DB, &item, candJSON, &bs)
	DarkadiaCheckJobCompletion(w.DB, item.JobID)
	return nil
}

func darkadiaMarkPendingReview(ctx context.Context, db *bun.DB, item *models.JobItem, candidates json.RawMessage, confidence *float64) {
	item.Status = models.JobItemStatusPendingReview
	if candidates != nil {
		item.IGDBCandidates = candidates
	}
	item.MatchConfidence = confidence
	if _, err := db.NewUpdate().Model(item).
		Column("status", "igdb_candidates", "match_confidence").
		Where("id = ?", item.ID).Exec(ctx); err != nil {
		slog.Error("darkadia_match: mark pending_review", "id", item.ID, "err", err)
	}
}

// ── Stage 2: finalize ────────────────────────────────────────────────────────

type DarkadiaFinalizeArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (DarkadiaFinalizeArgs) Kind() string { return "darkadia_finalize" }
func (DarkadiaFinalizeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type DarkadiaFinalizeWorker struct {
	river.WorkerDefaults[DarkadiaFinalizeArgs]
	DB          *bun.DB
	IGDBClient  *igdb.Client
	StoragePath string
}

func (w *DarkadiaFinalizeWorker) Work(ctx context.Context, job *river.Job[DarkadiaFinalizeArgs]) error {
	bg := context.Background()
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.Error("darkadia_finalize: load job_item", "id", job.Args.JobItemID, "err", err)
		return nil
	}
	if item.ResolvedIGDBID == nil {
		markItemFailed(bg, w.DB, &item, "no resolved IGDB id", "darkadia_finalize: markItemFailed")
		DarkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}
	igdbID := int32(*item.ResolvedIGDBID) //nolint:gosec // resolved id fits int32

	var payload darkadia.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("parse payload: %v", err), "darkadia_finalize: markItemFailed")
		DarkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, igdbID, payload.Title); err != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("ensure game: %v", err), "darkadia_finalize: markItemFailed")
		DarkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	var ug models.UserGame
	err := w.DB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", item.UserID, igdbID).Scan(ctx)
	alreadyExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("load user_game: %v", err), "darkadia_finalize: markItemFailed")
		DarkadiaCheckJobCompletion(w.DB, item.JobID)
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
		ps := payload.PlayStatus
		ug = models.UserGame{
			ID: uuid.NewString(), UserID: item.UserID, GameID: igdbID,
			PlayStatus: &ps, PersonalRating: payload.PersonalRating, IsLoved: payload.IsLoved,
			PersonalNotes: payload.PersonalNotes, CreatedAt: created, UpdatedAt: now,
		}
		if _, ierr := w.DB.NewInsert().Model(&ug).Exec(ctx); ierr != nil {
			markItemFailed(bg, w.DB, &item, fmt.Sprintf("insert user_game: %v", ierr), "darkadia_finalize: markItemFailed")
			DarkadiaCheckJobCompletion(w.DB, item.JobID)
			return nil
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
	for _, pl := range payload.Platforms {
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
		if _, ierr := w.DB.NewInsert().Model(&ugp).Exec(ctx); ierr != nil {
			slog.Error("darkadia_finalize: insert platform", "err", ierr)
		} else {
			newPlatforms++
		}
	}

	changeType := "added"
	if alreadyExists {
		if newPlatforms > 0 {
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
		slog.Error("darkadia_finalize: insert change", "err", err)
	}

	markItemCompletedWithResult(bg, w.DB, &item, map[string]any{
		"game_id": igdbID, "user_game_id": ug.ID, "is_new_addition": !alreadyExists,
	}, "darkadia_finalize: markItemCompleted")
	DarkadiaCheckJobCompletion(w.DB, item.JobID)
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

// DarkadiaCheckJobCompletion finalizes a Darkadia import job once no active or
// pending_review items remain. pending_review blocks termination (the job stays
// processing until the user resolves/skips every such item).
func DarkadiaCheckJobCompletion(db *bun.DB, jobID string) {
	ctx := context.Background()
	active, ok := countJobItems(ctx, db, jobID, "status IN ('pending', 'processing')", "darkadia: count active")
	if !ok || active > 0 {
		return
	}
	review, ok := countJobItems(ctx, db, jobID, "status = 'pending_review'", "darkadia: count pending_review")
	if !ok || review > 0 {
		return
	}
	if !finalizeJobCompleted(ctx, db, jobID, "darkadia: finalize job", false) {
		return
	}
	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
	failed, ok := countJobItems(ctx, db, jobID, "status = 'failed'", "darkadia: count failed")
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
