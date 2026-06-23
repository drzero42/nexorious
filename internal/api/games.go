package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/dbutil"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/platformresolution"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// GamesHandler handles /api/games endpoints.
type GamesHandler struct {
	db          *bun.DB
	igdb        *igdb.Client
	cfg         *config.Config
	riverClient *river.Client[pgx.Tx]
}

// NewGamesHandler creates a GamesHandler.
func NewGamesHandler(db *bun.DB, igdbClient *igdb.Client, cfg *config.Config, riverClient *river.Client[pgx.Tx]) *GamesHandler {
	return &GamesHandler{db: db, igdb: igdbClient, cfg: cfg, riverClient: riverClient}
}

// GameListResponse is the paginated response for game listings.
type GameListResponse struct {
	Games   []models.Game `json:"games"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Pages   int           `json:"pages"`
}

// IGDBGameCandidate is the response shape for IGDB search results.
type IGDBGameCandidate struct {
	IgdbID                     int      `json:"igdb_id"`
	IgdbSlug                   string   `json:"igdb_slug"`
	Title                      string   `json:"title"`
	ReleaseDate                *string  `json:"release_date"`
	CoverArtUrl                *string  `json:"cover_art_url"`
	Description                *string  `json:"description"`
	Platforms                  []string `json:"platforms"`
	PlatformIDs                []int    `json:"platform_ids"`
	HowlongtobeatMain          *float64 `json:"howlongtobeat_main"`
	HowlongtobeatExtra         *float64 `json:"howlongtobeat_extra"`
	HowlongtobeatCompletionist *float64 `json:"howlongtobeat_completionist"`
	// UserGameID is the id of the requesting user's existing library entry for
	// this game, or nil when the game is not yet in their library. It lets the
	// Add Game UI surface "already in library"/"already in wishlist" and link
	// to the detail page instead of re-adding (#856).
	UserGameID *string `json:"user_game_id"`
	// UserGameIsWishlisted is set alongside UserGameID (non-nil) and indicates
	// whether the existing entry is a wishlist-only entry (true) or a regular
	// library entry (false). Nil when UserGameID is nil.
	UserGameIsWishlisted *bool `json:"user_game_is_wishlisted"`
}

// IGDBSearchResponse wraps IGDB search results.
type IGDBSearchResponse struct {
	Games []IGDBGameCandidate `json:"games"`
	Total int                 `json:"total"`
}

// IGDBSearchRequest is the request body for POST /api/games/search/igdb.
type IGDBSearchRequest struct {
	Query          string  `json:"query"`
	Limit          int     `json:"limit"`
	ExternalGameID *string `json:"external_game_id,omitempty"`
}

// IGDBImportRequest is the request body for POST /api/games/igdb-import.
type IGDBImportRequest struct {
	IgdbID           int            `json:"igdb_id"`
	CustomOverrides  map[string]any `json:"custom_overrides"`
	DownloadCoverArt *bool          `json:"download_cover_art"`
}

// allowedSortFields is the whitelist of sortable columns.
var allowedSortFields = map[string]bool{
	"title":          true,
	"release_date":   true,
	"created_at":     true,
	"rating_average": true,
}

// HandleListGames handles GET /api/games.
func (h *GamesHandler) HandleListGames(c *echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page")) //nolint:errcheck // invalid/empty query param clamped to default below
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page")) //nolint:errcheck // invalid/empty query param clamped to default below
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	q := c.QueryParam("q")
	genre := c.QueryParam("genre")
	developer := c.QueryParam("developer")
	publisher := c.QueryParam("publisher")
	releaseYearStr := c.QueryParam("release_year")
	sortBy := c.QueryParam("sort_by")
	sortOrder := c.QueryParam("sort_order")

	if sortBy == "" {
		sortBy = "title"
	}
	if !allowedSortFields[sortBy] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid sort_by field: " + sortBy,
		})
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "sort_order must be 'asc' or 'desc'",
		})
	}

	ctx := c.Request().Context()
	query := h.db.NewSelect().Model((*models.Game)(nil))

	if q != "" {
		likeQ := dbutil.LikeContains(q)
		query = query.Where("title ILIKE ? OR description ILIKE ?", likeQ, likeQ)
	}
	if genre != "" {
		query = query.Where("genre ILIKE ?", dbutil.LikeContains(genre))
	}
	if developer != "" {
		query = query.Where("developer ILIKE ?", dbutil.LikeContains(developer))
	}
	if publisher != "" {
		query = query.Where("publisher ILIKE ?", dbutil.LikeContains(publisher))
	}
	if releaseYearStr != "" {
		if year, err := strconv.Atoi(releaseYearStr); err == nil {
			query = query.Where("EXTRACT(year FROM release_date) = ?", year)
		}
	}

	total, err := query.Count(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	orderExpr := sortBy + " " + strings.ToUpper(sortOrder)
	offset := (page - 1) * perPage

	var games []models.Game
	err = query.OrderExpr(orderExpr).Offset(offset).Limit(perPage).Scan(ctx, &games)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}
	if games == nil {
		games = []models.Game{}
	}

	pages := (total + perPage - 1) / perPage

	return c.JSON(http.StatusOK, GameListResponse{
		Games:   games,
		Total:   total,
		Page:    page,
		PerPage: perPage,
		Pages:   pages,
	})
}

// HandleGetGame handles GET /api/games/:id.
func (h *GamesHandler) HandleGetGame(c *echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid game ID"})
	}

	var game models.Game
	err = h.db.NewSelect().Model(&game).Where("id = ?", id).Scan(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Game not found"})
	}

	return c.JSON(http.StatusOK, game)
}

// HandleSearchIGDB handles POST /api/games/search/igdb.
func (h *GamesHandler) HandleSearchIGDB(c *echo.Context) error {
	var req IGDBSearchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query is required"})
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 50 {
		req.Limit = 50
	}

	ctx := c.Request().Context()
	userID := auth.UserIDFromContext(c)

	var platformIDs []int
	if req.ExternalGameID != nil && *req.ExternalGameID != "" {
		if userID == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		var exists bool
		if err := h.db.NewRaw(
			`SELECT EXISTS(SELECT 1 FROM external_games WHERE id = ? AND user_id = ?)`,
			*req.ExternalGameID, userID,
		).Scan(ctx, &exists); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "ownership check failed"})
		}
		if !exists {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "external_game not found or not owned by user"})
		}

		if ids, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, h.db, *req.ExternalGameID); perErr == nil {
			platformIDs = ids
		} else {
			slog.DebugContext(ctx, "HandleSearchIGDB: platform resolution failed, falling back to unfiltered",
				logging.KeyExternalGameID, *req.ExternalGameID, logging.KeyErr, perErr)
		}
	}

	if !h.igdbConfigured() {
		return h.mapIGDBError(c, igdb.ErrIGDBNotConfigured)
	}

	results, err := h.searchIGDBWithIDInference(ctx, req.Query, req.Limit, platformIDs)
	if err != nil {
		return h.mapIGDBError(c, err)
	}

	candidates := make([]IGDBGameCandidate, len(results))
	for i, md := range results {
		candidates[i] = metadataToCandidate(md)
	}
	if err := h.annotateLibraryMembership(ctx, userID, candidates); err != nil {
		// Annotation is best-effort enrichment; a failure here should not break
		// search. Log and return unannotated results.
		slog.ErrorContext(ctx, "HandleSearchIGDB: library membership annotation failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	return c.JSON(http.StatusOK, IGDBSearchResponse{
		Games: candidates,
		Total: len(candidates),
	})
}

// igdbIDQueryPattern matches the explicit "igdb:NNNN" ID-lookup form
// (case-insensitive); the digits are captured in group 1.
var igdbIDQueryPattern = regexp.MustCompile(`(?i)^igdb:(\d+)$`)

// bareIDQueryPattern matches a query that is only digits, e.g. "1020".
var bareIDQueryPattern = regexp.MustCompile(`^\d+$`)

// searchIGDBWithIDInference runs the IGDB search with query-level ID inference,
// mirroring the convenience the web search box used to implement client-side
// (issue #1153) so every front-end behaves identically:
//   - "igdb:NNNN" (case-insensitive) → pure ID lookup, no name search.
//   - bare "NNNN" → ID lookup AND name search, merged & deduped by igdb_id with
//     the ID match pinned first (keeps purely-numeric titles like "2048"
//     discoverable by name).
//   - anything else → ordinary name search.
//
// A not-found ID lookup is non-fatal: for the bare form it falls through to the
// name results; for the explicit form it yields an empty list (search
// semantics) rather than a 404.
func (h *GamesHandler) searchIGDBWithIDInference(ctx context.Context, query string, limit int, platformIDs []int) ([]igdb.GameMetadata, error) {
	if m := igdbIDQueryPattern.FindStringSubmatch(query); m != nil {
		id, err := strconv.Atoi(m[1])
		if err != nil {
			// Overflowing id can never match a real game.
			return nil, nil
		}
		md, err := h.igdb.GetGameByID(ctx, id)
		if err != nil {
			if errors.Is(err, igdb.ErrGameNotFound) {
				return nil, nil
			}
			return nil, err
		}
		return []igdb.GameMetadata{*md}, nil
	}

	if bareIDQueryPattern.MatchString(query) {
		if id, err := strconv.Atoi(query); err == nil {
			var idMatch *igdb.GameMetadata
			switch md, lookupErr := h.igdb.GetGameByID(ctx, id); {
			case lookupErr == nil:
				idMatch = md
			case errors.Is(lookupErr, igdb.ErrGameNotFound):
				// No game at that id — keep only the name results below.
			default:
				return nil, lookupErr
			}
			nameResults, err := h.igdb.SearchGames(ctx, query, limit, platformIDs)
			if err != nil {
				return nil, err
			}
			return mergeIGDBByID(idMatch, nameResults), nil
		}
		// Numeric but overflows int — fall through to a plain name search.
	}

	return h.igdb.SearchGames(ctx, query, limit, platformIDs)
}

// mergeIGDBByID prepends an ID-lookup match (if any) to name-search results,
// deduped by igdb_id so a game found both ways appears once, pinned first.
func mergeIGDBByID(idMatch *igdb.GameMetadata, nameResults []igdb.GameMetadata) []igdb.GameMetadata {
	if idMatch == nil {
		return nameResults
	}
	merged := make([]igdb.GameMetadata, 0, len(nameResults)+1)
	merged = append(merged, *idMatch)
	for _, g := range nameResults {
		if g.IgdbID != idMatch.IgdbID {
			merged = append(merged, g)
		}
	}
	return merged
}

// HandleGetIGDBGame handles GET /api/games/igdb/:igdb_id.
func (h *GamesHandler) HandleGetIGDBGame(c *echo.Context) error {
	igdbIDStr := c.Param("igdb_id")
	igdbID, err := strconv.Atoi(igdbIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid IGDB ID"})
	}
	if !h.igdbConfigured() {
		return h.mapIGDBError(c, igdb.ErrIGDBNotConfigured)
	}

	ctx := c.Request().Context()
	md, err := h.igdb.GetGameByID(ctx, igdbID)
	if err != nil {
		return h.mapIGDBError(c, err)
	}

	candidates := []IGDBGameCandidate{metadataToCandidate(*md)}
	if err := h.annotateLibraryMembership(ctx, auth.UserIDFromContext(c), candidates); err != nil {
		slog.ErrorContext(ctx, "HandleGetIGDBGame: library membership annotation failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	return c.JSON(http.StatusOK, IGDBSearchResponse{
		Games: candidates,
		Total: 1,
	})
}

// HandleImportFromIGDB handles POST /api/games/igdb-import.
func (h *GamesHandler) HandleImportFromIGDB(c *echo.Context) error {
	var req IGDBImportRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.IgdbID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "igdb_id is required"})
	}
	if !h.igdbConfigured() {
		return h.mapIGDBError(c, igdb.ErrIGDBNotConfigured)
	}

	ctx := c.Request().Context()

	md, err := h.igdb.FetchFullMetadata(ctx, req.IgdbID)
	if err != nil {
		return h.mapIGDBError(c, err)
	}

	// Check if game already exists by IGDB ID (which is the primary key)
	var existing models.Game
	existsErr := h.db.NewSelect().Model(&existing).Where("id = ?", req.IgdbID).Scan(ctx)
	isNew := existsErr != nil

	game := h.metadataToGame(md)

	// Apply custom overrides
	if req.CustomOverrides != nil {
		if title, ok := req.CustomOverrides["title"].(string); ok && title != "" {
			game.Title = title
		}
		if desc, ok := req.CustomOverrides["description"].(string); ok {
			game.Description = &desc
		}
		if genre, ok := req.CustomOverrides["genre"].(string); ok {
			game.Genre = &genre
		}
		if dev, ok := req.CustomOverrides["developer"].(string); ok {
			game.Developer = &dev
		}
		if pub, ok := req.CustomOverrides["publisher"].(string); ok {
			game.Publisher = &pub
		}
	}

	// Download cover art
	downloadCover := req.DownloadCoverArt == nil || *req.DownloadCoverArt
	if downloadCover && h.igdb != nil && md.CoverArtURL != nil {
		imageID := extractImageID(*md.CoverArtURL)
		if imageID != "" {
			localURL, dlErr := h.igdb.DownloadCoverArt(ctx, imageID, h.cfg.StoragePath)
			if dlErr == nil && localURL != "" {
				game.CoverArtUrl = &localURL
			}
		}
	}

	if isNew {
		_, err = h.db.NewInsert().Model(game).Exec(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create game"})
		}
		return c.JSON(http.StatusCreated, game)
	}

	game.CreatedAt = existing.CreatedAt
	_, err = h.db.NewUpdate().Model(game).WherePK().Exec(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update game"})
	}
	return c.JSON(http.StatusOK, game)
}

// startMaintenanceRefresh runs the "already active?" guard for a maintenance
// refresh and, when none is active, synchronously inserts a minimal pending jobs
// row owned by userID so the caller can return a real job_id immediately. It
// returns the job id to report to the client and created=true only when this
// call inserted the row; created=false means an equivalent job (same job_type +
// source) was already active — its id is returned and the caller must NOT enqueue
// a dispatch. Everything runs in one transaction, fronted by a transaction-scoped
// advisory lock on the (job_type, source) dedup key, so the guard and insert are
// atomic with respect to any concurrent start — without the lock the bare
// SELECT+INSERT races under READ COMMITTED (two POSTs both pass the guard and
// both insert). The handler always uses source=system, so the empty user_id
// discriminant matches the guard, which is not user-scoped.
func (h *GamesHandler) startMaintenanceRefresh(ctx context.Context, userID, jobType, source string) (jobID string, created bool, err error) {
	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if e := tasks.AcquireJobDedupLock(ctx, tx, jobType, source, ""); e != nil {
			return e
		}
		var existing string
		e := tx.NewRaw(
			`SELECT id FROM jobs WHERE job_type = ? AND source = ? AND status IN ('pending','processing') LIMIT 1`,
			jobType, source,
		).Scan(ctx, &existing)
		if e == nil {
			jobID, created = existing, false
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		jobID, created = uuid.NewString(), true
		_, e = tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'pending', 'low', 0, now())`,
			jobID, userID, jobType, source,
		).Exec(ctx)
		return e
	})
	return jobID, created, err
}

