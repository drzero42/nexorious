package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Rematch and unskip are edits to one External game and its editions, not
// sync work (#1338, ADR 0002): they write the library in the request's own
// transaction with the same library write the sync worker uses, and create no
// Job, no item and no River job. Rematch, Skip and Unskip act on every edition
// of the game they are given, through any edition's id (#1449). With the
// option only_this_edition=true they act on the given edition alone (#1452):
// the whole set is still locked, but only that game is changed and only its
// own open items are touched.

// onlyThisEdition reads the only_this_edition query parameter: only "true"
// sets it; anything else acts on the whole set.
func onlyThisEdition(c *echo.Context) bool {
	return c.QueryParam("only_this_edition") == "true"
}

// errSyncStillWorking is the 409 while a sync worker may be writing the game.
const errSyncStillWorking = "a sync is still working on this game"

// lockEditions locks the user's External game id and its editions
// (externalgame.LockEditions) and answers its refusals as HTTP errors: a
// missing game 404, a set that kept changing 409. Each is marked so a module
// transaction rolls back without logging it as a failed transition; errors.As
// still finds the HTTP error.
func lockEditions(ctx context.Context, tx bun.Tx, userID, id string) (*externalgame.Editions, error) {
	set, err := externalgame.LockEditions(ctx, tx, userID, id)
	switch {
	case errors.Is(err, externalgame.ErrNotFound):
		return nil, joblifecycle.Abort(echo.NewHTTPError(http.StatusNotFound, "not found"))
	case errors.Is(err, externalgame.ErrEditionsChanged):
		return nil, joblifecycle.Abort(echo.NewHTTPError(http.StatusConflict, "the game changed while it was being updated; try again"))
	}
	return set, err
}

// HandleRematchExternalGame handles POST /api/sync/external-games/:id/rematch:
// it re-resolves an External game and its editions (or, with
// only_this_edition=true, the game alone) to a new IGDB id and writes them to
// the library, all in one transaction (#1338).
func (h *SyncHandler) HandleRematchExternalGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	onlyThis := onlyThisEdition(c)
	ctx := context.Background()
	reqCtx := c.Request().Context()

	var body struct {
		IGDBID       int    `json:"igdb_id"`
		OrphanAction string `json:"orphan_action"`
	}
	if err := c.Bind(&body); err != nil || body.IGDBID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "igdb_id is required")
	}

	// A plain read, no lock: the active Job in which the game or one of its
	// editions has an open item, preferring the game's own (with the option:
	// the game's own only). Then everything runs in the module's transaction
	// for that Job: the Job row lock first, then the editions, as a sync
	// worker does, so the two cannot deadlock.
	findJobs := externalgame.EditionsOpenItemJobs
	if onlyThis {
		findJobs = externalgame.OwnOpenItemJobs
	}
	jobIDs, err := findJobs(ctx, h.db, userID, id)
	if err != nil {
		slog.ErrorContext(reqCtx, "sync: rematch: find items", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rematch external game")
	}
	jobID := ""
	if len(jobIDs) > 0 {
		jobID = jobIDs[0]
	}
	req := externalgame.RematchRequest{
		IGDBID: body.IGDBID, IGDBTitle: h.igdbTitleForNewGame(reqCtx, userID, id, body.IGDBID),
		OrphanAction: body.OrphanAction, OnlyThisEdition: onlyThis,
	}

	if jobID != "" {
		_, err = h.lc.CompleteItems(ctx, jobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
			return rematchEditions(ctx, tx, userID, id, req, job.ID)
		})
		if errors.Is(err, joblifecycle.ErrJobNotActive) || errors.Is(err, joblifecycle.ErrGone) {
			jobID, err = "", nil // ended meanwhile: no items to touch
		}
	}
	if jobID == "" && err == nil {
		err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			_, err := rematchEditions(ctx, tx, userID, id, req, "")
			return err
		})
	}
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		slog.ErrorContext(reqCtx, "sync: rematch failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rematch external game")
	}

	tasks.EnqueueImmediateMetadataFetch(ctx, h.db, h.riverClient, int32(body.IGDBID)) //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
	return c.NoContent(http.StatusNoContent)
}

