# reset-password CLI Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `nexorious reset-password <username>` cobra subcommand that connects directly to Postgres (never to a running instance) and sets a new password for the named user, wiping their sessions.

**Architecture:** A new cobra command in `cmd/nexorious` reuses the `migrate` command's `loadEnvAndConfig` + `openBunDB` plumbing. DB-mutation logic is factored into a pure, testable `resetUserPassword(ctx, db, username, plaintext)` function; the interactive no-echo double prompt is a thin wrapper mirroring `login.go`'s `readPassword`. The bcrypt cost constant is promoted to `internal/auth` as the single source of truth, and all admin password-setting paths are unified on a ≥ 8 minimum.

**Tech Stack:** Go, cobra, Bun (`uptrace/bun`), `golang.org/x/crypto/bcrypt`, `golang.org/x/term`, testcontainers-go (shared `testDB`).

**Spec:** `docs/superpowers/specs/2026-06-02-reset-password-cli-design.md`

---

## File Structure

- `internal/auth/bcrypt.go` — **create**: `const BcryptCost = 12` (single source of truth).
- `internal/api/auth.go` — **modify**: remove local `bcryptCost`; use `auth.BcryptCost`.
- `internal/api/setup.go` — **modify**: use `auth.BcryptCost`.
- `internal/api/admin_users.go` — **modify**: use `auth.BcryptCost`; bump two `< 6` checks to `< 8`.
- `internal/api/admin_users_test.go` — **modify**: update two expected error-message strings.
- `cmd/nexorious/reset_password.go` — **create**: command + `resetUserPassword` + prompt wrapper.
- `cmd/nexorious/main.go` — **modify**: register `newResetPasswordCmd()`.
- `cmd/nexorious/reset_password_test.go` — **create**: tests for `resetUserPassword`.

---

## Task 1: Promote bcrypt cost to `internal/auth`

Pure refactor, no behaviour change. `internal/api` already imports `internal/auth` in all three files; `internal/auth` never imports `internal/api`, so no cycle.

**Files:**
- Create: `internal/auth/bcrypt.go`
- Modify: `internal/api/auth.go:22` (remove const), and the `bcryptCost` references in `internal/api/auth.go:204`, `internal/api/setup.go:63`, `internal/api/admin_users.go:149`, `internal/api/admin_users.go:320`

- [ ] **Step 1: Create the constant**

Create `internal/auth/bcrypt.go`:

```go
package auth

// BcryptCost is the bcrypt cost factor used for every password hash in the
// application. It lives here so the API handlers and the reset-password CLI
// command share one source of truth.
const BcryptCost = 12
```

- [ ] **Step 2: Remove the old const in `internal/api/auth.go`**

Delete this line (currently `internal/api/auth.go:22`):

```go
const bcryptCost = 12
```

- [ ] **Step 3: Update the four references**

In `internal/api/auth.go` (the `change-password` handler) and `internal/api/setup.go` and `internal/api/admin_users.go` (×2), replace `bcryptCost` with `auth.BcryptCost`. Each call currently looks like:

```go
bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
```

and becomes:

```go
bcrypt.GenerateFromPassword([]byte(req.NewPassword), auth.BcryptCost)
```

All three files already import `github.com/drzero42/nexorious/internal/auth`, so no import edits are needed. Verify no other `bcryptCost` references remain:

Run: `grep -rn "bcryptCost" internal/ cmd/`
Expected: no matches.