// HandleStartMetadataRefreshJob handles POST /api/games/metadata/refresh-job.
// Admin-only: creates a pending jobs row then enqueues a MetadataRefreshDispatch River job.
func (h *GamesHandler) HandleStartMetadataRefreshJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	if !auth.IsAdminFromContext(c) {
		return echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}

	if h.riverClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "worker not available")
	}

	ctx := c.Request().Context()
	jobID, created, err := h.startMaintenanceRefresh(ctx, userID, models.JobTypeMetadataRefresh, models.JobSourceSystem)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start metadata refresh", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue metadata refresh")
	}

	if created {
		if _, err := h.riverClient.Insert(ctx, tasks.MetadataRefreshDispatchArgs{JobID: jobID}, nil); err != nil {
			slog.ErrorContext(ctx, "failed to enqueue metadata refresh dispatch", logging.KeyErr, err, logging.KeyJobID, jobID)
			if _, derr := h.db.NewRaw(`DELETE FROM jobs WHERE id = ?`, jobID).Exec(ctx); derr != nil {
				slog.ErrorContext(ctx, "failed to roll back metadata refresh job row", logging.KeyErr, derr, logging.KeyJobID, jobID, logging.Cat(logging.CategoryDB))
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue metadata refresh")
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Metadata refresh job queued",
		"job_id":  jobID,
	})
}

