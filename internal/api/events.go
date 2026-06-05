package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/dbutil"
	"github.com/drzero42/nexorious/internal/notify"
)

// EncodeEventCursor packs (occurred_at, id) into an opaque base64 token used as
// the `before` keyset cursor for GET /api/admin/events.
func EncodeEventCursor(occurredAt time.Time, id string) string {
	raw := occurredAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeEventCursor reverses EncodeEventCursor. Returns an error on any
// malformed token so the handler can reply 400.
func DecodeEventCursor(token string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor time: %w", err)
	}
	return ts, parts[1], nil
}

const (
	eventsDefaultLimit = 50
	eventsMaxLimit     = 200
)

// EventsHandler serves the admin activity feed over the events table.
type EventsHandler struct {
	db *bun.DB
}

func NewEventsHandler(db *bun.DB) *EventsHandler {
	return &EventsHandler{db: db}
}

// eventRow is the scan target for the joined query. Explicit bun tags are
// required because the username comes from an aliased join column.
type eventRow struct {
	ID            string          `bun:"id"`
	Type          string          `bun:"type"`
	Scope         string          `bun:"scope"`
	ActorUserID   *string         `bun:"actor_user_id"`
	ActorUsername *string         `bun:"actor_username"`
	Payload       json.RawMessage `bun:"payload"`
	OccurredAt    time.Time       `bun:"occurred_at"`
}

// eventResponse is the serialized shape returned to the client.
type eventResponse struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Category      string          `json:"category"`
	Scope         string          `json:"scope"`
	OccurredAt    time.Time       `json:"occurred_at"`
	ActorUserID   *string         `json:"actor_user_id"`
	ActorUsername *string         `json:"actor_username"`
	Title         string          `json:"title"`
	Body          string          `json:"body"`
	Payload       json.RawMessage `json:"payload"`
}

// eventTypesForCategory returns the event types belonging to a registry
// category. Returns a single empty string when unknown so the IN clause
// matches nothing rather than everything.
func eventTypesForCategory(category string) []string {
	var types []string
	for _, m := range notify.Registry() {
		if m.Category == category {
			types = append(types, m.Type)
		}
	}
	if len(types) == 0 {
		return []string{""}
	}
	return types
}

// HandleList serves GET /api/admin/events.
func (h *EventsHandler) HandleList(c *echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit")) //nolint:errcheck // invalid/empty query param clamped below
	if limit < 1 {
		limit = eventsDefaultLimit
	}
	if limit > eventsMaxLimit {
		limit = eventsMaxLimit
	}

	q := h.db.NewSelect().
		ColumnExpr("e.id, e.type, e.scope, e.actor_user_id, e.payload, e.occurred_at").
		ColumnExpr("u.username AS actor_username").
		TableExpr("events AS e").
		Join("LEFT JOIN users AS u ON u.id = e.actor_user_id").
		OrderExpr("e.occurred_at DESC, e.id DESC").
		Limit(limit + 1)

	if t := c.QueryParam("type"); t != "" {
		q = q.Where("e.type = ?", t)
	}
	if cat := c.QueryParam("category"); cat != "" {
		q = q.Where("e.type IN (?)", bun.List(eventTypesForCategory(cat)))
	}
	if scope := c.QueryParam("scope"); scope != "" {
		q = q.Where("e.scope = ?", scope)
	}
	if user := c.QueryParam("user"); user != "" {
		q = q.Where("(e.actor_user_id = ? OR u.username ILIKE ?)", user, dbutil.LikeContains(user))
	}
	if since := c.QueryParam("since"); since != "" {
		ts, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid since")
		}
		q = q.Where("e.occurred_at >= ?", ts)
	}
	if until := c.QueryParam("until"); until != "" {
		ts, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid until")
		}
		// Exclusive upper bound: callers pass the start of the day *after* the
		// range they want, so the whole final day is included with no
		// sub-second truncation. See ui .../admin/activity dayRangeToUTC.
		q = q.Where("e.occurred_at < ?", ts)
	}

	if before := c.QueryParam("before"); before != "" {
		ts, id, err := DecodeEventCursor(before)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid cursor")
		}
		q = q.Where("(e.occurred_at, e.id) < (?, ?)", ts, id)
	}

	var rows []eventRow
	if err := q.Scan(context.Background(), &rows); err != nil {
		slog.Error("events: list query failed", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list events")
	}

	var nextCursor *string
	if len(rows) > limit {
		last := rows[limit-1]
		cur := EncodeEventCursor(last.OccurredAt, last.ID)
		nextCursor = &cur
		rows = rows[:limit]
	}

	out := make([]eventResponse, 0, len(rows))
	for i := range rows {
		r := rows[i]
		title, body, derr := notify.Format(r.Type, r.Payload)
		if derr != nil {
			slog.Warn("notify: payload decode failed", "event_id", r.ID, "type", r.Type, "err", derr)
		}
		category := ""
		if m, ok := notify.Meta(r.Type); ok {
			category = m.Category
		}
		out = append(out, eventResponse{
			ID:            r.ID,
			Type:          r.Type,
			Category:      category,
			Scope:         r.Scope,
			OccurredAt:    r.OccurredAt,
			ActorUserID:   r.ActorUserID,
			ActorUsername: r.ActorUsername,
			Title:         title,
			Body:          body,
			Payload:       r.Payload,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"events":      out,
		"next_cursor": nextCursor,
	})
}
