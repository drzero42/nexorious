# Issue #737 — Allow full GOG URL for sync

## Problem

When connecting GOG, the user logs in and is redirected to a URL like:

```
https://embed.gog.com/on_login_success?origin=client&code=XXX
```

Today they must manually pick the `code` value out of that URL and paste only
that fragment into the Authorization Code field. The URL is predictable, so
forcing the user to surgically extract `code=` is unnecessary friction. We
should also accept the **entire pasted URL** and extract the code ourselves,
while still accepting a bare code for anyone who pastes just that.

## Goals

- Accept either a full GOG redirect URL or a bare authorization code in the
  existing `POST /sync/gog/connect` flow.
- Keep the existing bare-code behaviour working (backward compatible).
- Give the user a clear, persistent error when they paste something that is
  neither a recognized GOG URL nor a usable code.

## Non-goals

- No database or migration changes.
- No API contract change: the endpoint stays `POST /sync/gog/connect` with a
  JSON body `{ "auth_code": "..." }`. The field still carries either a code or
  a URL; only the server's interpretation widens.
- No slumber collection change (route and request body are unchanged).
- No change to the GOG token exchange itself.

## Design

### Where the logic lives

Extraction happens **on the backend only** (single source of truth, covered by
Go tests, benefits any API client — CLI/slumber included). A new pure,
package-level helper is added to the GOG domain package:

```go
// internal/services/gog/auth.go
func ParseAuthCode(input string) (string, error)
```

`HandleGOGConnect` (`internal/api/sync.go`) calls `gog.ParseAuthCode` on the
bound `auth_code` before calling `ExchangeCode`. The helper needs no client
state, so it stays a free function and is unit-tested directly in the `gog`
package.

### Parsing rules (strict, backward compatible)

`ParseAuthCode(input)`:

1. Trim surrounding whitespace.
2. Parse with `net/url`. If the parsed result has a **non-empty host** (i.e. it
   is a real URL, e.g. starts with `https://`):
   - The host must equal `embed.gog.com` (compared case-insensitively).
   - The path must be `/on_login_success` (a single trailing slash is
     tolerated).
   - If host or path do not match → return an error → handler responds `400`
     with a human-readable message:
     *"That doesn't look like a GOG login URL — paste the URL you were
     redirected to, or just the code."*
   - Read the `code` query parameter (by name, so parameter order does not
     matter). If missing or empty → return an error → `400`:
     *"Couldn't find an authorization code in that URL — make sure you copied
     the full URL after logging in."*
   - Otherwise return the `code` value.
3. If the input is **not** URL-like (no scheme/host — a bare token): return the
   trimmed input unchanged as the code. This preserves today's behaviour.

The strict host/path check is a deliberate choice: it gives precise error
messages and refuses to forward an unrelated URL to GOG. The accepted
brittleness is that if GOG ever changes its redirect host/path, the URL form
would need a code update — bare-code paste would still work in the meantime.

### Handler change

`HandleGOGConnect` keeps its existing empty-input guard, then:

```go
code, err := gog.ParseAuthCode(body.AuthCode)
if err != nil {
    return echo.NewHTTPError(http.StatusBadRequest, err.Error())
}
tok, err := h.gogClient.ExchangeCode(c.Request().Context(), code)
```

The error messages returned by `ParseAuthCode` are user-facing (see below), so
they are phrased for humans.

### Frontend

Extraction is backend-only, so no parsing logic is added to the client.
However, the help text currently instructs the user to extract `code=`, which
becomes misleading, so `ui/frontend/src/components/sync/gog-connection-card.tsx`
is updated:

- Input placeholder → *"Paste the full GOG URL or just the code"*.
- Help accordion step that currently says "copy the `code` value from the URL"
  → "copy the entire URL from your browser's address bar (or just the `code`
  value) and paste it above". The single-use / expires-in-minutes warning
  stays.

### Error display (already works, no change needed)

When the backend returns `400 {"message": "..."}`:

- `client.ts` `handleApiError` reads the `message` field and throws
  `ApiErrorException` (extends `Error`) carrying that text.
- The card's `onSubmit` catches it and calls both:
  - `setError('authCode', { message })` → renders **persistent red text below
    the input** (`{errors.authCode && <p class="text-destructive">…`), which
    stays until the user edits and resubmits; and
  - `toast.error(message)` → transient popup.

This is the same path already used for `gog auth failed: …`, so the new 400
messages surface identically — durably inline, plus a toast.

## Testing

**Go unit tests — `internal/services/gog/auth_test.go`** for `ParseAuthCode`:

- bare code returned unchanged
- full example URL → extracts `XXX`
- URL with reordered / extra query params → still extracts `code`
- URL whose host is not `embed.gog.com` → error
- URL with correct host but no `code` param → error
- path with a trailing slash (`/on_login_success/`) → accepted
- surrounding whitespace trimmed (both bare code and URL forms)

**Go handler test — `internal/api/sync_test.go`**: extend the existing
`TestGOGConnect_*` suite with a case that posts a full URL and asserts a
successful connect (the stub `ExchangeCode` should receive the extracted code).

**Frontend**: existing `gog-connection-card.test.tsx` stays green. Optionally
assert the updated placeholder/help text.

## Files touched

- `internal/services/gog/auth.go` — add `ParseAuthCode`.
- `internal/api/sync.go` — call `ParseAuthCode` in `HandleGOGConnect`.
- `internal/services/gog/auth_test.go` — unit tests.
- `internal/api/sync_test.go` — handler test for the URL form.
- `ui/frontend/src/components/sync/gog-connection-card.tsx` — help text +
  placeholder.
