package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/notify"
)

// SetupHandler handles the one-time admin setup endpoint.
type SetupHandler struct {
	db       *bun.DB
	cfg      *config.Config
	migrator *migrate.Migrator
}

// NewSetupHandler returns a new SetupHandler.
func NewSetupHandler(db *bun.DB, cfg *config.Config, migrator *migrate.Migrator) *SetupHandler {
	return &SetupHandler{db: db, cfg: cfg, migrator: migrator}
}

type setupAdminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleSetupAdmin handles POST /api/auth/setup/admin.
//
// Creates the first admin user and issues tokens. Idempotent against
// concurrent requests via a serializable transaction.
func (h *SetupHandler) HandleSetupAdmin(c *echo.Context) error {
	var req setupAdminRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username and password are required")
	}
	if req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username and password are required")
	}
	if len(req.Username) < 3 {
		return echo.NewHTTPError(http.StatusBadRequest, "username must be at least 3 characters")
	}
	if len(req.Password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}

	userID := uuid.NewString()
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		slog.Error("setup admin: bcrypt", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	for attempt := range 2 {
		_, err = h.tryCreateAdmin(context.Background(), userID, req.Username, string(hash))
		if err == nil {
			break
		}
		if isSerializationFailure(err) && attempt == 0 {
			continue
		}
		if isUserExistsError(err) {
			return echo.NewHTTPError(http.StatusForbidden, "setup already complete")
		}
		slog.Error("setup admin: create user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Always clear needsSetup — the user row has committed.
	h.migrator.SetNeedsSetup(false)

	if err := notify.SeedDefaultSubscriptions(context.Background(), h.db, userID); err != nil {
		slog.Error("setup: seed notification subscriptions", "user_id", userID, "err", err)
	}

	sessionID, sessionErr := issueSession(h.db, h.cfg.SessionExpireDays, userID,
		c.Request().Header.Get("User-Agent"),
		c.RealIP(),
	)
	if sessionErr != nil {
		slog.Error("setup admin: issue session", "err", sessionErr)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "setup succeeded but session could not be created — please log in",
		})
	}
	auth.SetSessionCookie(c, sessionID, h.cfg.SessionExpireDays, h.cfg.SessionCookieSecure)

	resp, loadErr := loadMeResponse(context.Background(), h.db, userID)
	if loadErr != nil {
		slog.Error("setup admin: load user", "err", loadErr)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusCreated, resp)
}

// tryCreateAdmin opens a serializable transaction, checks users is empty, and inserts the admin row.
func (h *SetupHandler) tryCreateAdmin(ctx context.Context, userID, username, passwordHash string) (time.Time, error) {
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return time.Time{}, err
	}
	if count > 0 {
		return time.Time{}, errUserExists
	}

	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (id, username, password_hash, is_admin)
		 VALUES (?, ?, ?, true) RETURNING created_at`,
		userID, username, passwordHash,
	).Scan(&createdAt)
	if err != nil {
		return time.Time{}, err
	}
	return createdAt, tx.Commit()
}

var errUserExists = errors.New("users already exist")

func isUserExistsError(err error) bool {
	return errors.Is(err, errUserExists)
}

func isSerializationFailure(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "40001"
	}
	return false
}
