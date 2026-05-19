# Pre-SPA Pages Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/migrate`, `/db-error`, and `/setup` visually cohesive with the SPA by introducing a shared static CSS file, without changing any page behavior.

**Architecture:** A single hand-written `ui/shared/app.css` is embedded via `//go:embed` and served at `/static/app.css`. The three pre-SPA HTML templates drop their inline `<style>` blocks and `<link>` to this shared stylesheet. The state-gate middlewares (DB-unavailable, migrations-pending, setup-required, maintenance) each allow-list the new path so the CSS loads in every pre-SPA state.

**Tech Stack:** Go 1.25, Echo v5, `html/template`, plain CSS3 with custom properties (no Tailwind, no build step), OKLCH color tokens copied from `ui/frontend/src/styles/globals.css`.

**Spec:** `docs/superpowers/specs/2026-05-19-pre-spa-pages-redesign-design.md`

**Issue:** #521

**Branch:** `feat/521-pre-spa-pages-redesign` (already created)

---

## Notes for the implementer

- **TDD does not apply cleanly here.** This is template + CSS work. Per the spec, no new Go unit tests are required. Each task ends with `make build` (verifies the embed resolves and the Go code compiles) and a commit. Visual verification happens at the end in Task 6.
- **Do not change any page behavior.** Same routes, same template variables, same JS flow, same fetch calls. Only markup, CSS, and class names change.
- **Light theme only.** Do not add a `prefers-color-scheme` media query or any theme-toggle JS.
- **System fonts only.** Do not bundle Geist or add any `@font-face` rules. The CSS sets `--font-sans: ui-sans-serif, system-ui, sans-serif` and that's it.
- **Run `make build` after every code change** to confirm `//go:embed` paths resolve and the binary compiles. The frontend doesn't need to rebuild for these tasks (`make` runs frontend first, which is slow; `make build` is the Go-only target).
- **The plan assumes you are already on branch `feat/521-pre-spa-pages-redesign`.** If not, `git switch feat/521-pre-spa-pages-redesign` before starting.

---

## Task 1: Static asset infrastructure

Wire the plumbing first with a placeholder CSS file so the embed compiles and the route returns something. The real CSS goes in Task 2.

**Files:**
- Create: `ui/shared/app.css` (placeholder, 2 lines)
- Modify: `ui/ui.go` (add SharedBox embed)
- Modify: `internal/api/router.go` (register `/static/app.css` route; exempt path from Gates 1/2/3)
- Modify: `internal/middleware/maintenance.go` (exempt path from Gate 4)

---

- [ ] **Step 1: Create the placeholder CSS file**

Create `ui/shared/app.css` with:

```css
/* Shared stylesheet for /migrate, /db-error, /setup. See spec 2026-05-19-pre-spa-pages-redesign-design.md */
```

This is enough for the embed to find a file. The real content goes in Task 2.

- [ ] **Step 2: Add the embed in `ui/ui.go`**

Open `ui/ui.go`. The current file has three embed boxes. Add a fourth:

```go
package ui

import "embed"

//go:embed all:frontend/dist
var UIBox embed.FS

//go:embed all:migrate
var MigrateBox embed.FS

//go:embed db-error
var DBErrorBox embed.FS

//go:embed setup
var SetupBox embed.FS

//go:embed all:shared
var SharedBox embed.FS
```

Use `all:shared` to be consistent with the other directory embeds even though the directory currently has no dot-files. It's a small futureproofing.

- [ ] **Step 3: Verify `ui/ui.go` builds**

Run: `go build ./ui/...`
Expected: no output, exit 0.

If you get "pattern shared: no matching files found," the placeholder file in Step 1 wasn't created.

- [ ] **Step 4: Register the `/static/app.css` route**

Open `internal/api/router.go`. Find the existing `/db-error` registration (around line 148) inside `registerRoutes`. Add the new route immediately after it:

