package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db"
	"github.com/drzero42/nexorious/internal/notify"
)

// NotificationsHandler serves notification channel + subscription endpoints.
type NotificationsHandler struct {
	db        *bun.DB
	encrypter *crypto.Encrypter
	sender    notify.Sender
}

// NewNotificationsHandler constructs a NotificationsHandler.
func NewNotificationsHandler(db *bun.DB, encrypter *crypto.Encrypter, sender notify.Sender) *NotificationsHandler {
	return &NotificationsHandler{db: db, encrypter: encrypter, sender: sender}
}

// channelResponse is the public shape of a notification channel. It deliberately
// never includes the (encrypted) Shoutrrr URL — the secret must not leak.
type channelResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type createChannelRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type updateChannelRequest struct {
	Name *string `json:"name"`
	URL  *string `json:"url"`
}

// HandleListChannels handles GET /api/notifications/channels.
func (h *NotificationsHandler) HandleListChannels(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var rows []channelResponse
	if err := h.db.NewRaw(
		`SELECT id, name, created_at FROM notification_channels WHERE user_id = ? ORDER BY created_at`, userID,
	).Scan(context.Background(), &rows); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list channels")
	}
	if rows == nil {
		rows = []channelResponse{}
	}
	return c.JSON(http.StatusOK, rows)
}

// HandleCreateChannel handles POST /api/notifications/channels.
func (h *NotificationsHandler) HandleCreateChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req createChannelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "url is required")
	}
	ciphertext, err := h.encrypter.Encrypt([]byte(req.URL))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt url")
	}
	id := uuid.NewString()
	var out channelResponse
	err = h.db.NewRaw(
		`INSERT INTO notification_channels (id, user_id, name, encrypted_url, created_at)
		 VALUES (?, ?, ?, ?, now())
		 RETURNING id, name, created_at`,
		id, userID, req.Name, ciphertext,
	).Scan(context.Background(), &out)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return echo.NewHTTPError(http.StatusConflict, "a channel with that name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create channel")
	}
	return c.JSON(http.StatusCreated, out)
}

// HandleUpdateChannel handles PATCH /api/notifications/channels/:id.
func (h *NotificationsHandler) HandleUpdateChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	var req updateChannelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	setClauses := []string{}
	args := []any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name cannot be empty")
		}
		setClauses = append(setClauses, "name = ?")
		args = append(args, name)
	}
	if req.URL != nil {
		url := strings.TrimSpace(*req.URL)
		if url == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "url cannot be empty")
		}
		ct, err := h.encrypter.Encrypt([]byte(url))
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt url")
		}
		setClauses = append(setClauses, "encrypted_url = ?")
		args = append(args, ct)
	}
	if len(setClauses) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no fields to update")
	}
	args = append(args, id, userID)
	query := `UPDATE notification_channels SET ` + strings.Join(setClauses, ", ") +
		` WHERE id = ? AND user_id = ? RETURNING id, name, created_at`
	var out channelResponse
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &out)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		if db.IsUniqueViolation(err) {
			return echo.NewHTTPError(http.StatusConflict, "a channel with that name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update channel")
	}
	return c.JSON(http.StatusOK, out)
}

// HandleDeleteChannel handles DELETE /api/notifications/channels/:id.
func (h *NotificationsHandler) HandleDeleteChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	res, err := h.db.NewRaw(
		`DELETE FROM notification_channels WHERE id = ? AND user_id = ?`, id, userID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete channel")
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleTestChannel handles POST /api/notifications/channels/:id/test.
// It performs a synchronous test send via the channel's decrypted URL.
func (h *NotificationsHandler) HandleTestChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	var encURL string
	err := h.db.NewRaw(
		`SELECT encrypted_url FROM notification_channels WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(context.Background(), &encURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel")
	}
	plain, err := h.encrypter.Decrypt(encURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to decrypt channel url")
	}
	if err := h.sender.Send(context.Background(), string(plain),
		"Nexorious test notification",
		"This is a test notification from Nexorious."); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "test send failed: "+err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

type testURLRequest struct {
	URL string `json:"url"`
}

// HandleTestURL handles POST /api/notifications/test.
// It sends a test notification to a URL provided in the request body (no save,
// no encryption needed — it's the user's own URL being tried out).
func (h *NotificationsHandler) HandleTestURL(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req testURLRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "url is required")
	}
	if err := h.sender.Send(context.Background(), req.URL, "Nexorious test notification", "This is a test notification from Nexorious."); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "test send failed: "+err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleListEventTypes handles GET /api/notifications/event-types.
// Admin-scoped event types are only returned to admins.
func (h *NotificationsHandler) HandleListEventTypes(c *echo.Context) error {
	if auth.UserIDFromContext(c) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	isAdmin := auth.IsAdminFromContext(c)
	out := []notify.EventTypeMeta{}
	for _, m := range notify.Registry() {
		if m.Scope == notify.ScopeAdmin && !isAdmin {
			continue
		}
		out = append(out, m)
	}
	return c.JSON(http.StatusOK, out)
}

// HandleListSubscriptions handles GET /api/notifications/subscriptions.
func (h *NotificationsHandler) HandleListSubscriptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var types []string
	if err := h.db.NewRaw(
		`SELECT event_type FROM notification_subscriptions WHERE user_id = ? ORDER BY event_type`, userID,
	).Scan(context.Background(), &types); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list subscriptions")
	}
	if types == nil {
		types = []string{}
	}
	return c.JSON(http.StatusOK, map[string][]string{"event_types": types})
}

type putSubscriptionsRequest struct {
	EventTypes []string `json:"event_types"`
}

// HandlePutSubscriptions handles PUT /api/notifications/subscriptions.
// It fully replaces the user's subscription set. Admin-scoped event types are
// rejected for non-admin users.
func (h *NotificationsHandler) HandlePutSubscriptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	isAdmin := auth.IsAdminFromContext(c)
	var req putSubscriptionsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, t := range req.EventTypes {
		if !notify.IsKnownType(t) {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown event type: "+t)
		}
		if notify.IsAdminType(t) && !isAdmin {
			return echo.NewHTTPError(http.StatusBadRequest, "not permitted to subscribe to "+t)
		}
		if !seen[t] {
			seen[t] = true
			clean = append(clean, t)
		}
	}
	ctx := context.Background()
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM notification_subscriptions WHERE user_id = ?`, userID).Exec(ctx); err != nil {
			return err
		}
		for _, t := range clean {
			if _, err := tx.NewRaw(
				`INSERT INTO notification_subscriptions (user_id, event_type, created_at) VALUES (?, ?, now())`,
				userID, t,
			).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update subscriptions")
	}
	return c.JSON(http.StatusOK, map[string][]string{"event_types": clean})
}

// HandleResetSubscriptions handles POST /api/notifications/subscriptions/reset.
// It destructively resets the user's subscriptions to the registry defaults.
func (h *NotificationsHandler) HandleResetSubscriptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	isAdmin := auth.IsAdminFromContext(c)
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM notification_subscriptions WHERE user_id = ?`, userID).Exec(ctx); err != nil {
			return err
		}
		return notify.SeedDefaultSubscriptions(ctx, tx, userID, isAdmin)
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset subscriptions")
	}
	return h.HandleListSubscriptions(c)
}
