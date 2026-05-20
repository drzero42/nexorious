# Brand icon design

## Summary

Nexorious has no brand mark today: [ui/frontend/index.html:5](ui/frontend/index.html#L5) references a `/favicon.ico` that does not exist in [ui/frontend/public/](ui/frontend/public/), [sidebar.tsx:22](ui/frontend/src/components/navigation/sidebar.tsx#L22) shows only the wordmark "Nexorious", and [deploy/helm/Chart.yaml](deploy/helm/Chart.yaml) has no `icon:` field. This design adds an original brand mark — a stylized old-school ball-top joystick inside a rounded purple badge with a faint constellation background — packaged as SVG and PNG assets and wired into the existing Vite `public/` pipeline (auto-embedded by Go), the app shell, and the Helm chart.

The brand color is `#6d28d9` (Tailwind violet-700).

## Visual design

**Form:** rounded-square purple badge, centered white joystick (trapezoidal base + tapered shaft + ball top), 8 white dots scattered at low opacity behind the joystick.

**Master SVG** (source of truth, includes constellation):

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

**Elements:**

- 100×100 viewBox, 92×92 badge inset 4px on all sides, corner radius 20
- Badge fill: `#6d28d9`
- Joystick (white): trapezoidal base, vertical shaft, ball top
- Action button: `#6d28d9` cutout circle on the white base
- Constellation: 8 white dots @ 0.5 opacity, radii 1.2–1.8 px

**Favicon variant** drops the constellation (it rasterizes poorly below ~32px and the dots vanish anyway). Same badge + joystick geometry, no constellation `<g>`:

```svg
<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Nexorious">
  <rect x="4" y="4" width="92" height="92" rx="20" fill="#6d28d9"/>
  <path d="M 24 78 L 76 78 L 70 64 L 30 64 Z" fill="#ffffff"/>
  <rect x="46.5" y="30" width="7" height="34" fill="#ffffff"/>
  <circle cx="50" cy="26" r="12" fill="#ffffff"/>
  <circle cx="63" cy="72" r="3.5" fill="#6d28d9"/>
</svg>
```

## Asset files

All files are committed to the repo. PNGs are produced from the master SVG (see "PNG generation" below).

| Path | Size | Purpose |
|------|------|---------|
| [ui/frontend/public/logo.svg](ui/frontend/public/logo.svg) | viewBox 100×100 | Master SVG with constellation; used in the app UI |
| [ui/frontend/public/favicon.svg](ui/frontend/public/favicon.svg) | viewBox 100×100 | Modern-browser SVG favicon (no constellation) |
| [ui/frontend/public/favicon.ico](ui/frontend/public/favicon.ico) | 16/32/48 multi-size | Legacy browser favicon |
| [ui/frontend/public/apple-touch-icon.png](ui/frontend/public/apple-touch-icon.png) | 180×180 | iOS home-screen icon |
| [deploy/helm/icon.png](deploy/helm/icon.png) | 256×256 | Helm chart icon (referenced from Chart.yaml) |

Vite's `public/` contents copy as-is into `ui/frontend/dist/`, which Go embeds via `//go:embed all:frontend/dist` in [ui/ui.go](ui/ui.go). No build pipeline changes required.

## Wiring

### index.html

Update [ui/frontend/index.html](ui/frontend/index.html) `<head>`:

```html
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<link rel="icon" type="image/x-icon" href="/favicon.ico"><!-- legacy fallback -->
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
<meta name="theme-color" content="#6d28d9">
```

The SVG favicon takes precedence in modern browsers; the .ico stays as a fallback for older browsers and feeds (RSS readers, etc.).

### App sidebar

In [sidebar.tsx:20-24](ui/frontend/src/components/navigation/sidebar.tsx#L20-L24), replace the bare wordmark with an icon + wordmark:

```tsx
<Link to="/games" className="flex items-center gap-2">
  <img src="/logo.svg" alt="" className="h-8 w-8" />
  <h1 className="text-xl font-bold">Nexorious</h1>
</Link>
```

[mobile-nav.tsx](ui/frontend/src/components/navigation/mobile-nav.tsx) gets the same treatment in its header block.

### Login page

[login.tsx](ui/frontend/src/routes/_public/login.tsx) renders the wordmark on its own — add the icon above it for the login-screen brand moment.

### Setup and migrate pages

[ui/setup/index.html](ui/setup/index.html) and [ui/migrate/index.html](ui/migrate/index.html) are static templates served before the React app is ready. Add a small icon next to the page title in each card header (single `<img src="/logo.svg" alt="" width="32" height="32">` line). In scope — the brand needs to be consistent across all entry points, including the pages users see before the SPA boots.

## Brand color in CSS

Add to [globals.css](ui/frontend/src/styles/globals.css) `:root`:

```css
--brand-purple: #6d28d9;
```

The same `#6d28d9` value is used in both `:root` and `.dark` — the badge background is dark enough to read on light surfaces and light enough to read on dark surfaces. No dark-mode-specific override.

Components that want to use the brand color reference `var(--brand-purple)`. This is **additive**: the existing `--primary` (zinc) and shadcn theme tokens stay unchanged. This task is not a wholesale theme repaint.

## Helm chart

Add to [deploy/helm/Chart.yaml](deploy/helm/Chart.yaml):

```yaml
icon: https://raw.githubusercontent.com/drzero42/nexorious/main/deploy/helm/icon.png
```

ArtifactHub indexes Helm charts by dereferencing `icon:` over HTTP, so the URL must point to a stable raw file. Using the `main` branch URL means the icon updates with the file. Pinning to a release tag would require manual updates each release and is rejected on those grounds.

The PNG lives at [deploy/helm/icon.png](deploy/helm/icon.png) versioned alongside the chart.

## PNG generation

PNGs are produced once from the master SVG and committed. There's no Makefile target; an `assets/README.md` note records the regeneration command:

```bash
# from repo root, requires librsvg (rsvg-convert) or imagemagick
rsvg-convert -w 180 -h 180 ui/frontend/public/logo.svg -o ui/frontend/public/apple-touch-icon.png
rsvg-convert -w 256 -h 256 ui/frontend/public/logo.svg -o deploy/helm/icon.png
# favicon.ico needs imagemagick to multi-size:
convert -background none ui/frontend/public/favicon.svg \
  -define icon:auto-resize=16,32,48 \
  ui/frontend/public/favicon.ico
```

This is one-time setup; the icon is unlikely to change often. If it does change, the regeneration is two commands.

## Testing

- Visual smoke test: `make && ./nexorious`, confirm:
  - Browser tab shows the favicon
  - Sidebar icon renders alongside the wordmark
  - Login page shows the icon above the form
  - Setup and migrate pages show the icon
- Helm chart test: `helm lint deploy/helm/` passes; `curl -I` the icon URL after the asset is on `main` and confirm 200.

No automated tests are added — this is a static-asset change. The frontend test suite continues to pass.

## Out of scope

- PWA manifest (separate decision; can be added later using these same icon assets)
- Changing `--primary` or any existing shadcn theme tokens
- Per-platform / per-storefront logos (already exist in [ui/frontend/public/logos/](ui/frontend/public/logos/))
- README header art or marketing site assets
- Animated or interactive logo variants
- A light-mode badge variant (the badge is the same in light and dark; tested visually in both)

## Risks

- **Helm icon URL race**: ArtifactHub fetches `icon:` at chart-index time. If a chart is published before `deploy/helm/icon.png` lands on `main`, the icon is broken until the next index refresh. Mitigation: include the icon file in the same PR that adds the `icon:` field to Chart.yaml.
- **Favicon caching**: browsers aggressively cache favicons. After deployment, users may not see the new icon until they hard-refresh or restart the browser. Not blocking; just a known limitation.
