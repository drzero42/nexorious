# Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users configure multiple Shoutrrr notification channels and opt into per-event notifications (sync, import/export, game-diff digest, and admin-only backup/maintenance events), delivered fire-and-forget through an append-only `events` outbox.

**Architecture:** Emission sites call a package-level `notify.Emit(ctx, db, …)` that inserts one row into an append-only `events` table (idempotent on a `dedup_key` for fire-once job events) and best-effort enqueues a River `NotifyWorker` job. The worker resolves recipients from `notification_subscriptions` (pure-presence rows), loads + decrypts each recipient's `notification_channels`, renders a per-event `(title, body)` via a central registry of formatters, and sends through a `Sender` interface (real impl = Shoutrrr; test impl = recorder). Per-channel failures are swallowed and logged at WARN; the `events` table is pruned by a daily River periodic job. River runs on its own pgx pool (separate from Bun), so emission uses bun for the DB write and a package-level River client for the enqueue — never a cross-pool transaction.

**Tech Stack:** Go 1.25, Bun ORM (+ `pgdriver`), River (`riverqueue/river` on its own pgx pool), Echo v5, `internal/crypto` (AES-256-GCM), `github.com/containrrr/shoutrrr`; React 19 + TanStack Query/Router + React Hook Form + Zod + shadcn/ui frontend.

---

## Key design decisions (read before starting)

1. **Package-level emit seam.** `SyncCheckJobCompletion` and the import/export completion helpers are free functions called from 18+ sites (including `internal/api/sync.go` HTTP handlers) that only have a `*bun.DB`. Threading a River client through all of them is too invasive, and River's pgx pool is separate from Bun's `pgdriver` pool so there is no shared transaction anyway. Therefore `internal/notify` holds a package-level River client set once at startup via `notify.SetRiverClient(rc)` (re-set in `RebuildServices`). `Emit` writes the `events` row via the passed `*bun.DB` (every call site has one) and best-effort enqueues `NotifyWorker`. If the River client is unset (tests), the row is still written and a DEBUG line is logged.