// HandleStartStoreLinkRefreshJob handles POST /api/games/store-links/refresh-job.
// Admin-only: creates a pending jobs row then enqueues a global, forced store-link re-resolution.
func (h *GamesHandler) HandleStartStoreLinkRefreshJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !auth.IsAdminFromContext(c) {
		return echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}
	if h.riverClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "worker not available")
	}

	ctx := c.Request().Context()
	jobID, created, err := h.startMaintenanceRefresh(ctx, userID, models.JobTypeStoreLinkRefresh, models.JobSourceSystem)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start store-link refresh", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue store link refresh")
	}

	if created {
		if _, err := h.riverClient.Insert(ctx, tasks.StoreLinkRefreshDispatchArgs{Force: true, JobID: jobID}, nil); err != nil {
			slog.ErrorContext(ctx, "failed to enqueue store-link refresh dispatch", logging.KeyErr, err, logging.KeyJobID, jobID)
			if _, derr := h.db.NewRaw(`DELETE FROM jobs WHERE id = ?`, jobID).Exec(ctx); derr != nil {
				slog.ErrorContext(ctx, "failed to roll back store-link refresh job row", logging.KeyErr, derr, logging.KeyJobID, jobID, logging.Cat(logging.CategoryDB))
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue store link refresh")
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Store link refresh job queued",
		"job_id":  jobID,
	})
}

