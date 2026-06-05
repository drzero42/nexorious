# Humble Bundle Sync Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Humble Bundle as a real sync source that imports only DRM-free games downloaded directly from Humble (never ebooks/audio/video, never Steam-key-only titles), reusing the existing `storefrontadapter.Adapter` pattern and 3-stage sync pipeline.

**Architecture:** A new `internal/services/humble/` package (client + adapter + models) mirrors the PSN adapter. The HTTP client authenticates with a pasted `_simpleauth_sess` session cookie, lists order gamekeys, fetches each order, and the adapter applies a whitelist-platform + non-empty-download-URL + launcher-blocklist filter to yield `ExternalGameEntry` values. Thin wiring adds the source to the API (PUT/GET/DELETE connection routes), the adapter factory, the job-source enum, platform→collection mapping, a `platform_storefronts` migration, and the React sync UI.

**Tech Stack:** Go 1.26, Bun ORM, Echo v5, River, `golang.org/x/time/rate`; React 19 + TypeScript, TanStack Query/Router, React Hook Form + Zod, Vitest.

---

## Naming decisions (read before starting)

This plan **intentionally deviates from the design spec on two points** — both confirmed with the project owner:

1. **Sync-source id is `humble-bundle`** (id == DB slug == storefront `name`), **not** the spec's short `humble`. The display name remains `"Humble Bundle"`. So every place that takes a sync-source id (`jobs.source`, `supportedStorefronts`, the `:storefront` URL segment, the `SyncStorefront` TS enum value, the adapter-factory `case`, the `StorefrontToCollectionSlug` key) uses the string **`humble-bundle`**. `StorefrontToCollectionSlug("humble-bundle")` returns `"humble-bundle"` (identity). Wherever the spec text says the id is `humble`, use `humble-bundle`.

2. **`ConnectedSummary` renders just `"Connected"` when given no name.** Humble's order API exposes no account/username, so the Humble connection card passes no name and the shared component must handle that. This is a one-line change to the shared component plus a test (Task 6, Step 1).

The Go **package** is still named `humble` (Go identifiers cannot contain hyphens) and is imported as `humblesvc`. Only the *string id* is `humble-bundle`.

Already seeded in `internal/db/migrations/20260503000001_initial.up.sql` (do **not** re-add): the `('humble-bundle', 'Humble Bundle', 'humble-bundle-icon-light.svg', …)` storefronts row, and the `('pc-linux', 'humble-bundle')` `platform_storefronts` association. Logos already exist at `ui/frontend/public/logos/storefronts/humble-bundle/`.

---

## File Structure

**New files (Go):**
- `internal/services/humble/models.go` — order JSON structs (`Order`, `Subproduct`, `Download`, `DownloadStruct`).
- `internal/services/humble/client.go` — HTTP client, rate limiter, `ErrCredentials`, `Verify`/`ListGamekeys`/`GetOrder`.
- `internal/services/humble/export_test.go` — test-only setters (`SetHTTPClient`, `SetBaseURL`, `SetLimiter`).
- `internal/services/humble/client_test.go` — client unit tests (httptest).
- `internal/services/humble/adapter.go` — `Adapter`, `NewAdapter`, `GetLibrary`, filtering + platform mapping.
- `internal/services/humble/adapter_test.go` — adapter unit tests (fake client).
- `internal/db/migrations/20260604000003_humble_platform_storefronts.up.sql` / `.down.sql`.

**Modified files (Go):**
- `internal/db/models/jobs.go` — add `JobSourceHumbleBundle`.
- `internal/services/platformresolution/resolution.go` — add `humble-bundle` case.
- `cmd/nexorious/serve.go` — adapter-factory `case "humble-bundle"` + import.
- `internal/api/sync.go` — `supportedStorefronts`, `storefrontDisplayName`, `HumbleClient` interface, `ErrInvalidHumbleCookie`, response struct, three handlers, route registration, struct field, `NewSyncHandler` param.
- `internal/api/router.go` — `humbleClientAdapter` bridge + handler construction.
- `internal/api/sync_test.go` — extend the four existing `NewSyncHandler` call sites with a typed-nil Humble client.

**New files (frontend):**
- `ui/frontend/src/components/sync/humble-connection-card.tsx`.

**Modified files (frontend):**
- `ui/frontend/src/components/sync/connection/connected-summary.tsx` — "Connected" fallback.
- `ui/frontend/src/components/sync/connection/connected-summary.test.tsx` — new branch test (create if absent).
- `ui/frontend/src/types/sync.ts` — enum value, supported list, display info, response types.
- `ui/frontend/src/api/sync.ts` — `connectHumble`/`getHumbleStatus`/`disconnectHumble` + request/response types.
- `ui/frontend/src/hooks/use-sync.ts` — `syncKeys.humbleStatus`, `useConnectHumble`/`useHumbleStatus`/`useDisconnectHumble`.
- `ui/frontend/src/hooks/index.ts` — export the three hooks.
- `ui/frontend/src/components/sync/index.ts` — export `HumbleConnectionCard`.
- `ui/frontend/src/routes/_authenticated/sync/index.tsx` — status fetch + credentials-error derivation.
- `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` — status fetch, credentials-error derivation, conditional card render.

---

## Task 1: Humble service package — models + client

**Files:**
- Create: `internal/services/humble/models.go`
- Create: `internal/services/humble/client.go`
- Create: `internal/services/humble/export_test.go`
- Test: `internal/services/humble/client_test.go`

These are pure unit tests over an `httptest.Server` — no database, no testcontainers (matches the PSN/Steam/GOG adapter-test convention).

- [ ] **Step 1: Write the failing client test**

Create `internal/services/humble/client_test.go`:

```go
package humble

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

// newTestClient points a Client at the given test server with an unlimited rate
// limiter so tests don't sleep.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetBaseURL(srv.URL)
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))
	return c
}

func TestVerify_SendsCookieAndHeader(t *testing.T) {
	var gotCookie, gotRequestedBy, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotRequestedBy = r.Header.Get("X-Requested-By")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if err := newTestClient(srv).Verify(context.Background(), "cookie123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCookie != "_simpleauth_sess=cookie123" {
		t.Errorf("Cookie header = %q, want %q", gotCookie, "_simpleauth_sess=cookie123")
	}
	if gotRequestedBy != "hb_android_app" {
		t.Errorf("X-Requested-By = %q, want hb_android_app", gotRequestedBy)
	}
	if gotPath != "/api/v1/user/order" {
		t.Errorf("path = %q, want /api/v1/user/order", gotPath)
	}
}

func TestVerify_401ReturnsErrCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := newTestClient(srv).Verify(context.Background(), "bad")
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("expected ErrCredentials on 401, got %v", err)
	}
}

func TestListGamekeys_DecodesAndFiltersEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"gamekey":"AAA"},{"gamekey":""},{"gamekey":"BBB"}]`))
	}))
	defer srv.Close()

	keys, err := newTestClient(srv).ListGamekeys(context.Background(), "cookie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 || keys[0] != "AAA" || keys[1] != "BBB" {
		t.Errorf("keys = %v, want [AAA BBB]", keys)
	}
}