```go
	// DB-error route (bypassed by Gate 1)
	dh := NewDBErrorHandler(resolvedDatabaseURL, migrator)
	e.GET("/db-error", dh.HandleDBError)

	// Shared stylesheet for /migrate, /db-error, /setup.
	// Must be allow-listed by every state gate (see Gate 1/2/3/4 above and the maintenance middleware).
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

The `ui` package is already imported in `router.go`; no new imports needed. `http` is also already imported.

- [ ] **Step 5: Exempt the path from Gate 1 (DB unavailable)**

In `internal/api/router.go`, find Gate 1 (around line 60–74). Change the path check:

```go
		// Gate 1: DB unavailable — redirect everything except /db-error, /health, and /static/app.css
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				state := migrator.State()
				if state == migrate.AppStateDBUnavailable {
					path := c.Request().URL.Path
					if path == "/db-error" || path == "/health" || path == "/static/app.css" {
						return next(c)
					}
					return c.Redirect(http.StatusFound,
						"/db-error?from="+url.QueryEscape(c.Request().RequestURI))
				}
				return next(c)
			}
		})
```

Update the comment on the line above to match the new allow-list.

- [ ] **Step 6: Exempt the path from Gate 2 (migrations pending)**

Still in `router.go`, find Gate 2 (around line 76–89):

```go
		// Gate 2: migrations pending — redirect everything except /migrate*, /api/migrate*, /health, /static/app.css
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				state := migrator.State()
				if state != migrate.AppStateReady && state != migrate.AppStateDBUnavailable {
					path := c.Request().URL.Path
					if strings.HasPrefix(path, "/migrate") || strings.HasPrefix(path, "/api/migrate") ||
						path == "/health" || path == "/static/app.css" {
						return next(c)
					}
					return c.Redirect(http.StatusFound, "/migrate")
				}
				return next(c)
			}
		})
```

- [ ] **Step 7: Exempt the path from Gate 3 (setup required)**

Find Gate 3 (around line 91–104):

```go
		// Gate 3: setup required — redirect everything except /setup, /api/auth/setup/*, /health, /api/migrate*, /static/app.css
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				if migrator.NeedsSetup() {
					path := c.Request().URL.Path
					if path == "/setup" || strings.HasPrefix(path, "/api/auth/setup") ||
						path == "/health" || strings.HasPrefix(path, "/api/migrate") ||
						path == "/static/app.css" {
						return next(c)
					}
					return c.Redirect(http.StatusFound, "/setup")
				}
				return next(c)
			}
		})
```

- [ ] **Step 8: Exempt the path from Gate 4 (maintenance middleware)**

Open `internal/middleware/maintenance.go`. The middleware's allow-list is at lines 35–38. Add `/static/app.css` to it:

```go
func MaintenanceMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !IsMaintenanceMode() {
				return next(c)
			}
			path := c.Request().URL.Path
			if path == "/health" ||
				strings.HasPrefix(path, "/api/admin/backups") ||
				path == "/api/auth/me" ||
				path == "/static/app.css" {
				return next(c)
			}
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"error":            "Service Unavailable",
				"detail":           "Restore in progress",
				"maintenance_mode": true,
			})
		}
	}
}
```

- [ ] **Step 9: Build and lint**

Run: `make build`
Expected: builds `./nexorious` with no errors.

Run: `golangci-lint run`
Expected: zero issues.

- [ ] **Step 10: Smoke test the route with a running server**

This requires a Postgres reachable via `DATABASE_URL`. If you don't have one set up, skip to Step 11 — the manual verification in Task 6 covers this.

Run the server in one terminal:
```bash
./nexorious
```

In another terminal:
```bash
curl -i http://localhost:8000/static/app.css
```

Expected response:
```
HTTP/1.1 200 OK
Content-Type: text/css; charset=utf-8
Cache-Control: public, max-age=3600
...

/* Shared stylesheet for /migrate, /db-error, /setup. See spec 2026-05-19-pre-spa-pages-redesign-design.md */
```

Stop the server (Ctrl-C).

- [ ] **Step 11: Commit**

```bash
git add ui/ui.go ui/shared/app.css internal/api/router.go internal/middleware/maintenance.go
git commit -m "$(cat <<'EOF'
feat(ui): add shared static stylesheet plumbing for pre-SPA pages

Wires up an embed-backed /static/app.css route used by the migrate,
db-error, and setup HTML templates. The path is exempted from all
four state gates (DB unavailable, migrations pending, setup required,
maintenance) so the stylesheet loads in every pre-SPA state. The CSS
file itself is a placeholder; real content follows in the next commit.

Refs #521

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Write the shared CSS file

Replace the placeholder with the full stylesheet. Light-theme tokens from the SPA's `globals.css`, system font stack, and the component classes the three templates will reference in Tasks 3–5.

