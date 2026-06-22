# Library Health page (web UI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a "Library Health" page in the React SPA that surfaces the 10 library-smell checks (grouped by tier), with per-game + apply-all one-click fixes for auto-fixable checks, deep-links for manual checks, and per-item dismiss/restore.

**Architecture:** A thin TanStack-Router route renders the page; a `useSmellSummary()` query drives a tier-grouped layout of `CheckSection` accordion items. Each section lazily fetches its flagged items on expand and renders a presentational `FlaggedItemsTable`. All server access goes through a new `api/library-health.ts` + `hooks/use-library-health.ts` layer. No backend changes — the REST API (`/api/library/smells`, from #1144) is consumed unchanged.

**Tech Stack:** React 19, TanStack Router (file-based), TanStack Query v5, shadcn/ui (Accordion, Table, Badge, AlertDialog, Card, Button, Skeleton), Tailwind v4, Vitest + Testing Library, Sonner toasts, lucide-react icons.

## Global Constraints

- **Frontend conventions (CLAUDE.md):** controlled Selects via `useState`, never RHF `watch()`; **no `setState` inside `useEffect`**; destructive confirm actions use `AlertDialogAction` with destructive styling.
- **Routing:** after adding the route, run `npm run build` to regenerate `ui/frontend/src/routeTree.gen.ts` and commit it alongside the route edit — CI fails if it drifts.
- **shadcn "include and prune":** only import UI components that exist in `src/components/ui/`. All components this plan uses already exist (Accordion, Badge, Card, Table, AlertDialog, Button, Skeleton, Checkbox). **Do not add Tabs** — the dismissed view is per-section, not a page-level tab.
- **Quality gates (must be clean):** `npm run check` (tsc + eslint, zero errors), `npm run knip` (zero findings — every exported symbol must be used), `npm run test` (all pass). Run from `ui/frontend/`.
- **No AI attribution** in commits.
- **Apply API cap:** `POST /:checkID/apply` accepts at most 200 `user_game_ids` per request; bulk apply must chunk.
- **Tier display order:** Inconsistencies (`inconsistency`) before Nudges (`nudge`); registry order within each tier (the summary array is already in registry order).
- **Auto-fixable checks (exactly these four):** `wishlisted-yet-owned`, `beat-but-not-marked`, `played-but-not-started`, `in-progress-untouched`. `auto_fixable` on each summary item is the source of truth — drive UI off that flag, do not hardcode the list in components.

---

### Task 1: API client layer (`api/library-health.ts`)

**Files:**
- Create: `ui/frontend/src/api/library-health.ts`
- Test: `ui/frontend/src/api/library-health.test.ts`

**Interfaces:**
- Consumes: `api` from `./client` (`api.get<T>(path, {params})`, `api.post<T>(path, data)`, `api.delete<T>(path, options)`).
- Produces (relied on by Tasks 2–6):
  - Types `SmellTier`, `SmellSummaryItem`, `FlaggedItem`, `FlaggedListResponse`, `IgnoredItem`, `IgnoredListResponse`, `ApplyResult`.
  - `getSmellSummary(): Promise<SmellSummaryItem[]>`
  - `getSmellItems(checkID: string, perPage?: number, page?: number): Promise<FlaggedListResponse>`
  - `getIgnoredItems(checkID: string, perPage?: number, page?: number): Promise<IgnoredListResponse>`
  - `applySmell(checkID: string, userGameIds: string[]): Promise<ApplyResult>` (chunks ≤200, aggregates)
  - `fetchAllFlaggedIds(checkID: string): Promise<string[]>` (walks all pages)
  - `applyAllSmell(checkID: string): Promise<ApplyResult>` (fetch all ids → applySmell)
  - `ignoreSmell(checkID: string, userGameIds: string[]): Promise<{ ignored: number }>`
  - `restoreSmell(checkID: string, userGameIds: string[]): Promise<{ restored: number }>`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/api/library-health.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { api } from './client';
import { applySmell, fetchAllFlaggedIds, applyAllSmell } from './library-health';
import type { FlaggedListResponse } from './library-health';

vi.mock('./client', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

const mockApi = vi.mocked(api);

function page(ids: string[], pageNo: number, pages: number): FlaggedListResponse {
  return {
    items: ids.map((id) => ({ user_game_id: id, game_id: 1, title: id })),
    total: pages * 200,
    page: pageNo,
    per_page: 200,
    pages,
  };
}

describe('applySmell', () => {
  beforeEach(() => vi.clearAllMocks());

  it('sends a single request when ids fit under the cap and sums the result', async () => {
    mockApi.post.mockResolvedValue({ applied: 3, skipped: 1 });
    const res = await applySmell('wishlisted-yet-owned', ['a', 'b', 'c', 'd']);
    expect(mockApi.post).toHaveBeenCalledTimes(1);
    expect(mockApi.post).toHaveBeenCalledWith('/api/library/smells/wishlisted-yet-owned/apply', {
      user_game_ids: ['a', 'b', 'c', 'd'],
    });
    expect(res).toEqual({ applied: 3, skipped: 1 });
  });

  it('chunks ids into groups of 200 and aggregates applied/skipped', async () => {
    const ids = Array.from({ length: 450 }, (_, i) => `g${i}`);
    mockApi.post
      .mockResolvedValueOnce({ applied: 200, skipped: 0 })
      .mockResolvedValueOnce({ applied: 200, skipped: 0 })
      .mockResolvedValueOnce({ applied: 40, skipped: 10 });
    const res = await applySmell('beat-but-not-marked', ids);
    expect(mockApi.post).toHaveBeenCalledTimes(3);
    expect((mockApi.post.mock.calls[0][1] as { user_game_ids: string[] }).user_game_ids).toHaveLength(200);
    expect((mockApi.post.mock.calls[2][1] as { user_game_ids: string[] }).user_game_ids).toHaveLength(50);
    expect(res).toEqual({ applied: 440, skipped: 10 });
  });
});

describe('fetchAllFlaggedIds', () => {
  beforeEach(() => vi.clearAllMocks());

  it('walks every page and returns all ids', async () => {
    mockApi.get
      .mockResolvedValueOnce(page(['a', 'b'], 1, 2))
      .mockResolvedValueOnce(page(['c', 'd'], 2, 2));
    const ids = await fetchAllFlaggedIds('orphan-game');
    expect(ids).toEqual(['a', 'b', 'c', 'd']);
    expect(mockApi.get).toHaveBeenCalledTimes(2);
  });

  it('stops after a single page when pages=1', async () => {
    mockApi.get.mockResolvedValueOnce(page(['a'], 1, 1));
    const ids = await fetchAllFlaggedIds('orphan-game');
    expect(ids).toEqual(['a']);
    expect(mockApi.get).toHaveBeenCalledTimes(1);
  });
});

describe('applyAllSmell', () => {
  beforeEach(() => vi.clearAllMocks());

  it('returns zero without calling apply when there are no flagged ids', async () => {
    mockApi.get.mockResolvedValueOnce(page([], 1, 0));
    const res = await applyAllSmell('wishlisted-yet-owned');
    expect(res).toEqual({ applied: 0, skipped: 0 });
    expect(mockApi.post).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui/frontend && npm run test -- library-health.test.ts`
Expected: FAIL — cannot resolve `./library-health` / functions undefined.

- [ ] **Step 3: Write minimal implementation**

Create `ui/frontend/src/api/library-health.ts`:

```ts
import { api } from './client';

const BASE = '/api/library/smells';
const MAX_IDS = 200;

export type SmellTier = 'inconsistency' | 'nudge';

export interface SmellSummaryItem {
  id: string;
  title: string;
  description: string;
  tier: SmellTier;
  auto_fixable: boolean;
  count: number;
}

export interface FlaggedItem {
  user_game_id: string;
  game_id: number;
  title: string;
  cover_art_url?: string;
  platform_row_id?: string;
  platform?: string;
  storefront?: string;
  suggested_storefront?: string;
  suggested_status?: string;
  detail?: string;
}

export interface FlaggedListResponse {
  items: FlaggedItem[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface IgnoredItem {
  user_game_id: string;
  title: string;
  created_at: string;
}

export interface IgnoredListResponse {
  items: IgnoredItem[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface ApplyResult {
  applied: number;
  skipped: number;
}

export function getSmellSummary(): Promise<SmellSummaryItem[]> {
  return api.get<SmellSummaryItem[]>(BASE);
}

export function getSmellItems(checkID: string, perPage = MAX_IDS, page = 1): Promise<FlaggedListResponse> {
  return api.get<FlaggedListResponse>(`${BASE}/${checkID}`, { params: { page, per_page: perPage } });
}

export function getIgnoredItems(checkID: string, perPage = MAX_IDS, page = 1): Promise<IgnoredListResponse> {
  return api.get<IgnoredListResponse>(`${BASE}/${checkID}/ignored`, { params: { page, per_page: perPage } });
}

// Applies in chunks of <=200 (the API cap) and aggregates the result.
export async function applySmell(checkID: string, userGameIds: string[]): Promise<ApplyResult> {
  let applied = 0;
  let skipped = 0;
  for (let i = 0; i < userGameIds.length; i += MAX_IDS) {
    const chunk = userGameIds.slice(i, i + MAX_IDS);
    const res = await api.post<ApplyResult>(`${BASE}/${checkID}/apply`, { user_game_ids: chunk });
    applied += res.applied;
    skipped += res.skipped;
  }
  return { applied, skipped };
}

// Walks every page (per_page=200) and returns all flagged user_game_ids.
export async function fetchAllFlaggedIds(checkID: string): Promise<string[]> {
  const ids: string[] = [];
  let page = 1;
  for (;;) {
    const res = await getSmellItems(checkID, MAX_IDS, page);
    ids.push(...res.items.map((it) => it.user_game_id));
    if (res.items.length === 0 || page >= res.pages) break;
    page += 1;
  }
  return ids;
}

export async function applyAllSmell(checkID: string): Promise<ApplyResult> {
  const ids = await fetchAllFlaggedIds(checkID);
  if (ids.length === 0) return { applied: 0, skipped: 0 };
  return applySmell(checkID, ids);
}

export function ignoreSmell(checkID: string, userGameIds: string[]): Promise<{ ignored: number }> {
  return api.post<{ ignored: number }>(`${BASE}/${checkID}/ignore`, { user_game_ids: userGameIds });
}

export function restoreSmell(checkID: string, userGameIds: string[]): Promise<{ restored: number }> {
  return api.delete<{ restored: number }>(`${BASE}/${checkID}/ignore`, {
    body: JSON.stringify({ user_game_ids: userGameIds }),
  });
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui/frontend && npm run test -- library-health.test.ts`
Expected: PASS (all describe blocks green).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/api/library-health.ts ui/frontend/src/api/library-health.test.ts
git commit -m "feat(ui): library-smells API client layer"
```

---

### Task 2: Query hooks (`hooks/use-library-health.ts`)

**Files:**
- Create: `ui/frontend/src/hooks/use-library-health.ts`
- Modify: `ui/frontend/src/hooks/index.ts` (add re-exports)

**Interfaces:**
- Consumes: everything from `@/api/library-health` (Task 1).
- Produces (relied on by Tasks 3–6, imported from `@/hooks`):
  - `smellKeys` (query-key factory)
  - `useSmellSummary()` → `UseQueryResult<SmellSummaryItem[], Error>`
  - `useSmellItems(checkID: string, enabled: boolean)` → `UseQueryResult<FlaggedListResponse, Error>`
  - `useIgnoredItems(checkID: string, enabled: boolean)` → `UseQueryResult<IgnoredListResponse, Error>`
  - `useApplySmell()` → mutation, variables `{ checkID: string; userGameIds: string[] }`, data `ApplyResult`
  - `useApplyAllSmell()` → mutation, variables `{ checkID: string }`, data `ApplyResult`
  - `useIgnoreSmell()` → mutation, variables `{ checkID: string; userGameIds: string[] }`, data `{ ignored: number }`
  - `useRestoreSmell()` → mutation, variables `{ checkID: string; userGameIds: string[] }`, data `{ restored: number }`

> **Note (testing policy):** these are thin TanStack-Query wrappers — no unit test (a test would be tautological). They are exercised through the component tests in Tasks 3–6. Verification here is typecheck + lint + knip.

- [ ] **Step 1: Write the hooks file**

Create `ui/frontend/src/hooks/use-library-health.ts`:

```ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as smellsApi from '@/api/library-health';
import type {
  SmellSummaryItem,
  FlaggedListResponse,
  IgnoredListResponse,
  ApplyResult,
} from '@/api/library-health';

export const smellKeys = {
  all: ['librarySmells'] as const,
  summary: () => [...smellKeys.all, 'summary'] as const,
  list: (checkID: string) => [...smellKeys.all, 'list', checkID] as const,
  ignored: (checkID: string) => [...smellKeys.all, 'ignored', checkID] as const,
};

export function useSmellSummary() {
  return useQuery<SmellSummaryItem[], Error>({
    queryKey: smellKeys.summary(),
    queryFn: () => smellsApi.getSmellSummary(),
  });
}

export function useSmellItems(checkID: string, enabled: boolean) {
  return useQuery<FlaggedListResponse, Error>({
    queryKey: smellKeys.list(checkID),
    queryFn: () => smellsApi.getSmellItems(checkID),
    enabled,
  });
}

export function useIgnoredItems(checkID: string, enabled: boolean) {
  return useQuery<IgnoredListResponse, Error>({
    queryKey: smellKeys.ignored(checkID),
    queryFn: () => smellsApi.getIgnoredItems(checkID),
    enabled,
  });
}

function useInvalidateSmells() {
  const queryClient = useQueryClient();
  return (checkID: string) => {
    queryClient.invalidateQueries({ queryKey: smellKeys.summary() });
    queryClient.invalidateQueries({ queryKey: smellKeys.list(checkID) });
    queryClient.invalidateQueries({ queryKey: smellKeys.ignored(checkID) });
  };
}

export function useApplySmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<ApplyResult, Error, { checkID: string; userGameIds: string[] }>({
    mutationFn: ({ checkID, userGameIds }) => smellsApi.applySmell(checkID, userGameIds),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}

export function useApplyAllSmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<ApplyResult, Error, { checkID: string }>({
    mutationFn: ({ checkID }) => smellsApi.applyAllSmell(checkID),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}

export function useIgnoreSmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<{ ignored: number }, Error, { checkID: string; userGameIds: string[] }>({
    mutationFn: ({ checkID, userGameIds }) => smellsApi.ignoreSmell(checkID, userGameIds),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}

export function useRestoreSmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<{ restored: number }, Error, { checkID: string; userGameIds: string[] }>({
    mutationFn: ({ checkID, userGameIds }) => smellsApi.restoreSmell(checkID, userGameIds),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}
```

- [ ] **Step 2: Re-export from the hooks barrel**

Add to the end of `ui/frontend/src/hooks/index.ts`:

```ts
// Library Health (smells) hooks
export {
  smellKeys,
  useSmellSummary,
  useSmellItems,
  useIgnoredItems,
  useApplySmell,
  useApplyAllSmell,
  useIgnoreSmell,
  useRestoreSmell,
} from './use-library-health';
```

- [ ] **Step 3: Verify typecheck + lint pass**

Run: `cd ui/frontend && npm run check`
Expected: PASS (no errors). (knip may report the new exports as unused until Tasks 3–6 consume them — that is expected and resolved by the end; do not delete them.)

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/hooks/use-library-health.ts ui/frontend/src/hooks/index.ts
git commit -m "feat(ui): library-smells query hooks"
```

---

### Task 3: Flagged-items table (`components/library-health/flagged-items-table.tsx`)

A purely presentational component (no hooks, no router) — takes data + callbacks so it is trivially testable.

**Files:**
- Create: `ui/frontend/src/components/library-health/flagged-items-table.tsx`
- Test: `ui/frontend/src/components/library-health/flagged-items-table.test.tsx`

**Interfaces:**
- Consumes: `FlaggedItem` type from `@/api/library-health`; shadcn `Table*`, `Badge`, `Button`.
- Produces:
  - `interface FlaggedItemsTableProps { items: FlaggedItem[]; autoFixable: boolean; busy?: boolean; onApply: (userGameId: string) => void; onIgnore: (userGameId: string) => void; onOpenGame: (userGameId: string) => void; }`
  - `export function FlaggedItemsTable(props: FlaggedItemsTableProps)`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/components/library-health/flagged-items-table.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { FlaggedItemsTable } from './flagged-items-table';
import type { FlaggedItem } from '@/api/library-health';

const item: FlaggedItem = { user_game_id: 'ug-1', game_id: 10, title: 'Celeste' };

function renderTable(over: Partial<React.ComponentProps<typeof FlaggedItemsTable>> = {}) {
  const props = {
    items: [item],
    autoFixable: true,
    onApply: vi.fn(),
    onIgnore: vi.fn(),
    onOpenGame: vi.fn(),
    ...over,
  };
  render(<FlaggedItemsTable {...props} />);
  return props;
}

describe('FlaggedItemsTable', () => {
  it('renders an Apply button for auto-fixable checks and fires onApply with the id', async () => {
    const user = userEvent.setup();
    const props = renderTable({ autoFixable: true });
    await user.click(screen.getByRole('button', { name: /apply/i }));
    expect(props.onApply).toHaveBeenCalledWith('ug-1');
  });

  it('renders a Fix button (not Apply) for manual checks and fires onOpenGame', async () => {
    const user = userEvent.setup();
    const props = renderTable({ autoFixable: false });
    expect(screen.queryByRole('button', { name: /^apply$/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /fix/i }));
    expect(props.onOpenGame).toHaveBeenCalledWith('ug-1');
  });

  it('fires onIgnore with the id', async () => {
    const user = userEvent.setup();
    const props = renderTable();
    await user.click(screen.getByRole('button', { name: /ignore/i }));
    expect(props.onIgnore).toHaveBeenCalledWith('ug-1');
  });

  it('shows the suggested storefront when present', () => {
    renderTable({
      autoFixable: false,
      items: [{ ...item, suggested_storefront: 'Steam' }],
    });
    expect(screen.getByText(/suggested:\s*steam/i)).toBeInTheDocument();
  });

  it('shows the detail text when present', () => {
    renderTable({ items: [{ ...item, detail: 'acquired 2031-04-01 (future)' }] });
    expect(screen.getByText('acquired 2031-04-01 (future)')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui/frontend && npm run test -- flagged-items-table.test.tsx`
Expected: FAIL — cannot resolve `./flagged-items-table`.

- [ ] **Step 3: Write minimal implementation**

Create `ui/frontend/src/components/library-health/flagged-items-table.tsx`:

```tsx
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { FlaggedItem } from '@/api/library-health';

export interface FlaggedItemsTableProps {
  items: FlaggedItem[];
  autoFixable: boolean;
  busy?: boolean;
  onApply: (userGameId: string) => void;
  onIgnore: (userGameId: string) => void;
  onOpenGame: (userGameId: string) => void;
}

function contextCell(item: FlaggedItem) {
  if (item.detail) return <span className="text-muted-foreground">{item.detail}</span>;
  if (item.suggested_storefront) {
    return <Badge variant="secondary">Suggested: {item.suggested_storefront}</Badge>;
  }
  const parts = [item.platform, item.storefront].filter(Boolean);
  if (parts.length > 0) return <span className="text-muted-foreground">{parts.join(' · ')}</span>;
  return null;
}

export function FlaggedItemsTable({
  items,
  autoFixable,
  busy = false,
  onApply,
  onIgnore,
  onOpenGame,
}: FlaggedItemsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Game</TableHead>
          <TableHead>Details</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={`${item.user_game_id}-${item.platform_row_id ?? ''}`}>
            <TableCell>
              <button
                type="button"
                className="text-left font-medium hover:underline"
                onClick={() => onOpenGame(item.user_game_id)}
              >
                {item.title}
              </button>
            </TableCell>
            <TableCell>{contextCell(item)}</TableCell>
            <TableCell className="space-x-2 text-right">
              {autoFixable ? (
                <Button size="sm" disabled={busy} onClick={() => onApply(item.user_game_id)}>
                  Apply
                </Button>
              ) : (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onOpenGame(item.user_game_id)}
                >
                  Fix
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => onIgnore(item.user_game_id)}
              >
                Ignore
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui/frontend && npm run test -- flagged-items-table.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/library-health/flagged-items-table.tsx ui/frontend/src/components/library-health/flagged-items-table.test.tsx
git commit -m "feat(ui): flagged-items table for library health"
```

---

### Task 4: Dismissed-items sub-view (`components/library-health/dismissed-items.tsx`)

Renders one check's ignored items with a Restore action. Self-fetches via `useIgnoredItems` (enabled only when shown).

**Files:**
- Create: `ui/frontend/src/components/library-health/dismissed-items.tsx`
- Test: `ui/frontend/src/components/library-health/dismissed-items.test.tsx`

**Interfaces:**
- Consumes: `useIgnoredItems`, `useRestoreSmell` from `@/hooks`; shadcn `Button`.
- Produces:
  - `interface DismissedItemsProps { checkID: string; }`
  - `export function DismissedItems({ checkID }: DismissedItemsProps)`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/components/library-health/dismissed-items.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { DismissedItems } from './dismissed-items';

vi.mock('@/hooks', () => ({
  useIgnoredItems: vi.fn(),
  useRestoreSmell: vi.fn(),
}));

const mkMutation = (over = {}) => ({ mutateAsync: vi.fn().mockResolvedValue({}), isPending: false, ...over });

describe('DismissedItems', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    const { useIgnoredItems, useRestoreSmell } = vi.mocked(await import('@/hooks'));
    useIgnoredItems.mockReturnValue({
      data: { items: [{ user_game_id: 'ug-1', title: 'Hades', created_at: '2026-06-01' }], total: 1, page: 1, per_page: 200, pages: 1 },
      isLoading: false,
    } as unknown as ReturnType<typeof useIgnoredItems>);
    useRestoreSmell.mockReturnValue(mkMutation() as unknown as ReturnType<typeof useRestoreSmell>);
  });

  it('lists dismissed items', () => {
    render(<DismissedItems checkID="orphan-game" />);
    expect(screen.getByText('Hades')).toBeInTheDocument();
  });

  it('calls restore with the check id and game id', async () => {
    const user = userEvent.setup();
    const mutateAsync = vi.fn().mockResolvedValue({ restored: 1 });
    const { useRestoreSmell } = vi.mocked(await import('@/hooks'));
    useRestoreSmell.mockReturnValue(mkMutation({ mutateAsync }) as unknown as ReturnType<typeof useRestoreSmell>);
    render(<DismissedItems checkID="orphan-game" />);
    await user.click(screen.getByRole('button', { name: /restore/i }));
    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith({ checkID: 'orphan-game', userGameIds: ['ug-1'] }),
    );
  });

  it('shows an empty message when there are no dismissed items', async () => {
    const { useIgnoredItems } = vi.mocked(await import('@/hooks'));
    useIgnoredItems.mockReturnValue({
      data: { items: [], total: 0, page: 1, per_page: 200, pages: 0 },
      isLoading: false,
    } as unknown as ReturnType<typeof useIgnoredItems>);
    render(<DismissedItems checkID="orphan-game" />);
    expect(screen.getByText(/no dismissed items/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui/frontend && npm run test -- dismissed-items.test.tsx`
Expected: FAIL — cannot resolve `./dismissed-items`.

- [ ] **Step 3: Write minimal implementation**

Create `ui/frontend/src/components/library-health/dismissed-items.tsx`:

```tsx
import { Button } from '@/components/ui/button';
import { useIgnoredItems, useRestoreSmell } from '@/hooks';

export interface DismissedItemsProps {
  checkID: string;
}

export function DismissedItems({ checkID }: DismissedItemsProps) {
  const { data, isLoading } = useIgnoredItems(checkID, true);
  const restore = useRestoreSmell();

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading dismissed…</p>;

  const items = data?.items ?? [];
  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">No dismissed items.</p>;
  }

  return (
    <ul className="divide-y rounded-md border">
      {items.map((it) => (
        <li key={it.user_game_id} className="flex items-center justify-between px-3 py-2">
          <span className="text-sm">{it.title}</span>
          <Button
            size="sm"
            variant="outline"
            disabled={restore.isPending}
            onClick={() => {
              void restore.mutateAsync({ checkID, userGameIds: [it.user_game_id] });
            }}
          >
            Restore
          </Button>
        </li>
      ))}
    </ul>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui/frontend && npm run test -- dismissed-items.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/library-health/dismissed-items.tsx ui/frontend/src/components/library-health/dismissed-items.test.tsx
git commit -m "feat(ui): dismissed-items restore sub-view for library health"
```

---

### Task 5: Check section (`components/library-health/check-section.tsx`)

The interactive unit: an accordion item (or "all clear" row), lazy listing, per-row + apply-all actions with a destructive confirm dialog, and the dismissed toggle.

**Files:**
- Create: `ui/frontend/src/components/library-health/check-section.tsx`
- Test: `ui/frontend/src/components/library-health/check-section.test.tsx`

**Interfaces:**
- Consumes: `SmellSummaryItem` from `@/api/library-health`; `useSmellItems`, `useApplySmell`, `useApplyAllSmell`, `useIgnoreSmell` from `@/hooks`; `FlaggedItemsTable` (Task 3); `DismissedItems` (Task 4); shadcn `Accordion*`, `Badge`, `Button`, `AlertDialog*`; `toast` from `sonner`.
- Produces:
  - `interface CheckSectionProps { check: SmellSummaryItem; onOpenGame: (userGameId: string) => void; }`
  - `export function CheckSection({ check, onOpenGame }: CheckSectionProps)` — renders **one** `AccordionItem` (must be used inside an `Accordion type="multiple"` parent), or a non-expandable muted row when `check.count === 0`.

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/components/library-health/check-section.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { Accordion } from '@/components/ui/accordion';
import { CheckSection } from './check-section';
import type { SmellSummaryItem } from '@/api/library-health';

vi.mock('@/hooks', () => ({
  useSmellItems: vi.fn(),
  useApplySmell: vi.fn(),
  useApplyAllSmell: vi.fn(),
  useIgnoreSmell: vi.fn(),
  useIgnoredItems: vi.fn(() => ({ data: { items: [], total: 0, page: 1, per_page: 200, pages: 0 }, isLoading: false })),
  useRestoreSmell: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

const autoCheck: SmellSummaryItem = {
  id: 'wishlisted-yet-owned',
  title: 'Wishlisted yet owned',
  description: 'Still on your wishlist even though it is already in your library.',
  tier: 'inconsistency',
  auto_fixable: true,
  count: 2,
};

const cleanCheck: SmellSummaryItem = { ...autoCheck, id: 'orphan-game', title: 'Orphan game', auto_fixable: false, count: 0 };

const mkMutation = (over = {}) => ({ mutateAsync: vi.fn().mockResolvedValue({ applied: 2, skipped: 0 }), isPending: false, ...over });

function renderInAccordion(check: SmellSummaryItem) {
  return render(
    <Accordion type="multiple">
      <CheckSection check={check} onOpenGame={vi.fn()} />
    </Accordion>,
  );
}

describe('CheckSection', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    const hooks = vi.mocked(await import('@/hooks'));
    hooks.useSmellItems.mockReturnValue({
      data: {
        items: [
          { user_game_id: 'ug-1', game_id: 1, title: 'A' },
          { user_game_id: 'ug-2', game_id: 2, title: 'B' },
        ],
        total: 2, page: 1, per_page: 200, pages: 1,
      },
      isLoading: false,
    } as unknown as ReturnType<typeof hooks.useSmellItems>);
    hooks.useApplySmell.mockReturnValue(mkMutation() as unknown as ReturnType<typeof hooks.useApplySmell>);
    hooks.useApplyAllSmell.mockReturnValue(mkMutation() as unknown as ReturnType<typeof hooks.useApplyAllSmell>);
    hooks.useIgnoreSmell.mockReturnValue(mkMutation({ mutateAsync: vi.fn().mockResolvedValue({ ignored: 1 }) }) as unknown as ReturnType<typeof hooks.useIgnoreSmell>);
  });

  it('renders a zero-count check as a non-expandable "All clear" row', () => {
    renderInAccordion(cleanCheck);
    expect(screen.getByText('Orphan game')).toBeInTheDocument();
    expect(screen.getByText(/all clear/i)).toBeInTheDocument();
    // No expand trigger for a clean check.
    expect(screen.queryByRole('button', { name: /orphan game/i })).not.toBeInTheDocument();
  });

  it('shows title, count and an Auto-fix badge for an auto-fixable check', () => {
    renderInAccordion(autoCheck);
    expect(screen.getByText('Wishlisted yet owned')).toBeInTheDocument();
    expect(screen.getByText(/auto-fix/i)).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  it('opens a confirm dialog for "Apply to all" and fires applyAll on confirm', async () => {
    const user = userEvent.setup();
    const mutateAsync = vi.fn().mockResolvedValue({ applied: 2, skipped: 0 });
    const hooks = vi.mocked(await import('@/hooks'));
    hooks.useApplyAllSmell.mockReturnValue(mkMutation({ mutateAsync }) as unknown as ReturnType<typeof hooks.useApplyAllSmell>);

    renderInAccordion(autoCheck);
    await user.click(screen.getByRole('button', { name: /wishlisted yet owned/i })); // expand
    await user.click(await screen.findByRole('button', { name: /apply to all/i }));
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^apply$/i }));
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({ checkID: 'wishlisted-yet-owned' }));
  });

  it('does not fire applyAll when the confirm dialog is cancelled', async () => {
    const user = userEvent.setup();
    const mutateAsync = vi.fn();
    const hooks = vi.mocked(await import('@/hooks'));
    hooks.useApplyAllSmell.mockReturnValue(mkMutation({ mutateAsync }) as unknown as ReturnType<typeof hooks.useApplyAllSmell>);

    renderInAccordion(autoCheck);
    await user.click(screen.getByRole('button', { name: /wishlisted yet owned/i }));
    await user.click(await screen.findByRole('button', { name: /apply to all/i }));
    await user.click(await screen.findByRole('button', { name: /cancel/i }));
    expect(mutateAsync).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui/frontend && npm run test -- check-section.test.tsx`
Expected: FAIL — cannot resolve `./check-section`.

- [ ] **Step 3: Write minimal implementation**

Create `ui/frontend/src/components/library-health/check-section.tsx`:

```tsx
import { useState } from 'react';
import { toast } from 'sonner';
import { AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Check } from 'lucide-react';
import type { SmellSummaryItem } from '@/api/library-health';
import {
  useSmellItems,
  useApplySmell,
  useApplyAllSmell,
  useIgnoreSmell,
} from '@/hooks';
import { FlaggedItemsTable } from './flagged-items-table';
import { DismissedItems } from './dismissed-items';

export interface CheckSectionProps {
  check: SmellSummaryItem;
  onOpenGame: (userGameId: string) => void;
}

export function CheckSection({ check, onOpenGame }: CheckSectionProps) {
  const [expanded, setExpanded] = useState(false);
  const [confirmAll, setConfirmAll] = useState(false);
  const [showDismissed, setShowDismissed] = useState(false);

  const items = useSmellItems(check.id, expanded);
  const apply = useApplySmell();
  const applyAll = useApplyAllSmell();
  const ignore = useIgnoreSmell();
  const busy = apply.isPending || applyAll.isPending || ignore.isPending;

  // Zero-count checks render as a muted, non-expandable "All clear" row.
  if (check.count === 0) {
    return (
      <div className="flex items-center justify-between rounded-md border border-dashed px-4 py-3 text-muted-foreground">
        <span className="flex items-center gap-2">
          <Check className="h-4 w-4 text-green-600" aria-hidden />
          {check.title}
        </span>
        <span className="text-sm">All clear</span>
      </div>
    );
  }

  const flagged = items.data?.items ?? [];
  const total = items.data?.total ?? check.count;

  const handleApply = (userGameId: string) => {
    void apply
      .mutateAsync({ checkID: check.id, userGameIds: [userGameId] })
      .then((r) => toast.success(`Applied (${r.applied}), skipped ${r.skipped}`))
      .catch(() => toast.error('Apply failed'));
  };

  const handleIgnore = (userGameId: string) => {
    void ignore
      .mutateAsync({ checkID: check.id, userGameIds: [userGameId] })
      .catch(() => toast.error('Ignore failed'));
  };

  const handleApplyAll = () => {
    setConfirmAll(false);
    void applyAll
      .mutateAsync({ checkID: check.id })
      .then((r) => toast.success(`Applied ${r.applied}, skipped ${r.skipped}`))
      .catch(() => toast.error('Apply-to-all failed'));
  };

  return (
    <AccordionItem value={check.id}>
      <AccordionTrigger onClick={() => setExpanded(true)}>
        <span className="flex flex-1 items-center justify-between gap-3 pr-2 text-left">
          <span className="flex items-center gap-2">
            <span className="font-medium">{check.title}</span>
            {check.auto_fixable && <Badge variant="secondary">Auto-fix</Badge>}
          </span>
          <Badge>{check.count}</Badge>
        </span>
      </AccordionTrigger>
      <AccordionContent>
        <p className="mb-3 text-sm text-muted-foreground">{check.description}</p>

        {check.auto_fixable && (
          <div className="mb-3">
            <Button size="sm" variant="outline" disabled={busy} onClick={() => setConfirmAll(true)}>
              Apply to all ({check.count})
            </Button>
          </div>
        )}

        {items.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <>
            <FlaggedItemsTable
              items={flagged}
              autoFixable={check.auto_fixable}
              busy={busy}
              onApply={handleApply}
              onIgnore={handleIgnore}
              onOpenGame={onOpenGame}
            />
            {total > flagged.length && (
              <p className="mt-2 text-xs text-muted-foreground">
                Showing first {flagged.length} of {total}.
              </p>
            )}
          </>
        )}

        <div className="mt-3">
          <Button size="sm" variant="ghost" onClick={() => setShowDismissed((v) => !v)}>
            {showDismissed ? 'Hide dismissed' : 'Show dismissed'}
          </Button>
          {showDismissed && (
            <div className="mt-2">
              <DismissedItems checkID={check.id} />
            </div>
          )}
        </div>
      </AccordionContent>

      <AlertDialog open={confirmAll} onOpenChange={setConfirmAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Apply to all flagged games?</AlertDialogTitle>
            <AlertDialogDescription>
              This will apply the suggested fix to all {check.count} games flagged by “{check.title}”.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleApplyAll}
            >
              Apply
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </AccordionItem>
  );
}
```

> **Note:** `setExpanded(true)` in the trigger's `onClick` is an event handler (allowed), not a `setState` in `useEffect`. It only flips on (enables the lazy fetch); collapsing does not need to disable it (the data stays cached). This keeps the query enabled once opened, satisfying the "no setState-in-effect" rule.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui/frontend && npm run test -- check-section.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/library-health/check-section.tsx ui/frontend/src/components/library-health/check-section.test.tsx
git commit -m "feat(ui): library-health check section with apply/ignore/dismiss"
```

---

### Task 6: Page route, tier grouping, nav entry + badge

Wires the summary into a tier-grouped page, adds the route + nav item with a Tier-1 badge, and regenerates the route tree.

**Files:**
- Create: `ui/frontend/src/routes/_authenticated/library-health.tsx`
- Modify: `ui/frontend/src/components/navigation/nav-items.tsx` (add nav item + Tier-1 badge)
- Modify: `ui/frontend/src/components/navigation/nav-items.test.tsx` (badge test)
- Auto-modified by build: `ui/frontend/src/routeTree.gen.ts`

**Interfaces:**
- Consumes: `useSmellSummary` from `@/hooks`; `useNavigate` from `@tanstack/react-router`; `CheckSection` (Task 5); shadcn `Accordion`, `Card*`, `Skeleton`, `Button`; lucide `Stethoscope`, `RefreshCw`.
- Produces: route at path `/library-health`; a nav item `{ href: '/library-health', label: 'Library Health', badge }`.

- [ ] **Step 1: Write the route component**

Create `ui/frontend/src/routes/_authenticated/library-health.tsx`:

```tsx
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { Stethoscope, RefreshCw } from 'lucide-react';
import { Accordion } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useSmellSummary } from '@/hooks';
import type { SmellSummaryItem, SmellTier } from '@/api/library-health';
import { CheckSection } from '@/components/library-health/check-section';

export const Route = createFileRoute('/_authenticated/library-health')({
  head: () => ({ meta: [{ title: 'Library Health | Nexorious' }] }),
  component: LibraryHealthPage,
});

const TIER_ORDER: { tier: SmellTier; label: string; blurb: string }[] = [
  { tier: 'inconsistency', label: 'Inconsistencies', blurb: 'Something looks wrong and probably needs fixing.' },
  { tier: 'nudge', label: 'Nudges', blurb: 'You might want to update these.' },
];

function TierBlock({
  label,
  blurb,
  checks,
  onOpenGame,
}: {
  label: string;
  blurb: string;
  checks: SmellSummaryItem[];
  onOpenGame: (id: string) => void;
}) {
  if (checks.length === 0) return null;
  return (
    <section className="space-y-2">
      <div>
        <h2 className="text-lg font-semibold">{label}</h2>
        <p className="text-sm text-muted-foreground">{blurb}</p>
      </div>
      <Accordion type="multiple" className="space-y-2">
        {checks.map((check) => (
          <CheckSection key={check.id} check={check} onOpenGame={onOpenGame} />
        ))}
      </Accordion>
    </section>
  );
}

function LibraryHealthPage() {
  const navigate = useNavigate();
  const { data, isLoading, isError, error, refetch } = useSmellSummary();

  const onOpenGame = (userGameId: string) => {
    void navigate({ to: '/games/$id/edit', params: { id: userGameId } });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <Stethoscope className="h-6 w-6" />
            Library Health
          </h1>
          <p className="text-muted-foreground">Data-quality checks across your collection.</p>
        </div>
        <Button variant="outline" onClick={() => void refetch()} disabled={isLoading}>
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh
        </Button>
      </div>

      {isLoading && (
        <div className="space-y-2">
          <Skeleton className="h-6 w-40" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {isError && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-10 text-center">
            <p className="font-semibold">Failed to load library health</p>
            <p className="text-sm text-muted-foreground">{error?.message}</p>
            <Button onClick={() => void refetch()}>Try again</Button>
          </CardContent>
        </Card>
      )}

      {data && (
        <>
          {data.every((c) => c.count === 0) && (
            <Card>
              <CardContent className="py-10 text-center">
                <div className="mb-2 text-4xl">🎉</div>
                <p className="font-semibold">Your library is in great shape</p>
                <p className="text-sm text-muted-foreground">No issues found across all checks.</p>
              </CardContent>
            </Card>
          )}
          {TIER_ORDER.map(({ tier, label, blurb }) => (
            <TierBlock
              key={tier}
              label={label}
              blurb={blurb}
              checks={data.filter((c) => c.tier === tier)}
              onOpenGame={onOpenGame}
            />
          ))}
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Add the nav item + Tier-1 badge**

In `ui/frontend/src/components/navigation/nav-items.tsx`:

(a) Add to the lucide import block: `Stethoscope,`

(b) Add the summary hook import near the others:
```ts
import { useSmellSummary } from '@/hooks';
```

(c) Inside `useNavItems()`, after the existing `syncReviewCount` line, compute the badge:
```ts
const { data: smellSummary } = useSmellSummary();
const inconsistencyCount = (smellSummary ?? [])
  .filter((c) => c.tier === 'inconsistency')
  .reduce((sum, c) => sum + c.count, 0);
```

(d) Add a nav item to `mainItems` (place it after the `Planning` (`/pools`) entry):
```ts
    {
      href: '/library-health',
      label: 'Library Health',
      icon: <Stethoscope className="h-4 w-4" />,
      badge: inconsistencyCount,
    },
```

- [ ] **Step 3: Regenerate the route tree and verify the build**

Run: `cd ui/frontend && npm run build`
Expected: build succeeds; `src/routeTree.gen.ts` now contains a `/_authenticated/library-health` entry.

- [ ] **Step 4: Add the nav badge test**

The existing file mocks `@/hooks` to export **only** `useImportSources`. Because Step 2 adds `import { useSmellSummary } from '@/hooks'` to `nav-items.tsx`, that mock must now also export `useSmellSummary` (otherwise it is `undefined` and the hook throws). Edit `ui/frontend/src/components/navigation/nav-items.test.tsx`:

(a) Add a module-level mock fn beside the others and extend the `@/hooks` mock:
```ts
const mockSmellSummary = vi.fn();
vi.mock('@/hooks', () => ({
  useImportSources: () => mockImportSources(),
  useSmellSummary: () => mockSmellSummary(),
}));
```

(b) Give the **existing** two tests a default summary so they don't crash. Add to the top of `beforeEach`:
```ts
mockSmellSummary.mockReturnValue({ data: [] });
```

(c) Append the new test:
```ts
  it('Library Health badge sums only inconsistency-tier counts', () => {
    mockReview.mockReturnValue({ data: { pendingReviewCount: 0, countsBySource: {} } });
    mockImportSources.mockReturnValue({ data: [] });
    mockSmellSummary.mockReturnValue({
      data: [
        { id: 'orphan-game', title: 'x', description: '', tier: 'inconsistency', auto_fixable: false, count: 2 },
        { id: 'missing-ownership-status', title: 'x', description: '', tier: 'inconsistency', auto_fixable: false, count: 3 },
        { id: 'unrated-after-finishing', title: 'x', description: '', tier: 'nudge', auto_fixable: false, count: 5 },
      ],
    });
    const { result } = renderHook(() => useNavItems());
    const health = result.current.mainItems.find((i) => i.href === '/library-health');
    expect(health?.badge).toBe(5); // 2 + 3 inconsistency only; the nudge (5) is excluded
  });
```

- [ ] **Step 5: Run the nav test**

Run: `cd ui/frontend && npm run test -- nav-items.test.tsx`
Expected: PASS (existing cases + the new badge case).

- [ ] **Step 6: Full frontend gate**

Run: `cd ui/frontend && npm run check && npm run knip && npm run test`
Expected: zero type/lint errors, zero knip findings (all Task-2 hook exports are now consumed), all tests pass.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/library-health.tsx \
        ui/frontend/src/routeTree.gen.ts \
        ui/frontend/src/components/navigation/nav-items.tsx \
        ui/frontend/src/components/navigation/nav-items.test.tsx
git commit -m "feat(ui): Library Health page route + nav entry"
```

---

### Task 7: Manual verification & PR

- [ ] **Step 1: Build the app and run it**

Run (from repo root): `make frontend && make build`
Expected: both succeed (the embed picks up the new route via `dist/`).

- [ ] **Step 2: Smoke-test in the browser**

Start the server (`./nexorious serve`), log in, and verify:
- "Library Health" appears in the sidebar with a badge equal to the Tier-1 issue count (or no badge when zero).
- The page groups checks under Inconsistencies then Nudges; zero-count checks show as muted "All clear" rows.
- Expanding an auto-fixable check lists flagged games; per-row **Apply** updates the game and the count refreshes; **Apply to all** prompts a confirm before bulk-applying.
- A manual check's **Fix** navigates to `/games/$id/edit`; the storefront-less check shows a "Suggested: …" badge.
- **Ignore** removes a row; **Show dismissed** lists it; **Restore** brings it back.

- [ ] **Step 3: Open the PR**

```bash
git push -u origin feat/1145-library-health-page
gh pr create --title "feat: Library Health page (web UI)" --label enhancement --body "$(cat <<'EOF'
Implements the Library Health page (#1145), the web-UI child of the Library Smells epic (#1143). Consumes the existing `/api/library/smells` REST API (#1144) — no backend changes.

- Tier-grouped checks (Inconsistencies, Nudges); zero-count checks render as "All clear" rows.
- Auto-fixable checks: per-row Apply + Apply-to-all (destructive confirm); manual checks deep-link to the game edit view (storefront-less shows the suggested storefront in-row).
- Per-item Ignore with a per-check "Show dismissed" restore view.
- New `api/library-health.ts` + `hooks/use-library-health.ts`; sidebar entry with a Tier-1 badge.

Closes #1145
EOF
)"
```

(The pre-push hook runs the full frontend suite. Do **not** merge — wait for the user.)

---

## Self-Review

**Spec coverage:**
- "Library Health route" → Task 6 route. ✓
- "Smells grouped by two tiers, each check an expandable section with count + games" → Task 6 tier blocks + Task 5 accordion. ✓
- "Auto-fixable checks render Apply per-game + apply-to-all; never cross-check fix-everything" → Task 5 (per-row Apply, Apply-to-all scoped to one check). ✓
- "Manual checks deep-link to edit; #1 pre-fills default_storefront as suggestion in the link/dialog" → Task 3 suggested-storefront badge + Task 6 deep-link. (Per approved decision the suggestion is shown in-row; the edit page is not modified.) ✓
- "Per-item Ignore + a Dismissed view to restore" → Task 3 Ignore, Task 4 DismissedItems, Task 5 toggle. ✓
- "Controlled Selects via useState" → no Select on this page (status comes pre-decided from the check); n/a. The one stateful control (Accordion expand) uses `useState`. ✓
- "No setState-in-effect" → all `setState` calls are in event handlers (Task 5 note). ✓
- "Destructive styling on confirm" → Task 5 `AlertDialogAction` with destructive classes. ✓
- "Regenerate routeTree.gen.ts" → Task 6 Step 3 + commit. ✓
- Zero-count display, empty/all-clear state, loading/error → Tasks 5 & 6. ✓
- 200-id apply cap → Task 1 chunking + Apply-to-all via `applyAllSmell`. ✓

**Placeholder scan:** Task 6 Step 4 (nav badge test) intentionally defers the exact `renderHook`/mock wiring to match the existing test file's pattern, which must be read at implementation time — the assertion, the mock return value, and the expected `5` are all given concretely. No other placeholders.

**Type consistency:** `SmellSummaryItem`, `FlaggedItem`, `ApplyResult`, `SmellTier` defined in Task 1 and used unchanged in Tasks 3/5/6. Mutation variable shapes (`{checkID, userGameIds}` / `{checkID}`) defined in Task 2 and called identically in Tasks 4/5. Component prop names (`onApply`/`onIgnore`/`onOpenGame`, `check`, `autoFixable`) consistent across Tasks 3/5/6.