2. **`events` is the outbox + future audit substrate (#734).** Append-only: `id, type, scope, actor_user_id (nullable), payload jsonb, dedup_key (nullable), occurred_at`. A plain unique index on `dedup_key` gives fire-once: job events use `dedup_key = "<job_id>:<type>"`; repeatable admin events use `NULL` (Postgres treats NULLs as distinct, so repeatable events always insert). `INSERT … ON CONFLICT (dedup_key) DO NOTHING RETURNING id` → a duplicate returns `sql.ErrNoRows`, which `Emit` treats as "already emitted, skip enqueue".

3. **Central event-type registry** is the single source of truth — adding a future event type is register + emit + formatter case + (optionally) default-on, zero migrations.

4. **Pure-presence subscriptions** — `(user_id, event_type)` PK. A row means subscribed. Defaults ("failures on, successes off") live only in `notify.DefaultSubscriptions()`, seeded on user creation; **Reset to defaults** deletes all rows for the user then re-inserts the defaults.

5. **Channel URLs encrypted at rest** via `crypto.Encrypter`, exactly like `storefront_credentials` (`*string` column, `enc:v1:` prefix, `json:"-"`).

---

## File structure

**New Go files**
- `internal/db/migrations/20260601000001_events.{up,down}.sql` — `events` table
- `internal/db/migrations/20260601000002_notification_channels.{up,down}.sql` — `notification_channels` table
- `internal/db/migrations/20260601000003_notification_subscriptions.{up,down}.sql` — `notification_subscriptions` table
- `internal/notify/registry.go` — event-type registry (`EventTypeMeta`, `Registry`, `DefaultSubscriptions`, scope/validation helpers)
- `internal/notify/registry_test.go`
- `internal/notify/emit.go` — `Emit`, `EmitParams`, package-level River client, scope constants
- `internal/notify/emit_test.go`
- `internal/notify/sender.go` — `Sender` interface, Shoutrrr impl, recorder impl
- `internal/notify/sender_test.go`
- `internal/notify/formatters.go` — per-event `(title, body)` rendering + payload structs
- `internal/notify/formatters_test.go`
- `internal/notify/worker.go` — `NotifyArgs`, `NotifyWorker`, recipient resolution
- `internal/notify/worker_test.go`
- `internal/notify/prune.go` — `PruneEventsArgs`/`PruneEventsWorker` + `PruneEvents`
- `internal/notify/prune_test.go`
- `internal/db/models/notifications.go` — `Event`, `NotificationChannel`, `NotificationSubscription` Bun models
- `internal/api/notifications.go` — channels CRUD + test-send + subscriptions + event-types handlers
- `internal/api/notifications_test.go`

**Modified Go files**
- `internal/config/config.go` — add `NotifyEventsRetentionDays`
- `cmd/nexorious/serve.go` — register `NotifyWorker`/`PruneEventsWorker`, call `notify.SetRiverClient`, wire `Sender` + `Encrypter`, register prune periodic job
- `internal/scheduler/scheduler.go` (`BuildPeriodicJobs`) — add daily prune job
- `internal/worker/tasks/sync.go` — emit `sync.completed` / `sync.completed_with_errors` / `sync.needs_review` / `sync.diff` in `SyncCheckJobCompletion`; `sync.failed` in `failSyncJob`
- `internal/worker/tasks/export.go` — emit `export.completed` / `export.failed`
- `internal/worker/tasks/import_item.go` — emit `import.completed`
- `internal/scheduler/backup_poll.go` — emit `admin.backup.completed` / `admin.backup.failed`
- `internal/scheduler/stale_jobs.go`, `internal/scheduler/orphaned_items.go` — emit `admin.maintenance.*`
- `internal/worker/tasks/metadata_refresh.go` — emit `admin.maintenance.*`
- `internal/api/router.go` — register notifications routes
- `internal/api/admin_users.go`, `internal/api/setup.go` — seed default subscriptions on user creation
- `slumber.yaml` — notifications domain folder
- `nix/package.nix` — bump `vendorHash` after `go.mod` change
- `go.mod` / `go.sum` — add Shoutrrr

**New frontend files**
- `ui/frontend/src/components/ui/switch.tsx` — added via `npx shadcn@latest add switch`
- `ui/frontend/src/api/notifications.ts` — typed API client
- `ui/frontend/src/hooks/use-notifications.ts` — TanStack Query hooks + keys
- `ui/frontend/src/components/notifications/notifications-section.tsx` — the profile section
- `ui/frontend/src/components/notifications/channel-dialog.tsx` — add/edit channel modal
- `ui/frontend/src/components/notifications/notifications-section.test.tsx`

**Modified frontend files**
- `ui/frontend/src/routes/_authenticated/profile.tsx` — mount `<NotificationsSection />`

---

# Phase 1 — Data model, registry, config

### Task 1: `events` table migration

**Files:**
- Create: `internal/db/migrations/20260601000001_events.up.sql`
- Create: `internal/db/migrations/20260601000001_events.down.sql`

- [ ] **Step 1: Write the up migration**

`internal/db/migrations/20260601000001_events.up.sql`:
```sql
-- Append-only notification/audit outbox. One row per emitted event.
-- dedup_key gives fire-once semantics for job-scoped events (NULL = repeatable;
-- Postgres treats NULLs as distinct so repeatable events always insert).
CREATE TABLE events (
    id            TEXT PRIMARY KEY,
    type          TEXT NOT NULL,
    scope         TEXT NOT NULL,
    actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    dedup_key     TEXT,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX events_dedup_key_idx ON events (dedup_key);
CREATE INDEX events_occurred_at_idx ON events (occurred_at);
CREATE INDEX events_actor_user_id_idx ON events (actor_user_id);
```

- [ ] **Step 2: Write the down migration**

`internal/db/migrations/20260601000001_events.down.sql`:
```sql
DROP TABLE events;
```

- [ ] **Step 3: Verify migrations discovered & apply cleanly**

Run: `go build ./... && ./nexorious migrate status`
Expected: build succeeds; `20260601000001_events` appears as a pending (then, after `./nexorious migrate`, applied) migration with no SQL error.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260601000001_events.*.sql
git commit -m "feat: add events outbox table"
```

---

### Task 2: `notification_channels` table migration

**Files:**
- Create: `internal/db/migrations/20260601000002_notification_channels.up.sql`
- Create: `internal/db/migrations/20260601000002_notification_channels.down.sql`

- [ ] **Step 1: Write the up migration**

`internal/db/migrations/20260601000002_notification_channels.up.sql`:
```sql
-- A user's notification channels. encrypted_url holds the Shoutrrr URL
-- encrypted at rest (enc:v1: prefix), same pattern as storefront_credentials.
CREATE TABLE notification_channels (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    encrypted_url TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX notification_channels_user_id_idx ON notification_channels (user_id);
```

- [ ] **Step 2: Write the down migration**

`internal/db/migrations/20260601000002_notification_channels.down.sql`:
```sql
DROP TABLE notification_channels;
```

- [ ] **Step 3: Verify**

Run: `go build ./... && ./nexorious migrate`
Expected: applies with no error.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260601000002_notification_channels.*.sql
git commit -m "feat: add notification_channels table"
```

---

### Task 3: `notification_subscriptions` table migration

**Files:**
- Create: `internal/db/migrations/20260601000003_notification_subscriptions.up.sql`
- Create: `internal/db/migrations/20260601000003_notification_subscriptions.down.sql`

- [ ] **Step 1: Write the up migration**

`internal/db/migrations/20260601000003_notification_subscriptions.up.sql`:
```sql
-- Pure-presence subscriptions: a row means the user wants this event type.
CREATE TABLE notification_subscriptions (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, event_type)
);
```

- [ ] **Step 2: Write the down migration**

`internal/db/migrations/20260601000003_notification_subscriptions.down.sql`:
```sql
DROP TABLE notification_subscriptions;
```

- [ ] **Step 3: Verify**

Run: `go build ./... && ./nexorious migrate`
Expected: applies with no error.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260601000003_notification_subscriptions.*.sql
git commit -m "feat: add notification_subscriptions table"
```

---

### Task 4: Bun models

**Files:**
- Create: `internal/db/models/notifications.go`

- [ ] **Step 1: Write the models**

`internal/db/models/notifications.go`:
```go
package models

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// Event mirrors the append-only events outbox table.
type Event struct {
	bun.BaseModel `bun:"table:events"`

	ID          string          `bun:"id,pk"             json:"id"`
	Type        string          `bun:"type,notnull"      json:"type"`
	Scope       string          `bun:"scope,notnull"     json:"scope"`
	ActorUserID *string         `bun:"actor_user_id"     json:"actor_user_id"`
	Payload     json.RawMessage `bun:"payload,notnull"   json:"payload"`
	DedupKey    *string         `bun:"dedup_key"         json:"dedup_key"`
	OccurredAt  time.Time       `bun:"occurred_at,notnull" json:"occurred_at"`
}

// NotificationChannel mirrors notification_channels. EncryptedURL is never
// exposed in API responses (json:"-").
type NotificationChannel struct {
	bun.BaseModel `bun:"table:notification_channels"`

	ID           string    `bun:"id,pk"                json:"id"`
	UserID       string    `bun:"user_id,notnull"      json:"user_id"`
	Name         string    `bun:"name,notnull"         json:"name"`
	EncryptedURL string    `bun:"encrypted_url,notnull" json:"-"`
	CreatedAt    time.Time `bun:"created_at,notnull"   json:"created_at"`
}

// NotificationSubscription mirrors notification_subscriptions (pure presence).
type NotificationSubscription struct {
	bun.BaseModel `bun:"table:notification_subscriptions"`

	UserID    string    `bun:"user_id,pk"     json:"user_id"`
	EventType string    `bun:"event_type,pk"  json:"event_type"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/db/models/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/db/models/notifications.go
git commit -m "feat: add notification Bun models"
```

---

### Task 5: Event-type registry

**Files:**
- Create: `internal/notify/registry.go`
- Test: `internal/notify/registry_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/registry_test.go`:
```go
package notify

import "testing"

func TestRegistryHasExpectedTypes(t *testing.T) {
	want := []string{
		"sync.completed", "sync.completed_with_errors", "sync.failed",
		"sync.needs_review", "sync.diff",
		"import.completed", "import.failed", "export.completed", "export.failed",
		"admin.backup.completed", "admin.backup.failed",
		"admin.maintenance.completed", "admin.maintenance.failed",
	}
	for _, typ := range want {
		if _, ok := Meta(typ); !ok {
			t.Errorf("registry missing event type %q", typ)
		}
	}
}

func TestDefaultSubscriptionsAreFailuresOnly(t *testing.T) {
	defaults := DefaultSubscriptions()
	got := map[string]bool{}
	for _, d := range defaults {
		got[d] = true
	}
	// failures on
	for _, typ := range []string{"sync.failed", "import.failed", "export.failed", "sync.completed_with_errors", "admin.backup.failed", "admin.maintenance.failed"} {
		if !got[typ] {
			t.Errorf("expected default-on for %q", typ)
		}
	}
	// successes off
	for _, typ := range []string{"sync.completed", "import.completed", "export.completed", "admin.backup.completed", "admin.maintenance.completed", "sync.needs_review", "sync.diff"} {
		if got[typ] {
			t.Errorf("expected default-off for %q", typ)
		}
	}
}

func TestIsAdminType(t *testing.T) {
	if !IsAdminType("admin.backup.failed") {
		t.Error("admin.backup.failed should be admin-scoped")
	}
	if IsAdminType("sync.failed") {
		t.Error("sync.failed should not be admin-scoped")
	}
	if IsAdminType("does.not.exist") {
		t.Error("unknown type should not be admin-scoped")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notify/ -run TestRegistry -v`
Expected: FAIL — `notify` package / `Meta` undefined (build error).

- [ ] **Step 3: Write the registry**

`internal/notify/registry.go`:
```go
// Package notify implements user/admin notification emission and delivery.
package notify

// Scope constants for events.
const (
	ScopeUser  = "user"
	ScopeAdmin = "admin"
)

// Event type string constants. These are the single source of truth.
const (
	TypeSyncCompleted           = "sync.completed"
	TypeSyncCompletedWithErrors = "sync.completed_with_errors"
	TypeSyncFailed              = "sync.failed"
	TypeSyncNeedsReview         = "sync.needs_review"
	TypeSyncDiff                = "sync.diff"
	TypeImportCompleted         = "import.completed"
	TypeImportFailed            = "import.failed"
	TypeExportCompleted         = "export.completed"
	TypeExportFailed            = "export.failed"
	TypeAdminBackupCompleted    = "admin.backup.completed"
	TypeAdminBackupFailed       = "admin.backup.failed"
	TypeAdminMaintCompleted     = "admin.maintenance.completed"
	TypeAdminMaintFailed        = "admin.maintenance.failed"
)

// EventTypeMeta describes one event type for the registry and the settings UI.
type EventTypeMeta struct {
	Type      string `json:"type"`
	Scope     string `json:"scope"`     // ScopeUser | ScopeAdmin
	Category  string `json:"category"`  // grouping label for the UI
	Label     string `json:"label"`     // human-readable name
	DefaultOn bool   `json:"default_on"`
}

// registry preserves declaration order (used for stable UI ordering).
var registry = []EventTypeMeta{
	{TypeSyncCompleted, ScopeUser, "Sync", "Sync completed", false},
	{TypeSyncCompletedWithErrors, ScopeUser, "Sync", "Sync completed with errors", true},
	{TypeSyncFailed, ScopeUser, "Sync", "Sync failed", true},
	{TypeSyncNeedsReview, ScopeUser, "Sync", "Sync has items needing review", false},
	{TypeSyncDiff, ScopeUser, "Sync", "Game changes digest per sync", false},
	{TypeImportCompleted, ScopeUser, "Import / Export", "Import completed", false},
	{TypeImportFailed, ScopeUser, "Import / Export", "Import failed", true},
	{TypeExportCompleted, ScopeUser, "Import / Export", "Export completed", false},
	{TypeExportFailed, ScopeUser, "Import / Export", "Export failed", true},
	{TypeAdminBackupCompleted, ScopeAdmin, "Backups", "Scheduled backup completed", false},
	{TypeAdminBackupFailed, ScopeAdmin, "Backups", "Scheduled backup failed", true},
	{TypeAdminMaintCompleted, ScopeAdmin, "Maintenance", "Maintenance tasks completed", false},
	{TypeAdminMaintFailed, ScopeAdmin, "Maintenance", "Maintenance tasks failed", true},
}

var metaByType = func() map[string]EventTypeMeta {
	m := make(map[string]EventTypeMeta, len(registry))
	for _, e := range registry {
		m[e.Type] = e
	}
	return m
}()

// Registry returns all event types in declaration order.
func Registry() []EventTypeMeta { return registry }

// Meta returns the metadata for a type and whether it is known.
func Meta(eventType string) (EventTypeMeta, bool) {
	m, ok := metaByType[eventType]
	return m, ok
}

// IsKnownType reports whether eventType is registered.
func IsKnownType(eventType string) bool {
	_, ok := metaByType[eventType]
	return ok
}

// IsAdminType reports whether eventType is admin-scoped. Unknown types → false.
func IsAdminType(eventType string) bool {
	m, ok := metaByType[eventType]
	return ok && m.Scope == ScopeAdmin
}

// DefaultSubscriptions returns the event types that are on by default
// ("failures on, successes off"). Seeded on user creation and on Reset.
func DefaultSubscriptions() []string {
	var out []string
	for _, e := range registry {
		if e.DefaultOn {
			out = append(out, e.Type)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/notify/ -run 'TestRegistry|TestDefault|TestIsAdmin' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/notify/registry.go internal/notify/registry_test.go
git commit -m "feat: notification event-type registry"
```

---

### Task 6: Config — events retention window

**Files:**
- Modify: `internal/config/config.go` (Scheduler section, near `SyncHistoryRetentionDays`)

- [ ] **Step 1: Add the config field**

In `internal/config/config.go`, in the Scheduler section (immediately after the `SyncHistoryRetentionDays` line), add:
```go
	NotifyEventsRetentionDays int `env:"NOTIFY_EVENTS_RETENTION_DAYS" envDefault:"90"`
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/config/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add NOTIFY_EVENTS_RETENTION_DAYS config"
```

---

# Phase 2 — Delivery core

### Task 7: Add Shoutrrr dependency

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `nix/package.nix` (`vendorHash`)

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/containrrr/shoutrrr@latest`
Expected: `go.mod` gains a `github.com/containrrr/shoutrrr vX.Y.Z` require line; `go.sum` updated.

- [ ] **Step 2: Verify it resolves & builds**

Run: `go build ./...`
Expected: success (no use yet, just the require line resolving).

- [ ] **Step 3: Update the Nix vendorHash**

Run:
```bash
# Temporarily set vendorHash = pkgs.lib.fakeHash; in nix/package.nix, then:
nix build .#nexorious 2>&1 | grep "got:"
```
Paste the `got:` hash into `nix/package.nix` → `vendorHash`.
(If `nix` is unavailable in this shell, leave a TODO note in the PR description and flag it — the maintainer can regenerate; do NOT guess a hash.)

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum nix/package.nix
git commit -m "build: add github.com/containrrr/shoutrrr"
```

---

### Task 8: Sender interface + Shoutrrr impl + recorder

**Files:**
- Create: `internal/notify/sender.go`
- Test: `internal/notify/sender_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/sender_test.go`:
```go
package notify

import (
	"context"
	"testing"
)

func TestRecorderSenderRecords(t *testing.T) {
	r := NewRecorderSender()
	if err := r.Send(context.Background(), "noop://", "Title", "Body"); err != nil {
		t.Fatalf("recorder Send returned error: %v", err)
	}
	sent := r.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 recorded send, got %d", len(sent))
	}
	if sent[0].URL != "noop://" || sent[0].Title != "Title" || sent[0].Body != "Body" {
		t.Errorf("unexpected recorded send: %+v", sent[0])
	}
}

func TestShoutrrrSenderInvalidURL(t *testing.T) {
	s := NewShoutrrrSender()
	// An unparseable URL must surface as an error (not a panic).
	if err := s.Send(context.Background(), "this-is-not-a-valid-shoutrrr-url", "T", "B"); err == nil {
		t.Error("expected error for invalid shoutrrr URL, got nil")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notify/ -run 'TestRecorderSender|TestShoutrrrSender' -v`
Expected: FAIL — `NewRecorderSender` / `NewShoutrrrSender` undefined.

- [ ] **Step 3: Write the sender**

`internal/notify/sender.go`:
```go
package notify

import (
	"context"
	"fmt"
	"sync"

	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/types"
)

// Sender delivers a single rendered notification to one channel URL.
// Implementations must not panic on malformed URLs; return an error instead.
type Sender interface {
	Send(ctx context.Context, url, title, body string) error
}

// ShoutrrrSender is the production Sender backed by github.com/containrrr/shoutrrr.
type ShoutrrrSender struct{}

// NewShoutrrrSender constructs a ShoutrrrSender.
func NewShoutrrrSender() *ShoutrrrSender { return &ShoutrrrSender{} }

// Send delivers body (with title param) to the given Shoutrrr URL. Shoutrrr's
// ServiceRouter.Send returns a slice of errors (one per service); the first
// non-nil is returned.
func (s *ShoutrrrSender) Send(_ context.Context, url, title, body string) error {
	sender, err := shoutrrr.CreateSender(url)
	if err != nil {
		return fmt.Errorf("notify: create sender: %w", err)
	}
	params := &types.Params{"title": title}
	for _, e := range sender.Send(body, params) {
		if e != nil {
			return fmt.Errorf("notify: send: %w", e)
		}
	}
	return nil
}

// SentMessage is one recorded delivery (test impl).
type SentMessage struct {
	URL   string
	Title string
	Body  string
}

// RecorderSender records sends instead of delivering them (test impl).
type RecorderSender struct {
	mu   sync.Mutex
	sent []SentMessage
	// Err, if set, is returned by every Send (to exercise failure paths).
	Err error
}

// NewRecorderSender constructs a RecorderSender.
func NewRecorderSender() *RecorderSender { return &RecorderSender{} }

// Send records the message and returns r.Err.
func (r *RecorderSender) Send(_ context.Context, url, title, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, SentMessage{URL: url, Title: title, Body: body})
	return r.Err
}

// Sent returns a copy of recorded messages.
func (r *RecorderSender) Sent() []SentMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SentMessage, len(r.sent))
	copy(out, r.sent)
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/notify/ -run 'TestRecorderSender|TestShoutrrrSender' -v`
Expected: PASS. (If the shoutrrr `types.Params` import path differs in the pinned version, run `go doc github.com/containrrr/shoutrrr/pkg/types Params` to confirm — do not guess.)

- [ ] **Step 5: Commit**

```bash
git add internal/notify/sender.go internal/notify/sender_test.go
git commit -m "feat: notification Sender interface + shoutrrr/recorder impls"
```

---

### Task 9: Emit + package-level River client

**Files:**
- Create: `internal/notify/emit.go`
- Test: `internal/notify/emit_test.go`

This task needs a real DB. Add a `TestMain` for the `notify` package that starts the shared Postgres container (mirror an existing package's `main_test.go`, e.g. `internal/scheduler/main_test.go`). The shared `testDB` variable + `truncateAllTables(t)` helper come from that file.

- [ ] **Step 1: Write the package test harness**

`internal/notify/main_test.go` — copy the structure of `internal/scheduler/main_test.go` (same container bootstrap, `testDB *bun.DB` package var, `truncateAllTables(t)` helper, migrations run once in `TestMain`). Keep the package name `notify`.

- [ ] **Step 2: Write the failing test**

`internal/notify/emit_test.go`:
```go
package notify

import (
	"context"
	"testing"
)

func TestEmitInsertsEventRow(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil) // no enqueue in this test

	Emit(ctx, testDB, EmitParams{
		Type:    TypeSyncFailed,
		Scope:   ScopeUser,
		ActorUserID: "user-1",
		Payload: map[string]any{"job_id": "job-1"},
		DedupKey: "job-1:sync.failed",
	})

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ?`, TypeSyncFailed).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event row, got %d", count)
	}
}

func TestEmitDedupFiresOnce(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)

	for i := 0; i < 3; i++ {
		Emit(ctx, testDB, EmitParams{
			Type: TypeSyncCompleted, Scope: ScopeUser, ActorUserID: "u1",
			DedupKey: "job-9:sync.completed",
		})
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, "job-9:sync.completed").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("dedup failed: expected 1 row, got %d", count)
	}
}

