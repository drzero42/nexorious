# Squash Migrations into a Single Baseline (v0.90.0) + Permanent Adopt-from-v0.17.1 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the 23 early-development migrations into one clean SQL baseline, and teach the migrator to *adopt* a fully-migrated v0.17.1 database in place (rewrite its `bun_migrations` rows to the single baseline row, then catch up any later migrations) or *refuse* anything older/partial — released as v0.90.0.

**Architecture:** A single `<ts>_baseline.up/.down.sql` (generated from the v0.17.1 end-state via `pg_dump`, not concatenation) replaces the 23 files. The state machine in `internal/migrate/migrator.go` gains an *adopt* and a *refuse* classification driven by the raw `bun_migrations` rows (read via Bun's `AppliedMigrations`), compared against a **frozen manifest of the 23 v0.17.1 migration timestamps**. Two new `AppState`s thread through the existing 4 entry points (web `/migrate`, `nexorious migrate`, `serve --migrate`, backup restore). The adopt/refuse logic is permanent.

**Tech Stack:** Go 1.26, Bun (`uptrace/bun` v1.2.18) + `uptrace/bun/migrate`, River (untouched), Echo v5, PostgreSQL 18, testcontainers-go.

## Global Constraints

- **The squash MUST be the next migration-touching change on `main`.** No other migration may merge between v0.17.1 and this baseline. (Verified: `main` and the `v0.17.1` tag ship byte-identical 23-migration sets today — `git diff v0.17.1 main -- internal/db/migrations/` is empty. The window is open.)
- **`bun_migrations.name` stores the 14-digit timestamp only** (e.g. `20260503000001`), NOT `20260503000001_initial`. The `Comment` half is `bun:"-"` and never persisted. The frozen manifest and all gate comparisons use **timestamps**. (Verified empirically against Postgres 18.)
- **All 23 v0.17.1 rows are `group_id = 1`.** A fresh single-migration `Migrate()` also produces `group_id = 1`. The adopt row MUST be written with `GroupID: 1` so an adopted DB's `bun_migrations` is byte-identical to a fresh-baseline DB's.
- **River is never touched.** River manages `river_migration`/`river_*` via `rivermigrate`; the baseline must exclude all River tables, and `baseline.down.sql` must NOT drop them.
- **Baseline migration name/timestamp:** `20260620000001_baseline` → stored name `20260620000001`. It sorts after the last v0.17.1 migration (`20260612000001`). Any future post-baseline migration must use a later timestamp.
- **Solo-user project:** no back-compat shims. Dev DBs that tracked `main` with experimental migrations are acceptable casualties (export/import or recreate).
- **Release:** land via a `Release-As: 0.90.0` trailer in the PR body (the empty-commit shortcut is dead — `main` is protected).
- **No AI attribution** in commits/PR/issues.
- **pg_dump must run inside the `postgres:18-alpine` container** (via `docker exec`) so the dump's version always matches the runtime (18) — never the host `pg_dump`.

## The frozen v0.17.1 manifest (23 timestamps)

```
20260503000001  20260531000001  20260531000002  20260601000001
20260601000002  20260601000003  20260601000004  20260602000001
20260602000002  20260602000003  20260604000001  20260604000002
20260604000003  20260604000004  20260605000001  20260605000002
20260605000003  20260608000001  20260608000002  20260608000003
20260608000004  20260609000001  20260612000001
```

## Classification (the single source of truth for both `determineState` and the under-lock re-check)

Given the set `S` of timestamps currently in `bun_migrations` (from `AppliedMigrations`):

| Condition | Decision | Resulting state |
|---|---|---|
| `20260620000001` ∈ S | **normal** | existing path: unapplied baseline/post-baseline + River → `NeedsMigration` else `Ready` |
| `20260620000001` ∉ S **and** S is empty | **normal (fresh)** | existing path → baseline is discovered-unapplied → `NeedsMigration` |
| `20260620000001` ∉ S **and** S == manifest (exact set, all 23, nothing else) | **adopt** | `NeedsAdopt` |
| `20260620000001` ∉ S **and** S non-empty **and** S != manifest | **refuse** | `MigrationRefused` |

---

## File Structure

**Generated / replaced:**
- `internal/db/migrations/20260620000001_baseline.up.sql` — single baseline (schema + 4 seed tables). Replaces the 23 `*.up.sql`.
- `internal/db/migrations/20260620000001_baseline.down.sql` — hand-written FK-ordered DROP of all Bun objects (NOT River). Replaces the 23 `*.down.sql`.
- (deleted) the 46 existing `internal/db/migrations/2026{0503..0612}*.up.sql` / `.down.sql` files.

**New:**
- `scripts/gen-baseline.sh` — reproducible generation + acceptance (schema byte-diff + seed-data diff). Committed so the user can re-run it.
- `internal/migrate/baseline.go` — frozen manifest, baseline-name constant, `classify()` decision function, `adopt()` action.
- `internal/migrate/baseline_test.go` — adopt/refuse/no-op/catch-up regression tests (the design's core promise).
- `internal/migrate/testdata/v0_17_1/*.sql` — the 23 frozen v0.17.1 `.up.sql` files, copied verbatim, used ONLY by the seed/schema acceptance test to build the "old DB" side after the real files are gone.

**Modified:**
- `internal/migrate/migrator.go` — injectable migrations seam; new states; classification wired into `determineState`, `PendingCount`, `Status`; adopt under lock in `RunMigrations`.
- `internal/migrate/handler.go` — `HandleMigrateUI` (adopt vs refuse messaging), `HandleStatus` (refuse message), `HandleRun` (block refuse).
- `ui/migrate/index.html` — render `needs_adopt` (adopt button) and `migration_refused` (refusal text, no button).
- `cmd/nexorious/migrate.go` — `runMigrate` (adopt → 0, refuse → non-zero), `runMigrateStatus`.
- `cmd/nexorious/serve.go` — `runStartupMigrations` (run for `NeedsAdopt`, error on `MigrationRefused`).
- `README.md` — replace the `[!WARNING]` block with beta / path-to-v1.0.0 + upgrade contract.
- `docs/admin-guide.md` — upgrade contract, adopt/refuse behaviour per entry point, export/import fallback.

No change needed in `internal/api/router.go`: **Gate 2 already** redirects every non-`Ready`/non-`DBUnavailable` state to `/migrate` (router.go:131), so `NeedsAdopt` and `MigrationRefused` route correctly with zero router edits. (Verified.)

---

## Task 1: Generate the baseline + acceptance script, replace the 23 files

**Files:**
- Create: `scripts/gen-baseline.sh`
- Create: `internal/db/migrations/20260620000001_baseline.up.sql` (script output)
- Create: `internal/db/migrations/20260620000001_baseline.down.sql` (hand-written)
- Create: `internal/migrate/testdata/v0_17_1/*.sql` (copy of the 23 current files)
- Delete: the 23 `internal/db/migrations/2026{0503..0612}_*.up.sql` and `.down.sql`

**Interfaces:**
- Produces: a migrations dir containing exactly one migration (`20260620000001_baseline`), and frozen v0.17.1 SQL fixtures under `internal/migrate/testdata/v0_17_1/`.

- [ ] **Step 1: Snapshot the 23 v0.17.1 files as test fixtures (before deleting them)**

```bash
cd /home/abo/workspace/home/nexorious
mkdir -p internal/migrate/testdata/v0_17_1
cp internal/db/migrations/2026*.up.sql internal/db/migrations/2026*.down.sql internal/migrate/testdata/v0_17_1/
ls internal/migrate/testdata/v0_17_1/*.up.sql | wc -l   # expect 23
```

- [ ] **Step 2: Write `scripts/gen-baseline.sh`**

The script: starts an ephemeral `postgres:18-alpine`, applies the 23 fixture `.up.sql` in name order, dumps schema (excluding `bun_migrations` + River) and the 4 seed tables, assembles `20260620000001_baseline.up.sql`, then verifies a baseline-only DB is byte-identical.

```bash
#!/usr/bin/env bash
# Regenerate and verify internal/db/migrations/20260620000001_baseline.up.sql
# from the frozen v0.17.1 migration set. Idempotent; safe to re-run.
#
# Requires: docker (postgres:18-alpine; pg_dump runs INSIDE the container so its
# version always matches the runtime — never the host pg_dump).
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
FIX="$REPO/internal/migrate/testdata/v0_17_1"
OUT="$REPO/internal/db/migrations/20260620000001_baseline.up.sql"
SEED_TABLES=(platforms storefronts platform_storefronts backup_config)

start_pg() {  # $1 = db name → echoes container id
  local db="$1"
  local cid
  cid=$(docker run -d --rm -e POSTGRES_PASSWORD=t -e POSTGRES_USER=t -e POSTGRES_DB="$db" postgres:18-alpine)
  for _ in $(seq 1 30); do docker exec "$cid" pg_isready -U t >/dev/null 2>&1 && break; sleep 1; done
  echo "$cid"
}

apply_v0171() {  # $1 = cid, $2 = db
  for f in "$FIX"/*.up.sql; do
    docker exec -i "$1" psql -v ON_ERROR_STOP=1 -U t -d "$2" < "$f" >/dev/null
  done
}

# dump_schema strips owner/privilege/SET/COMMENT/extension noise, drops the
# bun_migrations + river_* tables, and normalizes for a stable byte comparison.
dump_schema() {  # $1 = cid, $2 = db
  docker exec "$1" pg_dump -U t -d "$2" \
    --schema-only --no-owner --no-privileges --no-comments \
    --exclude-table=bun_migrations --exclude-table='river_*' \
  | grep -vE '^(SET|SELECT pg_catalog\.set_config|--|$)' \
  | grep -vE '^(CREATE EXTENSION|COMMENT ON EXTENSION)'
}

dump_seed() {  # $1 = cid, $2 = db
  local args=()
  for t in "${SEED_TABLES[@]}"; do args+=(-t "$t"); done
  docker exec "$1" pg_dump -U t -d "$2" --data-only --no-owner --no-privileges \
    --inserts --rows-per-insert=1 "${args[@]}" \
  | grep -vE '^(SET|SELECT pg_catalog\.set_config|--|$)'
}

echo "==> Building v0.17.1 end-state DB"
CID_A=$(start_pg src)
trap 'docker stop "$CID_A" >/dev/null 2>&1 || true' EXIT
apply_v0171 "$CID_A" src

echo "==> Assembling $OUT"
{
  echo "-- Baseline schema (squashed from the v0.17.1 migration set; see issue #1117)."
  echo "-- Generated by scripts/gen-baseline.sh — do not hand-edit; regenerate instead."
  dump_schema "$CID_A" src
  echo
  echo "-- Seed/reference data (platforms, storefronts, platform_storefronts, backup_config)."
  dump_seed "$CID_A" src
} > "$OUT"

echo "==> Acceptance: baseline-only DB must match v0.17.1 byte-for-byte (schema)"
CID_B=$(start_pg dst)
trap 'docker stop "$CID_A" "$CID_B" >/dev/null 2>&1 || true' EXIT
docker exec -i "$CID_B" psql -v ON_ERROR_STOP=1 -U t -d dst < "$OUT" >/dev/null

if diff <(dump_schema "$CID_A" src) <(dump_schema "$CID_B" dst); then
  echo "    SCHEMA OK (identical)"
else
  echo "    SCHEMA MISMATCH — see diff above"; exit 1
fi

echo "==> Acceptance: seed data must match for all 4 tables"
for t in "${SEED_TABLES[@]}"; do
  ca=$(docker exec "$CID_A" psql -tA -U t -d src -c "SELECT count(*) FROM $t")
  cb=$(docker exec "$CID_B" psql -tA -U t -d dst -c "SELECT count(*) FROM $t")
  if [ "$ca" != "$cb" ]; then echo "    $t row count differs: $ca vs $cb"; exit 1; fi
  echo "    $t: $ca rows OK"
done
if diff <(dump_seed "$CID_A" src) <(dump_seed "$CID_B" dst); then
  echo "    SEED DATA OK (identical)"
else
  echo "    SEED DATA MISMATCH — see diff above"; exit 1
fi

echo "==> All acceptance checks passed."
```

```bash
chmod +x scripts/gen-baseline.sh
```

- [ ] **Step 3: Delete the 23 live migration files (keep only the baseline target)**

```bash
cd /home/abo/workspace/home/nexorious
git rm internal/db/migrations/2026{0503,0531,0601,0602,0604,0605,0608,0609,0612}*.up.sql \
       internal/db/migrations/2026{0503,0531,0601,0602,0604,0605,0608,0609,0612}*.down.sql
ls internal/db/migrations/*.sql   # expect: only migrations.go's siblings; no 2026*_*.sql yet
```

- [ ] **Step 4: Run the generator to produce `baseline.up.sql` and prove acceptance**

```bash
cd /home/abo/workspace/home/nexorious
./scripts/gen-baseline.sh
```
Expected tail:
```
    SCHEMA OK (identical)
    ...
    SEED DATA OK (identical)
==> All acceptance checks passed.
```
If the schema diff is non-empty, inspect it: the usual culprits are `pg_dump` noise lines not covered by the `grep` filters (add them to `dump_schema`) — NOT a real schema difference.

- [ ] **Step 5: Hand-write `internal/db/migrations/20260620000001_baseline.down.sql`**

Reverse-of-creation `DROP`. Enumerate every Bun-managed table from `baseline.up.sql` (`grep -E '^CREATE TABLE' internal/db/migrations/20260620000001_baseline.up.sql`), drop them with `CASCADE` to avoid FK ordering pain, and **exclude** `bun_migrations` (Bun owns it) and every `river_*` table. Template:

```sql
-- Baseline rollback: drop all Bun-managed objects (NOT River, NOT bun_migrations).
DROP TABLE IF EXISTS pools_user_games CASCADE;
DROP TABLE IF EXISTS pools CASCADE;
DROP TABLE IF EXISTS user_settings CASCADE;
DROP TABLE IF EXISTS notification_subscriptions CASCADE;
DROP TABLE IF EXISTS notification_channels CASCADE;
DROP TABLE IF EXISTS changes CASCADE;
DROP TABLE IF EXISTS events CASCADE;
DROP TABLE IF EXISTS external_game_platforms CASCADE;
DROP TABLE IF EXISTS external_games CASCADE;
DROP TABLE IF EXISTS user_game_platforms CASCADE;
DROP TABLE IF EXISTS user_game_tags CASCADE;
DROP TABLE IF EXISTS user_games CASCADE;
DROP TABLE IF EXISTS tags CASCADE;
DROP TABLE IF EXISTS games CASCADE;
DROP TABLE IF EXISTS platform_storefronts CASCADE;
DROP TABLE IF EXISTS storefronts CASCADE;
DROP TABLE IF EXISTS platforms CASCADE;
DROP TABLE IF EXISTS backup_config CASCADE;
DROP TABLE IF EXISTS users CASCADE;
-- NOTE: the exact table list MUST be derived from baseline.up.sql's CREATE TABLE
-- statements at implementation time; the list above is illustrative. Verify with:
--   grep -E '^CREATE TABLE' internal/db/migrations/20260620000001_baseline.up.sql
```
After writing it, verify it actually reverses the baseline:
```bash
# Apply baseline, then down, then baseline again — must succeed clean.
CID=$(docker run -d --rm -e POSTGRES_PASSWORD=t -e POSTGRES_USER=t -e POSTGRES_DB=d postgres:18-alpine)
sleep 5
docker exec -i "$CID" psql -v ON_ERROR_STOP=1 -U t -d d < internal/db/migrations/20260620000001_baseline.up.sql >/dev/null
docker exec -i "$CID" psql -v ON_ERROR_STOP=1 -U t -d d < internal/db/migrations/20260620000001_baseline.down.sql >/dev/null
docker exec -i "$CID" psql -v ON_ERROR_STOP=1 -U t -d d < internal/db/migrations/20260620000001_baseline.up.sql >/dev/null
echo "round-trip OK"; docker stop "$CID" >/dev/null
```
Expected: `round-trip OK`.

- [ ] **Step 6: Confirm the full Go suite still builds and the data-layer tests pass against the baseline**

The data-layer test packages apply `migrations.Migrations` directly (e.g. `internal/api/main_test.go:73`), so they now exercise the baseline. They reference no migration filenames/counts (verified), so nothing else needs editing.
```bash
go build ./...
go test ./internal/api/... -run TestGamesList -v   # spot-check a DB-backed package boots on the baseline
```
Expected: build clean; test PASS.

- [ ] **Step 7: Commit**

```bash
git add scripts/gen-baseline.sh internal/db/migrations/20260620000001_baseline.up.sql \
        internal/db/migrations/20260620000001_baseline.down.sql \
        internal/migrate/testdata/v0_17_1/
git add -A internal/db/migrations/
git commit -m "feat(db): squash v0.17.1 migrations into a single baseline"
```

---

## Task 2: Frozen manifest, classification, injectable migrations seam, new states

**Files:**
- Create: `internal/migrate/baseline.go`
- Modify: `internal/migrate/migrator.go` (struct field + constructors + state enum)
- Test: `internal/migrate/baseline_test.go`

**Interfaces:**
- Produces:
  - `const baselineTimestamp = "20260620000001"`
  - `var v0171Manifest = map[string]struct{}{…23 timestamps…}`
  - `type adoptDecision int` with `decisionNormal`, `decisionAdopt`, `decisionRefuse`
  - `func classify(applied bunmigrate.MigrationSlice) adoptDecision`
  - `func NewMigratorWithMigrations(db *bun.DB, set *bunmigrate.Migrations) *Migrator`
  - `AppStateNeedsAdopt`, `AppStateMigrationRefused` (with `String()` `"needs_adopt"` / `"migration_refused"`)
- Consumes (Bun, verified v1.2.18): `(*bunmigrate.Migrator).AppliedMigrations(ctx) (MigrationSlice, error)` returns rows whose `.Name` is the 14-digit timestamp; `MigrationSlice` is `[]Migration`.

- [ ] **Step 1: Write the failing test for `classify`**

```go
// internal/migrate/baseline_test.go
package migrate

import (
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"
)

func slice(names ...string) bunmigrate.MigrationSlice {
	ms := make(bunmigrate.MigrationSlice, len(names))
	for i, n := range names {
		ms[i] = bunmigrate.Migration{Name: n}
	}
	return ms
}

func manifestNames() []string {
	out := make([]string, 0, len(v0171Manifest))
	for n := range v0171Manifest {
		out = append(out, n)
	}
	return out
}

func TestClassify(t *testing.T) {
	full := manifestNames()
	cases := []struct {
		name string
		in   bunmigrate.MigrationSlice
		want adoptDecision
	}{
		{"empty is normal/fresh", slice(), decisionNormal},
		{"baseline present is normal", slice(baselineTimestamp), decisionNormal},
		{"baseline plus post is normal", slice(baselineTimestamp, "20260621000001"), decisionNormal},
		{"exact manifest is adopt", slice(full...), decisionAdopt},
		{"manifest minus one is refuse", slice(full[1:]...), decisionRefuse},
		{"manifest plus stranger is refuse", slice(append(append([]string{}, full...), "29990101000001")...), decisionRefuse},
		{"unknown single row is refuse", slice("19990101000001"), decisionRefuse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.in); got != tc.want {
				t.Fatalf("classify(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
```
Note: this is `package migrate` (internal) — `classify`, `v0171Manifest`, etc. are unexported. The DB-backed tests in Task 4 live in the existing `package migrate_test`; these pure-logic tests are white-box.

- [ ] **Step 2: Run it — expect a compile failure (symbols undefined)**

```bash
go test ./internal/migrate/ -run TestClassify -v
```
Expected: FAIL (build error: `undefined: classify` / `v0171Manifest` / `baselineTimestamp` / `decisionNormal`…).

- [ ] **Step 3: Create `internal/migrate/baseline.go`**

```go
package migrate

import (
	bunmigrate "github.com/uptrace/bun/migrate"
)

// baselineTimestamp is the bun_migrations.name (14-digit timestamp) of the
// squashed baseline migration file 20260620000001_baseline.up.sql. Bun stores
// only the timestamp in the name column — the "_baseline" suffix is the
// (unpersisted) comment. See issue #1117.
const baselineTimestamp = "20260620000001"

// v0171Manifest is the frozen, permanent set of bun_migrations.name timestamps
// written by the 23 migrations shipped through v0.17.1 — the last release on the
// incremental chain. A database whose bun_migrations contains EXACTLY this set
// (and not the baseline) is byte-for-byte the baseline schema and is safe to
// adopt. This manifest is frozen forever; never edit it (a future squash would
// add a second, separate manifest).
var v0171Manifest = map[string]struct{}{
	"20260503000001": {}, "20260531000001": {}, "20260531000002": {}, "20260601000001": {},
	"20260601000002": {}, "20260601000003": {}, "20260601000004": {}, "20260602000001": {},
	"20260602000002": {}, "20260602000003": {}, "20260604000001": {}, "20260604000002": {},
	"20260604000003": {}, "20260604000004": {}, "20260605000001": {}, "20260605000002": {},
	"20260605000003": {}, "20260608000001": {}, "20260608000002": {}, "20260608000003": {},
	"20260608000004": {}, "20260609000001": {}, "20260612000001": {},
}

type adoptDecision int

const (
	// decisionNormal: run Bun migration as usual (fresh install → baseline;
	// baseline already present → catch up any post-baseline migrations).
	decisionNormal adoptDecision = iota
	// decisionAdopt: bun_migrations is exactly the v0.17.1 manifest with no
	// baseline row — rewrite it to the single baseline row, then catch up.
	decisionAdopt
	// decisionRefuse: bun_migrations is non-empty, has no baseline row, and is
	// not the exact manifest (older than v0.17.1, partial, or unknown).
	decisionRefuse
)

// classify decides how to treat a database from its raw bun_migrations rows.
// applied is the result of (*bunmigrate.Migrator).AppliedMigrations — each
// .Name is the 14-digit timestamp.
func classify(applied bunmigrate.MigrationSlice) adoptDecision {
	have := make(map[string]struct{}, len(applied))
	for _, m := range applied {
		have[m.Name] = struct{}{}
	}
	if _, ok := have[baselineTimestamp]; ok {
		return decisionNormal
	}
	if len(have) == 0 {
		return decisionNormal // fresh DB → baseline is discovered-unapplied
	}
	if len(have) == len(v0171Manifest) {
		for n := range have {
			if _, ok := v0171Manifest[n]; !ok {
				return decisionRefuse
			}
		}
		return decisionAdopt
	}
	return decisionRefuse
}
```

- [ ] **Step 4: Run the test — expect PASS**

```bash
go test ./internal/migrate/ -run TestClassify -v
```
Expected: PASS (all subtests).

- [ ] **Step 5: Add the two new `AppState`s and the injectable migrations seam to `migrator.go`**

In `internal/migrate/migrator.go`, extend the enum (insert before `AppStateMigrationFailed` so existing iota values that are persisted nowhere stay irrelevant; states are only compared by value at runtime):

```go
const (
	AppStateDBUnavailable AppState = iota
	AppStateNeedsMigration
	AppStateMigrating
	AppStateReady
	AppStateMigrationFailed
	AppStateNeedsAdopt
	AppStateMigrationRefused
)
```
Extend `String()`:
```go
	case AppStateNeedsAdopt:
		return "needs_adopt"
	case AppStateMigrationRefused:
		return "migration_refused"
```
Add a field to the `Migrator` struct:
```go
	migrationSet *bunmigrate.Migrations
```
Replace the constructor and add the seam:
```go
func NewMigrator(db *bun.DB) *Migrator {
	return NewMigratorWithMigrations(db, migrations.Migrations)
}

// NewMigratorWithMigrations is NewMigrator with an injectable migration set,
// so tests can register a synthetic post-baseline migration and exercise the
// adopt-then-catch-up path (which has no real second migration yet).
func NewMigratorWithMigrations(db *bun.DB, set *bunmigrate.Migrations) *Migrator {
	return &Migrator{db: db, migrationSet: set}
}
```
Replace BOTH hardcoded `bunmigrate.NewMigrator(mg.db, migrations.Migrations)` calls (migrator.go:69 and :174) with `bunmigrate.NewMigrator(mg.db, mg.migrationSet)`.

- [ ] **Step 6: Build to confirm the seam compiles (no behavior change yet)**

```bash
go build ./... && go test ./internal/migrate/ -run TestClassify -v
```
Expected: build clean; TestClassify PASS. (Existing migrate tests may still pass too — run `go test ./internal/migrate/` to confirm nothing regressed from the constructor refactor.)

- [ ] **Step 7: Commit**

```bash
git add internal/migrate/baseline.go internal/migrate/baseline_test.go internal/migrate/migrator.go
git commit -m "feat(migrate): add v0.17.1 manifest, classification, and injectable migration seam"
```

---

## Task 3: Wire classification into `determineState`/`PendingCount`/`Status` and perform adopt under lock in `RunMigrations`

**Files:**
- Modify: `internal/migrate/migrator.go`
- Modify: `internal/migrate/baseline.go` (add the `adopt` action)

**Interfaces:**
- Consumes: `classify`, `baselineTimestamp`, `AppStateNeedsAdopt`, `AppStateMigrationRefused` (Task 2); Bun `MarkApplied(ctx, *Migration)`, `AppliedMigrations(ctx)`, `Lock`/`Unlock` (verified).
- Produces: `func (mg *Migrator) adopt(ctx context.Context) error` — deletes the 23 manifest rows and writes the single baseline row (`group_id=1`).

- [ ] **Step 1: Add the `adopt` action to `baseline.go`**

`MarkApplied` issues a plain INSERT of the `Migration` model (verified) — `Name`, `GroupID`, `MigratedAt` only. Delete the 23 manifest rows by name, then write the baseline row.

```go
// adopt rewrites a fully-migrated v0.17.1 bun_migrations (the 23 manifest rows)
// to the single baseline row, in one transaction. Callers MUST hold the Bun
// advisory lock and MUST have just re-confirmed classify()==decisionAdopt under
// that lock. The baseline row uses GroupID 1 to match a fresh-install Migrate().
func (mg *Migrator) adopt(ctx context.Context) error {
	names := make([]string, 0, len(v0171Manifest))
	for n := range v0171Manifest {
		names = append(names, n)
	}
	return mg.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().
			Model((*bunmigrate.Migration)(nil)).
			ModelTableExpr("bun_migrations").
			Where("name IN (?)", bun.In(names)).
			Exec(ctx); err != nil {
			return fmt.Errorf("adopt: delete v0.17.1 rows: %w", err)
		}
		row := &bunmigrate.Migration{Name: baselineTimestamp, GroupID: 1}
		if _, err := tx.NewInsert().
			Model(row).
			ModelTableExpr("bun_migrations").
			Exec(ctx); err != nil {
			return fmt.Errorf("adopt: insert baseline row: %w", err)
		}
		return nil
	})
}
```
Add imports to `baseline.go`: `"context"`, `"fmt"`, `"github.com/uptrace/bun"`.

(Rationale for a direct INSERT instead of `bunMig.MarkApplied`: it lets us do delete+insert atomically in one `RunInTx`, and `MarkApplied` would otherwise need `MigratedAt` defaulting — the column default `current_timestamp` fills it on a bare insert just the same. Using the same `bunmigrate.Migration` model keeps the column mapping identical.)

- [ ] **Step 2: Rewrite `determineState` to classify first**

Replace the body of `determineState` (migrator.go:67-94). After ensuring `bunMig` is initialized, read raw applied rows and branch:

```go
func (mg *Migrator) determineState() error {
	if mg.bunMig == nil {
		mg.bunMig = bunmigrate.NewMigrator(mg.db, mg.migrationSet)
		if err := mg.bunMig.Init(context.Background()); err != nil {
			return fmt.Errorf("determine state: init: %w", err)
		}
	}
	applied, err := mg.bunMig.AppliedMigrations(context.Background())
	if err != nil {
		return fmt.Errorf("determine state: applied: %w", err)
	}
	switch classify(applied) {
	case decisionAdopt:
		mg.state.Store(int32(AppStateNeedsAdopt))
		return nil
	case decisionRefuse:
		mg.state.Store(int32(AppStateMigrationRefused))
		return nil
	}
	// decisionNormal: existing logic.
	ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
	if err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	if len(ms.Unapplied()) > 0 {
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}
	riverNeeds, err := mg.riverNeedsMigration(context.Background())
	if err != nil {
		return fmt.Errorf("determine state: river: %w", err)
	}
	if riverNeeds {
		mg.state.Store(int32(AppStateNeedsMigration))
		return nil
	}
	mg.state.Store(int32(AppStateReady))
	return nil
}
```

- [ ] **Step 3: Make `PendingCount` and `Status` adopt/refuse-aware**

`PendingCount` (used by the `/migrate` page) — count the adopt as one pending action; report 0 for refuse (nothing to run). Insert near the top of `PendingCount`, after `bunMig` is ensured:
```go
	applied, err := mg.bunMig.AppliedMigrations(context.Background())
	if err != nil {
		return 0, fmt.Errorf("pending count: applied: %w", err)
	}
	switch classify(applied) {
	case decisionAdopt:
		return 1, nil
	case decisionRefuse:
		return 0, nil
	}
	// decisionNormal: fall through to the existing Unapplied()+river count.
```
`Status` (used by `nexorious migrate status`) — same branch right after `bunMig` is ensured, returning a synthetic `current`:
```go
	applied, err := mg.bunMig.AppliedMigrations(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("status: applied: %w", err)
	}
	switch classify(applied) {
	case decisionAdopt:
		return 1, "v0.17.1 (adopt pending)", nil
	case decisionRefuse:
		return 0, "unknown (refused)", nil
	}
	// decisionNormal: fall through to existing Applied()/Unapplied() logic.
```

- [ ] **Step 4: Perform adopt (and refuse-guard) under the lock in `RunMigrations`**

In `RunMigrations`, immediately AFTER `defer mg.bunMig.Unlock(ctx)` (migrator.go:223) and BEFORE `mg.bunMig.Migrate(ctx)` (line 225), re-read state under the lock and act:

```go
	// Re-classify UNDER the lock: a concurrently-booting instance may have
	// already adopted. Never trust the pre-lock determineState for the
	// destructive adopt.
	applied, err := mg.bunMig.AppliedMigrations(ctx)
	if err != nil {
		wrapped := fmt.Errorf("migrate: read applied: %w", err)
		slog.ErrorContext(ctx, "migrate: read applied failed", logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", wrapped))
		mg.TransitionToFailed(wrapped)
		close(ch)
		return wrapped
	}
	switch classify(applied) {
	case decisionRefuse:
		wrapped := fmt.Errorf("migrate: refusing to migrate a database that is not a clean v0.17.1 or baseline install")
		mg.sendLog(ch, fmt.Sprintf("migration refused: %v\n", wrapped))
		mg.state.Store(int32(AppStateMigrationRefused))
		mg.lastError.Store(wrapped.Error())
		close(ch)
		return wrapped
	case decisionAdopt:
		mg.sendLog(ch, "Adopting existing v0.17.1 schema (rewriting migration history)…\n")
		if err := mg.adopt(ctx); err != nil {
			wrapped := fmt.Errorf("migrate: adopt: %w", err)
			slog.ErrorContext(ctx, "migrate: adopt failed", logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))
			mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", wrapped))
			mg.TransitionToFailed(wrapped)
			close(ch)
			return wrapped
		}
		mg.sendLog(ch, "Adopt complete; checking for newer migrations…\n")
	}
	// decisionNormal and post-adopt both fall through to Migrate(), which now
	// sees the baseline row applied and runs only post-baseline migrations.
```
Leave the existing `group, err := mg.bunMig.Migrate(ctx)` block and everything after unchanged.

Note: the refuse branch sets state directly (not `TransitionToFailed`) so the message and state read `migration_refused`, distinct from a transient `migration_failed`. It still returns a non-nil error so CLI callers exit non-zero.

- [ ] **Step 5: Build**

```bash
go build ./...
```
Expected: clean. (DB-backed behavior is tested in Task 4.)

- [ ] **Step 6: Commit**

```bash
git add internal/migrate/baseline.go internal/migrate/migrator.go
git commit -m "feat(migrate): adopt v0.17.1 under lock and classify state on startup"
```

---

## Task 4: Regression tests — adopt, refuse, no-op, and adopt-then-catch-up

**Files:**
- Test: `internal/migrate/baseline_test.go` is white-box (`package migrate`); these DB-backed tests go in a NEW file `internal/migrate/adopt_db_test.go` in `package migrate_test` (matches the existing `migrator_test.go` external style and the shared `testDSN`/`resetPublicSchema` harness in `main_test.go`).

**Interfaces:**
- Consumes: `migrate.NewMigrator`, `migrate.NewMigratorWithMigrations`, `migrate.AppState*`, `(*Migrator).DetermineState/State/RunMigrations`; the package test harness `testDSN` + `resetPublicSchema(t)`; `migrations.FS` (the embedded baseline) + `fstest.MapFS` for a synthetic post-baseline migration.

- [ ] **Step 1: Write a helper that fabricates a "fully-migrated v0.17.1" DB**

Because the 23 files are gone, build the v0.17.1 state by applying the baseline (identical schema, per Task 1 acceptance) and then rewriting `bun_migrations` to the 23 manifest rows.

```go
// internal/migrate/adopt_db_test.go
package migrate_test

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
	"github.com/drzero42/nexorious/internal/migrate"
)

// the 23 frozen v0.17.1 timestamps (must mirror v0171Manifest exactly).
var v0171Timestamps = []string{
	"20260503000001", "20260531000001", "20260531000002", "20260601000001",
	"20260601000002", "20260601000003", "20260601000004", "20260602000001",
	"20260602000002", "20260602000003", "20260604000001", "20260604000002",
	"20260604000003", "20260604000004", "20260605000001", "20260605000002",
	"20260605000003", "20260608000001", "20260608000002", "20260608000003",
	"20260608000004", "20260609000001", "20260612000001",
}

func makeBunDB(t *testing.T) *bun.DB {
	t.Helper()
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(testDSN))), pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedV0171 builds the v0.17.1 end-state: baseline schema, then bun_migrations
// rewritten to exactly the 23 manifest rows (no baseline row).
func seedV0171(t *testing.T, db *bun.DB) {
	t.Helper()
	ctx := context.Background()
	m := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := m.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil { // applies baseline
		t.Fatalf("apply baseline: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM bun_migrations"); err != nil {
		t.Fatalf("clear bun_migrations: %v", err)
	}
	for _, ts := range v0171Timestamps {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO bun_migrations (name, group_id, migrated_at) VALUES (?, 1, now())", ts); err != nil {
			t.Fatalf("seed row %s: %v", ts, err)
		}
	}
}
```

- [ ] **Step 2: Write the adopt + no-op + refuse tests; run them (expect FAIL → PASS)**

```go
func names(t *testing.T, db *bun.DB) []string {
	t.Helper()
	var out []string
	if err := db.NewSelect().ColumnExpr("name").
		ModelTableExpr("bun_migrations").OrderExpr("name").
		Scan(context.Background(), &out); err != nil {
		t.Fatalf("read names: %v", err)
	}
	return out
}

func TestDetermineState_AdoptPending(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	seedV0171(t, db)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsAdopt {
		t.Fatalf("state = %v, want NeedsAdopt", m.State())
	}
}

func TestRunMigrations_AdoptRewritesHistory(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	seedV0171(t, db)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	got := names(t, db)
	if len(got) != 1 || got[0] != "20260620000001" {
		t.Fatalf("bun_migrations = %v, want exactly [20260620000001]", got)
	}
}

func TestDetermineState_BaselinePresentIsReady(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	// Fresh baseline install (baseline row present, no post-baseline migration).
	m0 := migrate.NewMigrator(db)
	if err := m0.DetermineState(); err != nil {
		t.Fatalf("DetermineState(fresh): %v", err)
	}
	if err := m0.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations(fresh): %v", err)
	}
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateReady {
		t.Fatalf("state = %v, want Ready", m.State())
	}
}

func TestDetermineState_PartialIsRefused(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	seedV0171(t, db)
	// Drop one manifest row → no longer the exact set.
	if _, err := db.ExecContext(context.Background(),
		"DELETE FROM bun_migrations WHERE name = ?", "20260612000001"); err != nil {
		t.Fatalf("delete row: %v", err)
	}
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateMigrationRefused {
		t.Fatalf("state = %v, want MigrationRefused", m.State())
	}
}

func TestRunMigrations_RefusedReturnsError(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	seedV0171(t, db)
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO bun_migrations (name, group_id, migrated_at) VALUES (?, 1, now())", "29990101000001"); err != nil {
		t.Fatalf("insert stranger: %v", err)
	}
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err == nil {
		t.Fatal("RunMigrations: want error on refuse, got nil")
	}
	if m.State() != migrate.AppStateMigrationRefused {
		t.Fatalf("state = %v, want MigrationRefused", m.State())
	}
}
```

```bash
go test ./internal/migrate/ -run 'TestDetermineState|TestRunMigrations_Adopt|TestRunMigrations_Refused' -v
```
Expected: PASS.

- [ ] **Step 3: Write the adopt-then-catch-up test (the design's core promise)**

Inject a synthetic post-baseline migration via the seam and assert it applies after adopt.

```go
func TestRunMigrations_AdoptThenCatchUp(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	seedV0171(t, db)

	// Build a migration set = baseline (from FS) + a synthetic post-baseline one.
	set := bunmigrate.NewMigrations()
	if err := set.Discover(migrations.FS); err != nil {
		t.Fatalf("discover baseline: %v", err)
	}
	synth := fstest.MapFS{
		"20260621000001_test_addcol.up.sql": &fstest.MapFile{
			Data: []byte("ALTER TABLE platforms ADD COLUMN test_adopt_marker text;"),
		},
		"20260621000001_test_addcol.down.sql": &fstest.MapFile{
			Data: []byte("ALTER TABLE platforms DROP COLUMN test_adopt_marker;"),
		},
	}
	if err := set.Discover(synth); err != nil {
		t.Fatalf("discover synth: %v", err)
	}

	m := migrate.NewMigratorWithMigrations(db, set)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsAdopt {
		t.Fatalf("pre-run state = %v, want NeedsAdopt", m.State())
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// bun_migrations is now exactly [baseline, synthetic].
	got := names(t, db)
	want := []string{"20260620000001", "20260621000001"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("bun_migrations = %v, want %v", got, want)
	}
	// And the synthetic migration's column actually exists.
	var n int
	if err := db.NewSelect().ColumnExpr("count(*)").
		TableExpr("information_schema.columns").
		Where("table_name = 'platforms' AND column_name = 'test_adopt_marker'").
		Scan(context.Background(), &n); err != nil {
		t.Fatalf("check column: %v", err)
	}
	if n != 1 {
		t.Fatalf("synthetic column present = %d, want 1", n)
	}
}
```

```bash
go test ./internal/migrate/ -run TestRunMigrations_AdoptThenCatchUp -v
```
Expected: PASS.

- [ ] **Step 4: Add a seed-presence smoke test (guards the seed dump permanently)**

```go
func TestBaseline_SeedDataPresent(t *testing.T) {
	resetPublicSchema(t)
	db := makeBunDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	for _, tbl := range []string{"platforms", "storefronts", "platform_storefronts", "backup_config"} {
		var n int
		if err := db.NewSelect().ColumnExpr("count(*)").TableExpr(tbl).
			Scan(context.Background(), &n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if n == 0 {
			t.Fatalf("seed table %s is empty after baseline", tbl)
		}
	}
}
```

```bash
go test ./internal/migrate/ -v
```
Expected: all PASS (including the pre-existing migrate tests).

- [ ] **Step 5: Commit**

```bash
git add internal/migrate/adopt_db_test.go
git commit -m "test(migrate): cover adopt, refuse, no-op, and adopt-then-catch-up"
```

---

## Task 5: CLI threading — `nexorious migrate` / `migrate status`

**Files:**
- Modify: `cmd/nexorious/migrate.go`

**Interfaces:**
- Consumes: `migrate.AppStateNeedsAdopt`, `migrate.AppStateMigrationRefused`, `(*Migrator).State/RunMigrations/Status`.

- [ ] **Step 1: Make `runMigrate` adopt on the happy path and exit non-zero on refuse**

In `runMigrate` (migrate.go:145-157), replace the `if migrator.State() == migrate.AppStateReady { … }` early-return block with:

```go
	switch migrator.State() {
	case migrate.AppStateReady:
		slog.Info("migrate: no pending migrations")
		fmt.Println("No pending migrations.")
		return nil
	case migrate.AppStateMigrationRefused:
		return fmt.Errorf("migrate: refused — this database is not a clean v0.17.1 or baseline install; " +
			"upgrade to v0.17.1 first (let it migrate fully) then to v0.90.0+, or export → fresh install → import")
	}
	// AppStateNeedsMigration and AppStateNeedsAdopt both proceed to RunMigrations.
```
(`RunMigrations` performs the adopt internally when `NeedsAdopt`; the existing call below it is unchanged. On success it exits 0 — init-containers succeed.)

- [ ] **Step 2: `runMigrateStatus` already prints `state=`; no logic change needed, verify the message**

`Status` (Task 3) now returns `current="v0.17.1 (adopt pending)"` / `"unknown (refused)"` and the printf at migrate.go:188 already emits `current_version=…\npending=…\nstate=…`. No edit required. (Confirm by reading the function.)

- [ ] **Step 3: Build + a focused CLI test**

Add to `cmd/nexorious/` a test that a refused DB makes `runMigrate` return an error. If the package already has a DB-backed harness (check `cmd/nexorious/*_test.go`), reuse it; otherwise assert via a small table test calling the migrator directly (the CLI wrapper is thin). Minimum:
```bash
go build ./...
go test ./cmd/nexorious/ -run TestMigrate -v
```
Expected: build clean; tests PASS. (If no suitable harness exists, the migrator-level refuse test in Task 4 already covers the non-zero contract; note that in the commit message rather than forcing a brittle CLI harness.)

- [ ] **Step 4: Commit**

```bash
git add cmd/nexorious/migrate.go
git commit -m "feat(cli): adopt on migrate, exit non-zero on refused database"
```

---

## Task 6: Server startup threading — `serve --migrate`

**Files:**
- Modify: `cmd/nexorious/serve.go`

**Interfaces:**
- Consumes: `migrate.AppStateNeedsAdopt`, `migrate.AppStateMigrationRefused`.

- [ ] **Step 1: Make `runStartupMigrations` run for adopt and fail for refuse**

In `runStartupMigrations` (serve.go:539-556), replace the single `if migrator.State() != migrate.AppStateNeedsMigration { … return nil }` guard with:

```go
	switch migrator.State() {
	case migrate.AppStateReady:
		slog.Info("serve --migrate: no pending migrations")
		return nil
	case migrate.AppStateMigrationRefused:
		return fmt.Errorf("serve --migrate: refused — database is not a clean v0.17.1 or baseline install; " +
			"upgrade to v0.17.1 first then v0.90.0+, or export → fresh install → import")
	}
	// AppStateNeedsMigration and AppStateNeedsAdopt proceed.
```
Leave the `SetLogWriter` + `RunMigrations` tail unchanged.

- [ ] **Step 2: Confirm the non---migrate startup path tolerates refuse (serve the page, don't crash)**

Read serve.go:174-186 (the `else` branch). `initAppState` only calls `DetermineState` (which now may set `MigrationRefused`/`NeedsAdopt`) and `InitNeedsSetup` only when `Ready`. So a refused DB leaves the server up, and Gate 2 serves `/migrate`. No edit needed — confirm by reading. Add a one-line code comment at the `initAppState` definition noting that `NeedsAdopt`/`MigrationRefused` are served via the `/migrate` page.

- [ ] **Step 3: Build**

```bash
go build ./...
```
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "feat(serve): run adopt under --migrate, refuse to start on refused database"
```

---

## Task 7: HTTP handler + `/migrate` page — adopt vs refuse messaging

**Files:**
- Modify: `internal/migrate/handler.go`
- Modify: `ui/migrate/index.html`
- Test: `internal/migrate/handler_test.go` (extend)

**Interfaces:**
- Consumes: `(*Migrator).State`, `AppStateNeedsAdopt`, `AppStateMigrationRefused`, `LastError`.

- [ ] **Step 1: `HandleStatus` — surface refuse with a message**

In `handler.go` `HandleStatus` (handler.go:49-74), add a branch BEFORE the generic path so the page can render refusal text:
```go
	if state == AppStateMigrationRefused {
		return c.JSON(http.StatusOK, map[string]any{
			"pending_count": 0,
			"state":         state.String(),
			"error":         h.migrator.LastError(),
		})
	}
```
The existing `AppStateMigrationFailed` branch and generic path remain. `needs_adopt` flows through the generic path (`pending_count` = 1 from Task 3), which is correct — the page just shows the run button.

- [ ] **Step 2: `HandleRun` — block refuse**

In `handler.go` `HandleRun` (handler.go:76-84), add a case to the switch:
```go
	case AppStateMigrationRefused:
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "this database cannot be migrated automatically; see the page for upgrade instructions",
		})
```
`AppStateNeedsAdopt` falls through to the allowed path (alongside `NeedsMigration`/`Failed`) and triggers `RunMigrations`, which adopts.

- [ ] **Step 3: `HandleMigrateUI` — pass state + adopt/refuse copy to the template**

Replace the `data` struct in `HandleMigrateUI` (handler.go:39-47) so the template can branch:
```go
	state := h.migrator.State()
	data := struct {
		PendingCount int
		State        string
		Refused      bool
		Adopt        bool
		Error        string
	}{
		PendingCount: pending,
		State:        state.String(),
		Refused:      state == AppStateMigrationRefused,
		Adopt:        state == AppStateNeedsAdopt,
		Error:        h.migrator.LastError(),
	}
```

- [ ] **Step 4: Update `ui/migrate/index.html`**

Branch the card body on `.Refused` / `.Adopt`. Replace the `card-description` + button region (index.html:21-27) with:
```html
    {{if .Refused}}
    <p class="card-description">
      This database can't be upgraded automatically.
    </p>
    <div class="log" id="log">{{.Error}}</div>
    <p class="meta">
      First upgrade to <strong>v0.17.1</strong> and let it migrate fully, then upgrade to
      v0.90.0 or later. If the database is in an inconsistent state, the fallback is
      export → fresh install → import (see the admin guide).
    </p>
    {{else}}
    <p class="card-description">
      {{if .Adopt}}Existing v0.17.1 schema detected — ready to adopt and bring up to date.{{else}}{{.PendingCount}} migration{{if ne .PendingCount 1}}s{{end}} pending{{end}}
    </p>
    <div class="log" id="log"></div>
    <button class="btn btn-primary" id="btn" onclick="runMigrations()">{{if .Adopt}}Adopt &amp; Migrate{{else}}Run Migrations{{end}}</button>
    <p class="meta" id="status"></p>
    {{end}}
```
In the page `<script>`, handle the refused state in `checkStatusAndAct` so the poller doesn't keep trying to run: where it switches on `data.state` (index.html ~line 64), add:
```js
          } else if (data.state === 'migration_refused') {
            stopPolling();
            return; // page already shows the refusal; nothing to poll
```
(When `.Refused` the run button isn't rendered, so `runMigrations` is never called; this just stops the poller.)

- [ ] **Step 5: Extend `handler_test.go`**

Add tests: (a) `HandleRun` returns 409 when state is `AppStateMigrationRefused`; (b) `HandleStatus` returns `state":"migration_refused"` with the error message; (c) `HandleStatus` returns `state":"needs_adopt"`, `pending_count":1` when adopt-pending. Use the existing handler-test construction pattern (read `handler_test.go` first to match how it builds the `Handler`/migrator — likely `NewMigratorForTest` + `SetStateForTest`).
```bash
go test ./internal/migrate/ -run TestHandle -v
```
Expected: PASS.

- [ ] **Step 6: Build + commit**

```bash
go build ./...
git add internal/migrate/handler.go internal/migrate/handler_test.go ui/migrate/index.html
git commit -m "feat(migrate): adopt/refuse messaging on the /migrate page and status API"
```

---

## Task 8: Backup-restore adopt path (verification)

**Files:**
- Test: `internal/api/backup_test.go` or `cmd/nexorious/serve_test.go` (whichever already exercises `ReinitMigrator`); else a focused migrate-package test.

**Rationale:** `ReinitMigrator` (serve.go:449-454) calls `migrator.DetermineState()` after a restore. With Task 3, restoring a v0.17.1 dump (23 manifest rows) re-determines to `NeedsAdopt`, and Gate 2 serves `/migrate` where the admin clicks "Adopt & Migrate" — consistent with how restoring any behind-schema backup already behaves. **No production code change is required**; this task only proves it.

- [ ] **Step 1: Add a test that `DetermineState` after a simulated v0.17.1 restore yields `NeedsAdopt`**

Reuse `seedV0171` (Task 4). This is essentially `TestDetermineState_AdoptPending` framed as the restore contract; if it feels redundant with Task 4, instead add a one-line assertion to an existing restore test that the post-restore state for a v0.17.1 dump is `needs_adopt`. Keep whichever is least duplicative.
```bash
go test ./internal/migrate/ ./internal/api/ -run 'Adopt|Restore' -v
```
Expected: PASS.

- [ ] **Step 2: Commit (if a new test was added)**

```bash
git add -A
git commit -m "test(migrate): document the backup-restore adopt path"
```

---

## Task 9: Documentation — README + admin-guide

**Files:**
- Modify: `README.md`
- Modify: `docs/admin-guide.md`

- [ ] **Step 1: Replace the README `[!WARNING]` block**

Replace README.md:3-5 with beta / path-to-1.0 messaging + the upgrade contract:
```markdown
> [!IMPORTANT]
> Nexorious v0.90.0+ is a **beta** stabilizing toward v1.0.0. It is usable, but
> expect rough edges while the schema and features are hardened before 1.0.
>
> **Upgrade contract:** you must be on **v0.17.1** (let it migrate fully) before
> upgrading to **v0.90.0 or later**. v0.90.0 adopts a fully-migrated v0.17.1
> database in place with zero data loss. Installs older than v0.17.1, or in an
> inconsistent state, are refused — step through v0.17.1 first, or use the
> export → fresh install → import fallback (see the admin guide).
```

- [ ] **Step 2: Add an upgrade-contract subsection to the admin-guide**

In `docs/admin-guide.md`, in the existing `## Upgrades and versioning` section, add (before the "one big caveat is the 1.0.0 release" paragraph) a subsection documenting:
- The v0.17.1 → v0.90.0+ contract (be on v0.17.1 first, fully migrated).
- What adopt does: in-place, zero data loss for a fully-migrated v0.17.1 DB; API keys, sync credentials, notifications, settings all preserved.
- What refusal looks like at each entry point: web `/migrate` shows the refusal page; `nexorious migrate` exits non-zero (failing Helm init-containers / `serve --migrate` aborts startup).
- The export → fresh install → import fallback, with the "what you'll need to redo after import" list (users & passwords, API keys, sync connections/credentials, notification channels & subscriptions, user settings, backup schedule, sync state/match history) and what export DOES preserve (games + pools).

Concrete copy:
```markdown
### Upgrading to v0.90.0+ (the migration squash)

v0.90.0 collapses the early migration history into a single baseline. To upgrade
safely you must first be on **v0.17.1** with its schema fully migrated, then move
to v0.90.0 or any later version. On a fully-migrated v0.17.1 database the migrator
**adopts** the existing schema in place — it rewrites the migration history to the
baseline and applies anything newer, with **zero data loss** (collections, pools,
users, API keys, sync connections and credentials, notification channels and
subscriptions, and settings are all preserved).

If the database is older than v0.17.1, partially migrated, or otherwise in an
unrecognized state, the upgrade is **refused** rather than risking corruption:
- The web `/migrate` page shows a refusal with these instructions instead of a run button.
- `nexorious migrate` and `serve --migrate` exit non-zero (so a Helm `migrate`
  initContainer fails loudly rather than starting against a broken schema).

To recover a refused database, first upgrade it to v0.17.1 and let it migrate
fully, then upgrade to v0.90.0+. If it's in an inconsistent state and can't reach
v0.17.1, use the export/import fallback: export each user's collection (JSON
export from Import / Export), start fresh with an empty database on v0.90.0+, and
import. Export preserves **games** (status, rating, loved/wishlisted, notes,
platforms, tags) and **pools** (filters + members). After a fresh install + import
you must redo: users & passwords (re-run setup), API keys (re-mint), sync
connections / storefront credentials (`sync connect`), notification channels &
subscriptions, user settings (deal-region, etc.), the backup schedule, and sync
state / external-game match history (rebuilt by re-running syncs).
```

- [ ] **Step 3: Verify the docs build/serve (admin-guide is embedded)**

```bash
go build ./...   # docs/embed.go embeds admin-guide.md; a syntax-free .md change just needs a rebuild
```
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/admin-guide.md
git commit -m "docs: document the v0.17.1→v0.90.0 upgrade contract and adopt/refuse"
```

---

## Task 10: Final verification, deadcode, and release wiring

**Files:** none (verification + PR).

- [ ] **Step 1: Deadcode reconciliation**

This work removes/renames callers (the constructor refactor, deleted migration files). Run:
```bash
make deadcode
```
Reconcile any NEW entries against the diff. Expected new-but-legitimate: none — `NewMigratorWithMigrations` is used by tests (run with `-test`), `adopt`/`classify` are used by `migrator.go`. If `NewMigrator` shows as unused, that's a real signal something didn't wire up — investigate.

- [ ] **Step 2: Full suites (these are the pre-push gate; run them now)**

```bash
go build ./...
go test -timeout 600s ./...
```
Expected: all PASS. Pay attention to every DB-backed package — they now boot on the baseline.

- [ ] **Step 3: Re-run the baseline acceptance once more (clean-room proof for the PR)**

```bash
./scripts/gen-baseline.sh
git diff --stat internal/db/migrations/20260620000001_baseline.up.sql
```
Expected: acceptance passes; **no diff** to the committed baseline (the generator is deterministic). Capture the script's tail output for the PR body.

- [ ] **Step 4: Manual smoke (optional but recommended) — adopt against a real v0.17.1 binary**

If feasible locally: run the released **v0.17.1** binary against a fresh DB, let it migrate, stop it; then run the **branch** build's `nexorious migrate` against that same DB and confirm it prints adopt progress and exits 0, and `bun_migrations` holds exactly one row (`20260620000001`). This is the real-world proof the synthetic tests can't fully cover. (Detailed steps go in the PR's manual-testing plan.)

- [ ] **Step 5: Push and open the PR with `Release-As: 0.90.0` and a manual testing plan**

```bash
git push -u origin feat/squash-migrations-baseline
gh pr create --title "feat(db): squash migrations into a single v0.90.0 baseline with in-place adopt from v0.17.1" --body "$(cat <<'BODY'
## Summary
Squashes the 23 early-development migrations into one generated `20260620000001_baseline` and teaches the migrator to **adopt** a fully-migrated v0.17.1 database in place (rewrite `bun_migrations` to the single baseline row, then catch up) or **refuse** anything older/partial. Permanent upgrade contract: be on v0.17.1, then jump to v0.90.0+.

Closes #1117

## Key correctness facts (verified)
- `bun_migrations.name` stores the **14-digit timestamp only** — the manifest and gate compare timestamps.
- The adopt row is written with `group_id = 1`, matching a fresh-baseline install byte-for-byte.
- Baseline generated from the v0.17.1 end-state via `pg_dump` inside `postgres:18-alpine`; `scripts/gen-baseline.sh` reproduces and verifies it (schema byte-diff + 4-table seed-data diff). River is excluded and untouched.

## Manual testing plan
> Prereqs: Docker, and the released **v0.17.1** binary (or container) available for the adopt test.

### A. Fresh install (baseline)
1. Point a clean empty Postgres at the branch build. Run `./nexorious migrate`.
2. **Expect:** migrations run, exit 0, "Migrations complete."
3. `psql … -c "SELECT name FROM bun_migrations"` → **expect exactly one row `20260620000001`**.
4. `psql … -c "SELECT count(*) FROM platforms; SELECT count(*) FROM storefronts; SELECT count(*) FROM platform_storefronts; SELECT count(*) FROM backup_config"` → **all non-zero** (backup_config = 1).
5. Start `./nexorious serve`, complete setup, confirm the app works (list games, add a game).

### B. In-place adopt from v0.17.1 (the headline path)
1. Against a clean DB, run the **v0.17.1** binary's `migrate`; let it finish. Confirm `SELECT count(*) FROM bun_migrations` = **23**.
2. Add some data on v0.17.1 (a user, an API key, a game, a pool) so you can prove zero data loss.
3. Stop v0.17.1. Run the **branch** build's `./nexorious migrate` against the same DB.
4. **Expect:** output mentions "Adopting existing v0.17.1 schema…", exits **0**.
5. `SELECT name FROM bun_migrations` → **exactly one row `20260620000001`**. Your user/API key/game/pool are all still present.
6. Start the branch `serve`; log in with the same credentials; the API key still authenticates.

### C. Adopt via the web `/migrate` page
1. Repeat B steps 1–2, then start the branch build with `./nexorious serve` (no `--migrate`).
2. Visit `/migrate`. **Expect:** "Existing v0.17.1 schema detected — ready to adopt…" and an **"Adopt & Migrate"** button.
3. Click it; watch progress complete; get redirected into the app. `bun_migrations` = one row.

### D. Adopt via `serve --migrate`
1. Repeat B steps 1–2, then start the branch build with `./nexorious serve --migrate`.
2. **Expect:** it adopts during startup and serves normally; `bun_migrations` = one row.

### E. Refuse — older/partial/unknown database
1. Take the v0.17.1 DB from B step 1 and delete one row: `DELETE FROM bun_migrations WHERE name='20260612000001'`.
2. Run the branch `./nexorious migrate`. **Expect:** non-zero exit with the "upgrade to v0.17.1 first / export→import fallback" message.
3. Start the branch `serve`; visit `/migrate`. **Expect:** the **refusal page** (no run button), with upgrade instructions.
4. `./nexorious migrate status` → **expect** `state=migration_refused`.

### F. Backup restore of a v0.17.1 dump
1. With the app running on the branch build, restore a backup taken from a v0.17.1 instance (admin → backups → restore).
2. **Expect:** after restore the app surfaces the `/migrate` page in **adopt** mode; clicking "Adopt & Migrate" brings it up to date with the restored data intact.

### G. Baseline acceptance (automated, re-runnable by you)
1. `./scripts/gen-baseline.sh` → **expect** `SCHEMA OK`, `SEED DATA OK`, "All acceptance checks passed", and **no git diff** to the committed baseline.

🤖 (remove this line)
BODY
)"
```
**Important:** the `Release-As: 0.90.0` trailer must be in the PR body. Append it (and remove the stray robot line above per the no-AI-attribution rule):
```
Release-As: 0.90.0
```
Expected: PR opens, `CI Gate` runs green.

- [ ] **Step 6: Hand off to the user for manual testing before merge** (per the explicit request — do NOT merge).

---

## Self-Review

**Spec coverage** (issue #1117 acceptance criteria → task):
- 23 → one baseline, byte-identical schema: **Task 1** (gen + acceptance script).
- Seed data preserved, 4-table diff, backfills dropped: **Task 1** (`dump_seed` 4 tables; backfills are user-row, absent on fresh DB) + **Task 4 Step 4** (permanent smoke).
- Squash is next migration on `main`: **Global Constraints** (verified window open).
- Adopt v0.17.1 + catch up; CLI exits 0: **Tasks 3, 4, 5**.
- Older/partial/unknown refused everywhere; CLI non-zero: **Tasks 3, 5, 6, 7**.
- Adopt for startup, `nexorious migrate`, backup restore: **Tasks 5, 6, 8** (+ no router change needed, Gate 2 verified).
- Regression test, exact-manifest gate + adopt-then-catch-up: **Task 4** (the injectable seam makes catch-up testable).
- README beta messaging + contract + fallback: **Task 9**.
- admin-guide contract, refusal, fallback list: **Task 9**.
- Released as v0.90.0: **Task 10** (`Release-As` trailer).

**Type consistency:** `baselineTimestamp` ("20260620000001"), `v0171Manifest` (timestamp keys), `classify`, `adoptDecision`/`decision{Normal,Adopt,Refuse}`, `NewMigratorWithMigrations`, `adopt`, `AppStateNeedsAdopt`/`AppStateMigrationRefused` (`"needs_adopt"`/`"migration_refused"`) are used identically across Tasks 2–8.

**Open risk flagged for the implementer:** Task 1 Step 4 — `pg_dump` emits noise (ordering, `SET`, ownership) that the `grep` filters must fully neutralize for the byte-diff to pass; if the diff is non-empty, it is almost always filter gaps, not a real schema difference. Resolve by tightening `dump_schema`, not by hand-editing the baseline.
