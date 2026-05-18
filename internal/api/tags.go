package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
)

// TagsHandler handles tag CRUD endpoints.
type TagsHandler struct {
	db *bun.DB
}

// NewTagsHandler returns a new TagsHandler.
func NewTagsHandler(db *bun.DB) *TagsHandler {
	return &TagsHandler{db: db}
}

// tagListItem is the response shape for the list endpoint (includes game_count).
type tagListItem struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	GameCount int64     `json:"game_count"`
}

// tagResponse is the response shape for create/update (no game_count).
type tagResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HandleListTags handles GET /api/tags.
// Returns all tags for the authenticated user with their game_count.
func (h *TagsHandler) HandleListTags(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var tags []tagListItem
	err := h.db.NewRaw(`
		SELECT t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at,
		       COUNT(ugt.id) AS game_count
		FROM tags t
		LEFT JOIN user_game_tags ugt ON ugt.tag_id = t.id
		WHERE t.user_id = ?
		GROUP BY t.id
		ORDER BY t.name`,
		userID,
	).Scan(context.Background(), &tags)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list tags")
	}

	// Return an empty slice (not null) if there are no tags.
	if tags == nil {
		tags = []tagListItem{}
	}
	return c.JSON(http.StatusOK, tags)
}

// createTagRequest is the body for POST /api/tags.
type createTagRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// HandleCreateTag handles POST /api/tags.
func (h *TagsHandler) HandleCreateTag(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req createTagRequest
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

	now := time.Now().UTC()
	id := uuid.NewString()

	var tag tagResponse
	err := h.db.NewRaw(`
		INSERT INTO tags (id, user_id, name, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, user_id, name, color, created_at, updated_at`,
		id, userID, req.Name, req.Color, now, now,
	).Scan(context.Background(), &tag)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "tag name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create tag")
	}

	return c.JSON(http.StatusCreated, tag)
}

// updateTagRequest is the body for PUT /api/tags/:id.
// Both fields are optional (partial update).
type updateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

// HandleUpdateTag handles PUT /api/tags/:id.
func (h *TagsHandler) HandleUpdateTag(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	tagID := c.Param("id")

	var req updateTagRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate name if provided.
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
		if *req.Name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		if len(*req.Name) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "name must be 100 characters or less")
		}
	}

	// Build dynamic SET clause.
	setClauses := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Color != nil {
		setClauses = append(setClauses, "color = ?")
		args = append(args, *req.Color)
	}

	// WHERE args.
	args = append(args, tagID, userID)

	query := fmt.Sprintf(
		`UPDATE tags SET %s
		 WHERE id = ? AND user_id = ?
		 RETURNING id, user_id, name, color, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)

	var tag tagResponse
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &tag)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "tag name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update tag")
	}

	return c.JSON(http.StatusOK, tag)
}

// HandleDeleteTag handles DELETE /api/tags/:id.
func (h *TagsHandler) HandleDeleteTag(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	tagID := c.Param("id")

	result, err := h.db.NewRaw(
		`DELETE FROM tags WHERE id = ? AND user_id = ?`,
		tagID, userID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete tag")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete tag")
	}
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	return c.NoContent(http.StatusNoContent)
}

// isDuplicateKeyError reports whether err is a PostgreSQL unique_violation (code 23505).
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// pgdriver wraps the error; the code is embedded in the message.
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "unique constraint") ||
		strings.Contains(err.Error(), "unique_violation")
}