func TestEmitNullDedupAlwaysInserts(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)

	for i := 0; i < 3; i++ {
		Emit(ctx, testDB, EmitParams{Type: TypeAdminMaintCompleted, Scope: ScopeAdmin})
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ?`, TypeAdminMaintCompleted).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 repeatable rows, got %d", count)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/notify/ -run TestEmit -v`
Expected: FAIL — `Emit` / `EmitParams` / `SetRiverClient` undefined.

- [ ] **Step 4: Write emit.go**

`internal/notify/emit.go`:
```go
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"
)

// EmitParams describes one event to emit.
type EmitParams struct {
	Type        string         // a registry event type constant
	Scope       string         // ScopeUser | ScopeAdmin
	ActorUserID string         // "" for system/admin events with no single actor
	Payload     map[string]any // event-specific data; nil → {}
	DedupKey    string         // "" → repeatable; non-empty → fire-once
}

var (
	rcMu        sync.RWMutex
	riverClient *river.Client[pgx.Tx]
)

// SetRiverClient registers (or replaces) the River client used to enqueue
// NotifyWorker jobs. Called once at startup and again in RebuildServices.
func SetRiverClient(rc *river.Client[pgx.Tx]) {
	rcMu.Lock()
	defer rcMu.Unlock()
	riverClient = rc
}

// Emit writes an event row (idempotent on DedupKey) via db, then best-effort
// enqueues a NotifyWorker to deliver it. Never returns an error: emission is
// fire-and-forget and must not break the caller's primary work.
func Emit(ctx context.Context, db *bun.DB, p EmitParams) {
	if p.Payload == nil {
		p.Payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(p.Payload)
	if err != nil {
		slog.Error("notify: marshal payload", "type", p.Type, "err", err)
		payloadJSON = []byte("{}")
	}

	var actor *string
	if p.ActorUserID != "" {
		actor = &p.ActorUserID
	}
	var dedup *string
	if p.DedupKey != "" {
		dedup = &p.DedupKey
	}

	id := uuid.NewString()
	var insertedID string
	err = db.NewRaw(
		`INSERT INTO events (id, type, scope, actor_user_id, payload, dedup_key, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, now())
		 ON CONFLICT (dedup_key) DO NOTHING
		 RETURNING id`,
		id, p.Type, p.Scope, actor, json.RawMessage(payloadJSON), dedup,
	).Scan(ctx, &insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		// Duplicate dedup_key — already emitted; do not re-enqueue.
		return
	}
	if err != nil {
		slog.Error("notify: insert event", "type", p.Type, "err", err)
		return
	}

	enqueueNotify(ctx, insertedID)
}

func enqueueNotify(ctx context.Context, eventID string) {
	rcMu.RLock()
	rc := riverClient
	rcMu.RUnlock()
	if rc == nil {
		slog.Debug("notify: river client unset; event persisted but not enqueued", "event_id", eventID)
		return
	}
	if _, err := rc.Insert(ctx, NotifyArgs{EventID: eventID}, nil); err != nil {
		slog.Warn("notify: enqueue NotifyWorker", "event_id", eventID, "err", err)
	}
}
```

Note: `NotifyArgs` is defined in Task 11. Tasks 9–11 land together compile-wise; if implementing strictly in order, temporarily stub `type NotifyArgs struct{ EventID string }` with `func (NotifyArgs) Kind() string { return "notify" }` in `emit.go` and move it to `worker.go` in Task 11. Cleanest is to do Steps for Tasks 9 and 11's arg definition together. **To keep this task self-contained, add the `NotifyArgs` definition here** (in `worker.go` Task 11, do not redefine it):

Add to `internal/notify/emit.go`:
```go
// NotifyArgs is the River job payload for delivering one event.
type NotifyArgs struct {
	EventID string `json:"event_id"`
}

// Kind implements river.JobArgs.
func (NotifyArgs) Kind() string { return "notify" }

// InsertOpts limits retries — delivery is fire-and-forget; infra retry only.
func (NotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/notify/ -run TestEmit -v`
Expected: PASS (all three).

- [ ] **Step 6: Commit**

```bash
git add internal/notify/emit.go internal/notify/emit_test.go internal/notify/main_test.go
git commit -m "feat: notify.Emit with dedup + package-level river client"
```

---

### Task 10: Formatters

**Files:**
- Create: `internal/notify/formatters.go`
- Test: `internal/notify/formatters_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/formatters_test.go`:
```go
package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatSyncFailed(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"storefront": "steam", "error": "bad token"})
	title, body := Format(TypeSyncFailed, payload)
	if !strings.Contains(strings.ToLower(title), "sync") {
		t.Errorf("title missing 'sync': %q", title)
	}
	if !strings.Contains(body, "steam") || !strings.Contains(body, "bad token") {
		t.Errorf("body missing detail: %q", body)
	}
}

func TestFormatSyncDiff(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"added":   []map[string]any{{"title": "Hades", "platforms": []string{"Steam"}}},
		"removed": []map[string]any{{"title": "Old Game", "platforms": []string{"GOG"}}},
	})
	title, body := Format(TypeSyncDiff, payload)
	if title == "" {
		t.Error("expected non-empty title")
	}
	if !strings.Contains(body, "Hades") || !strings.Contains(body, "Old Game") {
		t.Errorf("diff body missing games: %q", body)
	}
}

func TestFormatUnknownTypeIsSafe(t *testing.T) {
	title, body := Format("totally.unknown", json.RawMessage(`{}`))
	if title == "" || body == "" {
		t.Errorf("unknown type should yield a safe fallback, got title=%q body=%q", title, body)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notify/ -run TestFormat -v`
Expected: FAIL — `Format` undefined.

- [ ] **Step 3: Write formatters.go**

`internal/notify/formatters.go`:
```go
package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DiffGame is one entry in a sync.diff payload.
type DiffGame struct {
	Title     string   `json:"title"`
	Platforms []string `json:"platforms"`
}

// Format renders a (title, body) pair for an event type + payload. Unknown
// types and malformed payloads fall back to a generic, never-empty message.
func Format(eventType string, payload json.RawMessage) (title, body string) {
	meta, ok := Meta(eventType)
	label := eventType
	if ok {
		label = meta.Label
	}

	switch eventType {
	case TypeSyncFailed:
		var p struct {
			Storefront string `json:"storefront"`
			Error      string `json:"error"`
		}
		_ = json.Unmarshal(payload, &p)
		title = "Sync failed"
		body = fmt.Sprintf("Your %s sync failed: %s", fallback(p.Storefront, "library"), fallback(p.Error, "unknown error"))

	case TypeSyncCompleted:
		var p struct {
			Storefront string `json:"storefront"`
		}
		_ = json.Unmarshal(payload, &p)
		title = "Sync completed"
		body = fmt.Sprintf("Your %s sync completed successfully.", fallback(p.Storefront, "library"))

	case TypeSyncCompletedWithErrors:
		var p struct {
			Storefront string `json:"storefront"`
			Failed     int    `json:"failed"`
		}
		_ = json.Unmarshal(payload, &p)
		title = "Sync completed with errors"
		body = fmt.Sprintf("Your %s sync finished with %d failed item(s).", fallback(p.Storefront, "library"), p.Failed)

	case TypeSyncNeedsReview:
		var p struct {
			Storefront string `json:"storefront"`
			Count      int    `json:"count"`
		}
		_ = json.Unmarshal(payload, &p)
		title = "Sync needs review"
		body = fmt.Sprintf("Your %s sync has %d item(s) needing review.", fallback(p.Storefront, "library"), p.Count)

	case TypeSyncDiff:
		var p struct {
			Added   []DiffGame `json:"added"`
			Removed []DiffGame `json:"removed"`
		}
		_ = json.Unmarshal(payload, &p)
		title = "Game library changes"
		body = formatDiff(p.Added, p.Removed)

	case TypeImportCompleted:
		title, body = "Import completed", "Your import finished successfully."
	case TypeImportFailed:
		title, body = "Import failed", failBody(payload, "Your import failed")
	case TypeExportCompleted:
		title, body = "Export completed", "Your export is ready."
	case TypeExportFailed:
		title, body = "Export failed", failBody(payload, "Your export failed")

	case TypeAdminBackupCompleted:
		title, body = "Backup completed", "A scheduled backup completed successfully."
	case TypeAdminBackupFailed:
		title, body = "Backup failed", failBody(payload, "A scheduled backup failed")
	case TypeAdminMaintCompleted:
		title, body = "Maintenance completed", maintBody(payload, "Maintenance task completed")
	case TypeAdminMaintFailed:
		title, body = "Maintenance failed", maintBody(payload, "Maintenance task failed")

	default:
		title = label
		body = "An event occurred: " + eventType
	}

	if title == "" {
		title = label
	}
	if body == "" {
		body = "An event occurred: " + eventType
	}
	return title, body
}

func formatDiff(added, removed []DiffGame) string {
	var b strings.Builder
	if len(added) > 0 {
		b.WriteString(fmt.Sprintf("Added (%d):\n", len(added)))
		for _, g := range added {
			b.WriteString(fmt.Sprintf("  + %s [%s]\n", g.Title, strings.Join(g.Platforms, ", ")))
		}
	}
	if len(removed) > 0 {
		b.WriteString(fmt.Sprintf("Removed (%d):\n", len(removed)))
		for _, g := range removed {
			b.WriteString(fmt.Sprintf("  - %s [%s]\n", g.Title, strings.Join(g.Platforms, ", ")))
		}
	}
	if b.Len() == 0 {
		return "No changes."
	}
	return strings.TrimRight(b.String(), "\n")
}

func failBody(payload json.RawMessage, prefix string) string {
	var p struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(payload, &p)
	if p.Error == "" {
		return prefix + "."
	}
	return prefix + ": " + p.Error
}

func maintBody(payload json.RawMessage, prefix string) string {
	var p struct {
		Action string `json:"action"`
		Error  string `json:"error"`
	}
	_ = json.Unmarshal(payload, &p)
	parts := []string{prefix}
	if p.Action != "" {
		parts = append(parts, "("+p.Action+")")
	}
	if p.Error != "" {
		parts = append(parts, "- "+p.Error)
	}
	return strings.Join(parts, " ") + "."
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/notify/ -run TestFormat -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/notify/formatters.go internal/notify/formatters_test.go
git commit -m "feat: notification event formatters"
```

---

### Task 11: NotifyWorker + recipient resolution

**Files:**
- Create: `internal/notify/worker.go`
- Test: `internal/notify/worker_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/worker_test.go`:
```go
package notify

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// insertUser creates a user row; returns the id.
func insertUser(t *testing.T, username string, admin bool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES (?, ?, 'x', ?)`,
		id, username, admin,
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertChannel(t *testing.T, userID, name, url string, enc *Encrypter) {
	t.Helper()
	ct, err := enc.Encrypt([]byte(url))
	if err != nil {
		t.Fatal(err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO notification_channels (id, user_id, name, encrypted_url, created_at) VALUES (?, ?, ?, ?, now())`,
		uuid.NewString(), userID, name, ct,
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func subscribe(t *testing.T, userID, eventType string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO notification_subscriptions (user_id, event_type, created_at) VALUES (?, ?, now())`,
		userID, eventType,
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNotifyWorkerUserScopeDelivers(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := mustEncrypter(t)
	rec := NewRecorderSender()

	uid := insertUser(t, "alice", false)
	insertChannel(t, uid, "phone", "noop://alice", enc)
	subscribe(t, uid, TypeSyncFailed)

	eventID := insertEvent(t, TypeSyncFailed, ScopeUser, &uid, `{"storefront":"steam"}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(rec.Sent()))
	}
}

func TestNotifyWorkerUserNotSubscribedNoDelivery(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := mustEncrypter(t)
	rec := NewRecorderSender()

	uid := insertUser(t, "bob", false)
	insertChannel(t, uid, "phone", "noop://bob", enc)
	// no subscribe()

	eventID := insertEvent(t, TypeSyncFailed, ScopeUser, &uid, `{}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 0 {
		t.Fatalf("expected 0 deliveries for unsubscribed user, got %d", len(rec.Sent()))
	}
}

func TestNotifyWorkerAdminScopeFansOut(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := mustEncrypter(t)
	rec := NewRecorderSender()

	a1 := insertUser(t, "admin1", true)
	a2 := insertUser(t, "admin2", true)
	nonAdmin := insertUser(t, "user1", false)
	insertChannel(t, a1, "c", "noop://a1", enc)
	insertChannel(t, a2, "c", "noop://a2", enc)
	insertChannel(t, nonAdmin, "c", "noop://u1", enc)
	subscribe(t, a1, TypeAdminBackupFailed)
	subscribe(t, a2, TypeAdminBackupFailed)
	subscribe(t, nonAdmin, TypeAdminBackupFailed) // must be ignored: non-admin

	eventID := insertEvent(t, TypeAdminBackupFailed, ScopeAdmin, nil, `{}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 2 {
		t.Fatalf("expected 2 admin deliveries (non-admin excluded), got %d", len(rec.Sent()))
	}
}

func TestNotifyWorkerSendFailureSwallowed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := mustEncrypter(t)
	rec := NewRecorderSender()
	rec.Err = errContext("boom")

	uid := insertUser(t, "carol", false)
	insertChannel(t, uid, "phone", "noop://carol", enc)
	subscribe(t, uid, TypeSyncFailed)
	eventID := insertEvent(t, TypeSyncFailed, ScopeUser, &uid, `{}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	// Send failure must NOT fail the job (fire-and-forget).
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatalf("send failure should be swallowed, got err: %v", err)
	}
}

// --- small test helpers ---

func mustEncrypter(t *testing.T) *Encrypter {
	t.Helper()
	enc, err := NewEncrypter("test-key-test-key-test-key-test-key")
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func insertEvent(t *testing.T, typ, scope string, actor *string, payload string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO events (id, type, scope, actor_user_id, payload, occurred_at) VALUES (?, ?, ?, ?, ?::jsonb, ?)`,
		id, typ, scope, actor, payload, time.Now().UTC(),
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type errContext string

func (e errContext) Error() string { return string(e) }
```

(Note: `Encrypter`/`NewEncrypter` referenced above come from `internal/crypto`. To avoid an import-name mismatch, the worker test imports it — adjust the helper to `crypto.NewEncrypter` and alias the type, OR re-export. Simplest: change `mustEncrypter` to return `*crypto.Encrypter` and add `import "…/internal/crypto"`, using `crypto.Encrypter` throughout. Use that form when writing the file.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notify/ -run TestNotifyWorker -v`
Expected: FAIL — `NotifyWorker` undefined.

- [ ] **Step 3: Write worker.go**

`internal/notify/worker.go`:
```go
package notify

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
)

// (Confirm the module path with `head -1 go.mod`; replace the import prefix
// above if it differs.)

// NotifyWorker delivers one event to all subscribed recipients' channels.
type NotifyWorker struct {
	river.WorkerDefaults[NotifyArgs]
	DB        *bun.DB
	Encrypter *crypto.Encrypter
	Sender    Sender
}

// Work loads the event, resolves recipients, renders, and sends. Per-channel
// failures are logged and swallowed; the job only errors on infra failures
// (which River will retry per InsertOpts — here MaxAttempts:1, so effectively
// no retry).
func (w *NotifyWorker) Work(ctx context.Context, job *river.Job[NotifyArgs]) error {
	var ev models.Event
	if err := w.DB.NewSelect().Model(&ev).Where("id = ?", job.Args.EventID).Scan(ctx); err != nil {
		slog.Warn("notify: load event", "event_id", job.Args.EventID, "err", err)
		return nil // nothing recoverable; do not retry a missing event
	}

	recipients, err := w.resolveRecipients(ctx, &ev)
	if err != nil {
		slog.Error("notify: resolve recipients", "event_id", ev.ID, "type", ev.Type, "err", err)
		return nil
	}

	title, body := Format(ev.Type, ev.Payload)

	for _, userID := range recipients {
		channels, err := w.loadChannels(ctx, userID)
		if err != nil {
			slog.Warn("notify: load channels", "user_id", userID, "err", err)
			continue
		}
		for _, ch := range channels {
			plain, derr := w.Encrypter.Decrypt(ch.EncryptedURL)
			if derr != nil {
				slog.Warn("notify: decrypt channel url", "channel_id", ch.ID, "err", derr)
				continue
			}
			if serr := w.Sender.Send(ctx, string(plain), title, body); serr != nil {
				slog.Warn("notify: send", "channel_id", ch.ID, "type", ev.Type, "err", serr)
				continue
			}
			slog.Debug("notify: sent", "channel_id", ch.ID, "type", ev.Type)
		}
	}
	return nil
}

// resolveRecipients returns the user IDs to deliver to.
//   - user scope:  the actor, iff a subscription row exists.
//   - admin scope: every is_admin user with a subscription row.
func (w *NotifyWorker) resolveRecipients(ctx context.Context, ev *models.Event) ([]string, error) {
	if ev.Scope == ScopeAdmin {
		var ids []string
		err := w.DB.NewRaw(
			`SELECT u.id FROM users u
			   JOIN notification_subscriptions s ON s.user_id = u.id
			  WHERE u.is_admin = true AND s.event_type = ?`,
			ev.Type,
		).Scan(ctx, &ids)
		return ids, err
	}

	// user scope
	if ev.ActorUserID == nil {
		return nil, nil
	}
	var ids []string
	err := w.DB.NewRaw(
		`SELECT user_id FROM notification_subscriptions WHERE user_id = ? AND event_type = ?`,
		*ev.ActorUserID, ev.Type,
	).Scan(ctx, &ids)
	return ids, err
}

func (w *NotifyWorker) loadChannels(ctx context.Context, userID string) ([]models.NotificationChannel, error) {
	var channels []models.NotificationChannel
	err := w.DB.NewSelect().Model(&channels).Where("user_id = ?", userID).Order("created_at").Scan(ctx)
	return channels, err
}
```

If `Scan(ctx, &ids)` into a `[]string` is not supported by the Bun raw API in this version, replace with the slice-scan idiom already used in `internal/scheduler/orphaned_items.go` (scan into a `[]struct{ID string}` then map). Verify with the orphaned_items pattern before finalizing.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/notify/ -run TestNotifyWorker -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/notify/worker.go internal/notify/worker_test.go
git commit -m "feat: NotifyWorker with subscription-based recipient resolution"
```

---

### Task 12: PruneEvents worker

**Files:**
- Create: `internal/notify/prune.go`
- Test: `internal/notify/prune_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/prune_test.go`:
```go
package notify

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPruneEventsDeletesOld(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// old event (100 days ago) and fresh event (now)
	_, err := testDB.NewRaw(
		`INSERT INTO events (id, type, scope, payload, occurred_at)
		 VALUES (?, ?, ?, '{}'::jsonb, now() - interval '100 days')`,
		uuid.NewString(), TypeSyncFailed, ScopeUser,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO events (id, type, scope, payload, occurred_at)
		 VALUES (?, ?, ?, '{}'::jsonb, now())`,
		uuid.NewString(), TypeSyncFailed, ScopeUser,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	PruneEvents(ctx, testDB, 90)

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events`).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event left after prune, got %d", count)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notify/ -run TestPruneEvents -v`
Expected: FAIL — `PruneEvents` undefined.

- [ ] **Step 3: Write prune.go**

`internal/notify/prune.go`:
```go
package notify

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"
)

// PruneEventsArgs is the River periodic job payload.
type PruneEventsArgs struct {
	RetentionDays int `json:"retention_days"`
}

// Kind implements river.JobArgs.
func (PruneEventsArgs) Kind() string { return "prune_events" }

// PruneEventsWorker deletes events older than the retention window. On
// completion it emits admin.maintenance.completed (or .failed on error).
type PruneEventsWorker struct {
	river.WorkerDefaults[PruneEventsArgs]
	DB *bun.DB
}

// Work runs the prune and emits a maintenance event.
func (w *PruneEventsWorker) Work(ctx context.Context, job *river.Job[PruneEventsArgs]) error {
	days := job.Args.RetentionDays
	if days <= 0 {
		days = 90
	}
	PruneEvents(ctx, w.DB, days)
	return nil
}

// PruneEvents deletes events older than retentionDays and emits a maintenance
// event describing the outcome.
func PruneEvents(ctx context.Context, db *bun.DB, retentionDays int) {
	res, err := db.NewRaw(
		`DELETE FROM events WHERE occurred_at < now() - (? || ' days')::interval`,
		retentionDays,
	).Exec(ctx)
	if err != nil {
		slog.Error("notify: prune events", "err", err)
		Emit(ctx, db, EmitParams{
			Type:    TypeAdminMaintFailed,
			Scope:   ScopeAdmin,
			Payload: map[string]any{"action": "prune_events", "error": err.Error()},
		})
		return
	}
	rows, _ := res.RowsAffected() //nolint:errcheck // advisory count only
	slog.Info("notify: pruned events", "count", rows)
	Emit(ctx, db, EmitParams{
		Type:    TypeAdminMaintCompleted,
		Scope:   ScopeAdmin,
		Payload: map[string]any{"action": "prune_events", "count": rows},
	})
	_ = fmt.Sprint // (remove if fmt unused)
}
```

Remove the unused `fmt` import/line if not needed once written.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/notify/ -run TestPruneEvents -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notify/prune.go internal/notify/prune_test.go
git commit -m "feat: events prune worker emitting maintenance events"
```

---

# Phase 3 — Wiring & emission points

### Task 13: Register workers + Sender + periodic prune in serve.go

**Files:**
- Modify: `cmd/nexorious/serve.go`
- Modify: `internal/scheduler/scheduler.go` (`BuildPeriodicJobs`)

- [ ] **Step 1: Register the workers**

In `cmd/nexorious/serve.go`, in the `workers := river.NewWorkers()` block (alongside the other `river.AddWorker` calls ~lines 188–207), add:
```go
	river.AddWorker(workers, &notify.NotifyWorker{DB: db, Encrypter: encrypter, Sender: notify.NewShoutrrrSender()})
	river.AddWorker(workers, &notify.PruneEventsWorker{DB: db})
```
Add the import `"github.com/drzero42/nexorious/internal/notify"` (confirm module prefix via `head -1 go.mod`).

- [ ] **Step 2: Set the package-level River client (startup + rebuild)**

After `riverClient` is constructed (~line 214, right after the `river.NewClient` error check) add:
```go
	notify.SetRiverClient(riverClient)
```
In the `RebuildServices` path, after `riverClient = newClient` (~line 305) add the same line:
```go
	notify.SetRiverClient(riverClient)
```

- [ ] **Step 3: Add the prune periodic job**

In `internal/scheduler/scheduler.go`, `BuildPeriodicJobs`, add to the returned slice (mirroring the cleanup jobs):
```go
		river.NewPeriodicJob(
			mustCron("0 5 * * *"),
			func() (river.JobArgs, *river.InsertOpts) {
				return notify.PruneEventsArgs{RetentionDays: cfg.NotifyEventsRetentionDays}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
```
Add import `"github.com/drzero42/nexorious/internal/notify"` to scheduler.go.

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/serve.go internal/scheduler/scheduler.go
git commit -m "feat: register NotifyWorker, PruneEventsWorker, and daily prune job"
```

---

### Task 14: Emit sync events in sync.go

**Files:**
- Modify: `internal/worker/tasks/sync.go` (`SyncCheckJobCompletion` ~939–973, `failSyncJob` ~328–342)
- Test: `internal/worker/tasks/sync_test.go` (extend; uses existing `testDB`)

Emission semantics: in `SyncCheckJobCompletion`, after the job is finalized to `completed`, count failed items → emit `sync.completed_with_errors` if any failed else `sync.completed` (dedup `"<job>:sync.completed"` / `"<job>:sync.completed_with_errors"`); then emit `sync.diff` if `sync_changes` rows exist (dedup `"<job>:sync.diff"`). When the early-return for `pendingReviewCount > 0` fires, emit `sync.needs_review` (dedup `"<job>:sync.needs_review"`). In `failSyncJob`, emit `sync.failed` (dedup `"<job>:sync.failed"`). Recipient is the job's `user_id` (look up alongside the existing queries).

- [ ] **Step 1: Write the failing test**

Add to `internal/worker/tasks/sync_test.go`:
```go
func TestSyncCheckJobCompletionEmitsCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	uid := insertTestUser(t, "syncuser") // existing helper; if absent, insert a user row inline
	jobID := uuid.NewString()
	// a job in 'processing' with dispatch_complete=true and one completed item
	mustExec(t, `INSERT INTO jobs (id, user_id, job_type, source, status, priority, dispatch_complete, created_at)
	             VALUES (?, ?, 'sync', 'steam', 'processing', 'low', true, now())`, jobID, uid)
	mustExec(t, `INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
	             VALUES (?, ?, ?, 'k', 't', '{}'::jsonb, 'completed', '{}'::jsonb, '[]'::jsonb, now())`, uuid.NewString(), jobID, uid)

	SyncCheckJobCompletion(ctx, testDB, jobID)

	var typ string
	if err := testDB.NewRaw(`SELECT type FROM events WHERE dedup_key = ?`, jobID+":sync.completed").Scan(ctx, &typ); err != nil {
		t.Fatalf("expected sync.completed event: %v", err)
	}
}

func TestSyncCheckJobCompletionEmitsCompletedWithErrors(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)
	uid := insertTestUser(t, "syncuser2")
	jobID := uuid.NewString()
	mustExec(t, `INSERT INTO jobs (id, user_id, job_type, source, status, priority, dispatch_complete, created_at)
	             VALUES (?, ?, 'sync', 'steam', 'processing', 'low', true, now())`, jobID, uid)
	mustExec(t, `INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
	             VALUES (?, ?, ?, 'k', 't', '{}'::jsonb, 'failed', '{}'::jsonb, '[]'::jsonb, now())`, uuid.NewString(), jobID, uid)

	SyncCheckJobCompletion(ctx, testDB, jobID)

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.completed_with_errors").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected sync.completed_with_errors, got %d", count)
	}
}
```
(If `insertTestUser`/`mustExec` helpers don't exist in `sync_test.go`, add small local helpers using `testDB.NewRaw(...).Exec` — follow the existing test style in that file.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestSyncCheckJobCompletionEmits -v`
Expected: FAIL — events not emitted (no `notify` import / no emission code).

- [ ] **Step 3: Implement emission in SyncCheckJobCompletion**

In `internal/worker/tasks/sync.go`, add import `"github.com/drzero42/nexorious/internal/notify"`.

Replace the `pendingReviewCount > 0` early-return block to emit needs-review first:
```go
	if pendingReviewCount > 0 {
		userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)
		notify.Emit(ctx, db, notify.EmitParams{
			Type:        notify.TypeSyncNeedsReview,
			Scope:       notify.ScopeUser,
			ActorUserID: userID,
			Payload:     map[string]any{"storefront": storefront, "count": pendingReviewCount, "job_id": jobID},
			DedupKey:    jobID + ":" + notify.TypeSyncNeedsReview,
		})
		return
	}
```

After the finalize `UPDATE jobs SET status = 'completed' …` succeeds, append:
```go
	userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)

	var failedCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'failed'`, jobID,
	).Scan(ctx, &failedCount); err != nil {
		slog.Error("sync: count failed items for notify", "job_id", jobID, "err", err)
	}

	if failedCount > 0 {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncCompletedWithErrors, Scope: notify.ScopeUser, ActorUserID: userID,
			Payload:  map[string]any{"storefront": storefront, "failed": failedCount, "job_id": jobID},
			DedupKey: jobID + ":" + notify.TypeSyncCompletedWithErrors,
		})
	} else {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncCompleted, Scope: notify.ScopeUser, ActorUserID: userID,
			Payload:  map[string]any{"storefront": storefront, "job_id": jobID},
			DedupKey: jobID + ":" + notify.TypeSyncCompleted,
		})
	}

	emitSyncDiff(ctx, db, jobID, userID)
```

Add two helpers at the bottom of `sync.go`:
```go
// syncJobUserAndStorefront fetches the owning user_id and storefront (source)
// for a job. Returns ("","") on error.
func syncJobUserAndStorefront(ctx context.Context, db *bun.DB, jobID string) (userID, storefront string) {
	var row struct {
		UserID string `bun:"user_id"`
		Source string `bun:"source"`
	}
	if err := db.NewRaw(`SELECT user_id, source FROM jobs WHERE id = ?`, jobID).Scan(ctx, &row); err != nil {
		slog.Error("sync: lookup job user/storefront", "job_id", jobID, "err", err)
		return "", ""
	}
	return row.UserID, row.Source
}

// emitSyncDiff emits sync.diff iff sync_changes rows exist for the job.
func emitSyncDiff(ctx context.Context, db *bun.DB, jobID, userID string) {
	var rows []struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := db.NewRaw(
		`SELECT change_type, title FROM sync_changes WHERE job_id = ? AND change_type IN ('added','removed') ORDER BY created_at`,
		jobID,
	).Scan(ctx, &rows); err != nil {
		slog.Error("sync: load sync_changes for diff notify", "job_id", jobID, "err", err)
		return
	}
	if len(rows) == 0 {
		return // no-op sync: do not emit
	}
	added := []map[string]any{}
	removed := []map[string]any{}
	for _, r := range rows {
		entry := map[string]any{"title": r.Title, "platforms": []string{}}
		if r.ChangeType == "added" {
			added = append(added, entry)
		} else {
			removed = append(removed, entry)
		}
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeSyncDiff, Scope: notify.ScopeUser, ActorUserID: userID,
		Payload:  map[string]any{"added": added, "removed": removed, "job_id": jobID},
		DedupKey: jobID + ":" + notify.TypeSyncDiff,
	})
}
```
(`sync_changes` has no platform column directly; platforms per change are out of cheap reach. Emit empty `platforms` arrays — the formatter renders `[]` gracefully. If per-platform detail is wanted later, join `external_games`/`user_game_platforms`; that is a follow-up, not blocking.)

In `failSyncJob`, after the `UPDATE jobs SET status = 'failed' …`, append:
```go
	userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeSyncFailed, Scope: notify.ScopeUser, ActorUserID: userID,
		Payload:  map[string]any{"storefront": storefront, "error": msg, "job_id": jobID},
		DedupKey: jobID + ":" + notify.TypeSyncFailed,
	})
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/worker/tasks/ -run TestSyncCheckJobCompletionEmits -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: emit sync notification events"
```

---

### Task 15: Emit import/export events

**Files:**
- Modify: `internal/worker/tasks/export.go` (`markJobCompleted` ~150, `markJobFailed` ~136)
- Modify: `internal/worker/tasks/import_item.go` (`checkJobCompletion` ~472)
- Test: `internal/worker/tasks/export_test.go` (extend or create)

Export jobs carry a `user_id` on the `models.Job` already in scope. Import's `checkJobCompletion` only has `jobID`; fetch `user_id` with a small query (reuse the `syncJobUserAndStorefront` helper from Task 14 — it lives in the same package — or add a tiny `jobUserID` helper).

- [ ] **Step 1: Write the failing test**

Add to `internal/worker/tasks/export_test.go`:
```go
func TestMarkJobCompletedEmitsExportCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)
	uid := insertTestUser(t, "exporter")
	jobID := uuid.NewString()
	mustExec(t, `INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
	             VALUES (?, ?, 'export', 'system', 'processing', 'low', now())`, jobID, uid)
	job := &models.Job{ID: jobID, UserID: uid, Status: models.JobStatusProcessing}

	markJobCompleted(ctx, testDB, job, "/tmp/x.json")

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ? AND actor_user_id = ?`, notify.TypeExportCompleted, uid).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected export.completed, got %d", count)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestMarkJobCompletedEmits -v`
Expected: FAIL — no emission.

- [ ] **Step 3: Implement emission**

In `export.go` add import `notify`. In `markJobCompleted`, after the update, append (the function must determine import-vs-export — export workers always set export types; use `job.JobType` to pick import vs export if this helper is shared, but it is export-only):
```go
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeExportCompleted, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  map[string]any{"job_id": job.ID, "file_path": filePath},
		DedupKey: job.ID + ":" + notify.TypeExportCompleted,
	})
```
In `markJobFailed`, after the update:
```go
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeExportFailed, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  map[string]any{"job_id": job.ID, "error": errMsg},
		DedupKey: job.ID + ":" + notify.TypeExportFailed,
	})
```
(Confirm `markJobFailed`/`markJobCompleted` are only used by export workers via `grep -rn "markJobFailed\|markJobCompleted" internal/worker/tasks/`. If import also uses them, branch on `job.JobType == models.JobTypeImport` to emit `import.*` instead. Per the exploration, export uses these; import uses `checkJobCompletion`. Verify before finalizing.)

In `import_item.go`, in `checkJobCompletion`, after the successful `UPDATE jobs SET status = ?`:
```go
	uid := jobUserID(ctx, db, jobID)
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeImportCompleted, Scope: notify.ScopeUser, ActorUserID: uid,
		Payload:  map[string]any{"job_id": jobID},
		DedupKey: jobID + ":" + notify.TypeImportCompleted,
	})
```
`checkJobCompletion` currently takes `(db *bun.DB, jobID string)` and uses `context.Background()` internally; reuse that ctx. Add a `jobUserID(ctx, db, jobID) string` helper (or reuse `syncJobUserAndStorefront` and ignore storefront). Add `notify` import to `import_item.go`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/worker/tasks/ -run 'TestMarkJobCompletedEmits' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/import_item.go internal/worker/tasks/export_test.go
git commit -m "feat: emit import/export notification events"
```

---

### Task 16: Emit admin backup + maintenance events

**Files:**
- Modify: `internal/scheduler/backup_poll.go` (`Work` ~26–58)
- Modify: `internal/scheduler/stale_jobs.go` (`CleanupStaleJobs`)
- Modify: `internal/scheduler/orphaned_items.go` (`RescueOrphanedPendingItems`)
- Modify: `internal/worker/tasks/metadata_refresh.go` (dispatch + completion paths)
- Test: `internal/scheduler/backup_poll_test.go` (extend or create)

These are admin-scoped (`ActorUserID` empty, `DedupKey` empty = repeatable).

- [ ] **Step 1: Write the failing test**

Add to `internal/scheduler/backup_poll_test.go` (or create it; mirror existing scheduler test setup):
```go
func TestBackupFailureEmitsAdminEvent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)
	// directly exercise the emit helper used on the failure path:
	notify.Emit(ctx, testDB, notify.EmitParams{
		Type: notify.TypeAdminBackupFailed, Scope: notify.ScopeAdmin,
		Payload: map[string]any{"error": "disk full"},
	})
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ?`, notify.TypeAdminBackupFailed).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected admin.backup.failed, got %d", count)
	}
}
```
(This documents the contract; the substantive coverage is the worker integration. Keep it minimal — the real logic is the emit calls below.)

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/scheduler/ -run TestBackupFailureEmitsAdminEvent -v`
Expected: FAIL — `notify` not imported in scheduler tests / package.

- [ ] **Step 3: Add emissions**

`backup_poll.go` (`Work`), on the `CreateBackup` error path:
```go
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminBackupFailed, Scope: notify.ScopeAdmin,
			Payload: map[string]any{"error": err.Error()},
		})
