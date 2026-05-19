# Pre-SPA Pages Redesign

**Date:** 2026-05-19
**Status:** Approved
**Issue:** #521

## Problem

Three HTML pages are rendered by the Go server before (or instead of) the React SPA: `/migrate`, `/db-error`, and `/setup`. Each has hand-written inline CSS with mismatched palettes, fonts, and visual idioms — the migrate page is dark, the db-error page uses red-on-white, and the setup page uses a third light theme. None match the SPA's visual language.

The SPA uses Tailwind v4 + shadcn/ui with OKLCH design tokens and the Geist font, defined in `ui/frontend/src/styles/globals.css`. The pre-SPA pages cannot load the SPA bundle (they run when the DB is unavailable, when migrations are pending, or when no users exist yet) and therefore need their own styling pipeline.

The goal is to make the three pages visually cohesive with the SPA while keeping functionality unchanged.

## Design

### Approach: shared static CSS file

A single hand-written `app.css` file lives alongside the templates, is embedded into the Go binary via `//go:embed`, and is served at a stable URL that all three pages reference via `<link>`. The CSS uses CSS custom properties for the SPA's light-theme OKLCH tokens and exposes a small set of semantic component classes (`.card`, `.btn`, `.input`, etc.).

This keeps the pages independent of the SPA bundle and the Vite/Tailwind build chain. Drift risk is contained to one file (`ui/shared/app.css`) that maps directly onto `ui/frontend/src/styles/globals.css` and can be diffed when the SPA theme changes.

**Theme:** light only. The SPA defaults to `system` via `next-themes`, but the pre-SPA pages always render in the light palette. No theme-switching JS, no `prefers-color-scheme` handling.

**Font:** system stack only — `ui-sans-serif, system-ui, sans-serif` for sans and `ui-monospace, monospace` for mono. No Geist files bundled. Pages will look ~95% like the SPA but with system letterforms.

### File layout

```
ui/
├── shared/
│   └── app.css        # new — shared stylesheet
├── migrate/
│   └── index.html     # refactored — drops inline <style>
├── db-error/
│   └── index.html     # refactored — drops inline <style>
├── setup/
│   └── index.html     # refactored — drops inline <style>
└── ui.go              # add SharedBox embed
```

### Go changes

**`ui/ui.go`** — add a fourth embed box:

```go
//go:embed all:shared
var SharedBox embed.FS
```

**`internal/api/router.go`** — register a static-asset route:

```go
e.GET("/static/app.css", func(c *echo.Context) error {
    f, err := ui.SharedBox.Open("shared/app.css")
    if err != nil {
        return err
    }
    defer func() { _ = f.Close() }()
    c.Response().Header().Set("Cache-Control", "public, max-age=3600")
    return c.Stream(http.StatusOK, "text/css; charset=utf-8", f)
})
```

This co-exists with the existing `/static/cover_art/*` route — neither prefix collides.

**State gate exemptions** (`internal/api/router.go:60-104`) — `/static/app.css` must pass all three gates. Add the path to each gate's allow-list:

- Gate 1 (DB unavailable, line 66): `if path == "/db-error" || path == "/health" || path == "/static/app.css"`
- Gate 2 (migrations pending, line 82): add `|| path == "/static/app.css"` to the existing condition
- Gate 3 (setup required, line 96–97): add `|| path == "/static/app.css"`

Gate 4 (maintenance mode, `maint.MaintenanceMiddleware()`) requires a brief check: if it has its own allow-list, add the path; if it serves a fixed maintenance page, the CSS exemption may not matter. The implementation step verifies this.

### Shared CSS contents (`ui/shared/app.css`)

A single ~200-line file. Structure:

**Reset + base**

```css
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
body {
  background: var(--background);
  color: var(--foreground);
  font-family: var(--font-sans);
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}
```

**Design tokens** — copied from the `:root` block in `ui/frontend/src/styles/globals.css` (light theme only):

`--background`, `--foreground`, `--card`, `--card-foreground`, `--popover`, `--popover-foreground`, `--primary`, `--primary-foreground`, `--muted`, `--muted-foreground`, `--accent`, `--accent-foreground`, `--destructive`, `--border`, `--input`, `--ring`, `--radius` (= `0.625rem`).

