# Brand Icon Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the brand icon defined in [2026-05-20-brand-icon-design.md](../specs/2026-05-20-brand-icon-design.md) — SVG and PNG assets, app-shell wiring, brand-color CSS variable, and Helm chart `icon:` field.

**Architecture:** Static assets committed to the repo. The master SVG drops into [ui/frontend/public/](../../../ui/frontend/public/), where Vite copies it as-is into `dist/`, which Go embeds via the existing `//go:embed all:frontend/dist` directive in [ui/ui.go](../../../ui/ui.go) — no build pipeline changes. PNG and `.ico` files are produced from the master SVG by a small shell script (`scripts/regen-icons.sh`) using `rsvg-convert` and `imagemagick`, both added to devenv. React shell components and two static HTML pages get the icon inline. Chart.yaml gets one new field pointing at a stable GitHub raw URL of the committed PNG.

**Tech Stack:** SVG, PNG, librsvg (rsvg-convert), imagemagick (convert), Vite static asset pipeline, React + TanStack Router, Tailwind CSS v4, Helm Chart.yaml.

---

## File Structure

**New files:**
- `ui/frontend/public/logo.svg` — master SVG with constellation
- `ui/frontend/public/favicon.svg` — favicon SVG (no constellation, same badge + joystick)
- `ui/frontend/public/favicon.ico` — 16/32/48 multi-size ICO, generated from favicon.svg
- `ui/frontend/public/apple-touch-icon.png` — 180×180 PNG, generated from logo.svg
- `deploy/helm/icon.png` — 256×256 PNG, generated from logo.svg
- `scripts/regen-icons.sh` — one-shot bash script that regenerates the PNGs and ICO

**Modified files:**
- `devenv.nix` — add `librsvg` and `imagemagick` packages
- `ui/frontend/index.html` — `<head>` icon links + `theme-color` meta
- `ui/frontend/src/styles/globals.css` — `--brand-purple: #6d28d9;` in `:root`
- `ui/frontend/src/components/navigation/sidebar.tsx` — icon next to wordmark
- `ui/frontend/src/components/navigation/mobile-nav.tsx` — icon in both wordmark instances (Sheet header and external header)
- `ui/frontend/src/routes/_public/login.tsx` — icon above the card title
- `ui/setup/index.html` — icon next to card title (both admin and restore views)
- `ui/migrate/index.html` — icon next to card title
- `deploy/helm/Chart.yaml` — add `icon:` field

The brand asset files live with their consumer: SPA assets in `ui/frontend/public/`, Helm asset in `deploy/helm/`. Both are versioned alongside the code that uses them.

---

## Task 1: Add image-conversion tools to devenv

**Files:**
- Modify: `devenv.nix:12-23` (the `packages` list)

- [ ] **Step 1: Add librsvg and imagemagick, and alphabetize the list**

Edit `devenv.nix`, in the `packages = with pkgs; [ ... ];` block (lines 12-23). Add `imagemagick` and `librsvg`, and sort the whole list alphabetically. After the edit the block reads:

```nix
  packages = with pkgs; [
    git
    gnumake
    go-task
    golangci-lint
    imagemagick
    inputs.drzero42.packages.${system}.slumber
    legendary-gl
    librsvg
    nodejs_24
    procps
    uv
    yamllint
  ];
```

`librsvg` provides `rsvg-convert`; `imagemagick` provides `convert`/`magick`. Both are stable nixpkgs packages with no extra config required.

- [ ] **Step 2: Verify the tools are on PATH inside the dev shell**

Run, from the repo root:

```bash
devenv shell -- which rsvg-convert convert
```

Expected: two absolute paths print (both under the nix store), exit code 0. If you're already inside an active dev shell (e.g. via direnv), you may need to re-enter it for the new packages to appear — `exit` then `devenv shell`, or `direnv reload`.

- [ ] **Step 3: Commit**

```bash
git add devenv.nix devenv.lock
git commit -m "chore(devenv): add librsvg and imagemagick for icon regeneration"
```

