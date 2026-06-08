# Storefront Deep-Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render each storefront on the game details page as a deep-link to that game's product page, with the link target resolved by a decoupled background enrichment worker.

**Architecture:** A new nullable `external_games.store_link` column holds a per-store identifier (appid / slug / concept id). A dedicated River dispatch+item worker pair (modelled on `metadata_refresh`) is the sole writer of `store_link`; it resolves links via a new `internal/services/storelink` package. Sync only *captures* Epic's `namespace` into a new `external_games.source_metadata` jsonb (never writes `store_link`). Enrichment is triggered scoped+incremental on sync completion, and globally+forced by a manual admin action — nothing periodic. A pure `buildStoreURL` in the API layer turns `(storefront, store_link)` into a URL served on the game details endpoint; the frontend renders a link when present.

**Tech Stack:** Go 1.26, Bun ORM, River queue, Echo v5, PostgreSQL; React 19 + TanStack Router/Query + Vitest.

**Design doc:** `docs/superpowers/specs/2026-06-08-storefront-deep-links-design.md`

---

## File Structure

**Create:**
- `internal/db/migrations/20260608000001_add_store_link_and_source_metadata_to_external_games.up.sql` / `.down.sql`
- `internal/services/storelink/resolver.go` — `Resolver` interface + Steam/GOG/Epic/PSN implementations
- `internal/services/storelink/resolver_test.go`
- `internal/worker/tasks/store_link_refresh.go` — dispatch + item worker
- `internal/worker/tasks/store_link_refresh_test.go`
- `internal/api/store_url.go` — pure `buildStoreURL`
- `internal/api/store_url_test.go`

**Modify:**
- `internal/db/models/models.go` — `ExternalGame` fields; `UserGamePlatform` relation
- `internal/db/models/jobs.go` — `JobTypeStoreLinkRefresh`
- `internal/services/storefrontadapter/storefrontadapter.go` — `SourceMetadata` on entry
- `internal/services/epic/adapter.go` — populate `SourceMetadata["namespace"]`
- `internal/services/psn/client.go` — `Authenticate` + `ResolveConceptID`
- `internal/worker/tasks/sync.go` — write `source_metadata` in upsert; thread `riverClient` into `SyncCheckJobCompletion`; enqueue scoped dispatch on completion
- `internal/api/sync.go` — update `SyncCheckJobCompletion` call sites
- `internal/api/user_games.go` — load `ExternalGame` relation; emit `store_url`
- `internal/api/games.go` — admin endpoint `HandleStartStoreLinkRefreshJob`
- `internal/api/router.go` — register the admin route
- `cmd/nexorious/serve.go` — resolver factory + register both workers (two blocks)
- `ui/frontend/src/types/game.ts` — `store_url?` on `UserGamePlatform`
- `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` — link rendering
- `ui/frontend/src/api/admin.ts` — `startStoreLinkRefreshJob`
- `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` — "Refresh store links" button

---

## Task 1: Migration — add `store_link` and `source_metadata`

**Files:**
- Create: `internal/db/migrations/20260608000001_add_store_link_and_source_metadata_to_external_games.up.sql`
- Create: `internal/db/migrations/20260608000001_add_store_link_and_source_metadata_to_external_games.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
ALTER TABLE external_games
    ADD COLUMN store_link      TEXT,
    ADD COLUMN source_metadata JSONB;
```

- [ ] **Step 2: Write the down migration**

```sql
ALTER TABLE external_games
    DROP COLUMN IF EXISTS source_metadata,
    DROP COLUMN IF EXISTS store_link;
```

- [ ] **Step 3: Verify migrations build & apply**

Run: `go build ./... && go test ./internal/db/... -run TestMigrations -v` (if no such test exists, instead run `go vet ./internal/db/...`)
Expected: PASS / no errors. The migration is auto-discovered via `Migrations.Discover(FS)`.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260608000001_add_store_link_and_source_metadata_to_external_games.*.sql
git commit -m "feat: add store_link and source_metadata columns to external_games (#831)"
```

---

## Task 2: Model fields

**Files:**
- Modify: `internal/db/models/models.go` (`ExternalGame` struct ~L165-183; `UserGamePlatform` ~L93-110)

- [ ] **Step 1: Add fields to `ExternalGame`**

In the `ExternalGame` struct, after the `ParentID` line and before `CreatedAt`, add:

```go
	StoreLink      *string         `bun:"store_link"               json:"store_link,omitempty"`
	SourceMetadata json.RawMessage `bun:"source_metadata"          json:"source_metadata,omitempty"`
```

Ensure `encoding/json` is imported in this file (add it to the import block if absent).

- [ ] **Step 2: Add an `ExternalGame` relation to `UserGamePlatform`**

In `UserGamePlatform`, alongside the existing `PlatformRecord`/`StorefrontRecord` relation fields (~L109-110), add:

```go
	ExternalGame *ExternalGame `bun:"rel:belongs-to,join:external_game_id=id" json:"-"`
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/db/models/models.go
git commit -m "feat: add store_link/source_metadata fields and ExternalGame relation to models (#831)"
```

---

## Task 3: Pure `buildStoreURL` (API layer)

**Files:**
- Create: `internal/api/store_url.go`
- Test: `internal/api/store_url_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import "testing"

