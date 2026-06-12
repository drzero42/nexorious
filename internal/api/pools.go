package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/filter"
)

// PoolsHandler handles /api/pools endpoints (Play Planning, #955).
type PoolsHandler struct {
	db *bun.DB
}

// NewPoolsHandler returns a new PoolsHandler.
func NewPoolsHandler(db *bun.DB) *PoolsHandler {
	return &PoolsHandler{db: db}
}

// poolListItem is the response shape for GET /api/pools.
type poolListItem struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Color          *string `json:"color"`
	Position       int     `json:"position"`
	HasFilter      bool    `json:"has_filter"`
	QueueCount     int64   `json:"queue_count"`
	CandidateCount int64   `json:"candidate_count"`
}

// poolResponse is the response shape for create/update.
type poolResponse struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Name      string          `json:"name"`
	Color     *string         `json:"color"`
	Position  int             `json:"position"`
	Filter    json.RawMessage `json:"filter"`
	HasFilter bool            `json:"has_filter"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// HandleListPools handles GET /api/pools.
func (h *PoolsHandler) HandleListPools(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var pools []poolListItem
	err := h.db.NewRaw(`
		SELECT p.id, p.name, p.color, p.position,
		       (p.filter IS NOT NULL) AS has_filter,
		       COUNT(pg.id) FILTER (WHERE pg.position IS NOT NULL) AS queue_count,
		       COUNT(pg.id) FILTER (WHERE pg.position IS NULL)     AS candidate_count
		FROM pools p
		LEFT JOIN pool_games pg ON pg.pool_id = p.id
		WHERE p.user_id = ?
		GROUP BY p.id
		ORDER BY p.position`,
		userID,
	).Scan(context.Background(), &pools)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list pools")
	}
	if pools == nil {
		pools = []poolListItem{}
	}
	return c.JSON(http.StatusOK, pools)
}

// createPoolRequest is the body for POST /api/pools.
type createPoolRequest struct {
	Name   string          `json:"name"`
	Color  *string         `json:"color"`
	Filter json.RawMessage `json:"filter"`
}

// HandleCreatePool handles POST /api/pools.
func (h *PoolsHandler) HandleCreatePool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req createPoolRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if len(req.Name) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be 100 characters or less")
	}

	normFilter, err := normalizePoolFilter(req.Filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	now := time.Now().UTC()
	id := uuid.NewString()
	ctx := context.Background()

	var pool poolResponse
	err = h.db.NewRaw(`
		INSERT INTO pools (id, user_id, name, color, position, filter, created_at, updated_at)
		VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM pools WHERE user_id = ?), 0), ?, ?, ?)
		RETURNING id, user_id, name, color, position, filter, created_at, updated_at`,
		id, userID, req.Name, req.Color, userID, normFilter, now, now,
	).Scan(ctx, &pool)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "pool name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create pool")
	}
	pool.HasFilter = pool.Filter != nil
	return c.JSON(http.StatusCreated, pool)
}

// updatePoolRequest is documented inline; PUT decodes into a raw map to detect
// which fields were present (absent → unchanged).

// HandleUpdatePool handles PUT /api/pools/:id.
func (h *PoolsHandler) HandleUpdatePool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	setClauses := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}

	if nameRaw, ok := raw["name"]; ok {
		var name string
		if err := json.Unmarshal(nameRaw, &name); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid name")
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		if len(name) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "name must be 100 characters or less")
		}
		setClauses = append(setClauses, "name = ?")
		args = append(args, name)
	}
	if colorRaw, ok := raw["color"]; ok {
		var color *string
		if err := json.Unmarshal(colorRaw, &color); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid color")
		}
		setClauses = append(setClauses, "color = ?")
		args = append(args, color)
	}
	if filterRaw, ok := raw["filter"]; ok {
		normFilter, err := normalizePoolFilter(filterRaw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		setClauses = append(setClauses, "filter = ?")
		args = append(args, normFilter)
	}

	if len(setClauses) == 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "no fields to update")
	}

	args = append(args, poolID, userID)
	query := fmt.Sprintf(`
		UPDATE pools SET %s
		WHERE id = ? AND user_id = ?
		RETURNING id, user_id, name, color, position, filter, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)

	var pool poolResponse
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &pool)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "pool name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update pool")
	}
	pool.HasFilter = pool.Filter != nil
	return c.JSON(http.StatusOK, pool)
}