```
and on success after `slog.Info("scheduled backup created", …)`:
```go
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminBackupCompleted, Scope: notify.ScopeAdmin,
			Payload: map[string]any{"backup_id": id},
		})
```

`stale_jobs.go` (`CleanupStaleJobs`): on the sync-cleanup error path emit `admin.maintenance.failed` with `{"action":"stale_jobs_cleanup","error":err.Error()}`; at the end (after both updates succeed) emit `admin.maintenance.completed` with `{"action":"stale_jobs_cleanup","count": rows+syncRows}`. (Keep `count` accurate by capturing both `RowsAffected` values.)

`orphaned_items.go` (`RescueOrphanedPendingItems`): on the query-error path emit `admin.maintenance.failed` `{"action":"rescue_orphaned_items","error":...}`; at the end emit `admin.maintenance.completed` `{"action":"rescue_orphaned_items","rescued":successCount,"failed":failureCount}`.

`metadata_refresh.go`: on the transaction-failure path emit `admin.maintenance.failed` `{"action":"metadata_refresh_dispatch","error":...}`; in `metaRefreshCheckJobCompletion` after the completed update, emit `admin.maintenance.completed` `{"action":"metadata_refresh","job_id":jobID}`.

Add `notify` import to each modified file. Each emit uses `Scope: notify.ScopeAdmin`, empty `ActorUserID`, empty `DedupKey`.

To avoid repetition, add one helper in `internal/scheduler/scheduler.go`:
```go
// emitMaint emits an admin.maintenance.{completed,failed} event.
func emitMaint(ctx context.Context, db *bun.DB, failed bool, action string, extra map[string]any) {
	typ := notify.TypeAdminMaintCompleted
	if failed {
		typ = notify.TypeAdminMaintFailed
	}
	payload := map[string]any{"action": action}
	for k, v := range extra {
		payload[k] = v
	}
	notify.Emit(ctx, db, notify.EmitParams{Type: typ, Scope: notify.ScopeAdmin, Payload: payload})
}
```
Use `emitMaint(ctx, db, false, "stale_jobs_cleanup", map[string]any{"count": total})` etc. (`metadata_refresh.go` is in package `tasks`, not `scheduler` — define a sibling `emitMaint` there or inline the `notify.Emit` call; do not cross packages for a private helper.)

- [ ] **Step 4: Run the test + build**

Run: `go test ./internal/scheduler/ -run TestBackupFailureEmitsAdminEvent -v && go build ./...`
Expected: PASS + build success.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/backup_poll.go internal/scheduler/stale_jobs.go internal/scheduler/orphaned_items.go internal/scheduler/scheduler.go internal/worker/tasks/metadata_refresh.go internal/scheduler/backup_poll_test.go
git commit -m "feat: emit admin backup and maintenance notification events"
```