// igdbConfigured reports whether the handler has a usable IGDB client. A nil
// client pointer (no IGDB client wired at all) is treated as "not configured",
// the same as a non-nil client built without credentials — without this guard,
// calling a method on the nil pointer dereferences it and panics (issue #1051).
func (h *GamesHandler) igdbConfigured() bool {
	return h.igdb != nil && h.igdb.Configured()
}

func (h *GamesHandler) mapIGDBError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, igdb.ErrIGDBNotConfigured):
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case errors.Is(err, igdb.ErrGameNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Game not found in IGDB"})
	case errors.Is(err, igdb.ErrTwitchAuth):
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "IGDB authentication failed"})
	default:
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "IGDB API error: " + err.Error()})
	}
}

// annotateLibraryMembership stamps UserGameID and UserGameIsWishlisted onto
// every candidate that already exists in userID's library, so the Add Game UI
// can surface "already in library"/"already in wishlist" and link to the detail
// page instead of re-adding (#856). Candidates not in the library are left with
// nil fields. A nil/empty userID or empty candidate slice is a no-op.
func (h *GamesHandler) annotateLibraryMembership(ctx context.Context, userID string, candidates []IGDBGameCandidate) error {
	if userID == "" || len(candidates) == 0 {
		return nil
	}

	igdbIDs := make([]int, len(candidates))
	for i, c := range candidates {
		igdbIDs[i] = c.IgdbID
	}

	var rows []struct {
		ID           string `bun:"id"`
		GameID       int32  `bun:"game_id"`
		IsWishlisted bool   `bun:"is_wishlisted"`
	}
	if err := h.db.NewSelect().
		Table("user_games").
		Column("id", "game_id", "is_wishlisted").
		Where("user_id = ?", userID).
		Where("game_id IN (?)", bun.List(igdbIDs)).
		Scan(ctx, &rows); err != nil {
		return err
	}

	type ownedEntry struct {
		id           string
		isWishlisted bool
	}
	owned := make(map[int32]ownedEntry, len(rows))
	for _, r := range rows {
		owned[r.GameID] = ownedEntry{id: r.ID, isWishlisted: r.IsWishlisted}
	}
	for i := range candidates {
		if entry, ok := owned[int32(candidates[i].IgdbID)]; ok { //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
			ugID := entry.id
			candidates[i].UserGameID = &ugID
			wishlisted := entry.isWishlisted
			candidates[i].UserGameIsWishlisted = &wishlisted
		}
	}
	return nil
}

