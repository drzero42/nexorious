# Design: `reset-password` CLI command

Issue: [#727](https://github.com/drzero42/nexorious/issues/727)
Date: 2026-06-02

## Problem

An admin who has forgotten their password but has direct database access has no
way to recover. The web UI requires a logged-in admin to reset another user's
password, which is useless when nobody can log in. We need an offline recovery
path that talks directly to Postgres.

## Goal

A `nexorious reset-password <username>` cobra subcommand that connects directly
to the database (via `DATABASE_URL`), never to a running instance, and sets a
new password for the named user. It reuses the plumbing the `migrate` command
already established.

Non-goals:

- Talking to a running server instance (this is deliberately offline).
- Creating users, changing usernames, or toggling admin/active flags.
- Any interactive selection — the username is a required positional argument.

## Decisions

Two ambiguities in the issue text were resolved before design:

1. **Minimum-length unification scope.** The issue says to bump the admin
   reset-password check from `< 6` to `< 8` so "every password-setting path
   enforces the same ≥ 8 minimum", but the admin **create-user** path
   (`HandleCreate`) also still used `< 6` and was not named. Decision: bump it
   too, so all paths share the ≥ 8 minimum.
2. **Min vs. max.** `change-password` enforces both `≥ 8` and `≤ 128`; setup and
   admin reset only check the minimum. Decision: enforce **min only (≥ 8)** in
   the new command and the bumped admin paths — do not introduce a maximum where
   one does not already exist. `change-password`'s existing `≤ 128` is left
   untouched.

Note on phrasing: the **policy** is "≥ 8" (8 or more characters accepted). The
**code** expresses this as a rejection guard `if len(password) < 8 { error }`,
matching the existing `setup.go` / `auth.go` style. "Bump `< 6` to `< 8`" means
moving that rejection threshold — passwords shorter than 8 are refused.

## Architecture

### 1. Shared bcrypt cost

`bcryptCost` is currently an unexported const in `internal/api/auth.go`
(`const bcryptCost = 12`). Move it to `internal/auth` as the exported
`auth.BcryptCost = 12` so there is one source of truth.

- No import cycle: `internal/api` already imports `internal/auth`; `internal/auth`
  never imports `internal/api`.
- Update the four existing references in `internal/api` (`auth.go`, `setup.go`,
  `admin_users.go` × 2) to use `auth.BcryptCost`.
- The new command references `auth.BcryptCost` as well.

### 2. Password-minimum unification

Bump both `< 6` checks in `internal/api/admin_users.go` to `< 8`:

- `HandleResetPassword` (admin reset-password API).
- `HandleCreate` (admin create-user API).

After this change every password-setting path — setup, change-password, admin
create, admin reset, and the new CLI — enforces a ≥ 8 minimum. Update the
associated error message strings ("at least 6 characters" → "at least 8
characters"). Min only; no maximum is added.

### 3. New command: `cmd/nexorious/reset_password.go`

- `newResetPasswordCmd() *cobra.Command`, registered in `main.go` alongside
  `serve`/`migrate`/`version`/`login`/etc.
- `Use: "reset-password <username>"`, `Args: cobra.ExactArgs(1)`.
- Reuses `loadEnvAndConfig(cmd)` + `openBunDB(cfg.DatabaseURL)` from `migrate.go`,
  so it honours the global `--config` flag and connects straight to Postgres.

**Flow:**

1. `loadEnvAndConfig(cmd)` → `openBunDB(cfg.DatabaseURL)`; `defer db.Close()`.
2. Ping the DB with a short timeout; a connection failure → clear error,
   non-zero exit.
3. Look up the user by `username`. Unknown user → clear error, non-zero exit.
4. Print the resolved account (`username`, `is_admin`) so the admin can confirm
   the target before typing a password.
5. Prompt **"New password:"** then **"Confirm new password:"** with no echo,
   using `golang.org/x/term`'s `ReadPassword`. Mirrors the existing
   `readPassword` helper in `login.go`:
   - On a TTY: read without echo, print a newline after each prompt.
   - Non-TTY (piped) stdin: read one line per prompt, enabling
     scripting/automation. Two lines are consumed (password, then confirmation).
6. Mismatch between the two entries → error, **no DB write**.
7. Validate length ≥ 8 → otherwise error, no DB write.
8. bcrypt-hash with `auth.BcryptCost`, then in **one transaction**:
   - `UPDATE users SET password_hash = ?, updated_at = now() WHERE id = ?`
   - delete every `user_sessions` row for that user — mirrors the admin
     reset-password API, forcing re-login everywhere.
9. Print a success line; exit 0.

`golang.org/x/term` is already a direct dependency (`go.mod` require block,
used by `login.go`), so no dependency change is needed — contrary to the issue
text, which predates the `login` command landing.

### 4. Testable split

Factor the DB-mutation logic into a pure function separate from terminal
prompting:

```go
func resetUserPassword(ctx context.Context, db *bun.DB, username, plaintext string) error
```

It performs: lookup by username → validate length (≥ 8) → bcrypt-hash → tx
(update `users`, delete `user_sessions`). It returns a typed/wrapped error for
the "unknown user" and "too short" cases so the caller and tests can
distinguish them. This function lives in `cmd/nexorious` and is tested against
the existing shared `testDB` (`truncateAllTables(t)` at the top of each test).

The interactive prompting (read twice, compare, no-echo) stays a thin wrapper
that is exercised manually.

## Data flow

```
reset-password <username>
  → loadEnvAndConfig (--config / .env → config.Config)
  → openBunDB(DATABASE_URL)
  → ping
  → lookup user (username → id, is_admin)   [unknown → exit 1]
  → confirm target printed
  → prompt new password (no echo)
  → prompt confirm (no echo)                 [mismatch → exit 1]
  → resetUserPassword(ctx, db, username, plaintext)
       validate ≥8                            [too short → exit 1]
       bcrypt(plaintext, auth.BcryptCost)
       tx: UPDATE users; DELETE user_sessions
  → success line, exit 0
```

## Error handling

- DB unreachable → wrapped error, exit 1.
- Unknown username → clear "user not found" error, exit 1, no write.
- Password mismatch → error, exit 1, no write (caught in the prompt wrapper
  before `resetUserPassword`).
- Password too short (< 8) → error, exit 1, no write (caught inside
  `resetUserPassword`, so DB tests cover it).
- Errors are returned up to `main()`, which prints `error: <msg>` to stderr and
  exits non-zero — the existing root-command convention.

## Testing

`cmd/nexorious/reset_password_test.go`, against the shared `testDB`:

- **happy path** — seed a user with a known hash and ≥ 1 session row; call
  `resetUserPassword`; assert the new hash verifies with bcrypt
  (`CompareHashAndPassword`) and the old one does not, and that all
  `user_sessions` rows for that user are deleted.
- **unknown username** — call against a missing username; assert the typed
  "not found" error and that no rows changed.
- **too-short password** — call with a 7-char password; assert the validation
  error and that the stored hash is unchanged.

The prompt-layer mismatch path (two differing entries) is a thin wrapper tested
manually.

## Files touched

- `internal/auth/` — add `BcryptCost` const (new small file or existing file).
- `internal/api/auth.go`, `setup.go`, `admin_users.go` — reference
  `auth.BcryptCost`; bump the two `< 6` checks in `admin_users.go` to `< 8`.
- `cmd/nexorious/reset_password.go` — new command + `resetUserPassword`.
- `cmd/nexorious/main.go` — register `newResetPasswordCmd()`.
- `cmd/nexorious/reset_password_test.go` — tests.

(`go.mod` is unchanged — `golang.org/x/term` is already a direct dependency.)