---

# Phase 4 — API & subscription seeding

### Task 17: Default-subscription seeding on user creation

**Files:**
- Modify: `internal/api/admin_users.go` (`HandleCreate`, after the user insert ~166)
- Modify: `internal/api/setup.go` (after the initial-admin insert ~123–131)
- Create helper: `internal/notify/seed.go`
- Test: `internal/notify/seed_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/seed_test.go`:
```go
package notify

import (
	"context"
	"testing"
)

func TestSeedDefaultSubscriptions(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertUser(t, "seeded", false) // helper from worker_test.go (same package)

	if err := SeedDefaultSubscriptions(ctx, testDB, uid); err != nil {
		t.Fatal(err)
	}

	var got []string
	if err := testDB.NewRaw(`SELECT event_type FROM notification_subscriptions WHERE user_id = ?`, uid).Scan(ctx, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(DefaultSubscriptions()) {
		t.Fatalf("expected %d default subs, got %d", len(DefaultSubscriptions()), len(got))
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertUser(t, "seeded2", false)
	if err := SeedDefaultSubscriptions(ctx, testDB, uid); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaultSubscriptions(ctx, testDB, uid); err != nil {
		t.Fatalf("second seed should not error: %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/notify/ -run TestSeed -v`
Expected: FAIL — `SeedDefaultSubscriptions` undefined.