**Files:**
- Modify: `ui/shared/app.css`

---

- [ ] **Step 1: Write the full CSS file**

Replace the entire contents of `ui/shared/app.css` with:

```css
/* Shared stylesheet for /migrate, /db-error, /setup.
   Tokens mirror the light theme in ui/frontend/src/styles/globals.css (:root).
   Spec: docs/superpowers/specs/2026-05-19-pre-spa-pages-redesign-design.md */

:root {
  /* Light theme OKLCH tokens, mirrored from ui/frontend/src/styles/globals.css */
  --background: oklch(1 0 0);
  --foreground: oklch(0.141 0.005 285.823);
  --card: oklch(1 0 0);
  --card-foreground: oklch(0.141 0.005 285.823);
  --primary: oklch(0.21 0.006 285.885);
  --primary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.967 0.001 286.375);
  --muted-foreground: oklch(0.552 0.016 285.938);
  --accent: oklch(0.967 0.001 286.375);
  --accent-foreground: oklch(0.21 0.006 285.885);
  --destructive: oklch(0.577 0.245 27.325);
  --destructive-foreground: oklch(0.985 0 0);
  --border: oklch(0.92 0.004 286.32);
  --input: oklch(0.92 0.004 286.32);
  --ring: oklch(0.705 0.015 286.067);
  --radius: 0.625rem;

  /* Log accent — readable green on the muted background (no equivalent token in the SPA) */
  --log-text: oklch(0.5 0.13 145);

  --font-sans: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  --font-mono: ui-monospace, SFMono-Regular, "JetBrains Mono", "Fira Code", monospace;
}

*, *::before, *::after {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  background: var(--background);
  color: var(--foreground);
  font-family: var(--font-sans);
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}

/* Card — the centered panel used by all three pages */
.card {
  background: var(--card);
  color: var(--card-foreground);
  border: 1px solid var(--border);
  border-radius: calc(var(--radius) + 4px);
  padding: 1.5rem;
  width: 100%;
  max-width: 28rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06);
}

.card--wide {
  max-width: 40rem;
}

.card-title {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: 0.25rem;
  letter-spacing: -0.01em;
}

.card-description {
  font-size: 0.875rem;
  color: var(--muted-foreground);
  margin-bottom: 1rem;
}

/* Vertical rhythm utility */
.stack > * + * {
  margin-top: 1rem;
}

/* Form primitives */
.label {
  display: block;
  font-size: 0.875rem;
  font-weight: 500;
  margin-bottom: 0.25rem;
  color: var(--foreground);
}

.input {
  width: 100%;
  background: var(--background);
  color: var(--foreground);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 0.5rem 0.75rem;
  font-size: 0.875rem;
  font-family: inherit;
  transition: border-color 0.15s, box-shadow 0.15s;
}

.input:focus {
  outline: 2px solid var(--ring);
  outline-offset: 1px;
  border-color: transparent;
}

.input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-family: inherit;
  font-size: 0.875rem;
  font-weight: 500;
  border-radius: var(--radius);
  padding: 0.625rem 1rem;
  border: 1px solid transparent;
  cursor: pointer;
  transition: opacity 0.15s, background-color 0.15s;
}

.btn:focus-visible {
  outline: 2px solid var(--ring);
  outline-offset: 2px;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: var(--primary);
  color: var(--primary-foreground);
  width: 100%;
}

.btn-primary:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-link {
  background: none;
  border: none;
  color: var(--muted-foreground);
  text-decoration: underline;
  font-size: 0.875rem;
  padding: 0;
  cursor: pointer;
}

.btn-link:hover:not(:disabled) {
  color: var(--foreground);
}

/* Alert (destructive) — hidden by default, shown when JS sets text */
.alert-destructive {
  background: color-mix(in oklch, var(--destructive) 12%, var(--background));
  border: 1px solid color-mix(in oklch, var(--destructive) 30%, var(--background));
  color: var(--destructive);
  border-radius: var(--radius);
  padding: 0.5rem 0.75rem;
  font-size: 0.875rem;
  margin-bottom: 0.75rem;
  display: none;
}

.alert-destructive.visible {
  display: block;
}

/* Code block — used for the redacted DSN and last-failed timestamp on db-error */
.code {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: calc(var(--radius) - 2px);
  padding: 0.5rem 0.75rem;
  font-family: var(--font-mono);
  font-size: 0.8125rem;
  color: var(--foreground);
  overflow-x: auto;
  word-break: break-all;
  white-space: pre-wrap;
  margin-bottom: 0.5rem;
}

/* Log stream — used by migrate page for the live EventSource output */
.log {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 0.75rem;
  font-family: var(--font-mono);
  font-size: 0.8125rem;
  color: var(--log-text);
  height: 18rem;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  margin-bottom: 1rem;
  display: none;
}

.log.visible {
  display: block;
}

/* Status / meta text under buttons and at page footers */
.meta {
  font-size: 0.875rem;
  color: var(--muted-foreground);
  margin-top: 0.75rem;
}

.meta--error {
  color: var(--destructive);
}

.meta--success {
  color: var(--log-text);
}

.meta--center {
  text-align: center;
}

/* Setup page file picker */
.drop-zone {
  border: 2px dashed var(--border);
  border-radius: var(--radius);
  padding: 2rem;
  text-align: center;
  cursor: pointer;
  color: var(--muted-foreground);
  font-size: 0.875rem;
  transition: border-color 0.15s, background-color 0.15s;
  margin-bottom: 1rem;
}

.drop-zone:hover {
  border-color: var(--muted-foreground);
  background: var(--muted);
}

.drop-zone-icon {
  font-size: 2rem;
  margin-bottom: 0.5rem;
}

.drop-zone strong {
  display: block;
  color: var(--foreground);
  font-weight: 500;
  margin-bottom: 0.25rem;
}

.file-info {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 0.75rem 1rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.file-info-text {
  min-width: 0;
}

.file-info-name {
  font-size: 0.875rem;
  color: var(--foreground);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: block;
}

.file-info-size {
  color: var(--muted-foreground);
  font-size: 0.75rem;
  display: block;
}

.clear-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--muted-foreground);
  font-size: 1.125rem;
  line-height: 1;
  padding: 0 0.25rem;
  flex-shrink: 0;
}

.clear-btn:hover {
  color: var(--foreground);
}

/* Footer link area (setup page toggle row) */
.footer-row {
  margin-top: 1rem;
  text-align: center;
}
```

