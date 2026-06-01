# Allow full GOG URL for sync — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users paste either the full GOG redirect URL or a bare authorization code when connecting GOG, extracting the code on the backend.

**Architecture:** A new pure helper `gog.ParseAuthCode` parses the input — strictly validating a GOG redirect URL (`https://embed.gog.com/on_login_success?...&code=...`) and extracting the `code`, or passing through a bare code unchanged. `HandleGOGConnect` calls it before exchanging the code, mapping parse errors to a `400` with a human-readable message. The frontend only updates its help/placeholder text; the existing error-display path already surfaces the `400` message inline and as a toast.

**Tech Stack:** Go (`net/url`, stdlib `testing`), Echo v5, React + react-hook-form (no new deps).

**Spec:** `docs/superpowers/specs/2026-06-01-issue-737-gog-full-url-sync-design.md`

---

## File Structure

- **Modify** `internal/services/gog/auth.go` — add the `ParseAuthCode` free function. Lives here because it is GOG-domain logic and needs no client state.
- **Modify** `internal/services/gog/auth_test.go` — table-driven unit tests for `ParseAuthCode`.
- **Modify** `internal/api/sync.go` — `HandleGOGConnect` calls `gogsvc.ParseAuthCode` and adds the `gogsvc` import.
- **Modify** `internal/api/sync_test.go` — capture the code in `stubGOGClient`; add a handler test posting a full URL.
- **Modify** `ui/frontend/src/components/sync/gog-connection-card.tsx` — placeholder + help-text wording.
- **Modify** `ui/frontend/src/components/sync/gog-connection-card.test.tsx` — assert the new placeholder text.

---

## Task 1: `ParseAuthCode` helper (backend extraction)

**Files:**
- Modify: `internal/services/gog/auth.go`
- Test: `internal/services/gog/auth_test.go`

- [ ] **Step 1: Write the failing test**

Add this table-driven test to `internal/services/gog/auth_test.go` (append at end of file; `gog` and `testing` are already imported):

```go
func TestParseAuthCode(t *testing.T) {
	const fullURL = "https://embed.gog.com/on_login_success?origin=client&code=XXX"

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"bare code", "XXX", "XXX", false},
		{"full url", fullURL, "XXX", false},
		{"reordered params", "https://embed.gog.com/on_login_success?code=XXX&origin=client", "XXX", false},
		{"extra params", "https://embed.gog.com/on_login_success?origin=client&code=XXX&foo=bar", "XXX", false},
		{"uppercase host", "https://EMBED.GOG.COM/on_login_success?code=XXX", "XXX", false},
		{"whitespace around bare code", "  XXX  ", "XXX", false},
		{"whitespace around url", "  " + fullURL + "  ", "XXX", false},
		{"wrong host", "https://evil.example.com/on_login_success?code=XXX", "", true},
		{"missing code", "https://embed.gog.com/on_login_success?origin=client", "", true},
		{"trailing slash path", "https://embed.gog.com/on_login_success/?code=XXX", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gog.ParseAuthCode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got code %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/gog/ -run TestParseAuthCode -v`
Expected: FAIL — compile error `undefined: gog.ParseAuthCode`.

- [ ] **Step 3: Add the imports**

In `internal/services/gog/auth.go`, the import block currently is:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)
```

Replace it with (adds `errors` and `strings`):

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)
```

- [ ] **Step 4: Implement `ParseAuthCode`**

Add this function to `internal/services/gog/auth.go` (place it just after the `redirectURI` const block / before `BuildAuthURL`):

```go
// ParseAuthCode extracts a GOG authorization code from user input. The input
// may be either a bare code or the full redirect URL the user lands on after
// logging in, e.g.:
//
//	https://embed.gog.com/on_login_success?origin=client&code=XXX
//
// If the input is a URL it must be the GOG redirect URL (host embed.gog.com,
// path /on_login_success) and carry a non-empty code query parameter;
// otherwise an error is returned. Input that is not a URL is treated as a bare
// code and returned trimmed, preserving the original paste-the-code flow.
func ParseAuthCode(input string) (string, error) {
	trimmed := strings.TrimSpace(input)

	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		// Not a URL — treat the whole input as a bare authorization code.
		return trimmed, nil
	}

	if !strings.EqualFold(u.Host, "embed.gog.com") || u.Path != "/on_login_success" {
		return "", errors.New("that doesn't look like a GOG login URL — paste the URL you were redirected to, or just the code")
	}

	code := u.Query().Get("code")
	if code == "" {
		return "", errors.New("couldn't find an authorization code in that URL — make sure you copied the full URL after logging in")
	}

	return code, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/services/gog/ -run TestParseAuthCode -v`
