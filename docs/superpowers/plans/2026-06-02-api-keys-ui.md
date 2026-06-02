# API Keys Management UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a profile-page UI for users to list, create (with one-time key reveal), and revoke their API keys.

**Architecture:** Frontend-only. The backend CRUD (`GET/POST/DELETE /api/auth/api-keys`) already exists and is unchanged. We add an `api/auth.ts` client layer, a `use-api-keys.ts` TanStack Query hook module, and two presentational components (`ApiKeysSection` + `CreateApiKeyDialog`) under `components/api-keys/`, rendered from `profile.tsx`. This mirrors the #514 notifications pattern exactly.

**Tech Stack:** React 19, TypeScript, TanStack Query, react-hook-form + Zod, shadcn/ui (`Dialog`, `Select`, `Badge`, `AlertDialog`, `Button`, `Input`, `Label`), `sonner` toasts, Vitest + @testing-library/react.

---

## File Structure

- `ui/frontend/src/api/auth.ts` (modify) — add `ApiKey`/`CreatedApiKey` types + `listApiKeys`/`createApiKey`/`revokeApiKey`.
- `ui/frontend/src/lib/api-key-expiry.ts` (create) — pure `expiryPresetToRFC3339` helper + `EXPIRY_PRESETS`.
- `ui/frontend/src/lib/api-key-expiry.test.ts` (create) — unit test for the helper.
- `ui/frontend/src/hooks/use-api-keys.ts` (create) — `apiKeysKeys`, `useApiKeys`, `useCreateApiKey`, `useRevokeApiKey`.
- `ui/frontend/src/hooks/index.ts` (modify) — re-export the new hooks.
- `ui/frontend/src/components/api-keys/create-api-key-dialog.tsx` (create) — two-state dialog (form → reveal).
- `ui/frontend/src/components/api-keys/create-api-key-dialog.test.tsx` (create) — one-time reveal + copy + close-clears test.
- `ui/frontend/src/components/api-keys/api-keys-section.tsx` (create) — the list Card + revoke confirm.
- `ui/frontend/src/routes/_authenticated/profile.tsx` (modify) — render `<ApiKeysSection/>`.

All commands run from `ui/frontend/`.

---

## Task 1: API client functions

**Files:**
- Modify: `ui/frontend/src/api/auth.ts` (append after `updatePreferences`)

- [ ] **Step 1: Add types and functions**

Append to `ui/frontend/src/api/auth.ts`:

```ts
export interface ApiKey {
  id: string;
  name: string;
  scopes: 'read' | 'write';
  last_used_at: string | null;
  created_at: string;
  expires_at: string | null;
}

// The create response returns the raw key exactly once. It omits last_used_at
// (always null for a brand-new key), so it is not part of this shape.
export interface CreatedApiKey {
  id: string;
  name: string;
  scopes: 'read' | 'write';
  key: string;
  created_at: string;
  expires_at: string | null;
}

export function listApiKeys(): Promise<ApiKey[]> {
  return api.get<ApiKey[]>('/auth/api-keys');
}

export function createApiKey(body: {
  name: string;
  scopes: 'read' | 'write';
  expires_at: string | null;
}): Promise<CreatedApiKey> {
  return api.post<CreatedApiKey>('/auth/api-keys', body);
}

export function revokeApiKey(id: string): Promise<void> {
  return api.delete(`/auth/api-keys/${encodeURIComponent(id)}`);
}
```

- [ ] **Step 2: Verify typecheck passes**

Run: `npm run check`
Expected: PASS (no type errors). The functions are unused so far — that is fine; they are exported.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/api/auth.ts
git commit -m "feat: add API key client functions"
```

---

## Task 2: Expiry preset helper (pure logic, TDD)

**Files:**
- Create: `ui/frontend/src/lib/api-key-expiry.ts`
- Test: `ui/frontend/src/lib/api-key-expiry.test.ts`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/lib/api-key-expiry.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { expiryPresetToRFC3339, EXPIRY_PRESETS } from './api-key-expiry';

describe('expiryPresetToRFC3339', () => {
  // Fixed base time so the test is deterministic.
  const now = new Date('2026-06-02T12:00:00.000Z');

  it('returns null for "never"', () => {
    expect(expiryPresetToRFC3339('never', now)).toBeNull();
  });

  it('returns an RFC3339 string 30 days out', () => {
    expect(expiryPresetToRFC3339('30', now)).toBe('2026-07-02T12:00:00.000Z');
  });

  it('returns an RFC3339 string 90 days out', () => {
    expect(expiryPresetToRFC3339('90', now)).toBe('2026-08-31T12:00:00.000Z');
  });

  it('returns an RFC3339 string 365 days out', () => {
    expect(expiryPresetToRFC3339('365', now)).toBe('2027-06-02T12:00:00.000Z');
  });

  it('exposes preset options with "write"-friendly defaults order', () => {
    expect(EXPIRY_PRESETS.map((p) => p.value)).toEqual(['30', '90', '365', 'never']);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test api-key-expiry`