Plus `--font-sans: ui-sans-serif, system-ui, sans-serif;` and `--font-mono: ui-monospace, monospace;`.

**Component classes**

- `.card` — `background: var(--card)`, `border: 1px solid var(--border)`, `border-radius: calc(var(--radius) + 4px)`, `padding: 1.5rem`, `width: 100%`, `max-width: 28rem`.
- `.card--wide` — modifier with `max-width: 40rem` for the migrate page.
- `.card-title` — `font-size: 1.5rem`, `font-weight: 700`, `margin-bottom: 0.25rem`.
- `.card-description` — `font-size: 0.875rem`, `color: var(--muted-foreground)`, `margin-bottom: 1rem`.
- `.btn` (base) and `.btn-primary` (filled) — `background: var(--primary)`, `color: var(--primary-foreground)`, `padding: 0.625rem 1rem`, `border-radius: var(--radius)`, `font-weight: 500`, focus ring via outline. `:disabled { opacity: 0.5; cursor: not-allowed; }`.
- `.btn-link` — text-only link button: transparent background, underlined `var(--muted-foreground)` text. Used by the setup page toggle.
- `.input` — `border: 1px solid var(--border)`, `border-radius: var(--radius)`, `padding: 0.5rem 0.75rem`, `font-size: 0.875rem`, full-width; focused state uses `outline: 2px solid var(--ring)`.
- `.label` — `font-size: 0.875rem`, `font-weight: 500`, `display: block`, `margin-bottom: 0.25rem`.
- `.alert-destructive` — error message box. Hidden by default (`display: none`); toggled visible by JS.
- `.code` — `background: var(--muted)`, `border: 1px solid var(--border)`, `border-radius: calc(var(--radius) - 2px)`, mono font, padded, scrollable overflow. Used for the redacted DSN and timestamp on db-error.
- `.log` — same base as `.code` but `height: 280px`, `overflow-y: auto`, accent text color for the migration log stream. `display: none` → `.log.visible { display: block; }`.
- `.meta` — small `var(--muted-foreground)` text for status and footer. Modifiers `.meta--error` (destructive color) and `.meta--success` (accent color).
- `.drop-zone`, `.file-info`, `.clear-btn` — setup-page file-picker affordances, restyled against tokens but functionally equivalent to today.
- `.stack > * + * { margin-top: 1rem; }` — vertical rhythm utility used inside `.card`.

### Per-page changes

#### `ui/migrate/index.html`

- Drop the inline `<style>` block.
- Add `<link rel="stylesheet" href="/static/app.css">` in `<head>`.
- Replace the wrapper markup with:
  ```html
  <div class="card card--wide">
    <h1 class="card-title">Database Migration Required</h1>
    <p class="card-description">
      {{.PendingCount}} migration{{if ne .PendingCount 1}}s{{end}} pending
    </p>
    <div class="log" id="log"></div>
    <button class="btn btn-primary" id="btn" onclick="runMigrations()">Run Migrations</button>
    <p class="meta" id="status"></p>
  </div>
  ```
- Status-text JS swaps `.status.error` / `.status.success` for `.meta--error` / `.meta--success` (rename only — same semantics).
- All other JS (status polling, EventSource log streaming, redirect on `complete`) is unchanged.

#### `ui/db-error/index.html`

- Drop the inline `<style>` block.
- Add `<link rel="stylesheet" href="/static/app.css">`.
- Markup:
  ```html
  <div class="card">
    <h1 class="card-title">Database Unavailable</h1>
    <p class="card-description">
      The server cannot reach the database. This page refreshes every 5 seconds.
    </p>
    <label class="label">Connection</label>
    <pre class="code">{{.RedactedDSN}}</pre>
    <label class="label">Last failed</label>
    <pre class="code">{{.LastUnavailableAt}}</pre>
  </div>
  ```
- The red `<h1>` severity color is dropped — the card communicates state through layout and copy, not color.
- `setTimeout(() => location.reload(), 5000)` is unchanged.

