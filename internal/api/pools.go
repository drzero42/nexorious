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

// Stubs replaced in Task 7.
func (h *PoolsHandler) HandleGetPool(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
func (h *PoolsHandler) HandleAddPoolGame(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
func (h *PoolsHandler) HandleRemovePoolGame(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
func (h *PoolsHandler) HandleSetQueue(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
