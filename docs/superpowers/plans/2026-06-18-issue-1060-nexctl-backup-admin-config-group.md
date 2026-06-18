# `nexctl` Phase 6 — `backup` + `admin` + `config` Implementation Plan (Epic #1060)

Design: [../specs/2026-06-18-issue-1060-nexctl-backup-admin-config-group-design.md](../specs/2026-06-18-issue-1060-nexctl-backup-admin-config-group-design.md)

One PR, whole phase. Subagent-driven: per-task implementer → controller commit → independent review → whole-branch review → merge on explicit user instruction. **Serialize same-package implementers** (cliclient tasks C1–C3; cmd/nexctl tasks N1–N4) — concurrent edits in one Go package collide. Reviewers (read-only) run concurrently with the next implementer.

## Tasks

- **C1 — backup client methods.** `internal/cliclient/backup.go`: `GetBackupConfig`/`UpdateBackupConfig`, `ListBackups`, `CreateBackup`, `DeleteBackup`, `DownloadBackup` (stream), `RestoreBackup` (`{confirm:true}`), `RestoreBackupUpload` (multipart). Types `BackupConfig`, `Backup`, `CreateBackupResult`. Tests: httptest contracts + multipart + stream + 409/503 error paths.
- **C2 — admin client methods.** `internal/cliclient/admin.go`: `CreateUser`, `ListUsers`, `GetUser`, `UpdateUser` (map fields), `ResetUserPassword`, `GetDeletionImpact`, `DeleteUser`, `AdminReset`. Types `AdminUser`, `DeletionImpact`. Tests incl. partial-update body assertions + reset count. (After C1 — same package.)
- **C3 — settings + notify client methods.** `internal/cliclient/config.go`: `GetSettings`/`UpdateSettings`; notify channels (`List/Create/Update/Delete/TestChannel`, `TestURL`), subscriptions (`List/Put/ResetSubscriptions`), `ListEventTypes`. Types `Settings`, `NotifyChannel`, `EventType`. Tests: envelope decodes, secret-URL never in responses, 204/502 paths. (After C2.)
- **N1 — backup group.** `cmd/nexctl/backup.go`: `newBackupCmd()` (list/create/rm/download/restore/schedule[/set]); register on root + want-map. Loud confirm on restore; `download` mirrors export's `--out`/`-`/default. Tests via root + httptest.
- **N2 — admin group.** `cmd/nexctl/admin.go`: `newAdminCmd()` → `admin user` subgroup (list/show/create/enable/disable/set-admin/passwd/rm) + `admin reset`. Password no-echo; `rm` shows deletion-impact then confirms; `reset` loud confirm. Register on root + want-map. (After N1 — same package.)
- **N3 — config settings + parent.** `cmd/nexctl/config.go`: `newConfigCmd()` with `get`/`set --deal-region`; register the `config` parent on root + want-map. (After N2.)
- **N4 — config notify.** `cmd/nexctl/config_notify.go`: `config notify` subgroup — `channel list/create/edit/rm/test`, `test-url`, `sub list/set/reset`, `events`. Secret URL no-echo; add subcommands to `newConfigCmd`. (After N3.)
- **T-docs — docs + finalize.** Update `CLAUDE.md` `cmd/nexctl/` bullet (backup/admin/config groups, admin-key requirement, setup-restore exclusion, secret URLs). Confirm spec/plan committed. Whole-branch review.

## Verification gates (per task + final)

`go build ./...`, `go test ./internal/cliclient/... ./cmd/nexctl/...`, `golangci-lint run`, `make deadcode` (after command wiring), `nexctl` REST-boundary import check (no server/DB packages).