- [ ] **Step 3: Write seed.go**

`internal/notify/seed.go`:
```go
package notify

import (
	"context"

	"github.com/uptrace/bun"
)

// SeedDefaultSubscriptions inserts the default-on event types for a user.
// Idempotent (ON CONFLICT DO NOTHING).
func SeedDefaultSubscriptions(ctx context.Context, db bun.IDB, userID string) error {
	for _, eventType := range DefaultSubscriptions() {
		if _, err := db.NewRaw(
			`INSERT INTO notification_subscriptions (user_id, event_type, created_at)
			 VALUES (?, ?, now()) ON CONFLICT (user_id, event_type) DO NOTHING`,
			userID, eventType,
		).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
```
(`bun.IDB` lets callers pass either `*bun.DB` or a `bun.Tx`. Verify `bun.IDB` exists in the pinned Bun version with `go doc github.com/uptrace/bun.IDB`; if not, accept `*bun.DB` and call after commit.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/notify/ -run TestSeed -v`
Expected: PASS (both).

- [ ] **Step 5: Wire into user creation**

In `internal/api/admin_users.go` `HandleCreate`, after `h.db.NewInsert().Model(user).Exec(ctx)` succeeds:
```go
	if err := notify.SeedDefaultSubscriptions(ctx, h.db, user.ID); err != nil {
		slog.Error("admin create user: seed notification subscriptions", "user_id", user.ID, "err", err)
	}
```
In `internal/api/setup.go`, after the initial-admin user is inserted (within or after the tx; if `bun.IDB` works, pass `tx`, else call with `h.db` after commit using the returned `userID`):
```go
	if err := notify.SeedDefaultSubscriptions(ctx, db, userID); err != nil {
		slog.Error("setup: seed notification subscriptions", "user_id", userID, "err", err)
	}
```
Add the `notify` import to both files. (setup.go uses raw `database/sql` via `tx` — if `bun.IDB` is incompatible there, call `notify.SeedDefaultSubscriptions(ctx, h.db, userID)` after `tx.Commit()`.)

- [ ] **Step 6: Verify build + commit**

Run: `go build ./...`
```bash
git add internal/notify/seed.go internal/notify/seed_test.go internal/api/admin_users.go internal/api/setup.go
git commit -m "feat: seed default notification subscriptions on user creation"
```

---

### Task 18: Notifications API handlers

**Files:**
- Create: `internal/api/notifications.go`
- Test: `internal/api/notifications_test.go`

Endpoints (all under `/api/notifications`, auth required):
- `GET /channels` — list (no URL exposed)
- `POST /channels` — create (encrypt URL)
- `PATCH /channels/:id` — rename / replace URL
- `DELETE /channels/:id`
- `POST /channels/:id/test` — synchronous test send via saved channel
- `GET /event-types` — registry; admin types only for admins
- `GET /subscriptions` — list current subscribed types
- `PUT /subscriptions` — full replace; rejects `admin.*` for non-admins
- `POST /subscriptions/reset` — destructive reset to defaults

- [ ] **Step 1: Write the failing tests**

`internal/api/notifications_test.go` — mirror existing API test setup (shared `testDB`, an authenticated test context helper). Key tests:
```go
func TestCreateChannelEncryptsURL(t *testing.T) { /* POST /channels, then assert DB column starts with "enc:v1:" and GET never returns the url */ }
func TestListChannelsHidesURL(t *testing.T)      { /* GET /channels returns name + id, no url field populated */ }
func TestCreateChannelRejectsBlankName(t *testing.T) { /* 400 */ }
func TestPutSubscriptionsRejectsAdminForNonAdmin(t *testing.T) { /* non-admin PUT with "admin.backup.failed" → 400 */ }
func TestPutSubscriptionsReplacesSet(t *testing.T) { /* PUT [a,b] then PUT [b,c] → only b,c remain */ }
func TestResetSubscriptionsRestoresDefaults(t *testing.T) { /* after reset, rows == DefaultSubscriptions() */ }
func TestEventTypesHidesAdminFromNonAdmin(t *testing.T) { /* GET /event-types as non-admin excludes admin.* */ }
func TestChannelOwnershipEnforced(t *testing.T) { /* user B cannot DELETE user A's channel → 404 */ }
```
Write these out fully following the binding/assertion idioms in `internal/api/tags_test.go` (read it for the exact authenticated-request helper and JSON assertion style before writing). Each test must construct the handler with a real `*crypto.Encrypter` (use `crypto.NewEncrypter("test-key-...")`) and a recorder `Sender` for the test-send endpoint.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestCreateChannel|TestListChannels|TestPutSubscriptions|TestResetSubscriptions|TestEventTypes|TestChannelOwnership' -v`
Expected: FAIL — handlers undefined.

- [ ] **Step 3: Write notifications.go**

`internal/api/notifications.go` — follow the tags.go handler patterns exactly (`auth.UserIDFromContext`, `auth.IsAdminFromContext`, `c.Bind`, inline validation, `WHERE … AND user_id = ?`, `isDuplicateKeyError` → 409, `sql.ErrNoRows` → 404). Full implementation:
```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/uptrace/bun"
)

// NotificationsHandler serves notification channel + subscription endpoints.
type NotificationsHandler struct {
	db        *bun.DB
	encrypter *crypto.Encrypter
	sender    notify.Sender
}

// NewNotificationsHandler constructs a NotificationsHandler.
func NewNotificationsHandler(db *bun.DB, encrypter *crypto.Encrypter, sender notify.Sender) *NotificationsHandler {
	return &NotificationsHandler{db: db, encrypter: encrypter, sender: sender}
}

type channelResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type createChannelRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type updateChannelRequest struct {
	Name *string `json:"name"`
	URL  *string `json:"url"`
}

func (h *NotificationsHandler) HandleListChannels(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var rows []channelResponse
	if err := h.db.NewRaw(
		`SELECT id, name, created_at FROM notification_channels WHERE user_id = ? ORDER BY created_at`, userID,
	).Scan(context.Background(), &rows); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list channels")
	}
	if rows == nil {
		rows = []channelResponse{}
	}
	return c.JSON(http.StatusOK, rows)
}

func (h *NotificationsHandler) HandleCreateChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req createChannelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "url is required")
	}
	ciphertext, err := h.encrypter.Encrypt([]byte(req.URL))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt url")
	}
	id := uuid.NewString()
	var out channelResponse
	err = h.db.NewRaw(
		`INSERT INTO notification_channels (id, user_id, name, encrypted_url, created_at)
		 VALUES (?, ?, ?, ?, now())
		 RETURNING id, name, created_at`,
		id, userID, req.Name, ciphertext,
	).Scan(context.Background(), &out)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "a channel with that name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create channel")
	}
	return c.JSON(http.StatusCreated, out)
}

func (h *NotificationsHandler) HandleUpdateChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	var req updateChannelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	setClauses := []string{}
	args := []any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name cannot be empty")
		}
		setClauses = append(setClauses, "name = ?")
		args = append(args, name)
	}
	if req.URL != nil {
		url := strings.TrimSpace(*req.URL)
		if url == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "url cannot be empty")
		}
		ct, err := h.encrypter.Encrypt([]byte(url))
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt url")
		}
		setClauses = append(setClauses, "encrypted_url = ?")
		args = append(args, ct)
	}
	if len(setClauses) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no fields to update")
	}
	args = append(args, id, userID)
	var out channelResponse
	query := `UPDATE notification_channels SET ` + strings.Join(setClauses, ", ") +
		` WHERE id = ? AND user_id = ? RETURNING id, name, created_at`
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &out)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "a channel with that name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update channel")
	}
	return c.JSON(http.StatusOK, out)
}

func (h *NotificationsHandler) HandleDeleteChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	res, err := h.db.NewRaw(
		`DELETE FROM notification_channels WHERE id = ? AND user_id = ?`, id, userID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete channel")
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *NotificationsHandler) HandleTestChannel(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	var encURL string
	err := h.db.NewRaw(
		`SELECT encrypted_url FROM notification_channels WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(context.Background(), &encURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel")
	}
	plain, err := h.encrypter.Decrypt(encURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to decrypt channel url")
	}
	if err := h.sender.Send(context.Background(), string(plain), "Nexorious test notification", "This is a test notification from Nexorious."); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "test send failed: "+err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *NotificationsHandler) HandleListEventTypes(c *echo.Context) error {
	if auth.UserIDFromContext(c) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	isAdmin := auth.IsAdminFromContext(c)
	out := []notify.EventTypeMeta{}
	for _, m := range notify.Registry() {
		if m.Scope == notify.ScopeAdmin && !isAdmin {
			continue
		}
		out = append(out, m)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *NotificationsHandler) HandleListSubscriptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var types []string
	if err := h.db.NewRaw(
		`SELECT event_type FROM notification_subscriptions WHERE user_id = ? ORDER BY event_type`, userID,
	).Scan(context.Background(), &types); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list subscriptions")
	}
	if types == nil {
		types = []string{}
	}
	return c.JSON(http.StatusOK, map[string][]string{"event_types": types})
}