- [ ] **Step 4: Build to verify the refactor compiles**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/bcrypt.go internal/api/auth.go internal/api/setup.go internal/api/admin_users.go
git commit -m "refactor: promote bcrypt cost to auth.BcryptCost"
```

---

## Task 2: Unify admin password minimum at ≥ 8

Bump both `< 6` checks in `admin_users.go` to `< 8` and update the matching error strings, so setup, change-password, admin create, and admin reset all enforce ≥ 8. Two existing tests assert the old message; update them. (Happy-path admin tests all use ≥ 8-char passwords already, so they are unaffected.)

**Files:**
- Modify: `internal/api/admin_users.go:130` (HandleCreate), `internal/api/admin_users.go:307` (HandleResetPassword)
- Modify: `internal/api/admin_users_test.go:168`, `internal/api/admin_users_test.go:590`

- [ ] **Step 1: Update the create-user test expectation to fail first**

In `internal/api/admin_users_test.go` around line 161-169, the test posts password `"12345"`. Change the expected message:

```go
	if resp["error"] != "password must be at least 8 characters" {
		t.Errorf("error = %q, want %q", resp["error"], "password must be at least 8 characters")
	}
```

- [ ] **Step 2: Update the reset-password test expectation**

In `internal/api/admin_users_test.go` around line 583-590, the test posts `new_password` `"12345"`. Change the expected message:

```go
	if resp["error"] != "new password must be at least 8 characters" {
		t.Errorf("error = %q, want %q", resp["error"], "new password must be at least 8 characters")
	}
```

- [ ] **Step 3: Run the two tests to verify they now fail**

Run: `go test ./internal/api/ -run 'TestAdmin' -v 2>&1 | grep -E 'FAIL|at least'`
Expected: the two assertions fail because the handler still emits "at least 6 characters".

- [ ] **Step 4: Bump the create-user check**

In `internal/api/admin_users.go` (HandleCreate, ~line 130):

```go
	if len(req.Password) < 8 {
		return errorJSON(c, http.StatusBadRequest, "password must be at least 8 characters")
	}
```

- [ ] **Step 5: Bump the reset-password check**

In `internal/api/admin_users.go` (HandleResetPassword, ~line 307):

```go
	if len(req.NewPassword) < 8 {
		return errorJSON(c, http.StatusBadRequest, "new password must be at least 8 characters")
	}
```

- [ ] **Step 6: Run the admin tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestAdmin' -v 2>&1 | tail -5`
Expected: PASS (no FAIL lines).

- [ ] **Step 7: Commit**

```bash
git add internal/api/admin_users.go internal/api/admin_users_test.go
git commit -m "feat: enforce 8-char minimum on admin create and reset password"
```

---

## Task 3: `resetUserPassword` DB-mutation function (TDD)

The pure function that the command and the tests both call. It looks up the user by username, validates length, hashes with `auth.BcryptCost`, and in one transaction updates `users.password_hash`/`updated_at` and deletes all `user_sessions` for that user. Lives in `cmd/nexorious` and is tested against the shared `testDB`.

**Files:**
- Create: `cmd/nexorious/reset_password.go` (function only in this task)
- Test: `cmd/nexorious/reset_password_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/nexorious/reset_password_test.go`:

```go
package main

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
)

// seedResetUser inserts a user with a known bcrypt hash and returns its id.
func seedResetUser(t *testing.T, username, password string, isAdmin bool) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.BcryptCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	id := "u-" + username
	_, err = testDB.ExecContext(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES (?, ?, ?, ?, ?)",
		id, username, string(hash), true, isAdmin,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedResetSession inserts one session row for userID.
func seedResetSession(t *testing.T, userID string) {
	t.Helper()
	sessionID, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID: %v", err)
	}
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at)
		 VALUES (gen_random_uuid()::text, ?, ?, now() + interval '30 days')`,
		userID, auth.HashToken(sessionID),
	)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestResetUserPassword_HappyPath(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	id := seedResetUser(t, "alice", "oldpassword", true)
	seedResetSession(t, id)
	seedResetSession(t, id)

	if err := resetUserPassword(ctx, testDB, "alice", "newpassword"); err != nil {
		t.Fatalf("resetUserPassword: %v", err)
	}

	var hash string
	if err := testDB.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash); err != nil {
		t.Fatalf("select hash: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("newpassword")) != nil {
		t.Error("new password does not verify against stored hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("oldpassword")) == nil {
		t.Error("old password still verifies; hash was not changed")
	}

	var sessions int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", id).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Errorf("sessions = %d, want 0 (all should be wiped)", sessions)
	}
}