- [ ] **Step 2: Build to confirm embed picks up the new content**

Run: `make build`
Expected: builds successfully.

- [ ] **Step 3: Smoke test the served bytes**

Optional if no DB is set up; rely on the manual verification in Task 6 instead.

Run the server, then:
```bash
curl -s http://localhost:8000/static/app.css | head -5
```

Expected: shows the comment line followed by the `:root {` block.

- [ ] **Step 4: Commit**

```bash
git add ui/shared/app.css
git commit -m "$(cat <<'EOF'
feat(ui): write shared stylesheet for pre-SPA pages

Mirrors the SPA's light-theme OKLCH tokens from globals.css and
defines the component classes (.card, .btn, .input, .label, .alert,
.code, .log, .drop-zone, .file-info) consumed by /migrate, /db-error,
and /setup. System fonts only; no Geist bundling, no dark theme.

Refs #521

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Refactor the migrate template

Strip the inline `<style>` block from `ui/migrate/index.html`, link the shared CSS, and convert the markup to use the new class names. All existing JavaScript (status polling, EventSource log streaming, redirect on `complete`) stays intact except for two class-name swaps in the status-text DOM updates (`.status.error` → `.meta--error`, `.status.success` → `.meta--success`).

**Files:**
- Modify: `ui/migrate/index.html`

---

- [ ] **Step 1: Replace the entire file**

Overwrite `ui/migrate/index.html` with:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Nexorious — Database Migration</title>
  <link rel="stylesheet" href="/static/app.css">
</head>
<body>
  <div class="card card--wide">
    <h1 class="card-title">Database Migration Required</h1>
    <p class="card-description">
      {{.PendingCount}} migration{{if ne .PendingCount 1}}s{{end}} pending
    </p>
    <div class="log" id="log"></div>
    <button class="btn btn-primary" id="btn" onclick="runMigrations()">Run Migrations</button>
    <p class="meta" id="status"></p>
  </div>

  <script>
    // Poll /api/migrate/status every 5s so the page reacts to server-side state
    // changes (DB going down → Gate 1 redirects, DB recovery → redirect to /).
    // Polling is stopped once the user kicks off a migration so the live log
    // stream is never interrupted by a reload.
    var pollTimer = setInterval(function() {
      fetch('/api/migrate/status')
        .then(function(res) {
          // Gate 1 redirects /api/migrate/status to /db-error when DB is down;
          // fetch follows the redirect and we land on HTML, not JSON.
          var ct = res.headers.get('content-type') || '';
          if (!ct.includes('application/json')) {
            window.location.reload();
            return;
          }
          return res.json();
        })
        .then(function(data) {
          if (!data) return;
          if (data.state === 'ready') {
            window.location.href = '/';
          }
        })
        .catch(function() { /* network blip — keep polling */ });
    }, 5000);

    function runMigrations() {
      clearInterval(pollTimer);
      const btn = document.getElementById('btn');
      const log = document.getElementById('log');
      const status = document.getElementById('status');

      btn.disabled = true;
      log.classList.add('visible');
      status.textContent = 'Running migrations…';
      status.className = 'meta';

      fetch('/api/migrate/run', { method: 'POST' })
        .then(res => {
          if (!res.ok) {
            throw new Error('Failed to start migration (HTTP ' + res.status + ')');
          }
          const es = new EventSource('/api/migrate/progress');

          es.onmessage = function(e) {
            log.textContent += e.data + '\n';
            log.scrollTop = log.scrollHeight;
          };

          es.addEventListener('complete', function() {
            es.close();
            status.textContent = 'Migration complete. Redirecting…';
            status.className = 'meta meta--success';
            setTimeout(() => { window.location.href = '/'; }, 1000);
          });

          es.onerror = function() {
            es.close();
            status.textContent = 'Connection lost. Check logs and refresh to retry.';
            status.className = 'meta meta--error';
            btn.disabled = false;
          };
        })
        .catch(err => {
          status.textContent = err.message;
          status.className = 'meta meta--error';
          btn.disabled = false;
        });
    }
  </script>
</body>
</html>
```