type putSubscriptionsRequest struct {
	EventTypes []string `json:"event_types"`
}

func (h *NotificationsHandler) HandlePutSubscriptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	isAdmin := auth.IsAdminFromContext(c)
	var req putSubscriptionsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	// validate each type: known, and admin types only for admins
	seen := map[string]bool{}
	clean := []string{}
	for _, t := range req.EventTypes {
		if !notify.IsKnownType(t) {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown event type: "+t)
		}
		if notify.IsAdminType(t) && !isAdmin {
			return echo.NewHTTPError(http.StatusBadRequest, "not permitted to subscribe to "+t)
		}
		if !seen[t] {
			seen[t] = true
			clean = append(clean, t)
		}
	}
	ctx := context.Background()
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM notification_subscriptions WHERE user_id = ?`, userID).Exec(ctx); err != nil {
			return err
		}
		for _, t := range clean {
			if _, err := tx.NewRaw(
				`INSERT INTO notification_subscriptions (user_id, event_type, created_at) VALUES (?, ?, now())`,
				userID, t,
			).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update subscriptions")
	}
	return c.JSON(http.StatusOK, map[string][]string{"event_types": clean})
}

func (h *NotificationsHandler) HandleResetSubscriptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`DELETE FROM notification_subscriptions WHERE user_id = ?`, userID).Exec(ctx); err != nil {
			return err
		}
		return notify.SeedDefaultSubscriptions(ctx, tx, userID)
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset subscriptions")
	}
	return h.HandleListSubscriptions(c)
}
```
(If `Scan(ctx, &types)` into `[]string` is unsupported, use the struct-slice scan idiom from `orphaned_items.go`. Confirm the `RunInTx`/`tx.NewRaw` signatures against `user_games.go`'s bulk-delete usage.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestCreateChannel|TestListChannels|TestPutSubscriptions|TestResetSubscriptions|TestEventTypes|TestChannelOwnership' -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/api/notifications.go internal/api/notifications_test.go
git commit -m "feat: notifications API handlers"
```

---

### Task 19: Register routes + slumber + wire handler

**Files:**
- Modify: `internal/api/router.go` (route registration + handler construction; `New`/`registerRoutes` must thread a `notify.Sender`)
- Modify: `slumber.yaml`

The handler needs `encrypter` (already passed to `New`) and a `notify.Sender`. Construct `notify.NewShoutrrrSender()` inside `registerRoutes` (no new param needed) so production uses the real sender.

- [ ] **Step 1: Register routes**

In `internal/api/router.go` `registerRoutes`, alongside the tags group, add:
```go
	nh := NewNotificationsHandler(db, encrypter, notify.NewShoutrrrSender())
	notificationsGroup := e.Group("/api/notifications", auth.AuthMiddleware(db))
	// static before parameterized (Echo v5)
	notificationsGroup.GET("/channels", nh.HandleListChannels)
	notificationsGroup.POST("/channels", nh.HandleCreateChannel)
	notificationsGroup.POST("/channels/:id/test", nh.HandleTestChannel)
	notificationsGroup.PATCH("/channels/:id", nh.HandleUpdateChannel)
	notificationsGroup.DELETE("/channels/:id", nh.HandleDeleteChannel)
	notificationsGroup.GET("/event-types", nh.HandleListEventTypes)
	notificationsGroup.GET("/subscriptions", nh.HandleListSubscriptions)
	notificationsGroup.PUT("/subscriptions", nh.HandlePutSubscriptions)
	notificationsGroup.POST("/subscriptions/reset", nh.HandleResetSubscriptions)
```
Add `"github.com/drzero42/nexorious/internal/notify"` import to router.go. Note `/channels/:id/test` (static `/test` suffix on a param route) is registered before `/channels/:id` to satisfy Echo v5 ordering.

- [ ] **Step 2: Add slumber requests**

In `slumber.yaml`, add a `notifications:` domain folder (alphabetical position; after the appropriate neighbor), each request using `$ref: "#/.authenticated"`:
```yaml
  notifications:
    name: Notifications
    requests:
      list_channels:
        name: List Channels
        method: GET
        url: "{{base_url}}/api/notifications/channels"
        $ref: "#/.authenticated"
      create_channel:
        name: Create Channel
        method: POST
        url: "{{base_url}}/api/notifications/channels"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            name: "My phone"
            url: "ntfy://ntfy.sh/my-topic"
      update_channel:
        name: Update Channel
        method: PATCH
        url: "{{base_url}}/api/notifications/channels/REPLACE_ID"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            name: "Renamed"
      delete_channel:
        name: Delete Channel
        method: DELETE
        url: "{{base_url}}/api/notifications/channels/REPLACE_ID"
        $ref: "#/.authenticated"
      test_channel:
        name: Test Channel
        method: POST
        url: "{{base_url}}/api/notifications/channels/REPLACE_ID/test"
        $ref: "#/.authenticated"
      list_event_types:
        name: List Event Types
        method: GET
        url: "{{base_url}}/api/notifications/event-types"
        $ref: "#/.authenticated"
      list_subscriptions:
        name: List Subscriptions
        method: GET
        url: "{{base_url}}/api/notifications/subscriptions"
        $ref: "#/.authenticated"
      put_subscriptions:
        name: Replace Subscriptions
        method: PUT
        url: "{{base_url}}/api/notifications/subscriptions"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            event_types: ["sync.failed", "import.failed"]
      reset_subscriptions:
        name: Reset Subscriptions
        method: POST
        url: "{{base_url}}/api/notifications/subscriptions/reset"
        $ref: "#/.authenticated"
```

- [ ] **Step 3: Verify**

Run: `go build ./... && slumber collection`
Expected: build success; slumber reports the collection loads with no error.

- [ ] **Step 4: Commit**

```bash
git add internal/api/router.go slumber.yaml
git commit -m "feat: register notifications routes and slumber requests"
```

---

# Phase 5 — Frontend

### Task 20: Add Switch component + API client + hooks

**Files:**
- Create: `ui/frontend/src/components/ui/switch.tsx` (via shadcn)
- Create: `ui/frontend/src/api/notifications.ts`
- Create: `ui/frontend/src/hooks/use-notifications.ts`

- [ ] **Step 1: Add the Switch component**

Run (from `ui/frontend/`): `npx shadcn@latest add switch`
Expected: creates `src/components/ui/switch.tsx`.

- [ ] **Step 2: Write the API client**

`ui/frontend/src/api/notifications.ts`:
```ts
import { api } from './client';

export interface NotificationChannel {
  id: string;
  name: string;
  created_at: string;
}

export interface EventTypeMeta {
  type: string;
  scope: 'user' | 'admin';
  category: string;
  label: string;
  default_on: boolean;
}

export const notificationsApi = {
  listChannels: () => api.get<NotificationChannel[]>('/notifications/channels'),
  createChannel: (data: { name: string; url: string }) =>
    api.post<NotificationChannel>('/notifications/channels', data),
  updateChannel: (id: string, data: { name?: string; url?: string }) =>
    api.patch<NotificationChannel>(`/notifications/channels/${id}`, data),
  deleteChannel: (id: string) => api.delete<void>(`/notifications/channels/${id}`),
  testChannel: (id: string) => api.post<void>(`/notifications/channels/${id}/test`),
  listEventTypes: () => api.get<EventTypeMeta[]>('/notifications/event-types'),
  listSubscriptions: () =>
    api.get<{ event_types: string[] }>('/notifications/subscriptions'),
  putSubscriptions: (eventTypes: string[]) =>
    api.put<{ event_types: string[] }>('/notifications/subscriptions', {
      event_types: eventTypes,
    }),
  resetSubscriptions: () =>
    api.post<{ event_types: string[] }>('/notifications/subscriptions/reset'),
};
```
(Confirm the `api.patch`/`api.delete` signatures in `src/api/client.ts` — the exploration confirmed `get/post/put/patch/delete` exist; match their arg shapes.)

- [ ] **Step 3: Write the hooks**

`ui/frontend/src/hooks/use-notifications.ts`:
```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { notificationsApi } from '@/api/notifications';

export const notificationKeys = {
  all: ['notifications'] as const,
  channels: () => [...notificationKeys.all, 'channels'] as const,
  eventTypes: () => [...notificationKeys.all, 'event-types'] as const,
  subscriptions: () => [...notificationKeys.all, 'subscriptions'] as const,
};

export function useChannels() {
  return useQuery({ queryKey: notificationKeys.channels(), queryFn: notificationsApi.listChannels });
}

export function useEventTypes() {
  return useQuery({ queryKey: notificationKeys.eventTypes(), queryFn: notificationsApi.listEventTypes });
}

export function useSubscriptions() {
  return useQuery({ queryKey: notificationKeys.subscriptions(), queryFn: notificationsApi.listSubscriptions });
}

export function useCreateChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; url: string }) => notificationsApi.createChannel(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.channels() }),
  });
}