#### `ui/setup/index.html`

- Drop the inline `<style>` block.
- Add `<link rel="stylesheet" href="/static/app.css">`.
- Both `#admin-view` and `#restore-view` are wrapped in a single `<div class="card">`. Toggle via the existing JS show/hide.
- Form fields use `.label` + `.input`. Primary actions use `.btn .btn-primary`. The toggle links ("Restore from backup instead", "Cancel — create a new account instead") use `.btn .btn-link`.
- Drop-zone uses `.drop-zone`; selected-file row uses `.file-info` + `.clear-btn`. Inline error messages use `.alert-destructive` with the existing show/hide JS.
- All form-submit logic (`POST /api/auth/setup/admin`, `POST /api/auth/setup/restore`, file validation, redirects on success) is preserved as-is.

## Trade-offs Considered

- **Compiled Tailwind bundle for these pages** — set up a tiny Tailwind v4 build that emits a CSS bundle covering just the classes used by the three pages. Tokens would stay perfectly in sync with the SPA. Rejected as overkill: adds a second build target and a per-page Tailwind config, when a hand-written file with the same token values gives the same look. Drift risk on the handful of tokens used is low.
- **Inline CSS in each page (refresh-only)** — keep each page self-contained, redo the inline `<style>` blocks in the SPA style. Simplest but triples the maintenance surface and makes future theme changes a three-place edit. Rejected.
- **Make the pages React routes inside the SPA** — require the SPA to be loadable when the DB is unavailable, migrations are pending, or no users exist. The SPA isn't structured for this (all its API calls and providers assume a Ready state). Rejected.
- **Self-host Geist** — exact visual parity with the SPA at the cost of ~50–100KB of font files bundled into the binary. Rejected in favour of system fonts; the visual difference is subtle and the simplification is worth it.
- **Load Geist from CDN** — zero bundling cost. Rejected because the migrate/db-error pages may run in environments without reliable outbound network.

## Files Changed

- `ui/shared/app.css` — **new**, ~200 lines.
- `ui/ui.go` — add `//go:embed all:shared` and `SharedBox` var.
- `internal/api/router.go` — register `GET /static/app.css`, add `/static/app.css` to Gate 1/2/3 allow-lists, verify Gate 4 behaviour.
- `ui/migrate/index.html` (renamed from `migrate.html` mid-implementation for consistency with `db-error/` and `setup/`) — drop inline `<style>`, link `/static/app.css`, convert to class-based markup, rename `.status.error`/`.success` to `.meta--error`/`.meta--success` in JS.
- `ui/db-error/index.html` — drop inline `<style>`, link `/static/app.css`, convert to class-based markup.
- `ui/setup/index.html` — drop inline `<style>`, link `/static/app.css`, convert to class-based markup; reuse existing JS.

## Out of Scope

- Dark theme support on the pre-SPA pages.
- Bundling Geist (or any non-system font).
- Functional changes to any of the three pages.
- SPA-internal pages (`/login`, etc.) — already styled.
- The existing `/static/cover_art/*` and `/logos/*` routes.
- Cache busting / hashed filenames for `app.css` — `Cache-Control: public, max-age=3600` is sufficient.

## Verification

- `make` succeeds; `go vet ./...` and `golangci-lint run` clean.
- `curl -i http://localhost:8000/static/app.css` returns `200` with `Content-Type: text/css; charset=utf-8` in all four app states: `DBUnavailable`, `NeedsMigration` (mid-migration `Migrating` counts here), `NeedsSetup`, and `Ready`. This is the gate-exemption smoke test.
- Manual browser pass for each page in its matching app state:
  - `/db-error` — stop the DB and load the URL.
  - `/migrate` — point at an empty database and load `/`.
  - `/setup` — finish migrations against a clean DB with no users; load `/`.
- Each page matches the approved mockup, no FOUC, no console errors. Existing interactions still work (run migrations + log stream, file picker + restore upload, 5-second auto-refresh on db-error).
- No new Go unit tests required. Existing handler tests stay green; if any test inspects HTML body content it gets updated to match the new markup.
