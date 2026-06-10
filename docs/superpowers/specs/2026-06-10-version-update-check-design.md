# `nexorious version` update availability — design

Date: 2026-06-10
Status: approved
Follow-up to: PR #914 (update-check notifications, issue #899)

## Goal

The `nexorious version` subcommand should report whether a newer release is
available, reusing the update-check building blocks introduced in PR #914
(`internal/services/updatecheck`).

## Behavior

```
$ nexorious version
nexorious 0.10.0 (a0c1c47)
Update available: 0.11.0 — https://github.com/drzero42/nexorious/releases/tag/v0.11.0
```

- The first line (`nexorious <version> (<commit>)`) is unchanged and printed
  immediately, before any network activity.
- The command then queries the GitHub "latest release" API with a **3-second
  timeout**, applied as a context deadline (the client's 30-second HTTP
  timeout stays as is; the shorter context deadline wins).
- Outcomes, printed to stdout:
  - A newer release exists → `Update available: <version> — <release URL>`
  - Running version is current → `You are running the latest version.`
  - Check fails (offline, rate-limited, timeout, non-200) → one line to
    **stderr**: `update check failed: <reason>`. Exit code remains 0 — the
    version information was already printed and is the command's primary job.
- The check is **skipped silently** (no extra output, exit 0) when any of:
  - the `--no-check` flag is passed;
  - `UPDATE_CHECK_ENABLED=false` (or any value `strconv.ParseBool` reads as
    false) is set in the environment — same opt-out as the server's periodic
    check. Read directly via `os.Getenv`: the full `internal/config` struct
    cannot be parsed here because it requires `DATABASE_URL` etc.;
  - the running version is not valid semver (e.g. `dev` builds) — avoids a
    pointless network call whose comparison would always be false.

## Implementation

- Reuse `updatecheck.Client.FetchLatest` and `updatecheck.UpdateAvailable`
  unchanged — no new HTTP or semver-comparison code.
- Add one exported helper to `internal/services/updatecheck` (e.g.
  `IsValidVersion(v string) bool`) wrapping the existing unexported
  `normalize` + `semver.IsValid` logic, so the CLI can decide whether to skip
  the network call without duplicating normalization.
- `cmd/nexorious/version.go`:
  - add the `--no-check` boolean flag;
  - factor the check into a small function that takes a context and a
    `*updatecheck.Client` and returns the result line (and whether it goes to
    stdout or stderr), keeping the cobra `Run` glue thin and the logic
    testable.

## Testing

Table-driven test in `cmd/nexorious` using `httptest` and the existing
`updatecheck.NewClientWithBaseURL`:

- newer release available → "Update available" line with version and URL
- running version equals latest → "latest version" line
- GitHub API failure (non-200) → stderr note, exit 0
- dev (non-semver) build → check skipped, no extra output
- `--no-check` → check skipped, no extra output

This meets the project test policy: multiple meaningful edge cases.

## Docs

One sentence in `docs/admin-guide.md`, in the update-check section added by
PR #914: the `version` subcommand also reports update availability and
respects the same `UPDATE_CHECK_ENABLED` opt-out (plus `--no-check`).

## Out of scope

- No caching or state: the CLI performs a fresh check each invocation; the
  server-side `updatecheck.State` is not involved.
- No changes to the periodic scheduler check or notifications.