(`devenv.lock` is updated by nix on first shell entry after the change. If it didn't change, commit only devenv.nix.)

---

## Task 2: Create master logo.svg and favicon.svg

**Files:**
- Create: `ui/frontend/public/logo.svg`
- Create: `ui/frontend/public/favicon.svg`

- [ ] **Step 1: Write logo.svg**

Create `ui/frontend/public/logo.svg` with the exact markup from the spec:

```svg
<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Nexorious">
  <rect x="4" y="4" width="92" height="92" rx="20" fill="#6d28d9"/>
  <g fill="#ffffff" opacity="0.5">
    <circle cx="16" cy="18" r="1.8"/>
    <circle cx="34" cy="12" r="1.4"/>
    <circle cx="60" cy="16" r="1.8"/>
    <circle cx="82" cy="22" r="1.4"/>
    <circle cx="14" cy="44" r="1.4"/>
    <circle cx="86" cy="46" r="1.8"/>
    <circle cx="22" cy="60" r="1.2"/>
    <circle cx="78" cy="58" r="1.2"/>
  </g>
  <path d="M 24 78 L 76 78 L 70 64 L 30 64 Z" fill="#ffffff"/>
  <rect x="46.5" y="30" width="7" height="34" fill="#ffffff"/>
  <circle cx="50" cy="26" r="12" fill="#ffffff"/>
  <circle cx="63" cy="72" r="3.5" fill="#6d28d9"/>
</svg>
```

- [ ] **Step 2: Write favicon.svg**

Create `ui/frontend/public/favicon.svg` — same badge + joystick, no constellation `<g>`:

```svg
<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Nexorious">
  <rect x="4" y="4" width="92" height="92" rx="20" fill="#6d28d9"/>
  <path d="M 24 78 L 76 78 L 70 64 L 30 64 Z" fill="#ffffff"/>
  <rect x="46.5" y="30" width="7" height="34" fill="#ffffff"/>
  <circle cx="50" cy="26" r="12" fill="#ffffff"/>
  <circle cx="63" cy="72" r="3.5" fill="#6d28d9"/>
</svg>
```

- [ ] **Step 3: Sanity-check the SVGs render**

Run:

```bash
rsvg-convert -w 256 -h 256 ui/frontend/public/logo.svg -o /tmp/logo-check.png
rsvg-convert -w 32 -h 32 ui/frontend/public/favicon.svg -o /tmp/favicon-check.png
file /tmp/logo-check.png /tmp/favicon-check.png
```

Expected: both files report as `PNG image data, ... 8-bit/color RGBA, non-interlaced` with the correct dimensions. (Don't commit /tmp/ files; this is just a sanity check.) Open them in an image viewer if you want to eyeball the result.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/public/logo.svg ui/frontend/public/favicon.svg
git commit -m "feat(branding): add master SVG and favicon SVG assets"
```

---

## Task 3: Create regen-icons.sh and generate binary assets

**Files:**
- Create: `scripts/regen-icons.sh`
- Create: `ui/frontend/public/apple-touch-icon.png`
- Create: `ui/frontend/public/favicon.ico`
- Create: `deploy/helm/icon.png`

- [ ] **Step 1: Write scripts/regen-icons.sh**

Create `scripts/regen-icons.sh`:

```bash
#!/usr/bin/env bash
# Regenerate brand-icon binary assets from the master SVGs.
# Requires librsvg (rsvg-convert) and imagemagick (convert) — provided by devenv.
set -euo pipefail

cd "$(dirname "$0")/.."

LOGO=ui/frontend/public/logo.svg
FAVICON_SVG=ui/frontend/public/favicon.svg

rsvg-convert -w 180 -h 180 "$LOGO" -o ui/frontend/public/apple-touch-icon.png
rsvg-convert -w 256 -h 256 "$LOGO" -o deploy/helm/icon.png

# favicon.ico carries multiple sizes; ImageMagick handles the multi-size pack.
convert -background none "$FAVICON_SVG" \
  -define icon:auto-resize=16,32,48 \
  ui/frontend/public/favicon.ico

echo "Regenerated:"
echo "  ui/frontend/public/apple-touch-icon.png"
echo "  ui/frontend/public/favicon.ico"
echo "  deploy/helm/icon.png"
```

- [ ] **Step 2: Make the script executable**

```bash
chmod +x scripts/regen-icons.sh
```

- [ ] **Step 3: Run it**

```bash
./scripts/regen-icons.sh
```

Expected: three "Regenerated:" lines, no errors. Three new binary files exist:

```bash
ls -la ui/frontend/public/apple-touch-icon.png ui/frontend/public/favicon.ico deploy/helm/icon.png
file ui/frontend/public/apple-touch-icon.png ui/frontend/public/favicon.ico deploy/helm/icon.png
```

The `file` output should report:
- `apple-touch-icon.png`: `PNG image data, 180 x 180`
- `favicon.ico`: `MS Windows icon resource - 3 icons, 16x16, ...; 32x32, ...; 48x48, ...`
- `icon.png`: `PNG image data, 256 x 256`

- [ ] **Step 4: Commit**

```bash
git add scripts/regen-icons.sh \
  ui/frontend/public/apple-touch-icon.png \
  ui/frontend/public/favicon.ico \
  deploy/helm/icon.png
git commit -m "feat(branding): generate PNG and ICO assets from master SVG"
```

---

## Task 4: Wire index.html head

**Files:**
- Modify: `ui/frontend/index.html:5` (replace single favicon link with full block)

- [ ] **Step 1: Replace the `<link rel="icon">` line and add neighboring tags**

In `ui/frontend/index.html`, the current `<head>` (lines 3-9) has a single favicon link. Replace it so the head reads:

```html
<head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
    <link rel="icon" type="image/x-icon" href="/favicon.ico" />
    <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
    <meta name="theme-color" content="#6d28d9" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Nexorious</title>
    <meta name="description" content="Game collection management" />
  </head>
```

The order matters: modern browsers pick the first matching `<link rel="icon">`, so the SVG goes first, the legacy ICO second.

- [ ] **Step 2: Verify the frontend still type-checks and builds, and the new tags are in the embedded HTML**

From the repo root:

```bash
make frontend
grep -E 'favicon\.svg|favicon\.ico|apple-touch-icon|theme-color' ui/frontend/dist/index.html
```

Expected: `make frontend` exits 0; the grep prints all four matching lines.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/index.html
git commit -m "feat(branding): wire favicon, apple-touch-icon, and theme-color into index.html"
```

---

## Task 5: Add --brand-purple CSS variable

**Files:**
- Modify: `ui/frontend/src/styles/globals.css:49-84` (`:root` block) and lines 86-118 (`.dark` block)

- [ ] **Step 1: Add the variable to :root**

In `ui/frontend/src/styles/globals.css`, inside the `:root { ... }` block (starts at line 49), add this line directly after the `--radius: 0.625rem;` line (around line 52):

```css
  --brand-purple: #6d28d9;
```

- [ ] **Step 2: Mirror in .dark**

In the `.dark { ... }` block (starts at line 86), add the same line near the top of the block (after the first variable):

```css
  --brand-purple: #6d28d9;
```

(Same value in both modes per the spec — the badge color reads on both light and dark surfaces.)

- [ ] **Step 3: Verify type-check still passes**

From `ui/frontend/`:

```bash
npm run check
```

Expected: passes. (CSS changes don't affect TypeScript, but this confirms we didn't accidentally break anything else.)

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/styles/globals.css
git commit -m "feat(branding): add --brand-purple CSS variable"
```

---

## Task 6: Add icon to desktop sidebar

**Files:**
- Modify: `ui/frontend/src/components/navigation/sidebar.tsx:20-24`

- [ ] **Step 1: Add the icon next to the wordmark**

In `ui/frontend/src/components/navigation/sidebar.tsx`, replace the current "Logo" block (lines 20-24):

```tsx
      {/* Logo */}
      <div className="p-4 border-b">
        <Link to="/games" className="block">
          <h1 className="text-xl font-bold">Nexorious</h1>
        </Link>
      </div>
```

with:

```tsx
      {/* Logo */}
      <div className="p-4 border-b">
        <Link to="/games" className="flex items-center gap-2">
          <img src="/logo.svg" alt="" className="h-8 w-8" />
          <h1 className="text-xl font-bold">Nexorious</h1>
        </Link>
      </div>
```

`alt=""` is intentional — the icon is decorative because the wordmark sits right next to it; an empty alt prevents screen readers from announcing the brand twice.

- [ ] **Step 2: Verify**

From `ui/frontend/`:

```bash
npm run check && npm run knip
```

Expected: both pass with no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/navigation/sidebar.tsx
git commit -m "feat(branding): show icon next to wordmark in desktop sidebar"
```

---

## Task 7: Add icon to mobile nav

**Files:**
- Modify: `ui/frontend/src/components/navigation/mobile-nav.tsx:43-47` (Sheet header) and `:115-117` (external header)

- [ ] **Step 1: Update the Sheet header brand**

In `ui/frontend/src/components/navigation/mobile-nav.tsx`, find the `<SheetTitle>` block at lines 43-47:

```tsx
              <SheetTitle>
                <Link to="/games" onClick={handleNavigate}>
                  Nexorious
                </Link>
              </SheetTitle>
```

Replace with:

```tsx
              <SheetTitle>
                <Link
                  to="/games"
                  onClick={handleNavigate}
                  className="flex items-center gap-2"
                >
                  <img src="/logo.svg" alt="" className="h-7 w-7" />
                  Nexorious
                </Link>
              </SheetTitle>
```

- [ ] **Step 2: Update the external mobile-header brand**

In the same file, find the second wordmark at lines 115-117:

```tsx
        <Link to="/games" className="font-bold text-lg">
          Nexorious
        </Link>
```

Replace with:

```tsx
        <Link to="/games" className="flex items-center gap-2 font-bold text-lg">
          <img src="/logo.svg" alt="" className="h-7 w-7" />
          Nexorious
        </Link>
```

- [ ] **Step 3: Verify**

From `ui/frontend/`:

```bash
npm run check && npm run knip && npm run test
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/navigation/mobile-nav.tsx
git commit -m "feat(branding): show icon next to wordmark in mobile nav"
```

---

## Task 8: Add icon to login page

**Files:**
- Modify: `ui/frontend/src/routes/_public/login.tsx:66-72` (CardHeader)

- [ ] **Step 1: Insert the icon above the title**

In `ui/frontend/src/routes/_public/login.tsx`, find the `<CardHeader>` block at lines 66-72:

```tsx
      <CardHeader className="space-y-1 text-center">
        <CardTitle className="text-2xl font-bold">Nexorious</CardTitle>
        <CardDescription>
          Sign in to your account to continue
        </CardDescription>
      </CardHeader>
```

Replace with:

```tsx
      <CardHeader className="space-y-2 text-center">
        <img
          src="/logo.svg"
          alt=""
          className="mx-auto h-14 w-14"
        />
        <CardTitle className="text-2xl font-bold">Nexorious</CardTitle>
        <CardDescription>
          Sign in to your account to continue
        </CardDescription>
      </CardHeader>
```

Note: `space-y-1` → `space-y-2` to give the icon a bit more breathing room from the title.

- [ ] **Step 2: Verify**

From `ui/frontend/`:

```bash
npm run check && npm run knip && npm run test
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/routes/_public/login.tsx
git commit -m "feat(branding): show icon above wordmark on login page"
```

---

## Task 9: Add icon to setup HTML

**Files:**
- Modify: `ui/setup/index.html` (admin view header at line 12 and restore view header at line 36)

- [ ] **Step 1: Add a brand-row container style**

In `ui/setup/index.html`, the `<head>` (lines 3-8) currently only links the external stylesheet. Since the static page can't reach Tailwind, add a small inline style block for the brand row directly after the existing `<link>`:

```html
  <link rel="stylesheet" href="/static/app.css">
  <style>
    .brand-row { display: flex; align-items: center; justify-content: center; gap: 12px; margin-bottom: 8px; }
    .brand-row img { width: 40px; height: 40px; }
  </style>
```

- [ ] **Step 2: Add the icon to the admin view**

Find the admin view's `<h1>` at line 12:

```html
      <h1 class="card-title">Welcome to Nexorious</h1>
```

Replace with:

```html
      <div class="brand-row">
        <img src="/logo.svg" alt="">
        <h1 class="card-title">Welcome to Nexorious</h1>
      </div>
```

- [ ] **Step 3: Add the icon to the restore view**

Find the restore view's `<h1>` at line 36:

```html
      <h1 class="card-title">Restore from Backup</h1>
```

Replace with:

```html
      <div class="brand-row">
        <img src="/logo.svg" alt="">
        <h1 class="card-title">Restore from Backup</h1>
      </div>
```

- [ ] **Step 4: Verify the setup page is served correctly**

Build and check the embedded file:

```bash
make build
strings nexorious | grep -o 'brand-row' | head -1
```

Expected: `brand-row` appears, confirming the modified HTML is embedded in the binary.

- [ ] **Step 5: Commit**

```bash
git add ui/setup/index.html
git commit -m "feat(branding): show icon next to title on setup page"
```

---

## Task 10: Add icon to migrate HTML

**Files:**
- Modify: `ui/migrate/index.html` (`<head>` and `<h1>` at line 11)

- [ ] **Step 1: Add the brand-row style**

In `ui/migrate/index.html`, after the existing `<link>` in `<head>`, add:

```html
  <link rel="stylesheet" href="/static/app.css">
  <style>
    .brand-row { display: flex; align-items: center; justify-content: center; gap: 12px; margin-bottom: 8px; }
    .brand-row img { width: 40px; height: 40px; }
  </style>
```

- [ ] **Step 2: Wrap the title with the brand row**

Find the title at line 11:

```html
    <h1 class="card-title">Database Migration Required</h1>
```

Replace with:

```html
    <div class="brand-row">
      <img src="/logo.svg" alt="">
      <h1 class="card-title">Database Migration Required</h1>
    </div>
```

- [ ] **Step 3: Verify the binary still builds**

```bash
make build
```

Expected: builds cleanly. No further checks needed — the file is embedded via existing `//go:embed all:migrate`.

- [ ] **Step 4: Commit**

```bash
git add ui/migrate/index.html
git commit -m "feat(branding): show icon next to title on migrate page"
```

---

## Task 11: Add Helm chart icon: field

**Files:**
- Modify: `deploy/helm/Chart.yaml`

- [ ] **Step 1: Add the icon: field**

In `deploy/helm/Chart.yaml`, add the `icon:` field directly under `home:` and `sources:`. After the existing block, the file should contain:

```yaml
home: https://github.com/drzero42/nexorious
sources:
  - https://github.com/drzero42/nexorious
icon: https://raw.githubusercontent.com/drzero42/nexorious/main/deploy/helm/icon.png
maintainers:
  - name: Nexorious
```

(The `icon:` field is placed between `sources:` and `maintainers:` for readability — Helm doesn't enforce field order, but this groups it with `home:` and `sources:`.)

- [ ] **Step 2: Lint the chart**

```bash
helm lint deploy/helm/
```

Expected: `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/Chart.yaml
git commit -m "feat(helm): add icon: field referencing brand asset"
```

---

## Task 12: Final verification

**Files:** none

- [ ] **Step 1: Run all quality gates**

From repo root:

```bash
make                                    # frontend + Go binary build
golangci-lint run                       # Go lint
go test -timeout 600s ./...             # Go tests
helm lint deploy/helm/                  # chart lint
```

Each must complete with zero errors.

From `ui/frontend/`:

```bash
npm run check && npm run knip && npm run test
```

Each must complete with zero errors / zero findings.

- [ ] **Step 2: Launch and visually verify**

```bash
./nexorious
```

In a browser, with `$DATABASE_URL` set and migrations applied:

| Where | Expected |
|-------|----------|
| Browser tab favicon | Purple badge with white joystick (no constellation at this size) |
| http://localhost:8000/login | Icon (≈56 px) centered above "Nexorious" title, with constellation visible |
| http://localhost:8000/games (signed in, desktop ≥ md) | Icon (≈32 px) next to "Nexorious" in the left sidebar |
| http://localhost:8000/games (signed in, mobile/narrow) | Icon next to "Nexorious" in the top header and inside the Sheet menu |

Drop the DB (`task db:reset` from devenv shell, then restart `./nexorious`) and verify:

| Where | Expected |
|-------|----------|
| http://localhost:8000/migrate | Icon next to "Database Migration Required" title |

After running migrations, drop users (delete row in `users` table) to land on setup:

| Where | Expected |
|-------|----------|
| http://localhost:8000/ (no admin yet) | Icon next to "Welcome to Nexorious" title |

- [ ] **Step 3: Verify the Helm icon URL is reachable once pushed**

After pushing the branch (or after merging the PR to main, since the URL points at the `main` branch):

```bash
curl -I https://raw.githubusercontent.com/drzero42/nexorious/main/deploy/helm/icon.png
```

Expected: `HTTP/2 200`. (Before merge to main, this returns 404 — that's the known race documented in the spec.)

- [ ] **Step 4: No commit**

This task only verifies. No new commits.

---

## Self-review notes

- **Spec coverage:** all asset files (logo.svg, favicon.svg, favicon.ico, apple-touch-icon.png, deploy/helm/icon.png) → Tasks 2, 3. index.html wiring → Task 4. Sidebar/mobile-nav/login/setup/migrate placements → Tasks 6, 7, 8, 9, 10. `--brand-purple` CSS variable → Task 5. Helm `icon:` → Task 11. PNG generation method (rsvg-convert + ImageMagick) → Task 3 (and tools added in Task 1). Final smoke test → Task 12. Spec "Risks" section's Helm-icon-URL race is explicitly called out in Task 12 Step 3.
- **Deviation from spec:** the spec described an `assets/README.md` recording regeneration commands; this plan creates `scripts/regen-icons.sh` instead — strictly more actionable, same intent (documented, one-shot, not auto-built into the Makefile). Devenv was also extended with `librsvg` and `imagemagick` to make the script runnable; the spec implied these tools were available but devenv.nix didn't actually include them.
- **PWA manifest, theme repaint, marketing art:** correctly remain out of scope; not in any task.
