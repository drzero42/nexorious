# Enable the errcheck `check-blank` lint gate (issue #587)

Date: 2026-05-27
Tracking issue: [#587](https://github.com/drzero42/nexorious/issues/587) (child of [#534](https://github.com/drzero42/nexorious/issues/534))
Parent design: [2026-05-21-issue-534-silent-errors-design.md](2026-05-21-issue-534-silent-errors-design.md)
Milestone: 0.1.0
Audit commit: `c08f5373`

## Background

Issue #587 is the final child of #534 — "lock in the win so the silent-error
bug class can't silently re-emerge." Children #583–#586 (Sev 1–4) are merged.

A re-audit on 2026-05-27 found that the issue, as originally written, **does not
do what it intends**:

1. **errcheck is already enabled.** It is part of golangci-lint's default
   `standard` linter set, so it already runs in CI and currently reports **0
   issues**. Adding `errcheck` to an `enable:` list is a no-op.
2. **errcheck ignores blank assignments by default.** The entire #534 effort
   targets `_ =` / `_, _ =` discards, but errcheck only flags *bare* unchecked
   calls (e.g. `resp.Body.Close()` with no assignment). The codebase has none —
   every error-returning call is either handled or explicitly `_ =`'d. So the
   default gate is already green and stays green no matter how many new `_ =`
   discards are added.
3. **The proposed `exclude-functions` allowlist therefore has no effect.**
   Exclusions only matter for calls errcheck would otherwise flag; with 0
   findings, the allowlist suppresses nothing.

The single setting that makes the gate enforce #534's goal is
**`errcheck.check-blank: true`**, which makes errcheck flag every `_ =`
discard. With it enabled (uncapped), the audit surfaces **70 production** and
**551 test** discard sites — far more than the parent spec's ~15-row
"Acceptable" inventory anticipated, because the parent spec's config snippet
omitted `check-blank` and underestimated the cleanup scope.

This spec supersedes the parent spec's "PR 5 — errcheck lint" section.

## Goal

Turn `errcheck.check-blank: true` on and bring the build back to zero findings
by **justifying or fixing every surfaced discard**, so that any *new*
unjustified `_ =` fails CI.

## Non-goals

- Re-architecting error handling (no new error types).
- Touching test files — `_ =` in tests is out of scope per #534. Test files are
  excluded from errcheck entirely.
- Broad function-class allowlisting. Per an explicit "maximum protection"
  decision, only one function is allowlisted (`Tx.Rollback`); every other
  acceptable discard gets a per-site `//nolint:errcheck` so the gate keeps
  catching *new* unchecked `Atoi`/`Marshal`/`RowsAffected`/etc. elsewhere.

## Approach: surgical / maximum-protection

| Mechanism | What it covers | Why |
|---|---|---|
| `errcheck.check-blank: true` | Flags all `_ =` / `_, _ =` discards | The actual gate that prevents #534 regressing |
| `exclusions.presets: [std-error-handling]` | The conventional no-recovery stdlib family: `(*T).Close`, `fmt.Fprint(f\|ln)`, `hash.Write`, etc. | Universally idiomatic; verified the codebase already checks every write-path `Close` (see "Verified non-issue") |
| `exclude-functions: [(github.com/uptrace/bun.Tx).Rollback]` | Deferred `_ = tx.Rollback()` cleanup (3 sites) | Rollback after commit is a no-op; the defer idiom is safe and repetitive enough to allowlist |
| `exclusions.rules` path `_test\.go` → `errcheck` | All test files | `_ =` in tests is out of scope per #534 |
| Per-site `//nolint:errcheck // <reason>` | 44 acceptable production discards | Keeps the gate sharp: a *new* unchecked call of the same function elsewhere still fails CI |
| Code fix (log the error) | 2 genuinely-swallowed DB writes | They are real Sev-3 defects #585 missed |

Of the ~70 raw production discards, the `std-error-handling` preset clears the
`Close`/`Fprint` family (~21 sites) and the `Tx.Rollback` allowlist 3 more,
leaving **46 production sites** to address individually — **44** annotated with
`//nolint:errcheck` and **2** fixed.

Rejected alternatives: a *broad* allowlist (fewer annotations, but globally
stops catching `json.Marshal`/`os.Stat`/etc. errors in future code) and a
*no-op literal reading* of the issue (add errcheck + allowlist without
`check-blank`, changing nothing).

## The config change

The complete new `.golangci.yml`. errcheck is not added to any `enable:` list
because it is already on; only its settings and the exclusions change. The
existing `node_modules` path exclusion is preserved.

```yaml
version: "2"

linters:
  settings:
    errcheck:
      # Flag `_ =` / `_, _ =` discards — the exact pattern issue #534 set out to
      # eliminate. Without this, errcheck ignores all blank assignments and the
      # regression it guards against can silently return.
      check-blank: true
      exclude-functions:
        # Rollback after a committed tx is a no-op; the deferred
        # `defer func() { _ = tx.Rollback() }()` cleanup idiom is safe.
        - (github.com/uptrace/bun.Tx).Rollback
  exclusions:
    presets:
      # Excludes the conventional no-recovery stdlib cases: (*T).Close,
      # fmt.Fprint/Fprintf/Fprintln, hash.Write, etc.
      - std-error-handling
    rules:
      # Test files are out of scope for the #534 audit — `_ =` in tests is fine.
      - path: _test\.go
        linters:
          - errcheck
    paths:
      # ui/frontend/node_modules is npm's dependency tree. A handful of npm
      # packages (notably `flatted`) ship .go files for cross-language tests.
      # Those files are not part of this module and must not be linted.
      - ui/frontend/node_modules
```

## Per-site suppression policy

Convention: `//nolint:errcheck // <one-line reason>` on the offending line,
keeping the blank assignment that Go syntax requires, e.g.:

```go
page, _ := strconv.Atoi(c.QueryParam("page")) //nolint:errcheck // invalid/empty input clamped to default below
if page < 1 {
    page = 1
}
```

Each reason must be *verified true at implementation time* — e.g. confirm the
`strconv.Atoi` result is actually clamped/defaulted and not used raw, and that
each `json.Marshal` argument is a fixed in-code struct that cannot fail. If a
site turns out not to be acceptable, fix it instead of annotating it.

### The 44 per-site `//nolint` sites, by reason

| Reason | Function | Sites |
|---|---|---|
| Query/path param parse, clamped to a default | `strconv.Atoi` | `api/backup.go:64,65,70`; `api/games.go:90,94`; `api/jobs.go:105,109,322,471,475`; `services/psn/duration.go:19` |
| Advisory row count; the pg driver never errors here | `result.RowsAffected` | `api/user_games.go:757,801,1052`; `scheduler/scheduler.go:220,319,375,390`; `scheduler/stale_jobs.go:44` |
| Marshal of a fixed in-code struct; cannot fail in practice | `json.Marshal` | `cmd/nexorious/serve.go:477`; `worker/tasks/import_item.go:415,457,462`; `worker/tasks/metadata_refresh.go:115,251,258`; `worker/tasks/sync.go:479` |
| Cosmetic stats; zero-value display is acceptable | `(*sql.Row).Scan` | `backup/service.go:97,98,99,102` |
| Hash write / `io.Discard` drain | `io.Copy` | `backup/service.go:421,436`; `services/igdb/igdb.go:530` |
| Best-effort backup/restore filesystem ops | `os.Stat`, `filepath.WalkDir`, `filepath.Rel`, `os.ReadDir`, `copyDir` | `backup/service.go:158,427,453,484,825,857` |
| Response body read only to build an error log line | `io.ReadAll` | `services/psn/client.go:280` |
| Flag is always registered; error is impossible | pflag `GetString` | `cmd/nexorious/migrate.go:78` |
| Graceful-shutdown best-effort; nowhere to surface | `riverClient.Stop` | `cmd/nexorious/serve.go:243` |
| Already records its own failure on the `job_items` row | `EnqueueOrFail` | `worker/tasks/sync.go:220` |

## Genuine fixes (2 sites — not suppressed)

[worker/tasks/sync.go:164](../../../internal/worker/tasks/sync.go#L164) and
[worker/tasks/sync.go:226](../../../internal/worker/tasks/sync.go#L226):

```go
_, _ = w.DB.NewRaw(
    `UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
    p.UserID, p.Storefront,
).Exec(ctx)
```

This state-mutating write drives the UI's "credentials error" indicator; if it
silently fails the user never sees that their stored credentials are bad. This
is the Sev-3 class #585 was meant to cover but missed (the audit ran on an
earlier commit and the line numbers had drifted). Fix per the parent spec's
worker convention (background worker → log-only; there is no caller to return
to):

```go
if _, err := w.DB.NewRaw(
    `UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
    p.UserID, p.Storefront,
).Exec(ctx); err != nil {
    slog.Error("sync: failed to flag credentials_error", "err", err, "user_id", p.UserID, "storefront", p.Storefront)
}
```

These add observability only — no control-flow or response change. Because the
PR includes these two genuine defect fixes, its Conventional-Commit type is
**`fix:`** (not `chore:`), so it produces a patch release.

## Verified non-issue (recorded so it is not re-investigated)

The `std-error-handling` preset broadly suppresses the `Close` family, which
raised the question of whether write-then-`Close` failures (a truncated archive)
would be hidden. They are not: in `backup/service.go` every *success-path* write
close is already explicitly checked — `tw.Close()` (511), `gw.Close()` (514),
`outFile.Close()` (621), `out.Close()` (893). Only deferred read-side cleanup
and *error-path* closes superseded by a returned `io.Copy` error (618, 890) are
discarded, both correct. There is nothing to harden here.

## Verification & testing

- **Acceptance test = CI.** `golangci-lint run` reports **0 issues** with the
  new config (the parent spec's "PR 5" verification: zero `errcheck` findings).
- `go test ./...` passes — the two fixes are log-only.
- No new unit tests. The fixes add observability, not behaviour; this matches
  the project's testing policy (test security-sensitive logic, non-obvious
  invariants, real bugs — logging a previously-swallowed error meets none).
- No manual smoke-testing — there are no response-code or control-flow changes.

## Risks

- **Test files lose default errcheck**, not just `check-blank` — there is no
  per-path `check-blank` toggle, so the whole linter is excluded on `_test\.go`.
  Test files currently have 0 errcheck findings, so nothing regresses; accepted
  per #534's "tests out of scope."
- **CI installs `golangci-lint@latest`**
  ([test.yaml:55](../../../.github/workflows/test.yaml#L55)). The v2 setting and
  preset names used here are stable, but a future v3 schema change is a
  pre-existing risk this PR neither adds nor removes.
- **`//nolint` reasons can rot.** Each is verified true at implementation; if a
  guarded `Atoi`/`Marshal` later changes so the error matters, the annotation
  would mask it. Mitigated by keeping reasons specific and per-site (not
  blanket), so review sees them on the relevant line.

## Post-merge follow-up (separate docs PR, not part of #587's diff)

After #587 merges, run the `claude-md-management:revise-claude-md` skill to
document the new policy in CLAUDE.md: that `errcheck.check-blank` is on; that the
default response to a flagged discard is to **handle** the error (handler → 500,
worker → `slog.Error`); and that `//nolint:errcheck // reason` is reserved for
genuinely-acceptable discards (clamped param parses, advisory `RowsAffected`,
marshals of fixed structs, best-effort cleanup) — each with a one-line
justification. This keeps #587's own diff scoped to the lint gate.

## Out of scope

- Adding context fields to existing `slog.Error` calls beyond the two fixes.
- Adopting structured/sentinel error types.
- Test files (`_ =` in tests).
- Re-auditing the `std-error-handling`-suppressed `Close`/`Fprint` family
  (verified correct above).