// HandleDeletePool handles DELETE /api/pools/:id.
func (h *PoolsHandler) HandleDeletePool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	res, err := h.db.NewRaw(`DELETE FROM pools WHERE id = ? AND user_id = ?`, poolID, userID).
		Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete pool")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete pool")
	}
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// reorderRequest is the body for POST /api/pools/reorder and PUT queue.
type reorderRequest struct {
	IDs []string `json:"ids"`
}

// HandleReorderPools handles POST /api/pools/reorder — renumber pools.position
// contiguous in the given order, in a txn. Only the caller's own pools move.
func (h *PoolsHandler) HandleReorderPools(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	ctx := context.Background()
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for i, id := range req.IDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE pools SET position = ?, updated_at = now() WHERE id = ? AND user_id = ?`,
				i, id, userID,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reorder pools")
	}
	return c.NoContent(http.StatusNoContent)
}

// normalizePoolFilter validates and canonicalises a raw pool filter.
//   - absent / JSON null / empty filters array → returns nil (pure manual pool)
//   - unknown keys → error
//   - any card with no facets → error
//
// It returns the canonical JSON to store (or nil for NULL).
func normalizePoolFilter(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	pf, err := filter.ParsePoolFilter(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %v", err)
	}
	if len(pf.Filters) == 0 {
		return nil, nil
	}
	for _, card := range pf.Filters {
		if !card.HasFacets() {
			return nil, errors.New("filter card has no facets")
		}
	}
	canonical, err := json.Marshal(pf)
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %v", err)
	}
	return canonical, nil
}

// poolDetailResponse is the response for GET /api/pools/:id.
type poolDetailResponse struct {
	poolResponse
	Queue      []userGameWithPlatformsResponse `json:"queue"`
	Candidates []userGameWithPlatformsResponse `json:"candidates"`
}

// poolMember pairs a user_game id with its queue position (NULL = candidate).
type poolMember struct {
	UserGameID string `bun:"user_game_id"`
	Position   *int   `bun:"position"`
}

// HandleGetPool handles GET /api/pools/:id — pool meta + members inline.
func (h *PoolsHandler) HandleGetPool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")
	ctx := context.Background()

	var pool poolResponse
	err := h.db.NewRaw(`
		SELECT id, user_id, name, color, position, filter, created_at, updated_at
		FROM pools WHERE id = ? AND user_id = ?`,
		poolID, userID,
	).Scan(ctx, &pool)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	pool.HasFilter = pool.Filter != nil

	// Membership rows ordered: queued first by (position, created_at), then
	// candidates by created_at.
	var members []poolMember
	err = h.db.NewRaw(`
		SELECT user_game_id, position
		FROM pool_games
		WHERE pool_id = ?
		ORDER BY (position IS NULL), position, created_at`,
		poolID,
	).Scan(ctx, &members)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	detail := poolDetailResponse{
		poolResponse: pool,
		Queue:        []userGameWithPlatformsResponse{},
		Candidates:   []userGameWithPlatformsResponse{},
	}
	if len(members) == 0 {
		return c.JSON(http.StatusOK, detail)
	}

	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserGameID
	}
	cards, err := h.loadUserGameCards(ctx, ids)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, m := range members {
		card, ok := cards[m.UserGameID]
		if !ok {
			continue
		}
		if m.Position != nil {
			detail.Queue = append(detail.Queue, card)
		} else {
			detail.Candidates = append(detail.Candidates, card)
		}
	}
	return c.JSON(http.StatusOK, detail)
}

// loadUserGameCards fetches user_games with relations for a set of ids and
// returns them keyed by id, reusing the list-item DTO shape.
func (h *PoolsHandler) loadUserGameCards(ctx context.Context, ids []string) (map[string]userGameWithPlatformsResponse, error) {
	var userGames []models.UserGame
	if err := h.db.NewSelect().
		Model(&userGames).
		Where("user_game.id IN (?)", bun.List(ids)).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make(map[string]userGameWithPlatformsResponse, len(userGames))
	for _, ug := range userGames {
		out[ug.ID] = toUserGameWithPlatformsResponse(ug)
	}
	return out, nil
}

// addPoolGameRequest is the body for POST /api/pools/:id/games.
type addPoolGameRequest struct {
	UserGameID string `json:"user_game_id"`
}

// HandleAddPoolGame handles POST /api/pools/:id/games — insert as a Candidate.
// Idempotent: re-adding an existing member is a 200 no-op. Pools never create
// user_games; the user_game must already exist and belong to the user.
func (h *PoolsHandler) HandleAddPoolGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	var req addPoolGameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.UserGameID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_id is required")
	}
	ctx := context.Background()

	// Pool must exist and belong to the user.
	poolOK, err := h.db.NewSelect().Table("pools").
		Where("id = ? AND user_id = ?", poolID, userID).Exists(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if !poolOK {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	// user_game must exist and belong to the user (owned OR wishlisted).
	ugOK, err := h.db.NewSelect().Table("user_games").
		Where("id = ? AND user_id = ?", req.UserGameID, userID).Exists(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if !ugOK {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game not found")
	}

	// Insert as Candidate; idempotent on (pool_id, user_game_id).
	_, err = h.db.NewRaw(`
		INSERT INTO pool_games (id, pool_id, user_game_id, position, created_at)
		VALUES (?, ?, ?, NULL, now())
		ON CONFLICT (pool_id, user_game_id) DO NOTHING`,
		uuid.NewString(), poolID, req.UserGameID,
	).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleRemovePoolGame handles DELETE /api/pools/:id/games/:userGameId.
func (h *PoolsHandler) HandleRemovePoolGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")
	userGameID := c.Param("userGameId")

	res, err := h.db.NewRaw(`
		DELETE FROM pool_games
		WHERE pool_id = ? AND user_game_id = ?
		  AND pool_id IN (SELECT id FROM pools WHERE user_id = ?)`,
		poolID, userGameID, userID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleSetQueue handles PUT /api/pools/:id/queue — declarative queue state.
// Body {ids:[…ordered]} is the desired queued set: every id must already be a
// member (else 400); each listed id gets position = index; any member not in
// the list is demoted to Candidate (position NULL). Atomic.
func (h *PoolsHandler) HandleSetQueue(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	ctx := context.Background()

	// Pool must exist and belong to the user.
	poolOK, err := h.db.NewSelect().Table("pools").
		Where("id = ? AND user_id = ?", poolID, userID).Exists(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if !poolOK {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Guard: every listed id must already be a member of this pool.
		if len(req.IDs) > 0 {
			var memberCount int
			if err := tx.NewSelect().Table("pool_games").
				Where("pool_id = ?", poolID).
				Where("user_game_id IN (?)", bun.List(req.IDs)).
				ColumnExpr("COUNT(*)").Scan(ctx, &memberCount); err != nil {
				return err
			}
			if memberCount != len(uniqueStrings(req.IDs)) {
				return errNotAllMembers
			}
		}
		// Demote everything to Candidate first.
		if _, err := tx.ExecContext(ctx,
			`UPDATE pool_games SET position = NULL WHERE pool_id = ?`, poolID,
		); err != nil {
			return err
		}
		// Promote listed ids to contiguous positions in order.
		for i, id := range req.IDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE pool_games SET position = ? WHERE pool_id = ? AND user_game_id = ?`,
				i, poolID, id,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errNotAllMembers) {
			return echo.NewHTTPError(http.StatusBadRequest, "all ids must already be pool members")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

var errNotAllMembers = errors.New("not all ids are members")

// uniqueStrings returns the distinct values of s, preserving first-seen order.
func uniqueStrings(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