func metadataToCandidate(md igdb.GameMetadata) IGDBGameCandidate {
	platforms := md.PlatformNames
	if platforms == nil {
		platforms = []string{}
	}
	platformIDs := md.PlatformIDs
	if platformIDs == nil {
		platformIDs = []int{}
	}
	return IGDBGameCandidate{
		IgdbID:                     md.IgdbID,
		IgdbSlug:                   md.IgdbSlug,
		Title:                      md.Title,
		ReleaseDate:                md.ReleaseDate,
		CoverArtUrl:                md.CoverArtURL,
		Description:                md.Description,
		Platforms:                  platforms,
		PlatformIDs:                platformIDs,
		HowlongtobeatMain:          md.HowlongtobeatMain,
		HowlongtobeatExtra:         md.HowlongtobeatExtra,
		HowlongtobeatCompletionist: md.HowlongtobeatCompletionist,
	}
}

func (h *GamesHandler) metadataToGame(md *igdb.GameMetadata) *models.Game {
	now := time.Now()
	game := &models.Game{
		ID:                         int32(md.IgdbID), //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		Title:                      md.Title,
		Description:                md.Description,
		Genre:                      md.Genre,
		Developer:                  md.Developer,
		Publisher:                  md.Publisher,
		CoverArtUrl:                md.CoverArtURL,
		RatingAverage:              md.RatingAverage,
		RatingCount:                md.RatingCount,
		HowlongtobeatMain:          md.HowlongtobeatMain,
		HowlongtobeatExtra:         md.HowlongtobeatExtra,
		HowlongtobeatCompletionist: md.HowlongtobeatCompletionist,
		IgdbSlug:                   &md.IgdbSlug,
		GameModes:                  md.GameModes,
		Themes:                     md.Themes,
		PlayerPerspectives:         md.PlayerPerspectives,
		LastUpdated:                now,
		CreatedAt:                  now,
	}

	if md.ReleaseDate != nil {
		t, err := time.Parse("2006-01-02", *md.ReleaseDate)
		if err == nil {
			game.ReleaseDate = &t
		}
	}

	if len(md.PlatformIDs) > 0 {
		ids := make([]string, len(md.PlatformIDs))
		for i, id := range md.PlatformIDs {
			ids[i] = strconv.Itoa(id)
		}
		s := strings.Join(ids, ",")
		game.IgdbPlatformIds = &s
	}
	if len(md.PlatformNames) > 0 {
		s := strings.Join(md.PlatformNames, ",")
		game.IgdbPlatformNames = &s
	}

	return game
}

func extractImageID(coverURL string) string {
	parts := strings.Split(coverURL, "/")
	last := parts[len(parts)-1]
	return strings.TrimSuffix(last, ".jpg")
}