func TestGetOrder_DecodesNestedStructure(t *testing.T) {
	// Real order JSON, scrubbed: one windows game subproduct plus an ebook
	// subproduct (no game-platform download). Confirms json tags are correct.
	const orderJSON = `{
	  "gamekey": "GK1",
	  "subproducts": [
	    {
	      "machine_name": "aquaria",
	      "human_name": "Aquaria",
	      "downloads": [
	        {"platform":"windows","machine_name":"aquaria_win","download_struct":[{"url":{"web":"https://dl.example/aquaria.zip"}}]}
	      ]
	    },
	    {
	      "machine_name": "world_of_goo_ebook",
	      "human_name": "World of Goo (ebook)",
	      "downloads": [
	        {"platform":"ebook","machine_name":"wog_pdf","download_struct":[{"url":{"web":"https://dl.example/wog.pdf"}}]}
	      ]
	    }
	  ]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("all_tpkds") != "true" {
			t.Errorf("expected all_tpkds=true, got query %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(orderJSON))
	}))
	defer srv.Close()

	order, err := newTestClient(srv).GetOrder(context.Background(), "cookie", "GK1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.Gamekey != "GK1" || len(order.Subproducts) != 2 {
		t.Fatalf("decoded order = %+v", order)
	}
	sp := order.Subproducts[0]
	if sp.MachineName != "aquaria" || sp.HumanName != "Aquaria" {
		t.Errorf("subproduct[0] = %+v", sp)
	}
	if len(sp.Downloads) != 1 || sp.Downloads[0].Platform != "windows" ||
		sp.Downloads[0].DownloadStruct[0].URL.Web != "https://dl.example/aquaria.zip" {
		t.Errorf("download not decoded: %+v", sp.Downloads)
	}
}

func TestGetOrder_403ReturnsErrCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetOrder(context.Background(), "bad", "GK1")
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("expected ErrCredentials on 403, got %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails to compile**

Run: `go test ./internal/services/humble/ -run TestVerify -v`
Expected: FAIL — build error, `undefined: NewClient` / `Client` / `Order` (package has no non-test files yet).

- [ ] **Step 3: Write `models.go`**

Create `internal/services/humble/models.go`:

```go
// Package humble implements a storefront sync adapter for Humble Bundle. It
// imports only DRM-free games downloaded directly from Humble — never ebooks,
// audio, video, or games for which only a third-party (Steam) key is granted.
package humble

// Order is one Humble order's detail (GET /api/v1/order/{gamekey}). Only the
// fields the adapter reads are modelled; tpkd_dict (third-party keys) is
// deliberately omitted so Steam-key-only titles are never imported.
type Order struct {
	Gamekey     string       `json:"gamekey"`
	Subproducts []Subproduct `json:"subproducts"`
}

// Subproduct is one item in an order: a game, a bundled ebook/audio/video, or a
// promo/info stub. Game-ness is decided by its downloads (see adapter.gameEntry).
type Subproduct struct {
	MachineName string     `json:"machine_name"`
	HumanName   string     `json:"human_name"`
	Downloads   []Download `json:"downloads"`
}

// Download is one platform-specific download for a subproduct.
type Download struct {
	Platform       string           `json:"platform"`
	MachineName    string           `json:"machine_name"`
	DownloadStruct []DownloadStruct `json:"download_struct"`
}

// DownloadStruct is one downloadable file within a Download. A non-empty
// URL.Web is what distinguishes a real download from an empty stub.
type DownloadStruct struct {
	URL struct {
		Web string `json:"web"`
	} `json:"url"`
}
```

- [ ] **Step 4: Write `client.go`**

Create `internal/services/humble/client.go`:

```go
package humble

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL    = "https://www.humblebundle.com"
	requestedByHeader = "hb_android_app"
)

// ErrCredentials is returned when Humble rejects the session cookie (401/403).
// The adapter wraps it into storefrontadapter.ErrCredentials.
var ErrCredentials = errors.New("humble: invalid session cookie")

// Client talks to the Humble Bundle order API using a pasted _simpleauth_sess
// session cookie. It rate-limits requests to 5/sec (matching steam/psn).
type Client struct {
	httpClient *http.Client
	baseURL    string
	limiter    *rate.Limiter
}

// NewClient creates a Humble client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient: http.DefaultClient,
		baseURL:    defaultBaseURL,
		limiter:    rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
	}
}

// doGet performs a rate-limited authenticated GET and returns the response body.
// A 401/403 maps to ErrCredentials; any other non-200 is a generic error.
func (c *Client) doGet(ctx context.Context, cookie, path string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("humble: rate limiter wait: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("humble: build request: %w", err)
	}
	req.Header.Set("Cookie", "_simpleauth_sess="+cookie)
	req.Header.Set("X-Requested-By", requestedByHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("humble: request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("humble: request %s: unexpected status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("humble: read %s: %w", path, err)
	}
	return body, nil
}

// Verify confirms the session cookie is valid by hitting the order-list
// endpoint. Returns ErrCredentials on 401/403.
func (c *Client) Verify(ctx context.Context, cookie string) error {
	_, err := c.doGet(ctx, cookie, "/api/v1/user/order")
	return err
}

// ListGamekeys returns the gamekeys for every order owned by the cookie's user.
func (c *Client) ListGamekeys(ctx context.Context, cookie string) ([]string, error) {
	body, err := c.doGet(ctx, cookie, "/api/v1/user/order")
	if err != nil {
		return nil, err
	}
	var orders []struct {
		Gamekey string `json:"gamekey"`
	}
	if err := json.Unmarshal(body, &orders); err != nil {
		return nil, fmt.Errorf("humble: decode gamekeys: %w", err)
	}
	keys := make([]string, 0, len(orders))
	for _, o := range orders {
		if o.Gamekey != "" {
			keys = append(keys, o.Gamekey)
		}
	}
	return keys, nil
}

// GetOrder fetches one order's full detail (with all third-party-key data, which
// the adapter ignores).
func (c *Client) GetOrder(ctx context.Context, cookie, gamekey string) (*Order, error) {
	body, err := c.doGet(ctx, cookie, "/api/v1/order/"+url.PathEscape(gamekey)+"?all_tpkds=true")
	if err != nil {
		return nil, err
	}
	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("humble: decode order %s: %w", gamekey, err)
	}
	return &order, nil
}
```

- [ ] **Step 5: Write `export_test.go`**

Create `internal/services/humble/export_test.go`:

```go
package humble

import (
	"net/http"

	"golang.org/x/time/rate"
)