Expected: PASS — all subtests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/services/gog/auth.go internal/services/gog/auth_test.go
git commit -m "feat: parse GOG auth code from full redirect URL"
```

---

## Task 2: Wire `ParseAuthCode` into `HandleGOGConnect`

**Files:**
- Modify: `internal/api/sync.go` (import block ~line 22-25; `HandleGOGConnect` ~line 1420-1424)
- Test: `internal/api/sync_test.go` (`stubGOGClient` ~line 1921-1936; new test near the `TestGOGConnect_*` suite ~line 2241)

- [ ] **Step 1: Capture the code in the test stub, then write the failing handler test**

First, update `stubGOGClient` in `internal/api/sync_test.go` so `ExchangeCode` records the code it receives. Replace:

```go
type stubGOGClient struct {
	authURL string
	token   *api.GOGTokenResponse
	err     error
}
```

with (adds `gotCode`):

```go
type stubGOGClient struct {
	authURL string
	token   *api.GOGTokenResponse
	err     error
	gotCode string
}
```

and replace:

```go
func (s *stubGOGClient) ExchangeCode(_ context.Context, _ string) (*api.GOGTokenResponse, error) {
	return s.token, s.err
}
```

with:

```go
func (s *stubGOGClient) ExchangeCode(_ context.Context, code string) (*api.GOGTokenResponse, error) {
	s.gotCode = code
	return s.token, s.err
}
```

Then add this test next to `TestGOGConnect_Success` (after it, ~line 2241):

```go
func TestGOGConnect_FullURL(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{
		token: &api.GOGTokenResponse{
			AccessToken:  "acc",
			RefreshToken: "ref",
			UserID:       "u1",
			Username:     "goguser",
		},
	}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-url")

	rec := postJSONAuth(t, app, "/api/sync/gog/connect", map[string]any{
		"auth_code": "https://embed.gog.com/on_login_success?origin=client&code=XXX",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotCode != "XXX" {
		t.Errorf("ExchangeCode received %q, want extracted code %q", stub.gotCode, "XXX")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestGOGConnect_FullURL -v`
Expected: FAIL — `ExchangeCode received "https://embed.gog.com/on_login_success?origin=client&code=XXX", want extracted code "XXX"` (the handler currently forwards the raw input).

- [ ] **Step 3: Add the `gogsvc` import to `sync.go`**

In `internal/api/sync.go`, the internal-import group is:

```go
	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
```

Add the `gogsvc` alias (matching `router.go`) so the group reads:

```go
	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/worker/tasks"
```

- [ ] **Step 4: Call `ParseAuthCode` in `HandleGOGConnect`**

In `internal/api/sync.go`, find this block in `HandleGOGConnect`:

```go
	if err := c.Bind(&body); err != nil || body.AuthCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	tok, err := h.gogClient.ExchangeCode(c.Request().Context(), body.AuthCode)
```

Replace it with:

```go
	if err := c.Bind(&body); err != nil || body.AuthCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	code, err := gogsvc.ParseAuthCode(body.AuthCode)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	tok, err := h.gogClient.ExchangeCode(c.Request().Context(), code)
```

- [ ] **Step 5: Run the GOG handler tests to verify they pass**

Run: `go test ./internal/api/ -run TestGOGConnect -v`
Expected: PASS — `TestGOGConnect_FullURL`, `TestGOGConnect_Success`, `TestGOGConnect_MissingAuthCode`, and `TestGOGConnect_ExchangeFailure` all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat: accept full GOG redirect URL in connect handler"
```

---

## Task 3: Update frontend help text and placeholder

**Files:**
- Modify: `ui/frontend/src/components/sync/gog-connection-card.tsx` (placeholder ~line 200; help `<li>` items ~line 232-237)
- Test: `ui/frontend/src/components/sync/gog-connection-card.test.tsx`

- [ ] **Step 1: Write the failing test**

In `gog-connection-card.test.tsx`, add this test inside the `describe('GOGConnectionCard', ...)` block (after the `renders not-configured state` test, ~line 74):

```tsx
  it('tells the user they can paste the full URL or just the code', () => {
    render(<GOGConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    expect(
      screen.getByPlaceholderText('Paste the full GOG URL or just the code'),
    ).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test -- gog-connection-card`
Expected: FAIL — `Unable to find an element with the placeholder text of: Paste the full GOG URL or just the code` (current placeholder is `Paste the authorization code from GOG`).

- [ ] **Step 3: Update the placeholder**

In `ui/frontend/src/components/sync/gog-connection-card.tsx`, change the `Input` placeholder:

```tsx
                  placeholder="Paste the authorization code from GOG"
```

to:

```tsx
                  placeholder="Paste the full GOG URL or just the code"
```

- [ ] **Step 4: Update the help-accordion instructions**

In the same file, find these two list items:

```tsx
                          <li>
                            After login, you will be redirected to a GOG page — copy the{' '}
                            <code>code</code> value from the URL (it appears as <code>?code=…</code>
                            )
                          </li>
                          <li>Paste the code into the field above</li>
```

Replace them with:

```tsx
                          <li>
                            After login, you will be redirected to a GOG page — copy the entire URL
                            from your browser&apos;s address bar (it contains <code>?code=…</code>)
                          </li>
                          <li>
                            Paste the URL into the field above (you can also paste just the{' '}
                            <code>code</code> value if you prefer)
                          </li>
```

- [ ] **Step 5: Run the frontend test to verify it passes**

Run (from `ui/frontend/`): `npm run test -- gog-connection-card`
Expected: PASS — all `GOGConnectionCard` tests pass, including the new placeholder assertion.

- [ ] **Step 6: Type-check and lint**

Run (from `ui/frontend/`): `npm run check`
Expected: no TypeScript or ESLint errors.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/sync/gog-connection-card.tsx ui/frontend/src/components/sync/gog-connection-card.test.tsx
git commit -m "feat: update GOG connect help text for full-URL paste"
```

---

## Final verification

- [ ] **Backend tests:** `go test ./internal/services/gog/ ./internal/api/ -run 'GOG|ParseAuthCode' -v` → all pass.
- [ ] **Backend build:** `go build ./...` → no errors.
- [ ] **Frontend:** from `ui/frontend/` — `npm run check && npm run test -- gog-connection-card` → clean.
- [ ] No migration, API-contract, or slumber changes were needed (confirmed: route and request body unchanged).
