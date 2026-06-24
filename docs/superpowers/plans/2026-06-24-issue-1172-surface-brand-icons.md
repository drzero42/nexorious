# Surface platform & storefront icons across the UI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface platform & storefront icons in game-detail rows, the add/edit selectors, and the filter facets, behind one shared theme-aware `BrandIcon` primitive.

**Architecture:** Extract the inner render of today's `PlatformIcon` into a brand-neutral `BrandIcon` (theme-aware variant swap + onError fallback + first-letter badge). `PlatformIcon`/`PlatformIconList` rewrap it with their existing API; new `StorefrontIcon`/`StorefrontIconList` mirror them. Consume these in the four text-only surfaces. Frontend-only — the data already carries `icon_url`.

**Tech Stack:** React 19 + TypeScript, Vite, Tailwind, shadcn/ui (Popover + Command combobox), `next-themes` (`useTheme().resolvedTheme`), Vitest + @testing-library/react.

**Spec:** `docs/superpowers/specs/2026-06-24-issue-1172-surface-brand-icons-design.md`

## Global Constraints

- Frontend only. **No backend, type, or API changes** — `Platform.icon_url`, `Storefront.icon_url` (`src/types/platform.ts`) and `storefront_details?: Storefront` (`src/types/game.ts`) already exist.
- All work in `ui/frontend/`. Run frontend commands from `ui/frontend/` (`npm run check`, `npm run test`, `npm run build`).
- Icon asset URL = `config.staticUrl` + the bare stored path (e.g. `/logos/storefronts/steam/steam-icon-light.svg`). **`BrandIcon` owns this prefix**; callers pass the bare `icon_url`.
- Dark variant = same path with the `-icon-light.svg` token replaced by `-icon-dark.svg`. Swap **only** when `resolvedTheme === 'dark'` **and** the path contains `-icon-light.svg`.
- `resolvedTheme` may be `undefined` (pre-hydration); treat anything other than `'dark'` as light (no swap).
- Missing/failed icon → existing first-letter badge (`displayName.charAt(0)` in a `text-muted-foreground` span).
- Preserve existing public APIs of `PlatformIcon` / `PlatformIconList`.
- Tests mock `next-themes` the same way `theme-sync.test.tsx` does: `vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: ... }) }))`.
- Conventional Commits; the PR title is the release line. This is a `feat:`.

## File Structure

| File | Responsibility |
|---|---|
| `src/components/ui/brand-icon.tsx` (new) | Brand-neutral `BrandIcon` primitive: staticUrl prefix, theme swap, onError fallback, badge, label/tooltip. |
| `src/components/ui/brand-icon.test.tsx` (new) | Unit tests for `BrandIcon`. |
| `src/components/ui/platform-icon.tsx` (modify) | `PlatformIcon`/`PlatformIconList` rewrap `BrandIcon`; add `StorefrontIcon`/`StorefrontIconList`. |
| `src/components/ui/platform-icon.test.tsx` (new) | Tests for the storefront wrappers (+ platform parity smoke). |
| `src/components/storefront-link.tsx` (modify) | `StorefrontLabel` gains a leading `StorefrontIcon`, drops the `(parens)`, keeps the store-URL link. |
| `src/components/storefront-link.test.tsx` (modify) | Update expectations for the new icon + de-parenthesised label. |
| `src/routes/_authenticated/games/$id.index.tsx` (modify) | Platform row: leading `PlatformIcon` before the platform name. |
| `src/components/ui/platform-selector.tsx` (modify) | `StorefrontSelector` → combobox w/ icons; platform picker uses real icons (keep `Monitor` as empty-state only). |
| `src/components/ui/platform-selector.test.tsx` (modify) | Add storefront-combobox + platform-icon assertions. |
| `src/components/ui/multi-select-filter.tsx` (modify) | `MultiSelectOption` gains optional `icon?: React.ReactNode`, rendered before the label. |
| `src/components/ui/multi-select-filter.test.tsx` (modify) | Add an icon-rendering test. |
| `src/components/games/game-filters.tsx` (modify) | Attach `icon` to `platformOptions` / `storefrontOptions`. |

---

### Task 1: `BrandIcon` primitive

**Files:**
- Create: `src/components/ui/brand-icon.tsx`
- Test: `src/components/ui/brand-icon.test.tsx`