func TestBuildStoreURL(t *testing.T) {
	cases := []struct {
		name       string
		storefront string
		link       string
		wantURL    string
		wantOK     bool
	}{
		{"steam", "steam", "440", "https://store.steampowered.com/app/440/", true},
		{"gog", "gog", "the-witcher-3-wild-hunt", "https://www.gog.com/game/the-witcher-3-wild-hunt", true},
		{"epic", "epic-games-store", "fortnite", "https://store.epicgames.com/en-US/p/fortnite", true},
		{"psn", "playstation-store", "10002694", "https://store.playstation.com/en-us/concept/10002694", true},
		{"humble null", "humble-bundle", "", "", false},
		{"empty link", "steam", "", "", false},
		{"unknown storefront", "itch", "abc", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotOK := buildStoreURL(c.storefront, c.link)
			if gotURL != c.wantURL || gotOK != c.wantOK {
				t.Fatalf("buildStoreURL(%q,%q) = (%q,%v), want (%q,%v)",
					c.storefront, c.link, gotURL, gotOK, c.wantURL, c.wantOK)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestBuildStoreURL -v`
Expected: FAIL — `undefined: buildStoreURL`.

- [ ] **Step 3: Implement `buildStoreURL`**

```go
package api

import "fmt"

// buildStoreURL turns a (storefront, store_link) pair into a product-page URL.
// It returns ("", false) when no reliable link can be built — an empty
// store_link, a storefront with no URL scheme (humble-bundle), or an unknown
// storefront. URL formats live here so a store changing its scheme is a code
// fix, not a re-sync. Storefront keys are the canonical storefronts.name slugs.
func buildStoreURL(storefront, storeLink string) (string, bool) {
	if storeLink == "" {
		return "", false
	}
	switch storefront {
	case "steam":
		return fmt.Sprintf("https://store.steampowered.com/app/%s/", storeLink), true
	case "gog":
		return fmt.Sprintf("https://www.gog.com/game/%s", storeLink), true
	case "epic-games-store":
		return fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", storeLink), true
	case "playstation-store":
		return fmt.Sprintf("https://store.playstation.com/en-us/concept/%s", storeLink), true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestBuildStoreURL -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/store_url.go internal/api/store_url_test.go
git commit -m "feat: add buildStoreURL product-page URL builder (#831)"
```

---

## Task 4: API read path — emit `store_url` on game details

**Files:**
- Modify: `internal/api/user_games.go` — `userGamePlatformResponse` struct (~L50-66), `toUserGamePlatformResponse` (~L68-92), `HandleGetUserGame` query (~L461-475)
- Test: `internal/api/user_games_test.go` (add a test; reuse the package's shared `testDB`)

- [ ] **Step 1: Add `StoreURL` to the response struct**

In `userGamePlatformResponse`, add after `StorefrontDetails`:

```go
	StoreURL *string `json:"store_url,omitempty"`
```

- [ ] **Step 2: Populate `StoreURL` in the converter**

In `toUserGamePlatformResponse`, before `return resp`, add:

```go
	if ugp.ExternalGame != nil && ugp.ExternalGame.StoreLink != nil {
		if url, ok := buildStoreURL(ugp.ExternalGame.Storefront, *ugp.ExternalGame.StoreLink); ok {
			resp.StoreURL = &url
		}
	}
```

- [ ] **Step 3: Load the `ExternalGame` relation in `HandleGetUserGame`**

In the `Relation("Platforms", …)` closure, chain `.Relation("ExternalGame")`:

```go
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord").Relation("ExternalGame")
		}).
```

- [ ] **Step 4: Write the failing test**

Add to `internal/api/user_games_test.go` (follow the existing test setup in that file for building a handler against `testDB`, seeding a user/game/user_game/external_game/user_game_platform, and an authed `echo` context — mirror an existing `HandleGetUserGame` test). The behavioural core:

```go
func TestHandleGetUserGame_StoreURL(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Seed: user, game, user_game, external_game with store_link, user_game_platform linking them.
	// (Use the same seeding helpers/inserts as the other user_games tests in this file.)
	userID := seedUser(t)
	gameID := seedGame(t, "Team Fortress 2")
	egID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, store_link, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, 'steam', '440', 'Team Fortress 2', '440', true, false, now(), now())`,
		egID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ugID := seedUserGame(t, userID, gameID)
	_, err = testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, external_game_id, is_available, hours_played, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', ?, true, 0, now(), now())`,
		uuid.NewString(), ugID, egID,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	rec, c := newAuthedGetContext(t, "/api/user-games/"+ugID, userID) // mirror existing helper
	c.SetParamNames("id")
	c.SetParamValues(ugID)

	h := newUserGamesHandlerForTest(t) // mirror existing helper
	if err := h.HandleGetUserGame(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var body userGameWithPlatformsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Platforms) != 1 || body.Platforms[0].StoreURL == nil {
		t.Fatalf("expected store_url to be set, got %+v", body.Platforms)
	}
	if *body.Platforms[0].StoreURL != "https://store.steampowered.com/app/440/" {
		t.Fatalf("unexpected store_url: %q", *body.Platforms[0].StoreURL)
	}
}
```

> If helper names (`seedUser`, `newAuthedGetContext`, etc.) differ in this package, use the actual helpers already present in `user_games_test.go`. Do not introduce a per-test container — use the package `testDB`.

- [ ] **Step 5: Run test to verify it fails, then passes**

Run: `go test ./internal/api/ -run TestHandleGetUserGame_StoreURL -v`
Expected: FAIL first (store_url nil / relation not loaded), then PASS after Steps 1-3.

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat: emit store_url on game details response (#831)"
```

---

## Task 5: Frontend — render storefront as a deep-link

**Files:**
- Modify: `ui/frontend/src/types/game.ts` — `UserGamePlatform` (~L76-87)
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` — Platforms & Ownership block (~L283-286)
- Test: `ui/frontend/src/routes/_authenticated/games/$id.index.test.tsx` (create if absent) — or co-locate a component test; follow the existing frontend test conventions.

- [ ] **Step 1: Add `store_url` to the type**

In `UserGamePlatform`, add:

```ts
  store_url?: string;
```

- [ ] **Step 2: Write the failing test**

Create a focused test that renders the storefront indicator. Because `$id.index.tsx` is a route component, extract the storefront-label JSX into a tiny pure component first (Step 3) and test that. Test file `ui/frontend/src/components/storefront-link.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { StorefrontLabel } from './storefront-link';

describe('StorefrontLabel', () => {
  it('renders a new-tab link when store_url is present', () => {
    render(<StorefrontLabel displayName="Steam" storeUrl="https://store.steampowered.com/app/440/" />);
    const link = screen.getByRole('link', { name: /steam/i });
    expect(link).toHaveAttribute('href', 'https://store.steampowered.com/app/440/');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('renders a plain label when store_url is absent', () => {
    render(<StorefrontLabel displayName="Humble" />);
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText(/humble/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run (from `ui/frontend/`): `npm run test storefront-link`
Expected: FAIL — module `./storefront-link` not found.

- [ ] **Step 4: Implement the component**

Create `ui/frontend/src/components/storefront-link.tsx`:

```tsx
interface StorefrontLabelProps {
  displayName: string;
  storeUrl?: string;
}

export function StorefrontLabel({ displayName, storeUrl }: StorefrontLabelProps) {
  if (storeUrl) {
    return (
      <a
        href={storeUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="text-sm text-muted-foreground underline-offset-2 hover:underline"
      >
        ({displayName})
      </a>
    );
  }
  return <span className="text-sm text-muted-foreground">({displayName})</span>;
}
```

- [ ] **Step 5: Use it in the route component**

In `$id.index.tsx`, replace the existing storefront span:

```tsx
                        {p.storefront_details && (
                            <span className="text-sm text-muted-foreground">
                                ({p.storefront_details.display_name})
                            </span>
                        )}
```

with:

```tsx
                        {p.storefront_details && (
                            <StorefrontLabel
                                displayName={p.storefront_details.display_name}
                                storeUrl={p.store_url}
                            />
                        )}
```

Add the import at the top with the other `@/` imports:

```tsx
import { StorefrontLabel } from '@/components/storefront-link';
```

- [ ] **Step 6: Run test + typecheck**

Run (from `ui/frontend/`): `npm run test storefront-link && npm run check`
Expected: PASS, no type errors.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/types/game.ts ui/frontend/src/components/storefront-link.tsx ui/frontend/src/components/storefront-link.test.tsx ui/frontend/src/routes/_authenticated/games/\$id.index.tsx
git commit -m "feat: render storefront as product-page deep-link on game details (#831)"
```

---

## Task 6: Sync captures Epic's `namespace` (never writes `store_link`)

**Files:**
- Modify: `internal/services/storefrontadapter/storefrontadapter.go` — add field to `ExternalGameEntry`
- Modify: `internal/services/epic/adapter.go` — populate `SourceMetadata`
- Modify: `internal/worker/tasks/sync.go` — `upsertExternalGame` writes `source_metadata`
- Test: `internal/worker/tasks/sync_test.go` — assert namespace persisted

- [ ] **Step 1: Add `SourceMetadata` to the generic entry**

In `storefrontadapter.go`, add to `ExternalGameEntry`:

```go
	// SourceMetadata carries per-source resolution inputs captured at sync time
	// (e.g. Epic's namespace). Persisted to external_games.source_metadata; never
	// used to render store_link directly. Nil/empty for stores that need nothing.
	SourceMetadata map[string]string
```

- [ ] **Step 2: Populate it in the Epic adapter**

In `internal/services/epic/adapter.go`, where the epic `ExternalGameEntry` is mapped to `storefrontadapter.ExternalGameEntry` (the block that currently drops `Namespace`/`CatalogItemID`), set:

```go
		var sourceMeta map[string]string
		if e.Namespace != "" {
			sourceMeta = map[string]string{"namespace": e.Namespace}
		}
		out = append(out, storefrontadapter.ExternalGameEntry{
			ExternalID:      e.ExternalID,
			Title:           e.Title,
			PlaytimeHours:   0,
			Platforms:       []string{"pc-windows"},
			OwnershipStatus: e.OwnershipStatus,
			IsSubscription:  false,
			SourceMetadata:  sourceMeta,
		})
```

(Adapt variable names to the actual loop in `adapter.go`.)

- [ ] **Step 3: Persist `source_metadata` in `upsertExternalGame`**

In `sync.go`, change the upsert to include `source_metadata`. Marshal the entry's map (nil → SQL NULL):

```go
	var sourceMetaJSON []byte
	if len(e.SourceMetadata) > 0 {
		sourceMetaJSON, _ = json.Marshal(e.SourceMetadata) //nolint:errcheck // marshaling a map[string]string cannot fail
	}
```

Then update the SQL (note: `store_link` is intentionally never referenced, so re-syncs cannot null it):

```go
	if err := db.NewRaw(`
		INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, source_metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, now(), now())
		ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			is_subscription = EXCLUDED.is_subscription,
			ownership_status = EXCLUDED.ownership_status,
			source_metadata = COALESCE(EXCLUDED.source_metadata, external_games.source_metadata),
			is_available = true,
			updated_at = now()
		RETURNING id, is_skipped, (xmax = 0) AS is_new`,
		uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
		e.IsSubscription, e.OwnershipStatus, nullableJSON(sourceMetaJSON),
	).Scan(ctx, &row); err != nil {
```

Add a tiny helper near the top of `sync.go` (or reuse if one exists) to pass `nil` rather than empty bytes:

```go
// nullableJSON returns nil when b is empty so the column is written as SQL NULL.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
```

- [ ] **Step 4: Write the failing test**

In `sync_test.go`, add a test that calls `upsertExternalGame` (it's package-private; the test is in-package) with an Epic entry carrying `SourceMetadata` and asserts the row's `source_metadata->>'namespace'`:

```go
func TestUpsertExternalGame_PersistsSourceMetadata(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUserForSync(t) // mirror existing sync_test helpers

	p := DispatchSyncArgs{JobID: uuid.NewString(), UserID: userID, Storefront: "epic-games-store"}
	e := ExternalGameEntry{
		ExternalID:      "abc123",
		Title:           "Some Epic Game",
		OwnershipStatus: "owned",
		SourceMetadata:  map[string]string{"namespace": "ns-xyz"},
	}
	egID, _ := upsertExternalGame(ctx, testDB, e, p)
	if egID == "" {
		t.Fatal("expected external_game id")
	}
	var ns string
	if err := testDB.NewRaw(
		`SELECT source_metadata->>'namespace' FROM external_games WHERE id = ?`, egID,
	).Scan(ctx, &ns); err != nil {
		t.Fatal(err)
	}
	if ns != "ns-xyz" {
		t.Fatalf("namespace = %q, want %q", ns, "ns-xyz")
	}
}
```

(Use the actual user-seeding helper present in `sync_test.go`.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/worker/tasks/ -run TestUpsertExternalGame_PersistsSourceMetadata -v` and `go test ./internal/services/epic/... -v`
Expected: PASS. Also `go build ./...`.

- [ ] **Step 6: Commit**

```bash
git add internal/services/storefrontadapter/storefrontadapter.go internal/services/epic/adapter.go internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: capture Epic namespace into external_games.source_metadata during sync (#831)"
```

---

## Task 7: `storelink` resolver package

**Files:**
- Create: `internal/services/storelink/resolver.go`
- Test: `internal/services/storelink/resolver_test.go`
- Modify: `internal/services/psn/client.go` — `Authenticate`, `ResolveConceptID`

- [ ] **Step 1: Add PSN client methods**

In `psn/client.go`, refactor the auth block out of `GetLibrary` into a reusable method and add concept resolution:

```go
// Authenticate exchanges an NPSSO token for a PSN access token.
func (c *Client) Authenticate(ctx context.Context, npssoToken string) (string, error) {
	if c.authFn != nil {
		return c.authFn(ctx, npssoToken)
	}
	psnClient, err := psnsdk.NewClient(&psnsdk.Options{Lang: "en", Region: "us", Npsso: npssoToken})
	if err != nil {
		return "", fmt.Errorf("psn: failed to create client: %w", err)
	}
	if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
		return "", ErrInvalidNPSSOToken
	}
	token, _ := psnClient.AccessToken()
	if token == "" {
		return "", fmt.Errorf("psn: access token unavailable after authentication")
	}
	return token, nil
}

type conceptsResponse struct {
	Concepts []struct {
		ID string `json:"id"`
	} `json:"concepts"`
}

// ResolveConceptID resolves a PSN titleId to its store concept ID via the
// catalog API. Returns "" (no error) when the title has no resolvable concept.
//
// NOTE: the exact JSON shape of /catalog/v2/titles/{id}/concepts must be
// confirmed against a live response during implementation; adjust
// conceptsResponse and the extraction below to match. The endpoint is
// authenticated with the access token from Authenticate.
func (c *Client) ResolveConceptID(ctx context.Context, accessToken, titleID string) (string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("psn: rate limiter wait: %w", err)
	}
	u := fmt.Sprintf("%s/api/catalog/v2/titles/%s/concepts", c.gamelistURL, titleID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("psn: concepts request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("psn: concepts fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil // no concept for this title
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("psn: concepts HTTP %d", resp.StatusCode)
	}
	var body conceptsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("psn: concepts decode: %w", err)
	}
	if len(body.Concepts) == 0 || body.Concepts[0].ID == "" {
		return "", nil
	}
	return body.Concepts[0].ID, nil
}
```

Then replace the inline auth block in `GetLibrary` (the `else` branch that builds `psnClient` and calls `AuthWithNPSSO`) with `accessToken, err = c.Authenticate(ctx, npssoToken)` for DRY (keep the `authFn` test path working — `Authenticate` already honours it).

- [ ] **Step 2: Write the failing resolver test**

`internal/services/storelink/resolver_test.go`:

```go
package storelink

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSteamResolver(t *testing.T) {
	got, err := NewSteamResolver().Resolve(context.Background(), "440", nil)
	if err != nil || got != "440" {
		t.Fatalf("steam resolve = (%q,%v), want (440,nil)", got, err)
	}
}

func TestGOGResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/12345" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":12345,"slug":"the-witcher-3-wild-hunt"}`))
	}))
	defer srv.Close()
	r := NewGOGResolver(srv.Client(), srv.URL)
	got, err := r.Resolve(context.Background(), "12345", nil)
	if err != nil || got != "the-witcher-3-wild-hunt" {
		t.Fatalf("gog resolve = (%q,%v)", got, err)
	}
}

func TestEpicResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ns-xyz":"fortnite","ns-abc":"other"}`))
	}))
	defer srv.Close()
	r := NewEpicResolver(srv.Client(), srv.URL)
	got, err := r.Resolve(context.Background(), "app123", map[string]string{"namespace": "ns-xyz"})
	if err != nil || got != "fortnite" {
		t.Fatalf("epic resolve = (%q,%v)", got, err)
	}
	// Missing namespace → no link, no error.
	got, err = r.Resolve(context.Background(), "app456", map[string]string{"namespace": "missing"})
	if err != nil || got != "" {
		t.Fatalf("epic resolve missing = (%q,%v), want empty", got, err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/services/storelink/ -v`
Expected: FAIL — package/functions undefined.

- [ ] **Step 4: Implement the resolver package**

`internal/services/storelink/resolver.go`:

```go
// Package storelink resolves per-storefront product-page identifiers
// (store_link values) used by the enrichment worker. Resolution is best-effort:
// a Resolver returns ("", nil) when no link can be determined.
package storelink

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Resolver resolves a single external game's store_link.
type Resolver interface {
	// Resolve returns the store_link value for the given external id and the
	// row's captured source_metadata, or "" when it cannot be resolved.
	Resolve(ctx context.Context, externalID string, sourceMeta map[string]string) (string, error)
}

// ── Steam ────────────────────────────────────────────────────────────────────

type steamResolver struct{}

func NewSteamResolver() Resolver { return steamResolver{} }

func (steamResolver) Resolve(_ context.Context, externalID string, _ map[string]string) (string, error) {
	return externalID, nil // store_link == appid == external_id
}

// ── GOG ──────────────────────────────────────────────────────────────────────

type gogResolver struct {
	httpClient *http.Client
	apiBase    string
}

const defaultGOGAPIBase = "https://api.gog.com"

func NewGOGResolver(httpClient *http.Client, apiBase string) Resolver {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if apiBase == "" {
		apiBase = defaultGOGAPIBase
	}
	return &gogResolver{httpClient: httpClient, apiBase: apiBase}
}

func (g *gogResolver) Resolve(ctx context.Context, externalID string, _ map[string]string) (string, error) {
	u := fmt.Sprintf("%s/products/%s", g.apiBase, externalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("gog: build product request: %w", err)
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gog: product fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gog: product HTTP %d", resp.StatusCode)
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("gog: product decode: %w", err)
	}
	return body.Slug, nil
}

// ── Epic ─────────────────────────────────────────────────────────────────────

type epicResolver struct {
	httpClient *http.Client
	mappingURL string
	once       sync.Once
	mapping    map[string]string
	mapErr     error
}

const defaultEpicMappingURL = "https://store-content-ipv4.ak.epicgames.com/api/content/productmapping"

func NewEpicResolver(httpClient *http.Client, mappingURL string) Resolver {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if mappingURL == "" {
		mappingURL = defaultEpicMappingURL
	}
	return &epicResolver{httpClient: httpClient, mappingURL: mappingURL}
}

func (e *epicResolver) Resolve(ctx context.Context, _ string, sourceMeta map[string]string) (string, error) {
	ns := sourceMeta["namespace"]
	if ns == "" {
		return "", nil // no namespace captured → cannot resolve
	}
	e.once.Do(func() { e.mapping, e.mapErr = e.fetchMapping(ctx) })
	if e.mapErr != nil {
		return "", e.mapErr
	}
	return e.mapping[ns], nil // "" when namespace absent from the mapping
}

func (e *epicResolver) fetchMapping(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.mappingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("epic: build productmapping request: %w", err)
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epic: productmapping fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epic: productmapping HTTP %d", resp.StatusCode)
	}
	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("epic: productmapping decode: %w", err)
	}
	return m, nil
}

// ── PSN ──────────────────────────────────────────────────────────────────────

// psnConceptResolver wraps an authenticated PSN client. It authenticates lazily
// on first Resolve so the access token is obtained once per group.
type psnConceptResolver struct {
	client     PSNConceptClient
	npsso      string
	once       sync.Once
	token      string
	authErr    error
}

// PSNConceptClient is satisfied by *psn.Client.
type PSNConceptClient interface {
	Authenticate(ctx context.Context, npsso string) (string, error)
	ResolveConceptID(ctx context.Context, accessToken, titleID string) (string, error)
}

func NewPSNResolver(client PSNConceptClient, npsso string) Resolver {
	return &psnConceptResolver{client: client, npsso: npsso}
}

func (p *psnConceptResolver) Resolve(ctx context.Context, externalID string, _ map[string]string) (string, error) {
	p.once.Do(func() { p.token, p.authErr = p.client.Authenticate(ctx, p.npsso) })
	if p.authErr != nil {
		return "", p.authErr
	}
	return p.client.ResolveConceptID(ctx, p.token, externalID)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/services/storelink/ -v && go test ./internal/services/psn/... -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/storelink/ internal/services/psn/client.go
git commit -m "feat: add storelink resolver package and PSN concept resolution (#831)"
```

---

## Task 8: `JobTypeStoreLinkRefresh` + enrichment dispatch worker

**Files:**
- Modify: `internal/db/models/jobs.go` — add constant
- Create: `internal/worker/tasks/store_link_refresh.go` — dispatch worker (item worker added in Task 9)
- Test: `internal/worker/tasks/store_link_refresh_test.go`

- [ ] **Step 1: Add the job-type constant**

In `jobs.go`, in the `JobType*` block, add:

```go
	JobTypeStoreLinkRefresh = "store_link_refresh"
```

- [ ] **Step 2: Write the failing dispatch test**

`store_link_refresh_test.go`:

```go
package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// seedExternalGame inserts a row with the given store_link (nil → NULL).
func seedExternalGame(t *testing.T, userID, storefront, externalID string, storeLink *string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, store_link, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, true, false, now(), now())`,
		uuid.NewString(), userID, storefront, externalID, "T-"+externalID, storeLink,
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoreLinkRefreshDispatch_IncrementalSelectsOnlyNullRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUserForSync(t)
	filled := "440"
	seedExternalGame(t, userID, "steam", "10", nil)        // null → eligible
	seedExternalGame(t, userID, "steam", "20", &filled)    // filled → skipped (incremental)

	w := &StoreLinkRefreshDispatchWorker{DB: testDB} // RiverClient nil: enqueue is logged, job_items still created
	groups, total, err := w.selectGroups(ctx, StoreLinkRefreshDispatchArgs{Force: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || total != 1 {
		t.Fatalf("incremental: groups=%v total=%d, want 1 group / 1 row", groups, total)
	}
}

func TestStoreLinkRefreshDispatch_ForceSelectsAllRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUserForSync(t)
	filled := "440"
	seedExternalGame(t, userID, "steam", "10", nil)
	seedExternalGame(t, userID, "steam", "20", &filled)

	w := &StoreLinkRefreshDispatchWorker{DB: testDB}
	groups, total, err := w.selectGroups(ctx, StoreLinkRefreshDispatchArgs{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || total != 2 {
		t.Fatalf("force: groups=%v total=%d, want 1 group / 2 rows", groups, total)
	}
}

func TestStoreLinkRefreshDispatch_ScopeFiltersStorefrontAndUser(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUserForSync(t)
	seedExternalGame(t, userID, "steam", "10", nil)
	seedExternalGame(t, userID, "gog", "99", nil)

	w := &StoreLinkRefreshDispatchWorker{DB: testDB}
	groups, _, err := w.selectGroups(ctx, StoreLinkRefreshDispatchArgs{UserID: userID, Storefront: "gog"})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Storefront != "gog" {
		t.Fatalf("scoped: groups=%v, want only gog", groups)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestStoreLinkRefreshDispatch -v`
Expected: FAIL — undefined types/methods.

- [ ] **Step 4: Implement the dispatch worker (with a testable `selectGroups`)**

In `store_link_refresh.go`:

```go
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"database/sql"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/storelink"
)

// resolvableStorefronts are the slugs the enrichment worker can resolve.
var resolvableStorefronts = []string{"steam", "gog", "epic-games-store", "playstation-store"}

// ── Dispatch worker ──────────────────────────────────────────────────────────

// StoreLinkRefreshDispatchArgs drives a store-link enrichment pass. When UserID
// and Storefront are set the pass is scoped to that one group (sync-triggered);
// when empty it covers all resolvable groups (admin). Force=false resolves only
// rows with a null store_link; Force=true re-resolves all rows.
type StoreLinkRefreshDispatchArgs struct {
	UserID     string `json:"user_id,omitempty"`
	Storefront string `json:"storefront,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

func (StoreLinkRefreshDispatchArgs) Kind() string { return "store_link_refresh_dispatch" }

func (StoreLinkRefreshDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type StoreLinkRefreshDispatchWorker struct {
	river.WorkerDefaults[StoreLinkRefreshDispatchArgs]
	DB          *bun.DB
	RiverClient *river.Client[pgx.Tx]
}

type storeLinkGroup struct {
	UserID     string `bun:"user_id"`
	Storefront string `bun:"storefront"`
}

// selectGroups returns the distinct (user, storefront) groups to enrich and the
// total number of target rows across them.
func (w *StoreLinkRefreshDispatchWorker) selectGroups(ctx context.Context, args StoreLinkRefreshDispatchArgs) ([]storeLinkGroup, int, error) {
	var groups []storeLinkGroup
	if err := w.DB.NewRaw(`
		SELECT DISTINCT user_id, storefront
		FROM external_games
		WHERE storefront IN (?)
		  AND is_available = true
		  AND (?::bool OR store_link IS NULL)
		  AND (? = '' OR user_id = ?)
		  AND (? = '' OR storefront = ?)
		ORDER BY user_id, storefront`,
		bun.In(resolvableStorefronts),
		args.Force,
		args.UserID, args.UserID,
		args.Storefront, args.Storefront,
	).Scan(ctx, &groups); err != nil {
		return nil, 0, fmt.Errorf("select groups: %w", err)
	}
	var total int
	if err := w.DB.NewRaw(`
		SELECT count(*)
		FROM external_games
		WHERE storefront IN (?)
		  AND is_available = true
		  AND (?::bool OR store_link IS NULL)
		  AND (? = '' OR user_id = ?)
		  AND (? = '' OR storefront = ?)`,
		bun.In(resolvableStorefronts),
		args.Force,
		args.UserID, args.UserID,
		args.Storefront, args.Storefront,
	).Scan(ctx, &total); err != nil {
		return nil, 0, fmt.Errorf("count rows: %w", err)
	}
	return groups, total, nil
}

func (w *StoreLinkRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshDispatchArgs]) error {
	args := job.Args

	// Scoped guard: skip if an equivalent dispatch is already active.
	source := models.JobSourceSystem
	if args.Storefront != "" {
		source = args.Storefront
	}
	var existing string
	guard := `SELECT id FROM jobs WHERE job_type = ? AND status IN ('pending','processing') AND source = ?`
	guardArgs := []any{models.JobTypeStoreLinkRefresh, source}
	if args.UserID != "" {
		guard += ` AND user_id = ?`
		guardArgs = append(guardArgs, args.UserID)
	}
	guard += ` LIMIT 1`
	err := w.DB.NewRaw(guard, guardArgs...).Scan(ctx, &existing)
	if err == nil {
		slog.Info("store_link_refresh_dispatch: equivalent job active, skipping", "existing", existing)
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("store_link_refresh_dispatch: guard query", "err", err)
		return nil
	}

	groups, total, err := w.selectGroups(ctx, args)
	if err != nil {
		slog.Error("store_link_refresh_dispatch: select groups", "err", err)
		return nil
	}
	if len(groups) == 0 {
		return nil
	}

	// Owning user for the jobs row: the scoped user, else any admin.
	jobUserID := args.UserID
	if jobUserID == "" {
		if e := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true LIMIT 1`).Scan(ctx, &jobUserID); e != nil {
			slog.Error("store_link_refresh_dispatch: no admin user", "err", e)
			return nil
		}
	}

	jobID := uuid.NewString()
	itemIDs := make([]string, 0, len(groups))
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, e := tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
			jobID, jobUserID, models.JobTypeStoreLinkRefresh, source, total,
		).Exec(ctx); e != nil {
			return fmt.Errorf("insert job: %w", e)
		}
		for _, g := range groups {
			itemID := uuid.NewString()
			itemIDs = append(itemIDs, itemID)
			meta, _ := json.Marshal(map[string]any{"storefront": g.Storefront, "force": args.Force}) //nolint:errcheck // fixed map
			if _, e := tx.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
				itemID, jobID, g.UserID, g.Storefront, g.Storefront, json.RawMessage(meta),
			).Exec(ctx); e != nil {
				return fmt.Errorf("insert job_item: %w", e)
			}
		}
		return nil
	}); err != nil {
		slog.Error("store_link_refresh_dispatch: tx failed", "err", err)
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
			Payload: notify.MaintPayload{Action: "store_link_refresh_dispatch", Error: err.Error()},
		})
		return nil
	}

	for _, itemID := range itemIDs {
		if e := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, StoreLinkRefreshItemArgs{JobItemID: itemID}); e != nil {
			slog.Error("store_link_refresh_dispatch: enqueue item", "err", e, "item_id", itemID)
		}
	}
	slog.Info("store_link_refresh_dispatch: job created", "job_id", jobID, "groups", len(groups), "rows", total)
	return nil
}

// resolverFactory builds a storelink.Resolver for a (storefront, user). Defined
// here so the item worker and tests can share the type; the real implementation
// is wired in cmd/nexorious/serve.go.
type resolverFactory func(ctx context.Context, storefront, userID string) (storelink.Resolver, error)
```

> `StoreLinkRefreshItemArgs` is defined in Task 9; this file references it but the package compiles only once Task 9 lands. Implement Tasks 8 and 9 back-to-back (commit at the end of Task 9 if the package won't build between them — or stub `StoreLinkRefreshItemArgs` here and flesh it out in Task 9). To keep each task green, add this minimal stub at the end of Task 8 and replace it in Task 9:

```go
// Stub replaced in Task 9.
type StoreLinkRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (StoreLinkRefreshItemArgs) Kind() string { return "store_link_refresh_item" }
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/worker/tasks/ -run TestStoreLinkRefreshDispatch -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/models/jobs.go internal/worker/tasks/store_link_refresh.go internal/worker/tasks/store_link_refresh_test.go
git commit -m "feat: add store-link enrichment dispatch worker (#831)"
```

---

## Task 9: Enrichment item worker

**Files:**
- Modify: `internal/worker/tasks/store_link_refresh.go` — replace the `StoreLinkRefreshItemArgs` stub with the full item worker
- Test: `internal/worker/tasks/store_link_refresh_test.go` — add item-worker test with a fake resolver

- [ ] **Step 1: Write the failing item test**

Append to `store_link_refresh_test.go`:

```go
type fakeResolver struct{ links map[string]string }

func (f fakeResolver) Resolve(_ context.Context, externalID string, _ map[string]string) (string, error) {
	return f.links[externalID], nil
}

func TestStoreLinkRefreshItem_ResolvesGroupRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUserForSync(t)
	seedExternalGame(t, userID, "steam", "10", nil)
	seedExternalGame(t, userID, "steam", "20", nil)

	// Create the parent job + one job_item for the steam group.
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, ?, 'steam', 'processing', 'low', 2, now())`,
		jobID, userID, models.JobTypeStoreLinkRefresh,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	itemID := uuid.NewString()
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'steam', 'steam', '{"storefront":"steam","force":false}', 'pending', '{}', '[]', now())`,
		itemID, jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	w := &StoreLinkRefreshItemWorker{
		DB: testDB,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return fakeResolver{links: map[string]string{"10": "10", "20": "20"}}, nil
		},
	}
	if err := w.processItem(ctx, itemID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := testDB.NewRaw(
		`SELECT count(*) FROM external_games WHERE user_id = ? AND storefront = 'steam' AND store_link IS NOT NULL`, userID,
	).Scan(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("resolved rows = %d, want 2", n)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestStoreLinkRefreshItem -v`
Expected: FAIL — `StoreLinkRefreshItemWorker`/`processItem` undefined.

- [ ] **Step 3: Implement the item worker**

Replace the Task-8 stub in `store_link_refresh.go` with:

```go
// ── Item worker ───────────────────────────────────────────────────────────────

type StoreLinkRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (StoreLinkRefreshItemArgs) Kind() string { return "store_link_refresh_item" }

func (StoreLinkRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 3}
}

type StoreLinkRefreshItemWorker struct {
	river.WorkerDefaults[StoreLinkRefreshItemArgs]
	DB          *bun.DB
	ResolverFor resolverFactory
}

func (w *StoreLinkRefreshItemWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshItemArgs]) error {
	if err := w.processItem(ctx, job.Args.JobItemID); err != nil {
		slog.Error("store_link_refresh_item: process", "err", err, "item_id", job.Args.JobItemID)
	}
	return nil // best-effort; never block the queue
}

type storeLinkItemMeta struct {
	Storefront string `json:"storefront"`
	Force      bool   `json:"force"`
}

func (w *StoreLinkRefreshItemWorker) processItem(ctx context.Context, jobItemID string) error {
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", jobItemID).Scan(ctx); err != nil {
		return fmt.Errorf("load job_item: %w", err)
	}

	var meta storeLinkItemMeta
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		markItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err), "store_link_refresh: markItemFailed")
		storeLinkCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	resolver, err := w.ResolverFor(ctx, meta.Storefront, item.UserID)
	if err != nil {
		markItemFailed(ctx, w.DB, &item, fmt.Sprintf("resolver: %v", err), "store_link_refresh: markItemFailed")
		storeLinkCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Target rows for this (user, storefront) group.
	var rows []struct {
		ID             string          `bun:"id"`
		ExternalID     string          `bun:"external_id"`
		SourceMetadata json.RawMessage `bun:"source_metadata"`
	}
	q := `SELECT id, external_id, source_metadata FROM external_games
	      WHERE user_id = ? AND storefront = ? AND is_available = true`
	if !meta.Force {
		q += ` AND store_link IS NULL`
	}
	if err := w.DB.NewRaw(q, item.UserID, meta.Storefront).Scan(ctx, &rows); err != nil {
		return fmt.Errorf("select target rows: %w", err)
	}

	for _, r := range rows {
		var sm map[string]string
		if len(r.SourceMetadata) > 0 {
			_ = json.Unmarshal(r.SourceMetadata, &sm) //nolint:errcheck // best-effort; nil map is fine
		}
		link, rerr := resolver.Resolve(ctx, r.ExternalID, sm)
		if rerr != nil {
			slog.Warn("store_link_refresh: resolve failed", "storefront", meta.Storefront, "external_id", r.ExternalID, "err", rerr)
			continue // best-effort; leave null
		}
		if link == "" {
			continue
		}
		if _, e := w.DB.NewRaw(
			`UPDATE external_games SET store_link = ?, updated_at = now() WHERE id = ?`, link, r.ID,
		).Exec(ctx); e != nil {
			slog.Error("store_link_refresh: update store_link", "err", e, "id", r.ID)
		}
	}

	markItemCompleted(ctx, w.DB, &item, "store_link_refresh: markItemCompleted")
	storeLinkCheckJobCompletion(ctx, w.DB, item.JobID)
	return nil
}

func storeLinkCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	remaining, ok := countJobItems(ctx, db, jobID, "status NOT IN ('completed','failed','skipped')", "store_link_refresh: check job completion")
	if !ok || remaining > 0 {
		return
	}
	finalizeJobCompleted(ctx, db, jobID, "store_link_refresh: finalize", false)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/worker/tasks/ -run TestStoreLinkRefresh -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/store_link_refresh.go internal/worker/tasks/store_link_refresh_test.go
git commit -m "feat: add store-link enrichment item worker (#831)"
```

---

## Task 10: Resolver factory + worker registration in `serve.go`

**Files:**
- Modify: `cmd/nexorious/serve.go` — add `buildStoreLinkResolverFactory`; register both new workers in **both** `river.AddWorker` blocks (~L191-213 and ~L275-300)

- [ ] **Step 1: Add the resolver factory**

After `buildAdapterFactory` (~end of file), add:

```go
func buildStoreLinkResolverFactory(db *bun.DB, encrypter *crypto.Encrypter) func(context.Context, string, string) (storelink.Resolver, error) {
	return func(ctx context.Context, storefront, userID string) (storelink.Resolver, error) {
		switch storefront {
		case "steam":
			return storelink.NewSteamResolver(), nil
		case "gog":
			return storelink.NewGOGResolver(nil, ""), nil
		case "epic-games-store":
			return storelink.NewEpicResolver(nil, ""), nil
		case "playstation-store":
			// PSN concept resolution needs the user's NPSSO token.
			var encCreds []byte
			err := db.NewRaw(
				`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'playstation-store'`, userID,
			).Scan(ctx, &encCreds)
			if err != nil {
				return nil, fmt.Errorf("load psn creds: %w", err)
			}
			plain, err := encrypter.Decrypt(encCreds)
			if err != nil {
				return nil, fmt.Errorf("decrypt psn creds: %w", err)
			}
			var creds struct {
				NPSSOToken string `json:"npsso_token"`
			}
			if err := json.Unmarshal(plain, &creds); err != nil {
				return nil, fmt.Errorf("parse psn creds: %w", err)
			}
			return storelink.NewPSNResolver(psnsvc.NewClient(), creds.NPSSOToken), nil
		default:
			return nil, fmt.Errorf("unknown storefront: %s", storefront)
		}
	}
}
```

Add the `storelink` import: `"github.com/drzero42/nexorious/internal/services/storelink"`.

- [ ] **Step 2: Construct + register the workers (primary block, ~L181-213)**

Near where `metaDispatchWorker` is constructed, add:

```go
	storeLinkResolverFor := buildStoreLinkResolverFactory(db, encrypter)
	storeLinkDispatchWorker := &tasks.StoreLinkRefreshDispatchWorker{DB: db}
	storeLinkItemWorker := &tasks.StoreLinkRefreshItemWorker{DB: db, ResolverFor: storeLinkResolverFor}
```

And in the `river.AddWorker(workers, …)` list:

```go
	river.AddWorker(workers, storeLinkDispatchWorker)
	river.AddWorker(workers, storeLinkItemWorker)
```

> `StoreLinkRefreshDispatchWorker.RiverClient` is set after the River client is created, the same way the other dispatch workers get theirs. Find where `dispatchSyncWorker.RiverClient`/`metaDispatchWorker.RiverClient` are assigned post-client-construction and add `storeLinkDispatchWorker.RiverClient = riverClient` alongside them. (If those workers are assigned via field at construction using an already-built client, mirror that exact pattern instead.)

- [ ] **Step 3: Register in the DB-reconnect re-init block (~L275-300)**

In the second block that rebuilds workers with `newDB`, mirror Step 2 using `newDB`:

```go
	newStoreLinkResolverFor := buildStoreLinkResolverFactory(newDB, encrypter)
	newStoreLinkDispatch := &tasks.StoreLinkRefreshDispatchWorker{DB: newDB}
	newStoreLinkItem := &tasks.StoreLinkRefreshItemWorker{DB: newDB, ResolverFor: newStoreLinkResolverFor}
	river.AddWorker(newWorkers, newStoreLinkDispatch)
	river.AddWorker(newWorkers, newStoreLinkItem)
```

And assign `newStoreLinkDispatch.RiverClient` wherever the re-init block assigns the other dispatch workers' clients.

- [ ] **Step 4: Build**

Run: `go build ./... && go vet ./cmd/...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "feat: wire store-link enrichment workers and resolver factory (#831)"
```

---

## Task 11: Trigger scoped enrichment on sync completion

**Files:**
- Modify: `internal/worker/tasks/sync.go` — `SyncCheckJobCompletion` signature + enqueue
- Modify: `internal/api/sync.go` — update the two call sites (~L970, ~L1001)
- Test: `internal/worker/tasks/sync_test.go` — assert a dispatch job is enqueued/created on completion

- [ ] **Step 1: Change `SyncCheckJobCompletion` to accept a River client and enqueue**

Update the signature:

```go
func SyncCheckJobCompletion(ctx context.Context, db *bun.DB, riverClient *river.Client[pgx.Tx], jobID string) {
```

In the success path, immediately after the existing `emitSyncDiff(ctx, db, jobID, userID)` call (the job is now fully finalized and `userID, storefront` are in scope), add:

```go
	// Kick off scoped, incremental store-link enrichment for this storefront.
	if riverClient != nil && userID != "" && storefront != "" {
		if _, err := riverClient.Insert(ctx, StoreLinkRefreshDispatchArgs{
			UserID: userID, Storefront: storefront, Force: false,
		}, nil); err != nil {
			slog.Error("sync: enqueue store_link_refresh", "err", err, "job_id", jobID)
		}
	}
```

- [ ] **Step 2: Update all in-package call sites**

In `sync.go`, every `SyncCheckJobCompletion(ctx, w.DB, …jobID)` call (lines ~305, 438, 445, 514, 568, 575, 582, 618, 625, 644, 668, 697, 708, 713, 865) becomes `SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, …jobID)`. All three owning workers (`DispatchSyncWorker`, `IGDBMatchWorker`, `UserGameWorker`) have a `RiverClient` field.

- [ ] **Step 3: Update the API call sites**

In `internal/api/sync.go` (~L970 and ~L1001), change `tasks.SyncCheckJobCompletion(ctx, h.db, …JobID)` to `tasks.SyncCheckJobCompletion(ctx, h.db, h.riverClient, …JobID)`.

- [ ] **Step 4: Write the failing test**

In `sync_test.go`, add a test that drives a sync job to completion and asserts a `store_link_refresh_dispatch` River job was inserted. If the package's tests use a real River client against `testDB`, assert via the `river_job` table:

```go
func TestSyncCompletion_EnqueuesStoreLinkRefresh(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUserForSync(t)

	// Create a sync job with a single completed item so SyncCheckJobCompletion finalizes it.
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, true, now())`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'x', 'x', '{}', 'completed', '{}', '[]', now())`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	rc := newTestRiverClient(t) // mirror however sync_test builds its River client; if none, pass a real client built on testDB's pool
	SyncCheckJobCompletion(ctx, testDB, rc, jobID)

	var n int
	if err := testDB.NewRaw(
		`SELECT count(*) FROM river_job WHERE kind = 'store_link_refresh_dispatch'`,
	).Scan(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 store_link_refresh_dispatch job, got %d", n)
	}
}
```

> If `sync_test.go` has no existing River-client helper, follow the pattern used by other tests in `internal/worker/tasks` that exercise enqueue (they construct a `river.NewClient` against the pgx pool). If building a River client in tests is impractical here, instead assert the *non-enqueue* safety (passing `nil` client does not panic and finalizes the job) and cover the enqueue path in an integration test. Prefer the real-client assertion if the helper exists.

- [ ] **Step 5: Run tests + build**

Run: `go build ./... && go test ./internal/worker/tasks/ -run 'TestSyncCompletion_EnqueuesStoreLinkRefresh|TestSyncCheck' -v && go test ./internal/api/ -run TestSync -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/api/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: enqueue scoped store-link enrichment on sync completion (#831)"
```

---

## Task 12: Admin manual trigger (API + route + frontend button)

**Files:**
- Modify: `internal/api/games.go` — `HandleStartStoreLinkRefreshJob`
- Modify: `internal/api/router.go` — register the route
- Modify: `ui/frontend/src/api/admin.ts` — `startStoreLinkRefreshJob`
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` — button
- Test: `internal/api/games_test.go` — admin-only guard + enqueue

- [ ] **Step 1: Add the handler**

In `games.go`, after `HandleStartMetadataRefreshJob`:

```go
// HandleStartStoreLinkRefreshJob handles POST /api/games/store-links/refresh-job.
// Admin-only: dispatches a global, forced store-link re-resolution.
func (h *GamesHandler) HandleStartStoreLinkRefreshJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !auth.IsAdminFromContext(c) {
		return echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}
	if h.riverClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "worker not available")
	}
	if _, err := h.riverClient.Insert(c.Request().Context(), tasks.StoreLinkRefreshDispatchArgs{Force: true}, nil); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue store link refresh")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Store link refresh job queued",
	})
}
```

- [ ] **Step 2: Register the route**

In `router.go`, next to the metadata refresh-job route registration (search for `metadata/refresh-job`), add the matching route:

```go
	gamesGroup.POST("/store-links/refresh-job", gamesHandler.HandleStartStoreLinkRefreshJob)