Expected: FAIL — cannot resolve `./api-key-expiry`.

- [ ] **Step 3: Write minimal implementation**

Create `ui/frontend/src/lib/api-key-expiry.ts`:

```ts
export type ExpiryPreset = '30' | '90' | '365' | 'never';

export const EXPIRY_PRESETS: { value: ExpiryPreset; label: string }[] = [
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' },
  { value: '365', label: '365 days' },
  { value: 'never', label: 'Never' },
];

/**
 * Convert an expiry preset into the RFC3339 string the API expects, or null for
 * "never". `now` is injectable so callers/tests can be deterministic.
 */
export function expiryPresetToRFC3339(preset: ExpiryPreset, now: Date = new Date()): string | null {
  if (preset === 'never') return null;
  const days = Number(preset);
  const expiry = new Date(now.getTime() + days * 24 * 60 * 60 * 1000);
  return expiry.toISOString();
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test api-key-expiry`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/lib/api-key-expiry.ts ui/frontend/src/lib/api-key-expiry.test.ts
git commit -m "feat: add API key expiry preset helper"
```

---

## Task 3: Query hooks

**Files:**
- Create: `ui/frontend/src/hooks/use-api-keys.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Create the hook module**

Create `ui/frontend/src/hooks/use-api-keys.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as authApi from '@/api/auth';
import type { ApiKey, CreatedApiKey } from '@/api/auth';

export const apiKeysKeys = {
  all: ['api-keys'] as const,
  list: () => [...apiKeysKeys.all, 'list'] as const,
};

export function useApiKeys() {
  return useQuery<ApiKey[]>({
    queryKey: apiKeysKeys.list(),
    queryFn: () => authApi.listApiKeys(),
  });
}

export function useCreateApiKey() {
  const queryClient = useQueryClient();
  return useMutation<
    CreatedApiKey,
    Error,
    { name: string; scopes: 'read' | 'write'; expires_at: string | null }
  >({
    mutationFn: (body) => authApi.createApiKey(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeysKeys.list() });
    },
  });
}

export function useRevokeApiKey() {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (id) => authApi.revokeApiKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeysKeys.list() });
    },
  });
}
```

- [ ] **Step 2: Re-export from the hooks barrel**

