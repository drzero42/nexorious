package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
)

// minPasswordLength is the minimum password length this command enforces.
// It matches the ≥ 8 minimum used independently by the other password-setting paths (setup, change-password, admin create/reset).
const minPasswordLength = 8

var (
	errUserNotFound     = errors.New("user not found")
	errPasswordTooShort = fmt.Errorf("password must be at least %d characters", minPasswordLength)
)

// resetUserPassword looks up the user by username, validates the new password,
// and in a single transaction updates the password hash and deletes every
// session for that user (forcing re-login everywhere). It is the testable core
// of the reset-password command, free of any terminal interaction.
func resetUserPassword(ctx context.Context, db *bun.DB, username, plaintext string) error {
	if len(plaintext) < minPasswordLength {
		return errPasswordTooShort
	}

	var userID string
	err := db.QueryRowContext(ctx,
		"SELECT id FROM users WHERE username = ?", username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return errUserNotFound
	}
	if err != nil {
		return fmt.Errorf("look up user %q: %w", username, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), auth.BcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
			string(hash), now, userID,
		); err != nil {
			return err
		}
		if _, err := tx.NewDelete().
			Model((*models.UserSession)(nil)).
			Where("user_id = ?", userID).
			Exec(ctx); err != nil {
			return err
		}
		return nil
	})
}