**Interfaces:**
- Consumes: `config.staticUrl` (`@/lib/env`); `useTheme` (`next-themes`); `Tooltip*` (`@/components/ui/tooltip`); `cn` (`@/lib/utils`).
- Produces:
  ```ts
  export interface BrandIconProps {
    iconUrl?: string;        // bare stored path, e.g. "/logos/platforms/pc-windows/pc-windows-icon-light.svg"
    displayName: string;
    size?: 'sm' | 'md' | 'lg';
    showTooltip?: boolean;
    showLabel?: boolean;
    className?: string;
  }
  export function BrandIcon(props: BrandIconProps): React.JSX.Element;
  export const brandIconSizeClasses: Record<'sm' | 'md' | 'lg', string>;
  ```

- [ ] **Step 1: Write the failing tests**

Create `src/components/ui/brand-icon.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { BrandIcon } from './brand-icon';

let mockResolvedTheme: string | undefined = 'light';
vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: mockResolvedTheme }),
}));

describe('BrandIcon', () => {
  beforeEach(() => {
    mockResolvedTheme = 'light';
  });

  it('renders the image when iconUrl is present', () => {
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    const img = screen.getByRole('img', { name: 'PC' });
    expect(img).toHaveAttribute('src', '/logos/platforms/pc/pc-icon-light.svg');
  });

  it('renders a first-letter badge when iconUrl is absent', () => {
    render(<BrandIcon displayName="Steam" />);
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('S')).toBeInTheDocument();
  });

  it('selects the dark variant under a dark resolvedTheme', () => {
    mockResolvedTheme = 'dark';
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    expect(screen.getByRole('img', { name: 'PC' })).toHaveAttribute(
      'src',
      '/logos/platforms/pc/pc-icon-dark.svg',
    );
  });

  it('does not swap when the path has no -icon-light.svg token', () => {
    mockResolvedTheme = 'dark';
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc.png" displayName="PC" />);
    expect(screen.getByRole('img', { name: 'PC' })).toHaveAttribute(
      'src',
      '/logos/platforms/pc/pc.png',
    );
  });

  it('falls back to the stored (light) asset when the dark variant errors', () => {
    mockResolvedTheme = 'dark';
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    const img = screen.getByRole('img', { name: 'PC' });
    expect(img).toHaveAttribute('src', '/logos/platforms/pc/pc-icon-dark.svg');
    fireEvent.error(img);
    expect(screen.getByRole('img', { name: 'PC' })).toHaveAttribute(
      'src',
      '/logos/platforms/pc/pc-icon-light.svg',
    );
  });

  it('falls back to the first-letter badge when the image fails with no further variant', () => {
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    fireEvent.error(screen.getByRole('img', { name: 'PC' }));
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('P')).toBeInTheDocument();
  });

  it('renders the label inline when showLabel is set', () => {
    render(<BrandIcon iconUrl="/logos/x/x-icon-light.svg" displayName="Epic" showLabel />);
    expect(screen.getByText('Epic')).toBeInTheDocument();
  });
});
```

> Note: `config.staticUrl` is `''` under Vitest (`VITE_STATIC_URL` unset, see `src/lib/env.ts`), so expected `src` values have no prefix.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run test brand-icon`
Expected: FAIL — `Cannot find module './brand-icon'`.

- [ ] **Step 3: Implement `BrandIcon`**

Create `src/components/ui/brand-icon.tsx`:

```tsx
import * as React from 'react';
import { useTheme } from 'next-themes';
import { config } from '@/lib/env';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

export interface BrandIconProps {
  /** Bare stored icon path (e.g. "/logos/platforms/pc/pc-icon-light.svg"); staticUrl is prefixed here. */
  iconUrl?: string;
  displayName: string;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  className?: string;
}

export const brandIconSizeClasses: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
  lg: 'h-6 w-6',
};

const LIGHT_TOKEN = '-icon-light.svg';
const DARK_TOKEN = '-icon-dark.svg';