Two semantic changes compared to before:
- `<style>` block removed; `<link rel="stylesheet" href="/static/app.css">` added.
- Status-text class transitions go from `status` / `status error` / `status success` to `meta` / `meta meta--error` / `meta meta--success`.

Template variable (`{{.PendingCount}}`) is unchanged; the handler at `internal/migrate/handler.go:37-41` keeps working.

- [ ] **Step 2: Build**

Run: `make build`
Expected: builds successfully. The embed picks up the modified file.

- [ ] **Step 3: Commit**

```bash
git add ui/migrate/index.html
git commit -m "$(cat <<'EOF'
feat(ui): restyle /migrate to use the shared stylesheet

Drops the inline dark-theme styles and links /static/app.css. Status
text JS uses the new meta--error / meta--success modifier classes.
Functionality unchanged — same polling, same EventSource log stream,
same redirect on complete.

Refs #521

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Refactor the db-error template

**Files:**
- Modify: `ui/db-error/index.html`

---

- [ ] **Step 1: Replace the entire file**

Overwrite `ui/db-error/index.html` with:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Nexorious — Database Unavailable</title>
  <link rel="stylesheet" href="/static/app.css">
</head>
<body>
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
  <script>setTimeout(() => location.reload(), 5000)</script>
</body>
</html>
```

Behavior is identical to before: `setTimeout(() => location.reload(), 5000)` is preserved. Template variables `{{.RedactedDSN}}` and `{{.LastUnavailableAt}}` match the handler at `internal/api/db_error.go:47-53`. The red severity color is dropped per the spec — informational layout only.

- [ ] **Step 2: Build**

Run: `make build`
Expected: builds successfully.

- [ ] **Step 3: Commit**