```

(Match the exact group variable and path style used for the metadata route — it registers under the `/api/games` group.)

- [ ] **Step 3: Write the failing handler test**

In `games_test.go`, mirror the existing `HandleStartMetadataRefreshJob` test: assert 403 for a non-admin context and 200 + a queued `store_link_refresh_dispatch` River job for an admin. (Reuse that test's River-client and admin-context setup.)

```go
func TestHandleStartStoreLinkRefreshJob_AdminOnly(t *testing.T) {
	// non-admin → 403
	// admin → 200 and one river_job with kind store_link_refresh_dispatch and args.force == true
	// (follow the metadata refresh-job test in this file for handler/context/river setup)
}
```

Fill the body using the metadata test as the template (same helpers).

- [ ] **Step 4: Run the Go test**

Run: `go test ./internal/api/ -run TestHandleStartStoreLinkRefreshJob -v`
Expected: PASS (after Steps 1-2).

- [ ] **Step 5: Add the frontend API call**

In `ui/frontend/src/api/admin.ts`, after `startMetadataRefreshJob`:

```ts
/**
 * Start a store-link refresh job (admin only). Re-resolves every storefront
 * deep-link from upstream.
 */
export async function startStoreLinkRefreshJob(): Promise<{ success: boolean; message: string }> {
  return apiClient.post<{ success: boolean; message: string }>('/games/store-links/refresh-job', {});
}
```

(Match the actual `apiClient` call shape used by `startMetadataRefreshJob` in this file.)

- [ ] **Step 6: Add the button**

In `maintenance.tsx`, mirror the metadata refresh section. Add a handler:

```tsx
  const [isStoreLinkLoading, setIsStoreLinkLoading] = useState(false);

  const handleStartStoreLinkRefresh = async () => {
    try {
      setIsStoreLinkLoading(true);
      await adminApi.startStoreLinkRefreshJob();
      toast.success('Store link refresh job started');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start store link refresh';
      toast.error(message);
    } finally {
      setIsStoreLinkLoading(false);
    }
  };
