package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

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

// newResetPasswordCmd returns the `reset-password` subcommand. It connects
// directly to the database (via DATABASE_URL / --config), never to a running
// instance, so an admin who has lost access can recover offline.
func newResetPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-password <username>",
		Short: "Reset a user's password directly in the database",
		Long: "Reset a user's password by connecting directly to the database, not to a\n" +
			"running server. Intended for an admin who has lost access but can reach\n" +
			"Postgres. Prompts for the new password twice (no echo) and wipes the\n" +
			"user's sessions so every device must log in again.",
		Args: cobra.ExactArgs(1),
		RunE: runResetPassword,
	}
}

func runResetPassword(cmd *cobra.Command, args []string) error {
	username := args[0]

	cfg, err := loadEnvAndConfig(cmd)
	if err != nil {
		return err
	}

	db := openBunDB(cfg.DatabaseURL)
	defer func() { _ = db.Close() }()

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		cancel()
		return fmt.Errorf("database unreachable: %w", err)
	}
	cancel()

	// Resolve and display the target so the admin can confirm before typing.
	var (
		isAdmin     bool
		resolvedUsr string
	)
	err = db.QueryRowContext(ctx,
		"SELECT username, is_admin FROM users WHERE username = ?", username).
		Scan(&resolvedUsr, &isAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return fmt.Errorf("look up user %q: %w", username, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Resetting password for user %q (is_admin=%t).\n", resolvedUsr, isAdmin)

	in := bufio.NewReader(cmd.InOrStdin())
	pw, err := readNewPassword(in, out)
	if err != nil {
		return err
	}

	if err := resetUserPassword(ctx, db, username, pw); err != nil {
		if errors.Is(err, errUserNotFound) {
			return fmt.Errorf("user %q not found", username)
		}
		if errors.Is(err, errPasswordTooShort) {
			return err
		}
		slog.Error("reset-password: db", "err", err)
		return fmt.Errorf("reset password: %w", err)
	}

	fmt.Fprintf(out, "Password reset for %q. All sessions were revoked.\n", resolvedUsr)
	return nil
}

// readNewPassword prompts for the new password twice with no echo on a TTY,
// or reads one line per prompt from the reader when stdin is not a TTY (piped
// input), enabling scripted use. It returns an error if the two entries differ.
func readNewPassword(in *bufio.Reader, out io.Writer) (string, error) {
	first, err := promptSecret(in, out, "New password: ")
	if err != nil {
		return "", err
	}
	second, err := promptSecret(in, out, "Confirm new password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("passwords do not match")
	}
	return first, nil
}

// promptSecret reads a single secret. On a TTY it disables echo; otherwise it
// reads one line from the provided reader so piped input still works.
func promptSecret(in *bufio.Reader, out io.Writer, label string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(out, label)
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	fmt.Fprint(out, label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(line), nil
}