export function BrandIcon({
  iconUrl,
  displayName,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  className,
}: BrandIconProps) {
  const { resolvedTheme } = useTheme();

  const basePath = iconUrl ? `${config.staticUrl}${iconUrl}` : null;
  const themedPath =
    basePath && resolvedTheme === 'dark' && basePath.includes(LIGHT_TOKEN)
      ? basePath.replace(LIGHT_TOKEN, DARK_TOKEN)
      : basePath;

  const [src, setSrc] = React.useState<string | null>(themedPath);
  const [failed, setFailed] = React.useState(false);

  // Reset when the resolved/themed path changes (theme toggle, new icon).
  React.useEffect(() => {
    setSrc(themedPath);
    setFailed(false);
  }, [themedPath]);

  const handleError = () => {
    if (src && basePath && src !== basePath) {
      // Dark variant missing — fall back to the stored (light) asset.
      setSrc(basePath);
    } else {
      // No usable image — show the badge.
      setFailed(true);
    }
  };

  const showImage = src != null && !failed;

  const icon = showImage ? (
    <img
      src={src}
      alt={displayName}
      width={24}
      height={24}
      className={cn(brandIconSizeClasses[size], 'object-contain', className)}
      loading="lazy"
      onError={handleError}
    />
  ) : (
    <span className={cn('text-muted-foreground', brandIconSizeClasses[size], className)}>
      {displayName.charAt(0)}
    </span>
  );

  const content = showLabel ? (
    <span className="inline-flex items-center gap-1.5">
      {icon}
      <span className="text-sm text-muted-foreground">{displayName}</span>
    </span>
  ) : (
    icon
  );

  if (showTooltip && !showLabel) {
    return (
      <TooltipProvider delayDuration={300}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex">{content}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{displayName}</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return content;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run test brand-icon`
Expected: PASS (7 tests).

- [ ] **Step 5: Typecheck**

Run: `npm run check`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add src/components/ui/brand-icon.tsx src/components/ui/brand-icon.test.tsx
git commit -m "feat: add theme-aware BrandIcon primitive"
```

---

### Task 2: Rewrap `PlatformIcon` and add storefront wrappers

**Files:**
- Modify: `src/components/ui/platform-icon.tsx`
- Test: `src/components/ui/platform-icon.test.tsx` (new)

**Interfaces:**
- Consumes: `BrandIcon`, `brandIconSizeClasses` from Task 1; `Platform`, `Storefront` (`@/types`).
- Produces:
  ```ts
  function PlatformIcon(props): JSX.Element;          // unchanged public API
  export function PlatformIconList(props: PlatformIconListProps): JSX.Element;  // unchanged
  export function StorefrontIcon(props: {
    storefront: Storefront; size?: 'sm'|'md'|'lg'; showTooltip?: boolean; showLabel?: boolean; className?: string;
  }): JSX.Element;
  export interface StorefrontIconListProps {
    storefronts: Array<{ storefront_details?: Storefront; storefront?: string }>;
    size?: 'sm' | 'md' | 'lg'; showTooltips?: boolean; showLabels?: boolean; className?: string;
  }
  export function StorefrontIconList(props: StorefrontIconListProps): JSX.Element;
  ```

- [ ] **Step 1: Write the failing tests**

Create `src/components/ui/platform-icon.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StorefrontIcon, StorefrontIconList } from './platform-icon';
import type { Storefront } from '@/types';

vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: 'light' }) }));

const steam: Storefront = {
  name: 'steam',
  display_name: 'Steam',
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  icon_url: '/logos/storefronts/steam/steam-icon-light.svg',
};

describe('StorefrontIcon', () => {
  it('renders the storefront image when icon_url is present', () => {
    render(<StorefrontIcon storefront={steam} />);
    expect(screen.getByRole('img', { name: 'Steam' })).toHaveAttribute(
      'src',
      '/logos/storefronts/steam/steam-icon-light.svg',
    );
  });

  it('renders a first-letter badge when icon_url is absent', () => {
    render(<StorefrontIcon storefront={{ ...steam, icon_url: undefined }} />);
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('S')).toBeInTheDocument();
  });
});