```bash
git add ui/db-error/index.html
git commit -m "$(cat <<'EOF'
feat(ui): restyle /db-error to use the shared stylesheet

Drops the bespoke red-on-white inline styles and links /static/app.css.
Template variables and the 5s auto-refresh are unchanged.

Refs #521

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Refactor the setup template

The setup page is the most involved of the three — two views (admin / restore) sharing a card, a file picker, error alerts, and two POST endpoints. The form-submit JavaScript is preserved verbatim except for class names on the error alerts (`.error` → `.alert-destructive` with a `.visible` toggle instead of style.display).

**Files:**
- Modify: `ui/setup/index.html`

---

- [ ] **Step 1: Replace the entire file**

Overwrite `ui/setup/index.html` with:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Nexorious — Setup</title>
  <link rel="stylesheet" href="/static/app.css">
</head>
<body>
  <div class="card">
    <div id="admin-view">
      <h1 class="card-title">Welcome to Nexorious</h1>
      <p class="card-description">Create your admin account to get started.</p>
      <div class="alert-destructive" id="admin-err"></div>
      <form id="admin-form" class="stack">
        <div>
          <label class="label" for="username">Username</label>
          <input class="input" type="text" id="username" name="username" autocomplete="username" required minlength="3">
        </div>
        <div>
          <label class="label" for="password">Password</label>
          <input class="input" type="password" id="password" name="password" autocomplete="new-password" required minlength="8">
        </div>
        <div>
          <label class="label" for="confirm">Confirm Password</label>
          <input class="input" type="password" id="confirm" name="confirm" autocomplete="new-password" required minlength="8">
        </div>
        <button type="submit" class="btn btn-primary" id="admin-btn">Create Admin Account</button>
      </form>
      <div class="footer-row">
        <button type="button" class="btn-link" onclick="showRestore()">Restore from backup instead</button>
      </div>
    </div>

    <div id="restore-view" style="display:none">
      <h1 class="card-title">Restore from Backup</h1>
      <p class="card-description">Upload a backup archive to restore your data.</p>
      <div class="alert-destructive" id="restore-err"></div>
      <input type="file" id="file-input" accept=".tar.gz" style="display:none">
      <div id="drop-zone" class="drop-zone" onclick="document.getElementById('file-input').click()">
        <div class="drop-zone-icon">📦</div>
        <strong>Click to select a backup file</strong>
        <span>.tar.gz files only</span>
      </div>
      <div id="file-info" class="file-info" style="display:none">
        <div class="file-info-text">
          <span class="file-info-name" id="file-name"></span>
          <span class="file-info-size" id="file-size"></span>
        </div>
        <button type="button" class="clear-btn" onclick="clearFile()" title="Remove">✕</button>
      </div>
      <button type="button" class="btn btn-primary" id="restore-btn" onclick="doRestore()" disabled>Restore</button>
      <div class="footer-row">
        <button type="button" class="btn-link" onclick="showAdmin()">Cancel — create a new account instead</button>
      </div>
    </div>
  </div>

  <script>
    function showRestore() {
      document.getElementById('admin-view').style.display = 'none';
      document.getElementById('restore-view').style.display = '';
      clearError('restore-err');
    }
    function showAdmin() {
      document.getElementById('restore-view').style.display = 'none';
      document.getElementById('admin-view').style.display = '';
      clearError('admin-err');
    }
    function showError(id, msg) {
      const el = document.getElementById(id);
      el.textContent = msg;
      el.classList.add('visible');
    }
    function clearError(id) {
      const el = document.getElementById(id);
      el.textContent = '';
      el.classList.remove('visible');
    }

    // File selection
    document.getElementById('file-input').addEventListener('change', (e) => {
      const file = e.target.files[0];
      if (!file) return;
      if (!file.name.endsWith('.tar.gz')) {
        showError('restore-err', 'Please select a .tar.gz backup file.');
        e.target.value = '';
        return;
      }
      clearError('restore-err');
      document.getElementById('drop-zone').style.display = 'none';
      document.getElementById('file-name').textContent = file.name;
      document.getElementById('file-size').textContent = formatBytes(file.size);
      document.getElementById('file-info').style.display = 'flex';
      document.getElementById('restore-btn').disabled = false;
    });

    function clearFile() {
      document.getElementById('file-input').value = '';
      document.getElementById('file-info').style.display = 'none';
      document.getElementById('drop-zone').style.display = '';
      document.getElementById('restore-btn').disabled = true;
      clearError('restore-err');
    }

    function formatBytes(bytes) {
      if (bytes < 1024) return bytes + ' B';
      if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
      return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    }

    // Create admin
    document.getElementById('admin-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      clearError('admin-err');
      const username = document.getElementById('username').value.trim();
      const password = document.getElementById('password').value;
      const confirm = document.getElementById('confirm').value;

      if (username.length < 3) return showError('admin-err', 'Username must be at least 3 characters.');
      if (password.length < 8) return showError('admin-err', 'Password must be at least 8 characters.');
      if (password !== confirm) return showError('admin-err', 'Passwords do not match.');

      const btn = document.getElementById('admin-btn');
      btn.disabled = true;
      btn.textContent = 'Creating account…';

      try {
        const res = await fetch('/api/auth/setup/admin', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username, password }),
        });
        if (res.status === 201) {
          const data = await res.json();
          localStorage.setItem('auth', JSON.stringify({
            accessToken: data.access_token,
            refreshToken: data.refresh_token,
            user: { id: data.user.id, username: data.user.username, isAdmin: data.user.is_admin, preferences: {} },
          }));
          window.location.href = '/';
        } else if (res.status === 400) {
          const body = await res.json();
          showError('admin-err', body.error || 'Validation error.');
        } else if (res.status === 403 || res.status === 500) {
          window.location.href = '/login';
        } else {
          showError('admin-err', 'Setup failed. Please try again.');
        }
      } catch {
        showError('admin-err', 'Setup failed. Please try again.');
      } finally {
        btn.disabled = false;
        btn.textContent = 'Create Admin Account';
      }
    });

    // Restore from backup
    async function doRestore() {
      const file = document.getElementById('file-input').files[0];
      if (!file) return;
      clearError('restore-err');

      const btn = document.getElementById('restore-btn');
      btn.disabled = true;
      btn.textContent = 'Restoring…';

      try {
        const form = new FormData();
        form.append('file', file);
        const res = await fetch('/api/auth/setup/restore', { method: 'POST', body: form });
        if (res.ok) {
          window.location.href = '/login';
        } else {
          const body = await res.json().catch(() => ({}));
          showError('restore-err', body.error || 'Restore failed. Please try again.');
          btn.disabled = false;
          btn.textContent = 'Restore';
        }
      } catch {
        showError('restore-err', 'Restore failed. Please try again.');
        btn.disabled = false;
        btn.textContent = 'Restore';
      }
    }
  </script>
</body>
</html>
```