// igdbTitleForNewGame looks up the IGDB title of igdbID when a Rematch of
// the user's External game id would create its games row, so the row takes
// the IGDB title rather than the storefront's (#1455). It returns "" (the row
// takes the storefront title until the metadata fetch replaces it) when the
// game is not the user's, the row exists, IGDB is not configured or a step
// fails. It runs before the transaction, so no lock is held while IGDB
// answers.
func (h *SyncHandler) igdbTitleForNewGame(ctx context.Context, userID, id string, igdbID int) string {
	if h.igdbClient == nil || !h.igdbClient.Configured() {
		return ""
	}
	needs, err := externalgame.NeedsGameRow(ctx, h.db, userID, id, igdbID)
	if err != nil {
		slog.WarnContext(ctx, "sync: rematch: check games row, using the storefront title",
			logging.KeyErr, err, logging.KeyExternalGameID, id, "game_id", igdbID, logging.Cat(logging.CategoryDB))
		return ""
	}
	if !needs {
		return ""
	}
	md, err := h.igdbClient.GetGameByID(ctx, igdbID)
	if err != nil {
		slog.WarnContext(ctx, "sync: rematch: look up IGDB title, using the storefront title",
			logging.KeyErr, err, "game_id", igdbID, logging.Cat(logging.CategoryExternalAPI))
		return ""
	}
	return md.Title
}

// rematchEditions re-resolves an External game and its editions to igdbID and
// writes them to the library (externalgame.Editions.Rematch). With jobID set it
// runs in the module's transaction for that Job and returns the items to
// complete there, with their diff rows written: every open item of the
// changed games in that Job. It refuses (409) while the given game's own item
// is pending: the sync is not done with it.
//
// With onlyThis it re-resolves the given game alone and completes only its
// own items.
func rematchEditions(ctx context.Context, tx bun.Tx, userID, id string, req externalgame.RematchRequest, jobID string) ([]string, error) {
	onlyThis := req.OnlyThisEdition
	set, err := lockEditions(ctx, tx, userID, id)
	if err != nil {
		return nil, err
	}

	var items []externalgame.OpenItem
	if jobID != "" {
		if items, err = externalgame.EditionsOpenItems(ctx, tx, jobID, set.Changed(onlyThis)); err != nil {
			return nil, err
		}
	}
	for _, it := range items {
		if it.ExternalGameID == id && it.Status == models.JobItemStatusPending {
			return nil, joblifecycle.Abort(echo.NewHTTPError(http.StatusConflict, errSyncStillWorking))
		}
	}

	writes, err := set.Rematch(ctx, req)
	if errors.Is(err, externalgame.ErrOrphanActionRequired) {
		return nil, joblifecycle.Abort(echo.NewHTTPError(http.StatusConflict, externalgame.ErrOrphanActionRequired.Error()))
	}
	if err != nil {
		return nil, err
	}

	// Diff rows for the items this completes, as the sync worker would have
	// written them: 'added' once per Job when any write created the user game.
	created := false
	for _, lw := range writes {
		if lw.Result.Created {
			created = true
		}
	}
	done := make([]string, 0, len(items))
	for _, it := range items {
		done = append(done, it.ID)
		lw, ok := writes[it.ExternalGameID]
		if !ok {
			continue // not written (no platform rows): no diff row
		}
		if err := tasks.InsertLibraryWriteChanges(ctx, tx, jobID, userID, lw, created); err != nil {
			return nil, err
		}
		created = false
	}
	return done, nil
}

// HandleUnskipGame handles DELETE /api/sync/ignored/:id. It clears the skip on
// an External game and its editions (with only_this_edition=true, on the game
// alone); matched ones are written to the library at once. Unmatched ones are
// not written, and show as unmatched until the user matches them or the next
// sync does (#1338). All of it is one transaction. It touches no items.
func (h *SyncHandler) HandleUnskipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	onlyThis := onlyThisEdition(c)
	ctx := context.Background()
	reqCtx := c.Request().Context()

	var wrote *int32
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		set, err := lockEditions(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		wrote, err = set.Unskip(ctx, onlyThis)
		return err
	})
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		slog.ErrorContext(reqCtx, "sync: unskip failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unskip game")
	}
	if wrote != nil {
		tasks.EnqueueImmediateMetadataFetch(ctx, h.db, h.riverClient, *wrote)
	}
	return c.NoContent(http.StatusNoContent)
}