```

And a card/button mirroring the IGDB refresh section (a `<Button onClick={handleStartStoreLinkRefresh} disabled={isStoreLinkLoading}>` labelled "Refresh store links", with a `RefreshCw` icon). Do not mention "force" in the copy — a manual run implicitly re-resolves everything. Suggested description: "Re-fetch storefront product links for your synced games."

- [ ] **Step 7: Frontend checks**

Run (from `ui/frontend/`): `npm run check && npm run knip`
Expected: no errors, no unused-export findings.

- [ ] **Step 8: Commit**

```bash
git add internal/api/games.go internal/api/router.go internal/api/games_test.go ui/frontend/src/api/admin.ts ui/frontend/src/routes/_authenticated/admin/maintenance.tsx
git commit -m "feat: add admin store-link refresh trigger (API + UI) (#831)"
```

---

## Final verification

- [ ] **Run the full backend suite**

Run: `go build ./... && go test -timeout 600s ./...`
Expected: all PASS.

- [ ] **Run the frontend suite**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all PASS.

- [ ] **Lint**

Run: `golangci-lint run`
Expected: zero findings (watch `errcheck`/`gosec` — every `_ =` discard must be justified per CLAUDE.md; the `//nolint` annotations above cover the intended ones).

- [ ] **Manual smoke (optional but recommended)**

Build, run a Steam sync end-to-end, confirm: the sync job completes, a `store_link_refresh` job appears and completes, `external_games.store_link` is populated for Steam rows, and the game details page renders the Steam storefront as a clickable link.

---

## Notes / risks carried from the spec

- **PSN concepts JSON shape is unconfirmed.** `ResolveConceptID` (Task 7) parses an assumed `{"concepts":[{"id":...}]}` shape; verify against a live response during implementation and adjust the struct + the test fixture together. The endpoint and auth are confirmed; only the field path is provisional.
- **GOG `api.gog.com/products/{id}` is treated as public (no auth).** If implementation finds it requires authorization, build the resolver with the GOG access token obtained via the refresh-token exchange (the GOG adapter already performs this) and pass it through `NewGOGResolver`.
- **Backfill:** columns ship empty. Existing rows get links only after their storefront re-syncs (which enqueues scoped enrichment) or an admin runs the manual refresh. No migration-time backfill (by design).
- **Humble-bundle and non-sync storefronts** are intentionally excluded from `resolvableStorefronts`; they render no link.
