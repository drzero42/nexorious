package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/internal/seed"
)

// SetupHandler handles the one-time admin setup endpoint.
type SetupHandler struct {
	pool     *pgxpool.Pool
	cfg      *config.Config
	migrator *migrate.Migrator
}

// NewSetupHandler returns a new SetupHandler.
func NewSetupHandler(pool *pgxpool.Pool, cfg *config.Config, migrator *migrate.Migrator) *SetupHandler {
	return &SetupHandler{pool: pool, cfg: cfg, migrator: migrator}
}

type setupAdminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupAdminResponse struct {
	User struct {
		ID        string    `json:"id"`
		Username  string    `json:"username"`
		IsAdmin   bool      `json:"is_admin"`
		IsActive  bool      `json:"is_active"`
		CreatedAt time.Time `json:"created_at"`
	} `json:"user"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// HandleSetupAdmin handles POST /api/auth/setup/admin.
//
// Creates the first admin user and issues tokens. Idempotent against
// concurrent requests via a serializable transaction.
func (h *SetupHandler) HandleSetupAdmin(c *echo.Context) error {
	var req setupAdminRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Username == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username and password are required"})
	}
	if req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username and password are required"})
	}
	if len(req.Username) < 3 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username must be at least 3 characters"})
	}
	if len(req.Password) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
	}

	userID := uuid.NewString()
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		slog.Error("setup admin: bcrypt", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	var createdAt time.Time
	for attempt := 0; attempt <= 1; attempt++ {
		createdAt, err = h.tryCreateAdmin(context.Background(), userID, req.Username, string(hash))
		if err == nil {
			break
		}
		if isSerializationFailure(err) && attempt == 0 {
			continue
		}
		if isUserExistsError(err) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "setup already complete"})
		}
		slog.Error("setup admin: create user", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if _, seedErr := seed.SeedAll(context.Background(), h.pool); seedErr != nil {
		slog.Warn("setup admin: seed failed", "err", seedErr)
	}

	accessToken, refreshToken, tokenErr := issueTokensAndSession(
		context.Background(), h.pool, h.cfg, userID,
		c.Request().Header.Get("User-Agent"),
		c.RealIP(),
	)

	// Always clear needsSetup — the user row has committed.
	h.migrator.SetNeedsSetup(false)

	if tokenErr != nil {
		slog.Error("setup admin: issue tokens", "err", tokenErr)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "setup succeeded but session could not be created — please log in",
		})
	}

	var resp setupAdminResponse
	resp.User.ID = userID
	resp.User.Username = req.Username
	resp.User.IsAdmin = true
	resp.User.IsActive = true
	resp.User.CreatedAt = createdAt
	resp.AccessToken = accessToken
	resp.RefreshToken = refreshToken
	return c.JSON(http.StatusCreated, resp)
}

// tryCreateAdmin opens a serializable transaction, checks users is empty, and inserts the admin row.
func (h *SetupHandler) tryCreateAdmin(ctx context.Context, userID, username, passwordHash string) (time.Time, error) {
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var count int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return time.Time{}, err
	}
	if count > 0 {
		return time.Time{}, errUserExists
	}

	var createdAt time.Time
	err = tx.QueryRow(ctx,
		`INSERT INTO users (id, username, password_hash, is_admin)
		 VALUES ($1, $2, $3, true) RETURNING created_at`,
		userID, username, passwordHash,
	).Scan(&createdAt)
	if err != nil {
		return time.Time{}, err
	}
	return createdAt, tx.Commit(ctx)
}

var errUserExists = errors.New("users already exist")

func isUserExistsError(err error) bool {
	return errors.Is(err, errUserExists)
}

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
