# Enforce gofmt in CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `golangci-lint run` fail on any unformatted Go file by enabling the `gofmt` formatter in golangci-lint v2, preceded by a one-time cleanup of the 26 currently-drifted files.

**Architecture:** Two commits on a feature branch — first a pure reformatting commit (no logic changes), then the `.golangci.yml` config change that locks in the enforcement. This separation makes the cleanup easy to review (whitespace-only diff) and keeps the CI gate change isolated.

**Tech Stack:** Go, golangci-lint v2 (`formatters:` block), gofmt

---

### Task 1: Create feature branch

**Files:**
- No files changed

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b chore/enforce-gofmt-ci
```

Expected: switched to new branch `chore/enforce-gofmt-ci`

---

### Task 2: Reformat the 26 drifted files

**Files:**
- Modify: `cmd/nexorious/setup_test.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/scheduler/main_test.go`
- Modify: `internal/scheduler/cleanup_test.go`
- Modify: `internal/auth/main_test.go`
- Modify: `internal/crypto/crypto_test.go`
- Modify: `internal/api/sync.go`
- Modify: `internal/api/export_test.go`
- Modify: `internal/api/jobs.go`
- Modify: `internal/api/backup.go`
- Modify: `internal/api/setup_test.go`
- Modify: `internal/api/games_test.go`
- Modify: `internal/api/games.go`
- Modify: `internal/api/main_test.go`
- Modify: `internal/backup/main_test.go`
- Modify: `internal/backup/service_test.go`
- Modify: `internal/worker/tasks/helpers_test.go`
- Modify: `internal/worker/tasks/export.go`
- Modify: `internal/worker/tasks/main_test.go`
- Modify: `internal/worker/tasks/export_helpers_test.go`
- Modify: `internal/services/epic/adapter_test.go`
- Modify: `internal/services/psn/export_test.go`
- Modify: `internal/services/matching/matching_test.go`
- Modify: `internal/services/igdb/keywords.go`
- Modify: `internal/services/igdb/models.go`
- Modify: `internal/db/models/backup_config.go`

- [ ] **Step 1: Run gofmt over all first-party Go files**

```bash
gofmt -w $(find . -name '*.go' \
  -not -path './ui/frontend/node_modules/*' \
  -not -path './.devenv/*' \
  -not -path './vendor/*')
```

Expected: exits 0, no output.

- [ ] **Step 2: Verify no files remain unformatted**

```bash
gofmt -l $(find . -name '*.go' \
  -not -path './ui/frontend/node_modules/*' \
  -not -path './.devenv/*' \
  -not -path './vendor/*')
```

Expected: empty output (all files clean).

- [ ] **Step 3: Verify golangci-lint still passes**

```bash
golangci-lint run
```

Expected: exits 0. This confirms the reformatting introduced no logic issues visible to the linter.

- [ ] **Step 4: Commit the cleanup**

```bash
git add \
  cmd/nexorious/setup_test.go \
  internal/config/config_test.go \
  internal/scheduler/main_test.go \
  internal/scheduler/cleanup_test.go \
  internal/auth/main_test.go \
  internal/crypto/crypto_test.go \
  internal/api/sync.go \
  internal/api/export_test.go \
  internal/api/jobs.go \
  internal/api/backup.go \
  internal/api/setup_test.go \
  internal/api/games_test.go \
  internal/api/games.go \
  internal/api/main_test.go \
  internal/backup/main_test.go \
  internal/backup/service_test.go \
  internal/worker/tasks/helpers_test.go \
  internal/worker/tasks/export.go \
  internal/worker/tasks/main_test.go \
  internal/worker/tasks/export_helpers_test.go \
  internal/services/epic/adapter_test.go \
  internal/services/psn/export_test.go \
  internal/services/matching/matching_test.go \
  internal/services/igdb/keywords.go \
  internal/services/igdb/models.go \
  internal/db/models/backup_config.go

git commit -m "chore: reformat Go files with gofmt"
```

---

### Task 3: Enable the gofmt formatter in golangci-lint

**Files:**
- Modify: `.golangci.yml`

- [ ] **Step 1: Add the `formatters:` block to `.golangci.yml`**

The file currently ends after the `paths:` exclusion block. Append the new block so the full file reads:

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

formatters:
  enable:
    - gofmt
```

- [ ] **Step 2: Verify golangci-lint passes on the clean tree**

```bash
golangci-lint run
```

Expected: exits 0. The formatter is enabled and all files are already clean.

- [ ] **Step 3: Verify the formatter catches an unformatted file**

Create a temporarily malformed file, confirm failure, then restore it:

```bash
# Introduce a formatting violation
printf 'package api\n\nvar _ = 1+1\n' > /tmp/gofmt_test_probe.go
cp internal/api/games.go internal/api/games.go.bak
cat /tmp/gofmt_test_probe.go >> internal/api/games.go

# Must fail
golangci-lint run ./internal/api/
echo "exit: $?"

# Restore
cp internal/api/games.go.bak internal/api/games.go
rm internal/api/games.go.bak /tmp/gofmt_test_probe.go
```

Expected: `golangci-lint run ./internal/api/` exits non-zero and reports a formatting issue before the restore.

- [ ] **Step 4: Commit the config change**

```bash
git add .golangci.yml
git commit -m "chore(ci): enable gofmt formatter in golangci-lint"
```

---

### Task 4: Open PR

**Files:**
- No files changed

- [ ] **Step 1: Push the branch**

```bash
git push -u origin chore/enforce-gofmt-ci
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create \
  --title "chore(ci): enforce Go formatting in CI (golangci-lint v2 formatter)" \
  --body "$(cat <<'EOF'
## Summary

- Adds `formatters: enable: [gofmt]` to `.golangci.yml` so `golangci-lint run` fails on any unformatted Go file
- One-time cleanup of 26 first-party files that had drifted from `gofmt` output (whitespace-only changes)

Closes #632.
EOF
)"
```