func TestResetUserPassword_UnknownUser(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	err := resetUserPassword(ctx, testDB, "ghost", "newpassword")
	if !errors.Is(err, errUserNotFound) {
		t.Fatalf("err = %v, want errUserNotFound", err)
	}
}

func TestResetUserPassword_TooShort(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	id := seedResetUser(t, "bob", "oldpassword", false)

	err := resetUserPassword(ctx, testDB, "bob", "short77") // 7 chars
	if !errors.Is(err, errPasswordTooShort) {
		t.Fatalf("err = %v, want errPasswordTooShort", err)
	}

	// Hash must be unchanged.
	var hash string
	if err := testDB.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash); err != nil {
		t.Fatalf("select hash: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("oldpassword")) != nil {
		t.Error("hash changed despite too-short password")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/nexorious/ -run TestResetUserPassword -v`
Expected: compile failure — `resetUserPassword`, `errUserNotFound`, `errPasswordTooShort` undefined.

- [ ] **Step 3: Implement the function**

Create `cmd/nexorious/reset_password.go`:

```go
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

// minPasswordLength is the shared ≥ 8 minimum enforced across every
// password-setting path (setup, change-password, admin reset, this command).
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/nexorious/ -run TestResetUserPassword -v`
Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/reset_password.go cmd/nexorious/reset_password_test.go
git commit -m "feat: add resetUserPassword DB-mutation core"
```

---

## Task 4: The command + interactive prompt wrapper

Wire `resetUserPassword` into a cobra command with the no-echo double prompt. The prompt wrapper mirrors `login.go`'s `readPassword` (TTY no-echo; non-TTY reads one line per prompt). This wrapper is exercised manually, per the spec; no automated test is added for the terminal interaction.

**Files:**
- Modify: `cmd/nexorious/reset_password.go` (add command + prompt wrapper to the file from Task 3)
- Modify: `cmd/nexorious/main.go` (register the command)

- [ ] **Step 1: Add the imports and command to `reset_password.go`**

Add these imports to the existing import block in `cmd/nexorious/reset_password.go`:

```go
	"bufio"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
```

(Combined with Task 3's imports, the block is: `bufio`, `context`, `database/sql`, `errors`, `fmt`, `io`, `log/slog`, `os`, `strings`, `time`, `github.com/spf13/cobra`, `github.com/uptrace/bun`, `golang.org/x/crypto/bcrypt`, `golang.org/x/term`, the two internal packages.)

Append the command, runner, and prompt wrapper to the file:

```go
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
```

- [ ] **Step 2: Register the command in `main.go`**

In `cmd/nexorious/main.go`, in `newRootCmd`, add the registration alongside the others:

```go
	root.AddCommand(newResetPasswordCmd())
```

Place it after `root.AddCommand(newMigrateCmd())` to keep DB-direct commands grouped.

- [ ] **Step 3: Build to verify it compiles**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 4: Verify the command is registered**

Run: `go run ./cmd/nexorious reset-password --help`
Expected: help text for `reset-password <username>` prints, mentioning it connects directly to the database.

- [ ] **Step 5: Re-run the package tests**

Run: `go test ./cmd/nexorious/ -run TestResetUserPassword -v`
Expected: all three PASS (still green after adding the command).

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/reset_password.go cmd/nexorious/main.go
git commit -m "feat: add reset-password CLI command (#727)"
```

---

## Task 5: Final verification

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 2: Lint**

Run: `golangci-lint run`
Expected: no findings.

- [ ] **Step 3: Targeted test sweep**

Run: `go test ./cmd/nexorious/... ./internal/api/... ./internal/auth/...`
Expected: PASS.

- [ ] **Step 4: Confirm no stray `bcryptCost` remains**

Run: `grep -rn "bcryptCost" internal/ cmd/`
Expected: no matches (all replaced by `auth.BcryptCost`).
```