describe('StorefrontIconList', () => {
  it('renders an icon per entry with storefront_details and a dash when empty', () => {
    const { rerender } = render(<StorefrontIconList storefronts={[{ storefront_details: steam }]} />);
    expect(screen.getByRole('img', { name: 'Steam' })).toBeInTheDocument();

    rerender(<StorefrontIconList storefronts={[{ storefront: 'steam' }]} />);
    expect(screen.getByText('-')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run test platform-icon`
Expected: FAIL — `StorefrontIcon`/`StorefrontIconList` are not exported.

- [ ] **Step 3: Rewrap `PlatformIcon` on `BrandIcon` and add the storefront wrappers**

Replace the body of `src/components/ui/platform-icon.tsx` with:

```tsx
import { BrandIcon } from '@/components/ui/brand-icon';
import { cn } from '@/lib/utils';
import type { Platform, Storefront } from '@/types';

interface PlatformIconProps {
  platform: Platform;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  className?: string;
}

function PlatformIcon({
  platform,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  className,
}: PlatformIconProps) {
  return (
    <BrandIcon
      iconUrl={platform.icon_url}
      displayName={platform.display_name}
      size={size}
      showTooltip={showTooltip}
      showLabel={showLabel}
      className={className}
    />
  );
}

export interface PlatformIconListProps {
  platforms: Array<{ platform_details?: Platform; platform?: string }>;
  size?: 'sm' | 'md' | 'lg';
  showTooltips?: boolean;
  showLabels?: boolean;
  className?: string;
}

export function PlatformIconList({
  platforms,
  size = 'md',
  showTooltips = false,
  showLabels = false,
  className,
}: PlatformIconListProps) {
  const validPlatforms = platforms.filter((p) => p.platform_details);

  if (validPlatforms.length === 0) {
    return <span className="text-sm text-muted-foreground">-</span>;
  }

  return (
    <span className={cn('inline-flex items-center gap-1.5 flex-wrap', className)}>
      {validPlatforms.map((p, index) => (
        <PlatformIcon
          key={p.platform_details!.name + index}
          platform={p.platform_details!}
          size={size}
          showTooltip={showTooltips}
          showLabel={showLabels}
        />
      ))}
    </span>
  );
}

interface StorefrontIconProps {
  storefront: Storefront;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  className?: string;
}

export function StorefrontIcon({
  storefront,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  className,
}: StorefrontIconProps) {
  return (
    <BrandIcon
      iconUrl={storefront.icon_url}
      displayName={storefront.display_name}
      size={size}
      showTooltip={showTooltip}
      showLabel={showLabel}
      className={className}
    />
  );
}

export interface StorefrontIconListProps {
  storefronts: Array<{ storefront_details?: Storefront; storefront?: string }>;
  size?: 'sm' | 'md' | 'lg';
  showTooltips?: boolean;
  showLabels?: boolean;
  className?: string;
}

export function StorefrontIconList({
  storefronts,
  size = 'md',
  showTooltips = false,
  showLabels = false,
  className,
}: StorefrontIconListProps) {
  const valid = storefronts.filter((s) => s.storefront_details);

  if (valid.length === 0) {
    return <span className="text-sm text-muted-foreground">-</span>;
  }

  return (
    <span className={cn('inline-flex items-center gap-1.5 flex-wrap', className)}>
      {valid.map((s, index) => (
        <StorefrontIcon
          key={s.storefront_details!.name + index}
          storefront={s.storefront_details!}
          size={size}
          showTooltip={showTooltips}
          showLabel={showLabels}
        />
      ))}
    </span>
  );
}

export { PlatformIcon };
```

> `PlatformIcon` keeps being exported exactly as before (it was a bare `function` + the `export {}` was implicit via `PlatformIconList`? No — re-check the original). The original file exported `PlatformIconList` and `PlatformIconListProps`; `PlatformIcon` itself was **not** exported (only used internally + by the list). Verify with `grep -n "export" src/components/ui/platform-icon.tsx` **before** editing, and preserve exactly the original export surface for `PlatformIcon` (add `export` only if the original had it). Keep `game-card.tsx` / `game-list.tsx` imports working unchanged.

- [ ] **Step 4: Run the storefront tests + the existing platform consumers**

Run: `npm run test platform-icon game-card game-list`
Expected: PASS (new storefront tests + unchanged platform list rendering).

- [ ] **Step 5: Typecheck + dead-code/knip**

Run: `npm run check && npm run knip`
Expected: no errors, no new knip findings (the new exports are consumed in later tasks; if knip flags them now, that's expected until Task 3–5 land — note it and continue, or sequence so the final knip run is clean).

- [ ] **Step 6: Commit**

```bash
git add src/components/ui/platform-icon.tsx src/components/ui/platform-icon.test.tsx
git commit -m "feat: rewrap PlatformIcon on BrandIcon and add StorefrontIcon wrappers"
```

---

### Task 3: Game-detail "Platforms & Ownership" rows

**Files:**
- Modify: `src/components/storefront-link.tsx`
- Modify: `src/components/storefront-link.test.tsx`
- Modify: `src/routes/_authenticated/games/$id.index.tsx:483-516`

**Interfaces:**
- Consumes: `StorefrontIcon`, `PlatformIcon` (Task 2); `Storefront` (`@/types`).
- Produces: updated `StorefrontLabel` signature:
  ```ts
  export function StorefrontLabel(props: {
    storefront: Storefront;   // carries display_name + icon_url
    storeUrl?: string;
  }): React.JSX.Element;
  ```

- [ ] **Step 1: Update the `StorefrontLabel` tests (red)**

Replace `src/components/storefront-link.test.tsx` with:

```tsx
import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { StorefrontLabel } from './storefront-link';
import type { Storefront } from '@/types';

vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: 'light' }) }));