// Test-only setters, mirroring internal/services/psn/export_test.go.
func (c *Client) SetHTTPClient(h *http.Client)  { c.httpClient = h }
func (c *Client) SetBaseURL(u string)           { c.baseURL = u }
func (c *Client) SetLimiter(l *rate.Limiter)    { c.limiter = l }
```

- [ ] **Step 6: Run the client tests to verify they pass**

Run: `go test ./internal/services/humble/ -v`
Expected: PASS — all `TestVerify*`, `TestListGamekeys*`, `TestGetOrder*` pass.

- [ ] **Step 7: Commit**

```bash
git add internal/services/humble/models.go internal/services/humble/client.go internal/services/humble/export_test.go internal/services/humble/client_test.go
git commit -m "feat(sync): add Humble Bundle API client"
```

---

## Task 2: Humble adapter — filtering + platform mapping

**Files:**
- Create: `internal/services/humble/adapter.go`
- Test: `internal/services/humble/adapter_test.go`

The adapter is the testable core (filtering rule, platform mapping, blocklist, dedup-friendly emission, credential-error wrapping). It depends on a small `libraryClient` interface so tests use a fake (mirrors `internal/services/epic/adapter_test.go`).

- [ ] **Step 1: Write the failing adapter test**

Create `internal/services/humble/adapter_test.go`:

```go
package humble

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeClient satisfies libraryClient for adapter tests.
type fakeClient struct {
	gamekeys  []string
	orders    map[string]*Order
	listErr   error
	orderErrs map[string]error
}

func (f *fakeClient) ListGamekeys(_ context.Context, _ string) ([]string, error) {
	return f.gamekeys, f.listErr
}

func (f *fakeClient) GetOrder(_ context.Context, _, gamekey string) (*Order, error) {
	if f.orderErrs != nil {
		if err := f.orderErrs[gamekey]; err != nil {
			return nil, err
		}
	}
	return f.orders[gamekey], nil
}

// gameDownload builds a qualifying game download for a platform.
func gameDownload(platform string) Download {
	return Download{
		Platform:       platform,
		DownloadStruct: []DownloadStruct{{URL: struct{ Web string `json:"web"` }{Web: "https://dl.example/file"}}},
	}
}

// collect runs the adapter over a single-order fake and returns all yielded entries.
func collect(t *testing.T, order *Order) []storefrontadapter.ExternalGameEntry {
	t.Helper()
	fc := &fakeClient{gamekeys: []string{"GK1"}, orders: map[string]*Order{"GK1": order}}
	a := NewAdapter(fc, "cookie")
	var got []storefrontadapter.ExternalGameEntry
	err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		got = append(got, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("GetLibrary error: %v", err)
	}
	return got
}

func TestGetLibrary_FilteringRule(t *testing.T) {
	tests := []struct {
		name    string
		sub     Subproduct
		include bool
	}{
		{
			name:    "windows game included",
			sub:     Subproduct{MachineName: "aquaria", HumanName: "Aquaria", Downloads: []Download{gameDownload("windows")}},
			include: true,
		},
		{
			name:    "android-only game included",
			sub:     Subproduct{MachineName: "aquaria_android", HumanName: "Aquaria", Downloads: []Download{gameDownload("android")}},
			include: true,
		},
		{
			name:    "ebook excluded",
			sub:     Subproduct{MachineName: "wog_ebook", HumanName: "WoG ebook", Downloads: []Download{gameDownload("ebook")}},
			include: false,
		},
		{
			name:    "audio excluded",
			sub:     Subproduct{MachineName: "ost", HumanName: "Soundtrack", Downloads: []Download{gameDownload("audio")}},
			include: false,
		},
		{
			name:    "video excluded",
			sub:     Subproduct{MachineName: "doc", HumanName: "Documentary", Downloads: []Download{gameDownload("video")}},
			include: false,
		},
		{
			name:    "asmjs excluded",
			sub:     Subproduct{MachineName: "webgame", HumanName: "Web Game", Downloads: []Download{gameDownload("asmjs")}},
			include: false,
		},
		{
			name:    "freegame_info stub excluded (empty downloads)",
			sub:     Subproduct{MachineName: "civ3_freegame_info", HumanName: "Civ III", Downloads: nil},
			include: false,
		},
		{
			name: "steam-key-only excluded (game-platform download but empty download_struct)",
			sub: Subproduct{MachineName: "abzu", HumanName: "ABZU", Downloads: []Download{
				{Platform: "windows", DownloadStruct: nil},
			}},
			include: false,
		},
		{
			name: "empty url.web excluded",
			sub: Subproduct{MachineName: "broken", HumanName: "Broken", Downloads: []Download{
				{Platform: "windows", DownloadStruct: []DownloadStruct{{}}},
			}},
			include: false,
		},
		{
			name:    "uplayclient launcher excluded despite real windows download",
			sub:     Subproduct{MachineName: "uplayclient", HumanName: "Uplay Client (will download latest version)", Downloads: []Download{gameDownload("windows")}},
			include: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{tt.sub}})
			if tt.include && len(got) != 1 {
				t.Fatalf("expected 1 entry, got %d: %+v", len(got), got)
			}
			if !tt.include && len(got) != 0 {
				t.Fatalf("expected 0 entries, got %d: %+v", len(got), got)
			}
		})
	}
}

func TestGetLibrary_EntryFields(t *testing.T) {
	got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "aquaria", HumanName: "Aquaria", Downloads: []Download{
			gameDownload("windows"), gameDownload("mac"), gameDownload("linux"),
		}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	e := got[0]
	if e.ExternalID != "aquaria" || e.Title != "Aquaria" {
		t.Errorf("ExternalID/Title = %q/%q", e.ExternalID, e.Title)
	}
	if e.PlaytimeHours != 0 || e.OwnershipStatus != "owned" || e.IsSubscription {
		t.Errorf("unexpected fields: %+v", e)
	}
	platforms := append([]string(nil), e.Platforms...)
	sort.Strings(platforms)
	want := []string{"mac", "pc-linux", "pc-windows"}
	if len(platforms) != 3 || platforms[0] != want[0] || platforms[1] != want[1] || platforms[2] != want[2] {
		t.Errorf("platforms = %v, want %v", platforms, want)
	}
}

func TestGetLibrary_GameWithDirectDownloadAndSteamKeyIncluded(t *testing.T) {
	// A game that has both a real direct download and (separately, in tpkd_dict
	// which we never read) a Steam key is included because it is downloadable.
	got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "braid", HumanName: "Braid", Downloads: []Download{gameDownload("windows")}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
}

func TestGetLibrary_SeparatePCAndAndroidEditionsBothEmitted(t *testing.T) {
	// PC and Android editions share a human_name but have distinct machine_names;
	// the adapter emits both faithfully (the pipeline collapses them downstream).
	got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "aquaria", HumanName: "Aquaria", Downloads: []Download{gameDownload("windows")}},
		{MachineName: "aquaria_android", HumanName: "Aquaria", Downloads: []Download{gameDownload("android")}},
	}})
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].ExternalID != "aquaria" || got[1].ExternalID != "aquaria_android" {
		t.Errorf("external ids = %q, %q", got[0].ExternalID, got[1].ExternalID)
	}
}