Differences vs. the original:
- Inline `<style>` block removed; `<link>` to shared CSS added.
- Form fields wrapped in `<div>`s and given `.label` / `.input` classes; the form itself gets `.stack` for vertical rhythm.
- Toggle links converted from `<button class="link">` to `<button class="btn-link">`.
- Error display switched from `style.display = 'block' / 'none'` to `classList.add('visible') / .remove('visible')` so it composes with the alert styling.
- Drop-zone uses `.drop-zone-icon`, and the `<p>` for ".tar.gz files only" is a `<span>` (so the global CSS reset doesn't double-space it).
- File-info uses `.file-info-text`, `.file-info-name`, `.file-info-size` instead of inline styles.

All fetch endpoints (`/api/auth/setup/admin`, `/api/auth/setup/restore`), localStorage writes, and redirects are unchanged.

- [ ] **Step 2: Build**

Run: `make build`
Expected: builds successfully.

- [ ] **Step 3: Commit**

```bash
git add ui/setup/index.html
git commit -m "$(cat <<'EOF'
feat(ui): restyle /setup to use the shared stylesheet

Drops the bespoke inline styles and links /static/app.css. Form fields,
drop-zone, file info row, and error alerts use the new shared classes.
Error visibility now toggles via .alert-destructive .visible instead of
inline display style. All submit logic, fetch endpoints, and redirects
are unchanged.

Refs #521

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: End-to-end verification

Walk the three pages through their app states and confirm they render correctly. No code changes in this task — only running the binary and checking the result. If anything is wrong, go back to the relevant task, fix it, and commit a follow-up.

**Files:** none (verification only)

---

- [ ] **Step 1: Build the full binary**

Run: `make`
Expected: `make frontend` runs first (rebuilds the SPA into `ui/frontend/dist`), then `make build` produces `./nexorious`. Both succeed with no warnings.

- [ ] **Step 2: Verify `/static/app.css` returns 200 in Ready state**

Start the server with a healthy DB and all migrations applied:
```bash
./nexorious &
SERVER_PID=$!
sleep 2
curl -s -o /dev/null -w "%{http_code} %{content_type}\n" http://localhost:8000/static/app.css
kill $SERVER_PID
```

Expected output: `200 text/css; charset=utf-8`.

- [ ] **Step 3: Verify the /migrate page renders correctly**

This requires the app to be in `NeedsMigration` state. Easiest setup: point at a fresh empty database.

```bash
# In a scratch terminal or session
createdb nexorious_visual_check    # or use any empty DB
export DATABASE_URL="postgres://...nexorious_visual_check..."
export SECRET_KEY=$(head -c 32 /dev/urandom | base64)
./nexorious
```

In a browser, open http://localhost:8000/ — it should redirect to /migrate.

Visual checklist:
- White/near-white background, centered card.
- Title "Database Migration Required" in dark text, bold, no theme-specific accent color.
- Pending count line in muted gray below.
- "Run Migrations" button is dark/near-black with light text, full-width.
- DevTools Network: `/static/app.css` returns 200 with `Content-Type: text/css; charset=utf-8`.
- Click the button: log box appears, lines stream in (green text on light gray background), and on completion the status reads "Migration complete. Redirecting…" in green before the page redirects.
- No console errors.

Stop the server. Drop the test database.

- [ ] **Step 4: Verify the /db-error page renders correctly**

Reuse the test DB created in Step 3, but stop Postgres before starting the server (or point at a known-bad DSN).

```bash
# Stop Postgres or use a bogus DSN
export DATABASE_URL="postgres://nobody:nopw@localhost:5432/nonexistent?sslmode=disable"
./nexorious
```

Open http://localhost:8000/ — should redirect to /db-error.

Visual checklist:
- Centered card, light theme.
- Title "Database Unavailable" in default dark text — no red severity color.
- Description paragraph in muted gray.
- "Connection" label followed by a code block showing the redacted DSN (no `***` peeking through unescaped).
- "Last failed" label followed by a code block with the RFC3339 timestamp.
- Auto-refresh fires after 5 seconds (Network tab shows a fresh request).
- DevTools Network: `/static/app.css` returns 200.
- No console errors.

Stop the server.

- [ ] **Step 5: Verify the /setup page renders correctly**

Start a fresh DB, run migrations, leave no admin user.

```bash
createdb nexorious_setup_check
export DATABASE_URL="postgres://...nexorious_setup_check..."
export SECRET_KEY=$(head -c 32 /dev/urandom | base64)
./nexorious migrate up      # apply schema
./nexorious                  # start server
```

Open http://localhost:8000/ — should redirect to /setup.

Visual checklist for the admin view:
- Centered card, "Welcome to Nexorious" title.
- Three labeled inputs (Username / Password / Confirm Password), each on its own row with consistent spacing.
- "Create Admin Account" button is the same primary style as the other pages.
- "Restore from backup instead" as a small underlined link below the button.
- DevTools Network: `/static/app.css` returns 200.

Submit an invalid form (e.g. password mismatch). Error message appears in a tinted red box above the form.

Click "Restore from backup instead". The view switches:
- "Restore from Backup" title.
- Drop zone with dashed border and the 📦 icon and instructions.
- Clicking the drop zone opens a file picker. Selecting a non-`.tar.gz` shows the destructive alert.
- Selecting a valid `.tar.gz` swaps the drop zone for a file-info row with name + size and a × clear button. The Restore button enables.
- "Cancel — create a new account instead" toggles back.

No console errors anywhere.

Stop the server. Drop the test database.

- [ ] **Step 6: Confirm linting and tests still pass**

```bash
golangci-lint run
go test -timeout 600s ./...
```

Expected: both exit 0.

- [ ] **Step 7: Update slumber.yaml if needed**

The new `/static/app.css` route is a static asset, not an API endpoint, and CLAUDE.md's slumber rule applies to API routes. **No slumber.yaml change is required.**

(If you're unsure, check `slumber.yaml` — there is no existing entry for `/static/cover_art/*`, confirming static assets are out of scope for the collection.)

- [ ] **Step 8: Final review of the branch**

```bash
git log --oneline main..HEAD
```

Expected: five commits on `feat/521-pre-spa-pages-redesign` from this plan, plus the spec commit (`d4b8e342`):

1. `docs(specs): add pre-SPA pages redesign design for #521`
2. `feat(ui): add shared static stylesheet plumbing for pre-SPA pages`
3. `feat(ui): write shared stylesheet for pre-SPA pages`
4. `feat(ui): restyle /migrate to use the shared stylesheet`
5. `feat(ui): restyle /db-error to use the shared stylesheet`
6. `feat(ui): restyle /setup to use the shared stylesheet`

The branch is ready for PR. Do not push or open a PR autonomously — wait for explicit user instruction per CLAUDE.md.