const steam: Storefront = {
  name: 'steam',
  display_name: 'Steam',
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  icon_url: '/logos/storefronts/steam/steam-icon-light.svg',
};

describe('StorefrontLabel', () => {
  it('renders a new-tab link (with icon) when store_url is present', () => {
    render(
      <StorefrontLabel storefront={steam} storeUrl="https://store.steampowered.com/app/440/" />,
    );
    const link = screen.getByRole('link', { name: /steam/i });
    expect(link).toHaveAttribute('href', 'https://store.steampowered.com/app/440/');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    expect(screen.getByRole('img', { name: 'Steam' })).toBeInTheDocument();
  });

  it('renders a plain label (no parens) when store_url is absent', () => {
    render(<StorefrontLabel storefront={{ ...steam, display_name: 'Humble' }} />);
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('Humble')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `npm run test storefront-link`
Expected: FAIL — `StorefrontLabel` still takes `displayName`, no icon rendered.

- [ ] **Step 3: Rewrite `StorefrontLabel`**

Replace `src/components/storefront-link.tsx` with:

```tsx
import { StorefrontIcon } from '@/components/ui/platform-icon';
import type { Storefront } from '@/types';

interface StorefrontLabelProps {
  storefront: Storefront;
  storeUrl?: string;
}

export function StorefrontLabel({ storefront, storeUrl }: StorefrontLabelProps) {
  const inner = (
    <span className="inline-flex items-center gap-1">
      <StorefrontIcon storefront={storefront} size="sm" />
      {storefront.display_name}
    </span>
  );

  if (storeUrl) {
    return (
      <a
        href={storeUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center text-sm text-muted-foreground underline-offset-2 hover:underline"
      >
        {inner}
      </a>
    );
  }
  return <span className="text-sm text-muted-foreground">{inner}</span>;
}
```

- [ ] **Step 4: Update the game-detail row call site**

In `src/routes/_authenticated/games/$id.index.tsx`, the platform row (around L483-516). Add a leading platform icon and update the `StorefrontLabel` props. Import `PlatformIcon`:

At the top, alongside the existing `StorefrontLabel` import (L41), add:

```tsx
import { PlatformIcon } from '@/components/ui/platform-icon';
```

> Confirm `PlatformIcon` is exported (Task 2 Step 3). If it is not exported in the original surface, export it in Task 2 instead of widening here.

Change the left-hand `<div className="flex items-center gap-2">` block from:

```tsx
<div className="flex items-center gap-2">
  <span className="font-medium">
    {p.platform_details?.display_name || p.platform || 'Unknown'}
  </span>
  {p.storefront_details && (
    <StorefrontLabel
      displayName={p.storefront_details.display_name}
      storeUrl={p.store_url}
    />
  )}
</div>
```

to:

```tsx
<div className="flex items-center gap-2">
  <span className="inline-flex items-center gap-1.5 font-medium">
    {p.platform_details && <PlatformIcon platform={p.platform_details} size="sm" />}
    {p.platform_details?.display_name || p.platform || 'Unknown'}
  </span>
  {p.storefront_details && (
    <StorefrontLabel storefront={p.storefront_details} storeUrl={p.store_url} />
  )}
</div>
```

- [ ] **Step 5: Run the tests**

Run: `npm run test storefront-link`
Expected: PASS.

- [ ] **Step 6: Typecheck**

Run: `npm run check`
Expected: no errors (the old `displayName` prop is gone; the only call site is updated above).

- [ ] **Step 7: Commit**

```bash
git add src/components/storefront-link.tsx src/components/storefront-link.test.tsx src/routes/_authenticated/games/\$id.index.tsx
git commit -m "feat: show platform & storefront icons in game-detail ownership rows"
```

---

### Task 4: Selectors — storefront combobox + real platform icons

**Files:**
- Modify: `src/components/ui/platform-selector.tsx`
- Modify: `src/components/ui/platform-selector.test.tsx`

**Interfaces:**
- Consumes: `PlatformIcon`, `StorefrontIcon` (Task 2); existing `Command*`/`Popover*`/`Button` imports already present in the file.
- Produces: no exported-API change — `StorefrontSelector` stays internal with the same props; visual/markup change only.

- [ ] **Step 1: Add failing selector tests**

Append to `src/components/ui/platform-selector.test.tsx` (inside the existing top-level `describe`, or a new `describe`). First add a `next-themes` mock at the top of the file if not present:

```tsx
vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: 'light' }) }));
```

Then add (note: storefronts in `mockStorefronts` need an `icon_url` to assert an image — extend one entry, e.g. add `icon_url: '/logos/storefronts/steam/steam-icon-light.svg'` to the Steam entry):

```tsx
describe('StorefrontSelector (via PlatformSelector row editor)', () => {
  it('renders a searchable combobox and selects a storefront with an icon', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <PlatformSelector
        selectedPlatforms={[sel('pc-windows')]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );
    // The storefront trigger is a combobox button, not a native <select>.
    const triggers = screen.getAllByRole('combobox');
    // Open the storefront combobox (the row's second combobox) and pick Steam.
    await user.click(triggers[triggers.length - 1]);
    await user.click(screen.getByText('Steam'));
    expect(handleChange).toHaveBeenCalled();
  });
});
```

> Adjust `mockPlatforms`/`sel(...)` to a platform in the existing fixtures that has Steam as a storefront. Read the current fixtures first and reuse them; do not invent platform names (FK-style seeded names like `pc-windows`/`steam` are what the file already uses).

- [ ] **Step 2: Run to verify failure**

Run: `npm run test platform-selector`
Expected: FAIL — current `StorefrontSelector` renders a native `<select>` (`getAllByRole('combobox')` count / interaction differs), no icon.

- [ ] **Step 3: Convert `StorefrontSelector` to a combobox with icons**

In `src/components/ui/platform-selector.tsx`, replace the `StorefrontSelector` implementation (L77-106) with a Popover+Command combobox mirroring `PlatformRowEditor`'s pattern. Remove the now-unused `Select*` imports if nothing else uses them (check first — they may be unused after this change; run knip):

```tsx
import { PlatformIcon, StorefrontIcon } from '@/components/ui/platform-icon';
// ...

function StorefrontSelector({
  storefronts,
  selectedStorefront,
  onStorefrontChange,
  allowNone = true,
  disabled = false,
}: StorefrontSelectorProps) {
  const [open, setOpen] = React.useState(false);
  const showNone = allowNone || selectedStorefront == null;
  const selected = storefronts.find((s) => s.name === selectedStorefront);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={selected ? `Change storefront: ${selected.display_name}` : 'Select storefront'}
          disabled={disabled}
          className={cn('h-8 w-full justify-between text-xs', !selected && 'text-muted-foreground')}
        >
          <span className="flex items-center gap-1.5 min-w-0">
            {selected && <StorefrontIcon storefront={selected} size="sm" />}
            <span className="truncate">{selected?.display_name ?? 'Select storefront'}</span>
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[220px] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search storefronts..." />
          <CommandList>
            <CommandEmpty>No storefronts found</CommandEmpty>
            <CommandGroup>
              {showNone && (
                <CommandItem
                  value="No storefront"
                  onSelect={() => {
                    onStorefrontChange(undefined);
                    setOpen(false);
                  }}
                  className="cursor-pointer"
                >
                  <Check
                    className={cn('mr-2 h-4 w-4', selectedStorefront == null ? 'opacity-100' : 'opacity-0')}
                  />
                  No storefront
                </CommandItem>
              )}
              {storefronts.map((storefront) => {
                const isCurrent = storefront.name === selectedStorefront;
                return (
                  <CommandItem
                    key={storefront.name}
                    value={storefront.display_name}
                    onSelect={() => {
                      onStorefrontChange(storefront.name);
                      setOpen(false);
                    }}
                    className="cursor-pointer"
                  >
                    <Check className={cn('mr-2 h-4 w-4', isCurrent ? 'opacity-100' : 'opacity-0')} />
                    <StorefrontIcon storefront={storefront} size="sm" className="mr-2" />
                    <span className="truncate">{storefront.display_name}</span>
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
```

> Use the default `Command` filtering (search by the `value` = `display_name`). `Check`/`ChevronsUpDown` are already imported (L2); ensure `React` is imported (it is, L1).

- [ ] **Step 4: Swap `Monitor` for real platform icons**

In the same file, replace the hardcoded `Monitor` glyphs with `PlatformIcon`, keeping `Monitor` only as the empty-state placeholder:

- **Trigger (L182):** when a platform is selected, render `<PlatformIcon platform={platform} size="sm" />`; when none, keep `<Monitor className="h-4 w-4 shrink-0" />`:
  ```tsx
  <span className="flex items-center gap-2 min-w-0">
    {platform ? (
      <PlatformIcon platform={platform} size="sm" />
    ) : (
      <Monitor className="h-4 w-4 shrink-0" />
    )}
    <span className="truncate">{platform?.display_name ?? placeholder}</span>
  </span>
  ```
- **Command item (L217):** replace `<Monitor className="mr-2 h-4 w-4 text-muted-foreground" />` with `<PlatformIcon platform={p} size="sm" className="mr-2" />`.
- **Compact checkbox row (L432):** replace `<Monitor className="h-4 w-4 text-muted-foreground flex-shrink-0" />` with `<PlatformIcon platform={platform} size="sm" className="flex-shrink-0" />`.
- **Empty-state (L399):** leave the large `<Monitor className="w-12 h-12 ..." />` as-is.

- [ ] **Step 5: Run the selector tests + typecheck + knip**

Run: `npm run test platform-selector && npm run check && npm run knip`
Expected: PASS; no type errors; knip clean (remove any now-unused `Select*`/`Monitor` import — `Monitor` is still used for the empty state, so it stays).

- [ ] **Step 6: Commit**

```bash
git add src/components/ui/platform-selector.tsx src/components/ui/platform-selector.test.tsx
git commit -m "feat: icon-aware storefront combobox and platform picker in selectors"
```

---

### Task 5: Filter facets — leading icons

**Files:**
- Modify: `src/components/ui/multi-select-filter.tsx`
- Modify: `src/components/ui/multi-select-filter.test.tsx`
- Modify: `src/components/games/game-filters.tsx`

**Interfaces:**
- Consumes: `PlatformIcon`, `StorefrontIcon` (Task 2).
- Produces:
  ```ts
  export interface MultiSelectOption {
    value: string;
    label: string;
    icon?: React.ReactNode;   // new — rendered before the label when present
  }
  ```

- [ ] **Step 1: Add a failing facet-icon test**

Append to `src/components/ui/multi-select-filter.test.tsx`:

```tsx
it('renders an option icon before its label when provided', async () => {
  const user = userEvent.setup();
  render(
    <MultiSelectFilter
      label="Platforms"
      options={[{ value: 'pc', label: 'PC', icon: <span data-testid="opt-icon" /> }]}
      selected={[]}
      onChange={vi.fn()}
    />,
  );
  await user.click(screen.getByRole('combobox'));
  expect(screen.getByTestId('opt-icon')).toBeInTheDocument();
});
```

- [ ] **Step 2: Run to verify failure**

Run: `npm run test multi-select-filter`
Expected: FAIL — `icon` is not a valid `MultiSelectOption` field / not rendered.

- [ ] **Step 3: Add the optional icon to `MultiSelectOption` + render it**

In `src/components/ui/multi-select-filter.tsx`:

Extend the interface:

```tsx
export interface MultiSelectOption {
  value: string;
  label: string;
  icon?: React.ReactNode;
}
```

In the option `<label>` row, render the icon before the label:

```tsx
<Checkbox checked={isSelected} onCheckedChange={() => handleToggle(option.value)} />
{option.icon}
<span className="text-sm">{option.label}</span>
```

- [ ] **Step 4: Wire icons into `game-filters.tsx`**

In `src/components/games/game-filters.tsx`, import the icons:

```tsx
import { PlatformIcon, StorefrontIcon } from '@/components/ui/platform-icon';
```

Change `platformOptions` / `storefrontOptions` (L73-81) to attach an icon (the `useAllPlatforms`/`useAllStorefronts` data are full `Platform`/`Storefront` objects carrying `icon_url`). The synthetic `'unknown'` option gets no icon:

```tsx
const platformOptions = [
  ...(platforms?.map((p) => ({
    value: p.name,
    label: p.display_name,
    icon: <PlatformIcon platform={p} size="sm" />,
  })) ?? []),
  { value: 'unknown', label: 'Unknown' },
].sort((a, b) => a.label.localeCompare(b.label));

const storefrontOptions = [
  ...(storefronts?.map((s) => ({
    value: s.name,
    label: s.display_name,
    icon: <StorefrontIcon storefront={s} size="sm" />,
  })) ?? []),
  { value: 'unknown', label: 'Unknown' },
].sort((a, b) => a.label.localeCompare(b.label));
```

> The `.sort()` comparator only reads `a.label`/`b.label`, so adding `icon` is type-compatible. If TS infers a union that rejects the icon-less `'unknown'` entry, annotate the arrays as `MultiSelectOption[]` (import the type from `@/components/ui/multi-select-filter`).

- [ ] **Step 5: Run the facet + filters tests, typecheck, knip**

Run: `npm run test multi-select-filter game-filters && npm run check && npm run knip`
Expected: PASS; no type errors; knip clean.

- [ ] **Step 6: Commit**

```bash
git add src/components/ui/multi-select-filter.tsx src/components/ui/multi-select-filter.test.tsx src/components/games/game-filters.tsx
git commit -m "feat: show platform & storefront icons in filter facets"
```

---

### Task 6: Full verification + route regen check

**Files:** none (verification only).

- [ ] **Step 1: Run the full frontend gate**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all green; zero knip findings. If knip reports an unused export from Task 2 (e.g. `StorefrontIconList` if no consumer landed), either wire it where appropriate or drop it — do not leave dead exports.

- [ ] **Step 2: Build (regenerates routeTree if touched) + visual sanity**

Run: `npm run build`
Expected: builds clean. No route files were added/removed, so `routeTree.gen.ts` should not change; if it does, commit it.

- [ ] **Step 3: Final commit if anything changed**

```bash
git add -A && git commit -m "chore: verify icon surfacing across UI" || echo "nothing to commit"
```

---

## Self-Review

**Spec coverage:**
- `BrandIcon` (theme swap, onError, badge, label/tooltip) → Task 1. ✓
- `PlatformIcon`/`PlatformIconList` rewrap + `StorefrontIcon`/`StorefrontIconList` → Task 2. ✓
- Game-detail rows (platform + storefront icon, store-URL link preserved) → Task 3. ✓
- Storefront combobox + platform real icons (row editor + compact wizard) → Task 4. ✓
- Filter facets (Platforms + Storefronts leading icons) → Task 5. ✓
- Testing notes (BrandIcon present/fallback/theme/onError; storefront parity; detail rows; combobox; facet icon) → Tasks 1,2,3,4,5. ✓
- Out-of-scope (cards/list/stats/sync untouched) → no task touches them. ✓

**Placeholder scan:** No TBD/"add error handling"/"similar to Task N"; every code step shows full code. One verify-before-edit note in Task 2/3 about `PlatformIcon`'s export surface (intentional — the original must be checked, not guessed).

**Type consistency:** `BrandIconProps` (Task 1) is consumed with matching prop names in Tasks 2; `StorefrontLabel({storefront, storeUrl})` (Task 3) matches its sole call site; `MultiSelectOption.icon?` (Task 5) matches the `game-filters.tsx` wiring. `StorefrontIcon`/`StorefrontIconList`/`PlatformIcon` signatures are defined once (Task 2) and used consistently downstream.

**Open verify-point carried into execution:** whether the original `platform-icon.tsx` exports `PlatformIcon` — Task 2/3 instruct verifying and preserving/adding the export rather than assuming.