In `ui/frontend/src/hooks/index.ts`, add (placement: after the existing hook export blocks, matching the file's grouping style):

```ts
// API key hooks
export { apiKeysKeys, useApiKeys, useCreateApiKey, useRevokeApiKey } from './use-api-keys';
```

- [ ] **Step 3: Verify typecheck passes**

Run: `npm run check`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/hooks/use-api-keys.ts ui/frontend/src/hooks/index.ts
git commit -m "feat: add API key query hooks"
```

---

## Task 4: Create dialog — failing reveal test first (TDD)

**Files:**
- Test: `ui/frontend/src/components/api-keys/create-api-key-dialog.test.tsx`

This task writes the test that pins the security invariant. The component does not exist yet, so the test fails to import — that is the expected red state.

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/components/api-keys/create-api-key-dialog.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { CreateApiKeyDialog } from './create-api-key-dialog';

// Mock the create mutation so no network call is made and we control the result.
const mockMutateAsync = vi.fn();
vi.mock('@/hooks', () => ({
  useCreateApiKey: () => ({ mutateAsync: mockMutateAsync, isPending: false }),
}));

describe('CreateApiKeyDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // jsdom has no clipboard by default; install a spy-able one.
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('reveals the raw key exactly once and copies it, then clears on close', async () => {
    const user = userEvent.setup();
    mockMutateAsync.mockResolvedValue({
      id: 'k1',
      name: 'CI token',
      scopes: 'write',
      key: 'nxr_secret_raw_value',
      created_at: '2026-06-02T12:00:00Z',
      expires_at: null,
    });

    const onOpenChange = vi.fn();
    render(<CreateApiKeyDialog open={true} onOpenChange={onOpenChange} />);

    await user.type(screen.getByLabelText(/name/i), 'CI token');
    await user.click(screen.getByRole('button', { name: /create/i }));

    // Reveal state: the raw key is now visible.
    await waitFor(() => {
      expect(screen.getByText('nxr_secret_raw_value')).toBeInTheDocument();
    });

    // Copy button writes the raw key to the clipboard.
    await user.click(screen.getByRole('button', { name: /copy/i }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('nxr_secret_raw_value');

    // Closing the dialog clears the key from state. Re-render as closed, then open
    // again: the freshly opened dialog must be back on the form (no raw key).
    fireEvent.click(screen.getByRole('button', { name: /done/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test create-api-key-dialog`
Expected: FAIL — cannot resolve `./create-api-key-dialog`.

- [ ] **Step 3: Commit the test**

```bash
git add ui/frontend/src/components/api-keys/create-api-key-dialog.test.tsx
git commit -m "test: pin one-time API key reveal invariant"
```

---

## Task 5: Create dialog — implementation

**Files:**
- Create: `ui/frontend/src/components/api-keys/create-api-key-dialog.tsx`

- [ ] **Step 1: Implement the component**

Create `ui/frontend/src/components/api-keys/create-api-key-dialog.tsx`:

```tsx
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Copy, Loader2, AlertTriangle } from 'lucide-react';
import { useCreateApiKey } from '@/hooks';
import { EXPIRY_PRESETS, expiryPresetToRFC3339, type ExpiryPreset } from '@/lib/api-key-expiry';
import type { CreatedApiKey } from '@/api/auth';

const schema = z.object({
  name: z.string().min(1, 'Name is required').trim(),
  scopes: z.enum(['read', 'write']),
  expiry: z.enum(['30', '90', '365', 'never']),
});

type FormValues = z.infer<typeof schema>;

interface CreateApiKeyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateApiKeyDialog({ open, onOpenChange }: CreateApiKeyDialogProps) {
  const createApiKey = useCreateApiKey();
  const [created, setCreated] = useState<CreatedApiKey | null>(null);

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', scopes: 'write', expiry: '30' },
  });

  const scopes = watch('scopes');
  const expiry = watch('expiry');

  const handleClose = () => {
    // Clear the revealed key and form state, then notify the parent.
    setCreated(null);
    reset({ name: '', scopes: 'write', expiry: '30' });
    onOpenChange(false);
  };

  const onSubmit = async (values: FormValues) => {
    try {
      const result = await createApiKey.mutateAsync({
        name: values.name,
        scopes: values.scopes,
        expires_at: expiryPresetToRFC3339(values.expiry as ExpiryPreset),
      });
      setCreated(result);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create API key');
    }
  };

  const handleCopy = async () => {
    if (!created) return;
    await navigator.clipboard.writeText(created.key);
    toast.success('API key copied to clipboard');
  };

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? onOpenChange(true) : handleClose())}>
      <DialogContent>
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>API key created</DialogTitle>
              <DialogDescription>Your new key is ready to use.</DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              <Alert variant="destructive">
                <AlertTriangle className="h-4 w-4" />
                <AlertDescription>Copy this now — it won&apos;t be shown again.</AlertDescription>
              </Alert>
              <div className="flex items-center gap-2">
                <code className="flex-1 break-all rounded-md border bg-muted/50 p-3 font-mono text-sm">
                  {created.key}
                </code>
                <Button type="button" variant="outline" size="icon" onClick={handleCopy}
                  aria-label="Copy API key">
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" onClick={handleClose}>
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)}>
            <DialogHeader>
              <DialogTitle>New API key</DialogTitle>
              <DialogDescription>Create a key for programmatic access.</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-2">
              <div>
                <Label htmlFor="api-key-name">Name</Label>
                <Input id="api-key-name" className="mt-1" placeholder="e.g. CI token"
                  {...register('name')} />
                {errors.name && <p className="mt-1 text-sm text-red-600">{errors.name.message}</p>}
              </div>
              <div>
                <Label htmlFor="api-key-scopes">Scopes</Label>
                <Select value={scopes} onValueChange={(v) => setValue('scopes', v as 'read' | 'write')}>
                  <SelectTrigger id="api-key-scopes" className="mt-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="write">Read &amp; write</SelectItem>
                    <SelectItem value="read">Read only</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="api-key-expiry">Expiry</Label>
                <Select value={expiry} onValueChange={(v) => setValue('expiry', v as ExpiryPreset)}>
                  <SelectTrigger id="api-key-expiry" className="mt-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {EXPIRY_PRESETS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={handleClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={createApiKey.isPending}>
                {createApiKey.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Run the reveal test to verify it passes**

Run: `npm run test create-api-key-dialog`
Expected: PASS.

> Note: the test mocks `@/hooks` (so `useCreateApiKey` is stubbed). Because the
> component imports `useCreateApiKey` from `@/hooks`, the mock intercepts it; the
> real hook module is not loaded in this test.

- [ ] **Step 3: Verify typecheck and lint**

Run: `npm run check`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/api-keys/create-api-key-dialog.tsx
git commit -m "feat: add create API key dialog with one-time reveal"
```

---

## Task 6: API keys section (list + revoke)

**Files:**
- Create: `ui/frontend/src/components/api-keys/api-keys-section.tsx`

- [ ] **Step 1: Implement the section**

Create `ui/frontend/src/components/api-keys/api-keys-section.tsx`:

```tsx
import { useState } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
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
import { KeyRound, Plus, Trash2, Loader2 } from 'lucide-react';
import { useApiKeys, useRevokeApiKey } from '@/hooks';
import { formatRelativeTime } from '@/types/jobs';
import type { ApiKey } from '@/api/auth';
import { CreateApiKeyDialog } from './create-api-key-dialog';

function isExpired(key: ApiKey): boolean {
  return key.expires_at !== null && new Date(key.expires_at).getTime() < Date.now();
}

export function ApiKeysSection() {
  const { data: keys, isLoading } = useApiKeys();
  const revokeApiKey = useRevokeApiKey();
  const [createOpen, setCreateOpen] = useState(false);
  const [toRevoke, setToRevoke] = useState<ApiKey | null>(null);

  const handleRevoke = async () => {
    if (!toRevoke) return;
    try {
      await revokeApiKey.mutateAsync(toRevoke.id);
      toast.success(`Revoked "${toRevoke.name}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke API key');
    } finally {
      setToRevoke(null);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" />
            API Keys
          </CardTitle>
          <CardDescription>Manage keys for programmatic access to your account.</CardDescription>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          New API key
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : !keys || keys.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">No API keys yet.</p>
        ) : (
          <ul className="divide-y">
            {keys.map((key) => (
              <li key={key.id} className="flex items-center justify-between gap-4 py-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{key.name}</span>
                    <Badge variant="secondary">{key.scopes}</Badge>
                    {isExpired(key) && <Badge variant="destructive">Expired</Badge>}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {key.last_used_at ? `Last used ${formatRelativeTime(key.last_used_at)}` : 'Never used'}
                    {' · '}Created {new Date(key.created_at).toLocaleDateString()}
                    {key.expires_at
                      ? ` · Expires ${new Date(key.expires_at).toLocaleDateString()}`
                      : ' · Never expires'}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Revoke ${key.name}`}
                  onClick={() => setToRevoke(key)}
                >
                  <Trash2 className="h-4 w-4 text-red-600" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <CreateApiKeyDialog open={createOpen} onOpenChange={setCreateOpen} />

      <AlertDialog open={toRevoke !== null} onOpenChange={(o) => !o && setToRevoke(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke API key?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently revokes <strong>{toRevoke?.name}</strong>. Any client using it will
              stop working immediately. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void handleRevoke();
              }}
              disabled={revokeApiKey.isPending}
            >
              {revokeApiKey.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Revoke
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
```

- [ ] **Step 2: Verify typecheck and lint**

Run: `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/api-keys/api-keys-section.tsx
git commit -m "feat: add API keys section with list and revoke"
```

---

## Task 7: Wire into the profile page

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/profile.tsx`

- [ ] **Step 1: Add the import**

In `ui/frontend/src/routes/_authenticated/profile.tsx`, next to the existing
`import { NotificationsSection } from '@/components/notifications/notifications-section';`, add:

```tsx
import { ApiKeysSection } from '@/components/api-keys/api-keys-section';
```

- [ ] **Step 2: Render the section**

Find the line `<NotificationsSection />` (inside the `lg:col-span-2` column) and add
`<ApiKeysSection />` immediately after it:

```tsx
          {/* Notifications Section */}
          <NotificationsSection />

          {/* API Keys Section */}
          <ApiKeysSection />
```

- [ ] **Step 3: Verify typecheck, lint, and full frontend test suite**

Run: `npm run check && npm run knip && npm run test`
Expected: PASS. (`knip` must report no unused exports — every new export is consumed.)

- [ ] **Step 4: Build to confirm the SPA compiles**

Run: `npm run build`
Expected: PASS. (No route changes were made, so `routeTree.gen.ts` should be unchanged; if `git status` shows it changed, commit it too.)

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/profile.tsx
git commit -m "feat: show API keys section on profile page"
```

---

## Final verification

- [ ] Run `npm run check && npm run knip && npm run test` from `ui/frontend/` — all green.
- [ ] Manually (optional, via `./nexorious serve`): create a key, confirm the raw key shows once with a working Copy button, confirm it disappears after closing, confirm the list shows the new key, revoke it and confirm it disappears.

## Out of scope (do not touch)

Backend (`internal/api/auth.go`), migrations, `router.go`, `slumber.yaml`. All API key endpoints already exist and are unchanged.