export function useUpdateChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { name?: string; url?: string } }) =>
      notificationsApi.updateChannel(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.channels() }),
  });
}

export function useDeleteChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => notificationsApi.deleteChannel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.channels() }),
  });
}

export function useTestChannel() {
  return useMutation({ mutationFn: (id: string) => notificationsApi.testChannel(id) });
}

export function usePutSubscriptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (eventTypes: string[]) => notificationsApi.putSubscriptions(eventTypes),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.subscriptions() }),
  });
}

export function useResetSubscriptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => notificationsApi.resetSubscriptions(),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.subscriptions() }),
  });
}
```

- [ ] **Step 4: Verify typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: no type errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/ui/switch.tsx ui/frontend/src/api/notifications.ts ui/frontend/src/hooks/use-notifications.ts
git commit -m "feat: notifications frontend API client + hooks"
```

---

### Task 21: Channel dialog + Notifications section

**Files:**
- Create: `ui/frontend/src/components/notifications/channel-dialog.tsx`
- Create: `ui/frontend/src/components/notifications/notifications-section.tsx`
- Test: `ui/frontend/src/components/notifications/notifications-section.test.tsx`

- [ ] **Step 1: Write the failing test**

`notifications-section.test.tsx` — render `<NotificationsSection />` inside a `QueryClientProvider`, mock `notificationsApi` (vi.mock), assert that event-type toggles render grouped by category and that toggling calls `putSubscriptions`. Follow the render/mocking idiom in an existing `*.test.tsx` (e.g. `game-card.test.tsx`). Minimum assertions:
```tsx
it('renders channel list and event-type toggles', async () => { /* expect a channel name + a toggle label */ });
it('hides admin event types for non-admin users', async () => { /* admin group absent */ });
```

- [ ] **Step 2: Run it to verify it fails**

Run (from `ui/frontend/`): `npm run test notifications-section.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the channel dialog**

`channel-dialog.tsx` — a `Dialog` with a React Hook Form + Zod form (`name` required, `url` required; for edit, `url` optional/blank = keep existing). On submit call `useCreateChannel`/`useUpdateChannel`; show `toast.success`/`toast.error`. Include a "Send test" button that calls `useTestChannel` for saved channels (edit mode only). Follow `steam-connection-card.tsx` for the RHF+Zod+toast idiom and `tags.tsx` for the dialog open/close state pattern.

- [ ] **Step 4: Write the section**

`notifications-section.tsx` — a `<Card>` titled "Notifications" containing:
1. **Channels** subsection: list from `useChannels()` (name + created date, Edit/Delete buttons, "Test" button using `useTestChannel`), an "Add channel" button opening `ChannelDialog`, and an `AlertDialog` confirm for delete.
2. **Events** subsection: from `useEventTypes()` + `useSubscriptions()`, render a `Switch` per event type grouped by `category` (admin categories only appear because the API already filters them for non-admins). Toggling builds the new `event_types` array and calls `usePutSubscriptions()`. A **Reset to defaults** button (with `AlertDialog` warning it discards customizations) calls `useResetSubscriptions()`.

Use `toast` for all mutation outcomes. Render the subscription state as `subscriptions.event_types.includes(type)`.

- [ ] **Step 5: Run the test to verify it passes**

Run (from `ui/frontend/`): `npm run test notifications-section.test.tsx`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/components/notifications/
git commit -m "feat: notifications profile section + channel dialog"
```

---

### Task 22: Mount section in profile

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/profile.tsx`

- [ ] **Step 1: Mount the component**

Import `NotificationsSection` and render `<NotificationsSection />` as a new `<Card>` in the main column of the profile grid (alongside Account Information / Password & Security).

- [ ] **Step 2: Verify typecheck, lint, dead-code, build**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run build`
Expected: no type errors, no knip findings, build succeeds (regenerates `routeTree.gen.ts` if needed — commit it if it changed).

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/profile.tsx
# include routeTree.gen.ts if it changed
git commit -m "feat: mount notifications section in profile"
```

---

### Task 23: Full-suite verification

- [ ] **Step 1: Backend suite**

Run: `go test -timeout 600s ./...`
Expected: all packages PASS.

- [ ] **Step 2: Lint**

Run: `golangci-lint run`
Expected: zero findings. (Watch for `errcheck` on any new `_ =` discards — annotate or handle per CLAUDE.md.)

- [ ] **Step 3: Frontend suite**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all green.

- [ ] **Step 4: Manual smoke (optional but recommended)**

Run the server, create a channel with `logger://` (Shoutrrr's stdout service) or `ntfy://`, hit **Test**, confirm a delivery; trigger a failing sync and confirm a `sync.failed` event row appears and a notification is attempted.

- [ ] **Step 5: Final commit / open PR**

Open a PR titled `feat: add notifications` (Conventional Commit → minor bump). Do not merge — wait for explicit instruction.

---

## Self-review notes

- **Spec coverage:** channels via Shoutrrr (Task 7,8,18,21) ✓; encrypted-at-rest URLs (Task 2,4,18) ✓; test send (Task 18 `HandleTestChannel`, Task 21 button) ✓; sync completed/with-errors/failed (Task 14) ✓; needs-review fire-once (Task 14, dedup) ✓; import/export completed+failed (Task 15) ✓; game-diff digest, only on real changes (Task 14 `emitSyncDiff`) ✓; admin backup + maintenance, multi-admin opt-in (Task 16, admin fan-out in Task 11) ✓; fire-and-forget WARN/DEBUG logging, no history UI (Task 11) ✓; per-event opt-in under profile, defaults failures-on (Task 5,17,18,21) ✓; events retention prune job (Task 12,13) ✓; non-admin cannot subscribe to admin.* (Task 18 `HandlePutSubscriptions`) ✓; slumber routes (Task 19) ✓; nix vendorHash (Task 7) ✓.
- **Deviation from issue:** the issue says emit "transactionally via river.Client[pgx.Tx]". That is impossible here — River uses a separate pgx pool from Bun's pgdriver pool, and the emit sites (18+ callers, incl. HTTP handlers) only have `*bun.DB`. The plan uses a package-level River client + best-effort enqueue after the bun event insert, matching the codebase's established pattern (e.g. `metadata_refresh.go`). The `events` row is the durable record; a missed enqueue loses only the push, not the event (a future audit/redelivery sweep over un-enqueued events is a possible follow-up).
- **Verify-before-finalize flags embedded in tasks:** Bun `Scan` into `[]string` vs struct-slice (Tasks 11,18); `bun.IDB` availability (Task 17); shoutrrr `types.Params` import path (Task 8); whether `markJobCompleted`/`markJobFailed` are export-only (Task 15); module import prefix from `go.mod` (Tasks 11,13,etc.). Each task says to confirm against the cited existing file rather than guess.