func TestGetLibrary_SkipsFailingOrder(t *testing.T) {
	fc := &fakeClient{
		gamekeys: []string{"GK1", "GK2"},
		orders: map[string]*Order{
			"GK2": {Gamekey: "GK2", Subproducts: []Subproduct{
				{MachineName: "braid", HumanName: "Braid", Downloads: []Download{gameDownload("windows")}},
			}},
		},
		orderErrs: map[string]error{"GK1": errors.New("boom")},
	}
	a := NewAdapter(fc, "cookie")
	var got []storefrontadapter.ExternalGameEntry
	err := a.GetLibrary(context.Background(), 10, func(b []storefrontadapter.ExternalGameEntry) error {
		got = append(got, b...)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error (bad order skipped), got %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "braid" {
		t.Errorf("expected only braid from GK2, got %+v", got)
	}
}

func TestGetLibrary_ListErrCredentialsWrapped(t *testing.T) {
	fc := &fakeClient{listErr: ErrCredentials}
	a := NewAdapter(fc, "cookie")
	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials, got %v", err)
	}
}

func TestGetLibrary_OrderErrCredentialsWrapped(t *testing.T) {
	fc := &fakeClient{
		gamekeys:  []string{"GK1"},
		orderErrs: map[string]error{"GK1": ErrCredentials},
	}
	a := NewAdapter(fc, "cookie")
	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials on per-order auth failure, got %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails to compile**

Run: `go test ./internal/services/humble/ -run TestGetLibrary -v`
Expected: FAIL — build error, `undefined: NewAdapter`.

- [ ] **Step 3: Write `adapter.go`**

Create `internal/services/humble/adapter.go`:

```go
package humble

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// libraryClient is the subset of *Client the adapter needs; declared as an
// interface so tests can supply a fake.
type libraryClient interface {
	ListGamekeys(ctx context.Context, cookie string) ([]string, error)
	GetOrder(ctx context.Context, cookie, gamekey string) (*Order, error)
}

// gamePlatforms is the whitelist of Humble download platforms that qualify a
// subproduct as a game, mapped to canonical platforms.name slugs. Every other
// platform (ebook, audio, video, asmjs, …) is excluded by absence — a whitelist
// handles unknown future non-game platforms safely.
var gamePlatforms = map[string]string{
	"windows": "pc-windows",
	"mac":     "mac",
	"linux":   "pc-linux",
	"android": "android",
}

// launcherBlocklist holds machine_names that pass the platform filter but are
// not games. A scan of a full 138-order test library found uplayclient to be
// the only such launcher. Extend only with confirmed entries.
var launcherBlocklist = map[string]bool{
	"uplayclient": true,
}

// Adapter wraps a Humble client with a session cookie and implements
// storefrontadapter.Adapter.
type Adapter struct {
	client libraryClient
	cookie string
}

// NewAdapter returns a storefrontadapter.Adapter for Humble Bundle.
func NewAdapter(client libraryClient, cookie string) storefrontadapter.Adapter {
	return &Adapter{client: client, cookie: cookie}
}

func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if batchSize <= 0 {
		batchSize = 10
	}

	gamekeys, err := a.client.ListGamekeys(ctx, a.cookie)
	if errors.Is(err, ErrCredentials) {
		return fmt.Errorf("%w: humble session cookie rejected", storefrontadapter.ErrCredentials)
	}
	if err != nil {
		return fmt.Errorf("humble: list gamekeys: %w", err)
	}

	batch := make([]storefrontadapter.ExternalGameEntry, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := onBatch(batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for _, gk := range gamekeys {
		order, err := a.client.GetOrder(ctx, a.cookie, gk)
		if errors.Is(err, ErrCredentials) {
			return fmt.Errorf("%w: humble session cookie rejected", storefrontadapter.ErrCredentials)
		}
		if err != nil {
			// A single failing order is logged and skipped so one bad order
			// doesn't sink the whole sync.
			slog.Error("humble: skipping order", "gamekey", gk, "err", err)
			continue
		}
		if order == nil {
			continue
		}
		for i := range order.Subproducts {
			entry, ok := gameEntry(&order.Subproducts[i])
			if !ok {
				continue
			}
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	return flush()
}

// gameEntry applies the filtering rule to a subproduct and returns an
// ExternalGameEntry plus true if it is a DRM-free game, false otherwise. A
// subproduct qualifies iff it is not in the launcher blocklist and has at least
// one download whose platform is in gamePlatforms with a non-empty url.web.
func gameEntry(sp *Subproduct) (storefrontadapter.ExternalGameEntry, bool) {
	if launcherBlocklist[sp.MachineName] {
		return storefrontadapter.ExternalGameEntry{}, false
	}

	var slugs []string
	seen := make(map[string]bool)
	for _, d := range sp.Downloads {
		slug, ok := gamePlatforms[d.Platform]
		if !ok {
			continue
		}
		if len(d.DownloadStruct) == 0 || d.DownloadStruct[0].URL.Web == "" {
			continue
		}
		if !seen[slug] {
			seen[slug] = true
			slugs = append(slugs, slug)
		}
	}
	if len(slugs) == 0 {
		return storefrontadapter.ExternalGameEntry{}, false
	}

	return storefrontadapter.ExternalGameEntry{
		ExternalID:      sp.MachineName,
		Title:           sp.HumanName,
		PlaytimeHours:   0,
		Platforms:       slugs,
		OwnershipStatus: "owned",
		IsSubscription:  false,
	}, true
}
```

- [ ] **Step 4: Run the adapter tests to verify they pass**

Run: `go test ./internal/services/humble/ -v`
Expected: PASS — all Task 1 + Task 2 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/services/humble/adapter.go internal/services/humble/adapter_test.go
git commit -m "feat(sync): add Humble Bundle library adapter with DRM-free filtering"
```

---

## Task 3: Backend wiring — job source, collection slug, adapter factory

**Files:**
- Modify: `internal/db/models/jobs.go:20-30`
- Modify: `internal/services/platformresolution/resolution.go`
- Modify: `cmd/nexorious/serve.go` (adapter factory + import)
- Test: `internal/services/platformresolution/resolution_test.go`

- [ ] **Step 1: Add the job-source constant**

In `internal/db/models/jobs.go`, inside the `const (` job-source block, add:

```go
	JobSourceHumbleBundle = "humble-bundle"
```

(Place it after `JobSourceGOG = "gog"`.)

- [ ] **Step 2: Write the failing platform-resolution test**

In `internal/services/platformresolution/resolution_test.go`, add:

```go
func TestStorefrontToCollectionSlug_HumbleBundle(t *testing.T) {
	slug, ok := StorefrontToCollectionSlug("humble-bundle")
	if !ok {
		t.Fatal("expected ok=true for humble-bundle")
	}
	if slug != "humble-bundle" {
		t.Errorf("got %q, want %q", slug, "humble-bundle")
	}
}
```

(If the test file uses an external test package `platformresolution_test`, qualify the call as `platformresolution.StorefrontToCollectionSlug` and match the existing file's import/package style.)

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/services/platformresolution/ -run HumbleBundle -v`
Expected: FAIL — `expected ok=true for humble-bundle` (default branch returns `"", false`).

- [ ] **Step 4: Add the resolution case**

In `internal/services/platformresolution/resolution.go`, add before `default:`:

```go
	case "humble-bundle":
		return "humble-bundle", true
```

- [ ] **Step 5: Run it to verify it passes**

Run: `go test ./internal/services/platformresolution/ -run HumbleBundle -v`
Expected: PASS.

- [ ] **Step 6: Add the adapter-factory case in `serve.go`**

In `cmd/nexorious/serve.go`, add the import alias alongside the other service imports:

```go
	humblesvc "github.com/drzero42/nexorious/internal/services/humble"
```

Then in `buildAdapterFactory`'s `switch storefront {`, add this case (after the `case "epic":` block, before `default:`):

```go
		case "humble-bundle":
			if cfg.StorefrontCredentials == nil {
				return nil, tasks.ErrCredentials
			}
			plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
			if err != nil {
				slog.Warn("adapter factory: humble-bundle decrypt failed", "user_id", cfg.UserID, "err", err)
				return nil, tasks.ErrCredentials
			}
			var creds struct {
				SessionCookie string `json:"session_cookie"`
			}
			if err := json.Unmarshal(plain, &creds); err != nil {
				return nil, tasks.ErrCredentials
			}
			return humblesvc.NewAdapter(humblesvc.NewClient(), creds.SessionCookie), nil
```

- [ ] **Step 7: Build to verify it compiles**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 8: Commit**

```bash
git add internal/db/models/jobs.go internal/services/platformresolution/resolution.go internal/services/platformresolution/resolution_test.go cmd/nexorious/serve.go
git commit -m "feat(sync): wire Humble Bundle into adapter factory and collection mapping"
```

---

## Task 4: Migration — platform_storefronts associations

**Files:**
- Create: `internal/db/migrations/20260604000003_humble_platform_storefronts.up.sql`
- Create: `internal/db/migrations/20260604000003_humble_platform_storefronts.down.sql`

These three associations are load-bearing for both the adapter's platform resolution and manual tagging (the `platform_storefronts` join controls which storefronts are selectable for a platform in the UI — notably enabling Humble Bundle for Android games). `pc-linux ↔ humble-bundle` is already seeded, so it is intentionally not re-added here.

- [ ] **Step 1: Write the up migration**

Create `internal/db/migrations/20260604000003_humble_platform_storefronts.up.sql`:

```sql
-- Associate Humble Bundle with the platforms it distributes DRM-free games for.
-- pc-linux <-> humble-bundle is already seeded in the initial migration. These
-- rows are used by both sync platform resolution and manual storefront tagging.
INSERT INTO platform_storefronts (platform, storefront) VALUES
    ('pc-windows', 'humble-bundle'),
    ('mac',        'humble-bundle'),
    ('android',    'humble-bundle')
ON CONFLICT (platform, storefront) DO NOTHING;
```

(The `ON CONFLICT DO NOTHING` guards against re-running; confirm the table's PK/unique is `(platform, storefront)` by checking the initial migration — it is the composite key used by the existing seed inserts.)

- [ ] **Step 2: Write the down migration**

Create `internal/db/migrations/20260604000003_humble_platform_storefronts.down.sql`:

```sql
-- Remove the three Humble Bundle associations added in the up migration. The
-- pc-linux <-> humble-bundle row predates this migration and is left intact.
DELETE FROM platform_storefronts
WHERE storefront = 'humble-bundle'
  AND platform IN ('pc-windows', 'mac', 'android');
```

- [ ] **Step 3: Verify migrations apply and roll back**

Run (against a dev DB; requires `DATABASE_URL`):
```bash
go build -o nexorious ./cmd/nexorious
./nexorious migrate
./nexorious migrate status
```
Expected: status shows `20260604000003_humble_platform_storefronts` applied. If you have a disposable dev DB, also confirm the down file is well-formed by reviewing it (the test suite's `TestMain` applies all up migrations on a fresh container, which is the authoritative check in Step 4).

- [ ] **Step 4: Run a DB-backed test to confirm the schema migrates cleanly**

Run: `go test ./internal/worker/tasks/ -run TestDispatchSync -count=1`
Expected: PASS — `TestMain` runs every up migration (including the new one) against a fresh testcontainer; a malformed migration fails here.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/20260604000003_humble_platform_storefronts.up.sql internal/db/migrations/20260604000003_humble_platform_storefronts.down.sql
git commit -m "feat(sync): add Humble Bundle platform_storefronts associations"
```

---

## Task 5: API wiring — connection routes + client bridge

**Files:**
- Modify: `internal/api/sync.go` (`supportedStorefronts`, `storefrontDisplayName`, `HumbleClient`, `ErrInvalidHumbleCookie`, `humbleStatusResponse`, `humbleConnectResponse`, three handlers, route registration, struct field, `NewSyncHandler`)
- Modify: `internal/api/router.go` (`humbleClientAdapter` bridge + construction)
- Modify: `internal/api/sync_test.go:49,1915,1948,1977` (extend `NewSyncHandler` call sites)

Per repo test policy, the thin connect/get/disconnect handlers get no dedicated handler tests; the existing sync test call sites only need to keep compiling.

- [ ] **Step 1: Add the storefront to the supported list and display-name switch**

In `internal/api/sync.go`, change:

```go
var supportedStorefronts = []string{"steam", "psn", "epic", "gog"}
```
to:
```go
var supportedStorefronts = []string{"steam", "psn", "epic", "gog", "humble-bundle"}
```

And in `storefrontDisplayName`, add before `default:`:

```go
	case "humble-bundle":
		return "Humble Bundle"
```

- [ ] **Step 2: Add the `HumbleClient` interface, sentinel, and response structs**

In `internal/api/sync.go`, after the `GOGClient` interface block, add:

```go
// HumbleClient abstracts the Humble Bundle credential-verification call.
type HumbleClient interface {
	Verify(ctx context.Context, sessionCookie string) error
}
```

Add to the `var (` error block that holds `ErrInvalidNPSSOToken`:

```go
	ErrInvalidHumbleCookie = errors.New("invalid humble session cookie")
```

Add these response structs near `psnStatusResponse`:

```go
type humbleConnectResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type humbleStatusResponse struct {
	IsConfigured     bool `json:"is_configured"`
	CredentialsError bool `json:"credentials_error,omitempty"`
}
```

- [ ] **Step 3: Add the `humbleClient` field and extend `NewSyncHandler`**

In the `SyncHandler` struct, add a field:

```go
	humbleClient HumbleClient
```

Change `NewSyncHandler` to accept it (append the parameter last):

```go
// NewSyncHandler constructs a SyncHandler.
func NewSyncHandler(encrypter *crypto.Encrypter, db *bun.DB, riverClient *river.Client[pgx.Tx], steam SteamClient, psn PSNClient, epic EpicClient, gog GOGClient, humble HumbleClient) *SyncHandler {
	return &SyncHandler{encrypter: encrypter, db: db, riverClient: riverClient, steamClient: steam, psnClient: psn, epicClient: epic, gogClient: gog, humbleClient: humble}
}
```

- [ ] **Step 4: Add the three handlers**

In `internal/api/sync.go`, add near the PSN handlers:

```go
// HandleHumbleConnect is PUT /sync/humble-bundle/connection: it verifies the
// pasted session cookie against the Humble order API and, on success, stores it
// (create-or-replace). A rejected cookie returns 400 before anything is stored.
func (h *SyncHandler) HandleHumbleConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req struct {
		SessionCookie string `json:"session_cookie"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	req.SessionCookie = strings.TrimSpace(req.SessionCookie)
	if req.SessionCookie == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_cookie is required")
	}

	if err := h.humbleClient.Verify(c.Request().Context(), req.SessionCookie); err != nil {
		if errors.Is(err, ErrInvalidHumbleCookie) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid_session_cookie")
		}
		slog.Error("humble: verify failed", "user_id", userID, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, "could not reach Humble Bundle")
	}

	creds := map[string]any{"session_cookie": req.SessionCookie}
	if err := h.persistStorefrontCredentials(context.Background(), userID, "humble-bundle", creds); err != nil {
		slog.Error("humble: persist storefront credentials failed", "user_id", userID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Humble Bundle connection")
	}

	return c.JSON(http.StatusOK, humbleConnectResponse{Success: true, Message: "Humble Bundle connected successfully"})
}

// HandleGetHumbleConnection is GET /sync/humble-bundle/connection.
func (h *SyncHandler) HandleGetHumbleConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "humble-bundle",
		humbleStatusResponse{},
		humbleStatusResponse{IsConfigured: true, CredentialsError: true},
		func(_ []byte, credentialsError bool) (any, error) {
			return humbleStatusResponse{IsConfigured: true, CredentialsError: credentialsError}, nil
		})
}

// HandleHumbleDisconnect is DELETE /sync/humble-bundle/connection.
func (h *SyncHandler) HandleHumbleDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "humble-bundle")
}
```

- [ ] **Step 5: Register the routes**

In `RegisterRoutes`, add the three static routes alongside the other `*/connection` registrations (before the parameterised `:storefront` routes — they already appear earlier in the function, so placement next to the GOG block is correct):

```go
	g.PUT("/humble-bundle/connection", h.HandleHumbleConnect)
	g.GET("/humble-bundle/connection", h.HandleGetHumbleConnection)
	g.DELETE("/humble-bundle/connection", h.HandleHumbleDisconnect)
```

- [ ] **Step 6: Add the client bridge and wire it in `router.go`**

In `internal/api/router.go`, add the import alias with the other service imports:

```go
	humblesvc "github.com/drzero42/nexorious/internal/services/humble"
```

Add the bridge type (next to `gogClientAdapter`):

```go
// humbleClientAdapter bridges humblesvc.Client to the HumbleClient interface
// without creating an import cycle between internal/api and internal/services/humble.
type humbleClientAdapter struct{ c *humblesvc.Client }

func (a *humbleClientAdapter) Verify(ctx context.Context, sessionCookie string) error {
	err := a.c.Verify(ctx, sessionCookie)
	if errors.Is(err, humblesvc.ErrCredentials) {
		return ErrInvalidHumbleCookie
	}
	return err
}
```

Update the handler construction block:

```go
		steamSvc := steamsvc.NewClient()
		psnSvc := psnsvc.NewClient()
		epicSvc := epicsvc.NewClient(cfg.LegendaryWorkDir)
		gogSvc := gogsvc.NewClient()
		humbleSvc := humblesvc.NewClient()
		synch := NewSyncHandler(encrypter, db, riverClient, &steamClientAdapter{c: steamSvc}, &psnClientAdapter{c: psnSvc}, &epicClientAdapter{c: epicSvc}, &gogClientAdapter{c: gogSvc}, &humbleClientAdapter{c: humbleSvc})
```

- [ ] **Step 7: Fix the four existing `NewSyncHandler` call sites in tests**

In `internal/api/sync_test.go`, append `(api.HumbleClient)(nil)` to each of the four `NewSyncHandler(...)` calls (lines 49, 1915, 1948, 1977). For example, line 49 becomes:

```go
	synch := api.NewSyncHandler(testEncrypter, db, nil, steam, psn, (api.EpicClient)(nil), (api.GOGClient)(nil), (api.HumbleClient)(nil))
```

Apply the same trailing `(api.HumbleClient)(nil)` argument to all four.

- [ ] **Step 8: Build and run the API package tests**

Run: `go build ./... && go test ./internal/api/ -run TestSync -count=1`
Expected: build succeeds; existing sync tests pass (no behavioural change, just the extra constructor arg).

- [ ] **Step 9: Commit**

```bash
git add internal/api/sync.go internal/api/router.go internal/api/sync_test.go
git commit -m "feat(sync): add Humble Bundle connection API routes"
```

---

## Task 6: Frontend — shared component fix, types, API, hooks

**Files:**
- Modify: `ui/frontend/src/components/sync/connection/connected-summary.tsx`
- Test: `ui/frontend/src/components/sync/connection/connected-summary.test.tsx` (create if absent)
- Modify: `ui/frontend/src/types/sync.ts`
- Modify: `ui/frontend/src/api/sync.ts`
- Modify: `ui/frontend/src/hooks/use-sync.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

All commands in this task run from `ui/frontend/`.

- [ ] **Step 1: Write the failing ConnectedSummary test**

Create (or extend) `ui/frontend/src/components/sync/connection/connected-summary.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { ConnectedSummary } from './connected-summary';

describe('ConnectedSummary', () => {
  it('renders "Connected as {name}" when a name is given', () => {
    render(<ConnectedSummary name="alice" />);
    expect(screen.getByText('Connected as alice')).toBeInTheDocument();
  });

  it('renders just "Connected" when no name is given', () => {
    render(<ConnectedSummary />);
    expect(screen.getByText('Connected')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run it to verify the no-name case fails**

Run: `npm run test connected-summary.test.tsx`
Expected: the "Connected as " (current) markup makes the no-name test FAIL (it finds "Connected as " not "Connected").

- [ ] **Step 3: Update `ConnectedSummary`**

In `ui/frontend/src/components/sync/connection/connected-summary.tsx`, change the line:

```tsx
        <p className="font-medium">Connected as {name}</p>
```
to:
```tsx
        <p className="font-medium">{name ? `Connected as ${name}` : 'Connected'}</p>
```

- [ ] **Step 4: Run it to verify both cases pass**

Run: `npm run test connected-summary.test.tsx`
Expected: PASS (both cases). The existing PSN/Steam/Epic/GOG cards always pass a name, so their behaviour is unchanged.

- [ ] **Step 5: Add the storefront to types**

In `ui/frontend/src/types/sync.ts`:

Add to the enum:
```ts
export enum SyncStorefront {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
  PSN = 'psn',
  HUMBLE = 'humble-bundle',
}
```

Add to `SUPPORTED_SYNC_STOREFRONTS`:
```ts
  SyncStorefront.HUMBLE,
```

Add to the `info` record inside `getStorefrontDisplayInfo`:
```ts
    [SyncStorefront.HUMBLE]: {
      name: 'Humble Bundle',
      color: 'text-[#cc2929]',
      bgColor: 'bg-[#cc2929]/10 dark:bg-[#cc2929]/30',
      iconUrl: '/logos/storefronts/humble-bundle/humble-bundle-icon-light.svg',
    },
```

Add the response types near `PSNStatusResponse`:
```ts
export interface HumbleConnectResponse {
  valid: boolean;
  error: string | null;
}

export interface HumbleStatusResponse {
  configured: boolean;
  credentialsError: boolean;
}
```

- [ ] **Step 6: Add the API functions**

In `ui/frontend/src/api/sync.ts`, add the internal request/response types (near the PSN ones) and the three functions. Match the existing import of the response types from `@/types/sync` (extend the existing import statement to include `HumbleConnectResponse` and `HumbleStatusResponse`):

```ts
interface HumbleConnectApiRequest {
  session_cookie: string;
}

interface HumbleConnectApiResponse {
  success: boolean;
  message: string;
}

interface HumbleStatusApiResponse {
  is_configured: boolean;
  credentials_error?: boolean;
}

/**
 * Connect Humble Bundle by submitting and verifying a session cookie.
 */
export async function connectHumble(sessionCookie: string): Promise<HumbleConnectResponse> {
  const response = await api.put<HumbleConnectApiResponse>('/sync/humble-bundle/connection', {
    session_cookie: sessionCookie,
  } as HumbleConnectApiRequest);

  return {
    valid: response.success,
    error: response.success ? null : response.message,
  };
}

/**
 * Get Humble Bundle connection status.
 */
export async function getHumbleStatus(): Promise<HumbleStatusResponse> {
  const response = await api.get<HumbleStatusApiResponse>('/sync/humble-bundle/connection');

  return {
    configured: response.is_configured,
    credentialsError: response.credentials_error ?? false,
  };
}

/**
 * Disconnect Humble Bundle integration.
 */
export async function disconnectHumble(): Promise<void> {
  await api.delete('/sync/humble-bundle/connection');
}
```

- [ ] **Step 7: Add the hooks**

In `ui/frontend/src/hooks/use-sync.ts`:

Add a key to `syncKeys`:
```ts
  humbleStatus: () => [...syncKeys.all, 'humbleStatus'] as const,
```

Add the three hooks (mirroring `useConfigurePSN`/`usePSNStatus`/`useDisconnectPSN`):

```ts
/**
 * Connect Humble Bundle. Invalidates sync configs and Humble status on success.
 */
export function useConnectHumble() {
  const queryClient = useQueryClient();

  return useMutation<HumbleConnectResponse, Error, string>({
    mutationFn: (sessionCookie: string) => syncApi.connectHumble(sessionCookie),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.HUMBLE) });
      queryClient.invalidateQueries({ queryKey: syncKeys.humbleStatus() });
    },
    onError: (error) => {
      console.error('Failed to connect Humble Bundle:', error);
    },
  });
}

/**
 * Humble Bundle connection status. Cached for 5 minutes.
 */
export function useHumbleStatus(options?: { enabled?: boolean }) {
  return useQuery<HumbleStatusResponse, Error>({
    queryKey: syncKeys.humbleStatus(),
    queryFn: syncApi.getHumbleStatus,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    enabled: options?.enabled,
  });
}

/**
 * Disconnect Humble Bundle. Invalidates all Humble-related queries on success.
 */
export function useDisconnectHumble() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectHumble,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.HUMBLE) });
      queryClient.invalidateQueries({ queryKey: syncKeys.humbleStatus() });
    },
    onError: (error) => {
      console.error('Failed to disconnect Humble Bundle:', error);
    },
  });
}
```

Make sure `HumbleConnectResponse` and `HumbleStatusResponse` are imported from `@/types/sync` at the top of `use-sync.ts` (extend the existing type import).

- [ ] **Step 8: Export the hooks**

In `ui/frontend/src/hooks/index.ts`, add to the `export { … } from './use-sync';` block:

```ts
  useConnectHumble,
  useHumbleStatus,
  useDisconnectHumble,
```

- [ ] **Step 9: Typecheck and lint**

Run: `npm run check && npm run knip`
Expected: no TypeScript errors. `knip` may report the new hooks/functions as unused until Task 7/8 consume them — if so, proceed to Task 7 before treating knip as a hard gate, then re-run knip at the end of Task 8.

- [ ] **Step 10: Commit**

```bash
git add src/components/sync/connection/connected-summary.tsx src/components/sync/connection/connected-summary.test.tsx src/types/sync.ts src/api/sync.ts src/hooks/use-sync.ts src/hooks/index.ts
git commit -m "feat(sync): add Humble Bundle frontend types, API, and hooks"
```

---

## Task 7: Frontend — Humble connection card

**Files:**
- Create: `ui/frontend/src/components/sync/humble-connection-card.tsx`
- Modify: `ui/frontend/src/components/sync/index.ts`

All commands run from `ui/frontend/`. The card mirrors `psn-connection-card.tsx` but uses a `<Textarea>` for the long cookie, passes **no name** to `ConnectedSummary` (renders "Connected"), and provides devtools copy instructions.

- [ ] **Step 1: Create the card component**

Create `ui/frontend/src/components/sync/humble-connection-card.tsx`:

```tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useConnectHumble, useDisconnectHumble } from '@/hooks';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

const humbleCredentialsSchema = z.object({
  sessionCookie: z.string().trim().min(1, 'Session cookie is required'),
});

type HumbleCredentialsForm = z.infer<typeof humbleCredentialsSchema>;

interface HumbleConnectionCardProps {
  isConfigured: boolean;
  credentialsError: boolean;
  onConnectionChange: () => void;
}

export function HumbleConnectionCard({
  isConfigured,
  credentialsError,
  onConnectionChange,
}: HumbleConnectionCardProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<HumbleCredentialsForm>({
    resolver: zodResolver(humbleCredentialsSchema),
  });

  const connectMutation = useConnectHumble();
  const disconnectMutation = useDisconnectHumble();

  const isConnecting = connectMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;

  const onSubmit = async (data: HumbleCredentialsForm) => {
    try {
      const result = await connectMutation.mutateAsync(data.sessionCookie);
      if (!result.valid) {
        const errorMessage = result.error || 'Connection failed';
        setError('sessionCookie', { message: errorMessage });
        toast.error(errorMessage);
        return;
      }
      toast.success('Humble Bundle connected successfully');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to connect Humble Bundle');
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Humble Bundle disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Humble Bundle');
    }
  };

  const showForm = !isConfigured || credentialsError;

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Humble Bundle Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your Humble Bundle account is connected'
              : 'Connect your Humble Bundle account to sync your DRM-free game downloads'}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {showForm ? (
          <div className="space-y-4">
            {credentialsError && (
              <CredentialsErrorBanner
                title="Your Humble Bundle session has expired"
                description="Please paste a fresh _simpleauth_sess cookie to continue syncing your library."
              />
            )}

            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="sessionCookie">Session cookie (_simpleauth_sess)</Label>
                <Textarea
                  id="sessionCookie"
                  rows={3}
                  placeholder="Paste the value of your _simpleauth_sess cookie"
                  {...register('sessionCookie')}
                  disabled={isConnecting}
                />
                {errors.sessionCookie && (
                  <p className="text-sm text-destructive">{errors.sessionCookie.message}</p>
                )}

                <CodeHelpAccordion
                  value="humble-help"
                  trigger="How do I get my _simpleauth_sess cookie?"
                >
                  <p className="font-medium text-foreground">
                    The _simpleauth_sess cookie is a session token that lets Nexorious read your
                    Humble Bundle library. Only DRM-free game downloads are imported — never
                    ebooks, audio, video, or Steam-key-only titles.
                  </p>
                  <ol className="list-inside list-decimal space-y-1">
                    <li>
                      Sign in at humblebundle.com in your browser.
                    </li>
                    <li>
                      Open your browser&apos;s developer tools (F12) and go to the{' '}
                      <strong>Application</strong> tab (Chrome/Edge) or{' '}
                      <strong>Storage</strong> tab (Firefox).
                    </li>
                    <li>
                      Under <strong>Cookies → https://www.humblebundle.com</strong>, find the
                      cookie named <code>_simpleauth_sess</code>.
                    </li>
                    <li>Copy its entire value and paste it into the field above.</li>
                  </ol>
                  <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                    <strong>Security Note:</strong> This cookie grants access to your Humble
                    library and expires periodically. It is stored encrypted and only used to sync
                    your games. You&apos;ll re-paste it when it expires.
                  </div>
                </CodeHelpAccordion>
              </div>

              <ConnectSubmitButton
                isPending={isConnecting}
                idleLabel={credentialsError ? 'Reconnect' : 'Connect Humble Bundle'}
                pendingLabel={credentialsError ? 'Reconnecting...' : 'Connecting...'}
              />
            </form>
          </div>
        ) : (
          <div className="space-y-4">
            <ConnectedSummary />
            <DisconnectDialog
              serviceLabel="Humble Bundle"
              isDisconnecting={isDisconnecting}
              onDisconnect={handleDisconnect}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 2: Confirm `Textarea` exists; add it if missing**

Run: `ls src/components/ui/textarea.tsx`
- If present: continue.
- If absent (knip's include-and-prune policy may have removed it): run `npx shadcn@latest add textarea` from `ui/frontend/`, then `git add src/components/ui/textarea.tsx`.

- [ ] **Step 3: Export the card**

In `ui/frontend/src/components/sync/index.ts`, add:

```ts
export { HumbleConnectionCard } from './humble-connection-card';
```

- [ ] **Step 4: Typecheck**

Run: `npm run check`
Expected: no TypeScript errors. (`knip` is deferred to the end of Task 8, after the route wires the card in.)

- [ ] **Step 5: Commit**

```bash
git add src/components/sync/humble-connection-card.tsx src/components/sync/index.ts
# include textarea.tsx in the add if you re-added it in Step 2
git commit -m "feat(sync): add Humble Bundle connection card"
```

---

## Task 8: Frontend — route wiring + routeTree regeneration

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/index.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

All commands run from `ui/frontend/`. These routes already wire PSN identically — add the Humble equivalents next to each PSN reference. Read each file first to match the exact local variable names and import block in your checkout (the snippets below show the shape, not exact line numbers).

- [ ] **Step 1: Wire status + credentials-error in the sync list route**

In `ui/frontend/src/routes/_authenticated/sync/index.tsx`:

Add the import for the hook (extend the existing `@/hooks` import) — `useHumbleStatus`.

Add the conditional status fetch next to the existing `usePSNStatus({ enabled: … })`:
```ts
  const { data: humbleStatus } = useHumbleStatus({
    enabled: config.storefront === SyncStorefront.HUMBLE,
  });
```

Extend the `credentialsError` derivation with a Humble clause:
```ts
    (config.storefront === SyncStorefront.HUMBLE && (humbleStatus?.credentialsError ?? false)) ||
```

- [ ] **Step 2: Wire status, credentials-error, and the card in the storefront detail route**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`:

Add to the `@/hooks` import: `useHumbleStatus`. Add to the `@/components/sync` import: `HumbleConnectionCard`.

Add the status fetch next to the others:
```ts
  const { data: humbleStatus } = useHumbleStatus();
```

Extend the `credentialsError` derivation:
```ts
    (storefront === SyncStorefront.HUMBLE && (humbleStatus?.credentialsError ?? false)) ||
```

Add the conditional card render next to the PSN card block:
```tsx
        {/* Humble Bundle Connection Card - only show for Humble storefront */}
        {storefront === SyncStorefront.HUMBLE && (
          <HumbleConnectionCard
            isConfigured={config.isConfigured}
            credentialsError={humbleStatus?.credentialsError ?? false}
            onConnectionChange={() => {
              queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
              queryClient.invalidateQueries({ queryKey: syncKeys.humbleStatus() });
            }}
          />
        )}
```

- [ ] **Step 3: Regenerate the route tree and run the full frontend gate**

The sync routes are file-based but already exist (no new route files were added), so `routeTree.gen.ts` should not change. Still run the build to be safe and run the gates:

Run: `npm run build && npm run check && npm run knip && npm run test`
Expected:
- `build` succeeds; `git status` shows `routeTree.gen.ts` **unchanged** (no new route files were created). If it did change, commit it alongside this task.
- `check` clean.
- `knip` clean — the hooks/API functions/card added in Tasks 6–7 are now all consumed.
- `test` passes (including the `connected-summary` test from Task 6).

- [ ] **Step 4: Commit**

```bash
git add src/routes/_authenticated/sync/index.tsx src/routes/_authenticated/sync/\$storefront.tsx
# add src/routeTree.gen.ts only if it actually changed
git commit -m "feat(sync): wire Humble Bundle into the sync UI routes"
```

---

## Final verification (whole feature)

- [ ] **Step 1: Backend build + targeted tests**

Run:
```bash
go build ./...
go test ./internal/services/humble/ ./internal/services/platformresolution/ -count=1
go test ./internal/worker/tasks/ -run TestDispatchSync -count=1
go test ./internal/api/ -run TestSync -count=1
```
Expected: all PASS. (The push hook runs the full `go test ./...` and frontend suites; these targeted runs are the in-loop check.)

- [ ] **Step 2: Frontend gate**

Run (from `ui/frontend/`):
```bash
npm run check && npm run knip && npm run test
```
Expected: all clean/PASS.

- [ ] **Step 3: Open the PR**

Push the branch and open a PR. Use a Conventional-Commit title (release-please parses the squash title):

```
feat(sync): add Humble Bundle sync source (#766)
```

PR body should note: imports only DRM-free direct downloads (whitelist of windows/mac/linux/android with non-empty download URL), excludes ebooks/audio/video/asmjs and Steam-key-only titles, single-entry `uplayclient` launcher blocklist, cookie-paste auth verified on save, and the sync-source id is `humble-bundle` (id == slug) per the owner's naming decision.

---

## Out of scope (v1)

- Humble Trove / "Humble Games Collection" (separate subscription catalog).
- Surfacing or redeeming Steam/third-party keys (`tpkd_dict` is never read).
- Any download management.
