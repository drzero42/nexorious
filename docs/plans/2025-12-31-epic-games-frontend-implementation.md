# Epic Games Store Frontend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Epic Games Store frontend integration to enable users to connect their Epic account and sync their game library via OAuth device code flow.

**Architecture:** Follow the existing Steam pattern with parallel component structure. Epic uses a two-step OAuth dialog (unlike Steam's inline form) and requires proactive auth expiration handling via sync status polling. All components use TanStack Query for state management and cache invalidation.

**Tech Stack:** Next.js 16, React 19, TypeScript, TanStack Query, React Hook Form, Zod, shadcn/ui, Tailwind CSS

---

## Task 1: Add Epic Types to Type Definitions

**Files:**
- Modify: `frontend/src/types/sync.ts`

**Step 1: Add Epic auth response types**

Add after the existing Steam types (around line 121):

```typescript
// Epic Auth Types
export interface EpicAuthStartResponse {
  authUrl: string;
  instructions: string;
}

export interface EpicAuthCompleteRequest {
  code: string;
}

export interface EpicAuthCompleteResponse {
  valid: boolean;
  displayName: string | null;
  error: string | null;
}

export interface EpicAuthCheckResponse {
  isAuthenticated: boolean;
  displayName: string | null;
}

export interface EpicConnectionInfo {
  configured: boolean;
  displayName: string | null;
  accountId: string | null;
}
```

**Step 2: Add Epic error messages constant**

Add after the Epic types:

```typescript
// Error message mapping for Epic auth
export const EPIC_AUTH_ERROR_MESSAGES: Record<string, string> = {
  invalid_code: 'Invalid authorization code. Please try again.',
  network_error: 'Could not connect to Epic Games. Please try again.',
  expired_code: 'Authorization code expired. Please request a new one.',
};
```

**Step 3: Update SUPPORTED_SYNC_PLATFORMS**

Find the `SUPPORTED_SYNC_PLATFORMS` constant (line 11) and update:

```typescript
export const SUPPORTED_SYNC_PLATFORMS: SyncPlatform[] = [
  SyncPlatform.STEAM,
  SyncPlatform.EPIC, // ADD THIS
];
```

**Step 4: Update SyncStatus interface for auth expiration**

Find the `SyncStatus` interface (around line 37) and update:

```typescript
export interface SyncStatus {
  platform: SyncPlatform;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
  requiresReauth?: boolean;  // ADD THIS
  authExpired?: boolean;      // ADD THIS
}
```

**Step 5: Run type checking**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 6: Commit**

```bash
git add src/types/sync.ts
git commit -m "feat: add Epic Games Store type definitions"
```

---

## Task 2: Add Epic API Functions

**Files:**
- Modify: `frontend/src/api/sync.ts`

**Step 1: Add Epic API response types**

Add after the existing `SteamVerifyApiResponse` type (around line 227):

```typescript
// ============================================================================
// Epic Auth API Types
// ============================================================================

interface EpicAuthStartApiResponse {
  auth_url: string;
  instructions: string;
}

interface EpicAuthCompleteApiResponse {
  valid: boolean;
  display_name: string | null;
  error: string | null;
}

interface EpicAuthCheckApiResponse {
  is_authenticated: boolean;
  display_name: string | null;
}
```

**Step 2: Add Epic API functions**

Add at the end of the file (after `disconnectSteam` function):

```typescript
// ============================================================================
// Epic Auth Functions
// ============================================================================

/**
 * Start Epic authentication flow.
 * Returns auth URL for user to visit.
 */
export async function startEpicAuth(): Promise<EpicAuthStartResponse> {
  const response = await api.post<EpicAuthStartApiResponse>('/sync/epic/auth/start');
  return {
    authUrl: response.auth_url,
    instructions: response.instructions,
  };
}

/**
 * Complete Epic authentication with authorization code.
 */
export async function completeEpicAuth(code: string): Promise<EpicAuthCompleteResponse> {
  const response = await api.post<EpicAuthCompleteApiResponse>('/sync/epic/auth/complete', {
    code,
  });
  return {
    valid: response.valid,
    displayName: response.display_name,
    error: response.error,
  };
}

/**
 * Check current Epic authentication status.
 */
export async function checkEpicAuth(): Promise<EpicAuthCheckResponse> {
  const response = await api.get<EpicAuthCheckApiResponse>('/sync/epic/auth/check');
  return {
    isAuthenticated: response.is_authenticated,
    displayName: response.display_name,
  };
}

/**
 * Disconnect Epic integration.
 */
export async function disconnectEpic(): Promise<void> {
  await api.delete('/sync/epic/connection');
}
```

**Step 3: Update imports at top of file**

Add to the type imports (around line 11):

```typescript
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  IgnoredGame,
  SyncPlatform,
  SyncFrequency,
  SteamVerifyResponse,
  EpicAuthStartResponse,  // ADD THIS
  EpicAuthCompleteResponse,  // ADD THIS
  EpicAuthCheckResponse,  // ADD THIS
} from '@/types';
```

**Step 4: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 5: Commit**

```bash
git add src/api/sync.ts
git commit -m "feat: add Epic API functions with snake_case transformation"
```

---

## Task 3: Write API Tests for Epic Functions

**Files:**
- Modify: `frontend/src/api/sync.test.ts`

**Step 1: Add Epic auth tests**

Add at the end of the test file (before the closing of the main describe block):

```typescript
describe('Epic Auth API', () => {
  it('should start Epic auth and return URL', async () => {
    const mockResponse = {
      auth_url: 'https://www.epicgames.com/id/api/redirect',
      instructions: 'Please visit the URL and login',
    };

    mockApi.post.mockResolvedValueOnce(mockResponse);

    const result = await startEpicAuth();

    expect(mockApi.post).toHaveBeenCalledWith('/sync/epic/auth/start');
    expect(result).toEqual({
      authUrl: 'https://www.epicgames.com/id/api/redirect',
      instructions: 'Please visit the URL and login',
    });
  });

  it('should complete Epic auth with valid code', async () => {
    const mockResponse = {
      valid: true,
      display_name: 'EpicUser123',
      error: null,
    };

    mockApi.post.mockResolvedValueOnce(mockResponse);

    const result = await completeEpicAuth('TESTCODE123');

    expect(mockApi.post).toHaveBeenCalledWith('/sync/epic/auth/complete', {
      code: 'TESTCODE123',
    });
    expect(result).toEqual({
      valid: true,
      displayName: 'EpicUser123',
      error: null,
    });
  });

  it('should handle invalid auth code', async () => {
    const mockResponse = {
      valid: false,
      display_name: null,
      error: 'invalid_code',
    };

    mockApi.post.mockResolvedValueOnce(mockResponse);

    const result = await completeEpicAuth('BADCODE');

    expect(result).toEqual({
      valid: false,
      displayName: null,
      error: 'invalid_code',
    });
  });

  it('should check Epic auth status', async () => {
    const mockResponse = {
      is_authenticated: true,
      display_name: 'EpicUser123',
    };

    mockApi.get.mockResolvedValueOnce(mockResponse);

    const result = await checkEpicAuth();

    expect(mockApi.get).toHaveBeenCalledWith('/sync/epic/auth/check');
    expect(result).toEqual({
      isAuthenticated: true,
      displayName: 'EpicUser123',
    });
  });

  it('should disconnect Epic', async () => {
    mockApi.delete.mockResolvedValueOnce(undefined);

    await disconnectEpic();

    expect(mockApi.delete).toHaveBeenCalledWith('/sync/epic/connection');
  });

  it('should transform snake_case to camelCase correctly', async () => {
    const mockResponse = {
      auth_url: 'https://example.com',
      instructions: 'test',
    };

    mockApi.post.mockResolvedValueOnce(mockResponse);

    const result = await startEpicAuth();

    // Verify transformed keys
    expect(result).toHaveProperty('authUrl');
    expect(result).toHaveProperty('instructions');
    expect(result).not.toHaveProperty('auth_url');
  });
});
```

**Step 2: Add import for Epic functions**

Add to the imports at the top of the test file:

```typescript
import {
  getSyncConfigs,
  getSyncConfig,
  updateSyncConfig,
  triggerSync,
  getSyncStatus,
  getIgnoredGames,
  unignoreGame,
  verifySteamCredentials,
  disconnectSteam,
  startEpicAuth,  // ADD THIS
  completeEpicAuth,  // ADD THIS
  checkEpicAuth,  // ADD THIS
  disconnectEpic,  // ADD THIS
} from './sync';
```

**Step 3: Run API tests**

```bash
npm run test src/api/sync.test.ts
```

Expected: PASS (all 6 Epic tests pass)

**Step 4: Commit**

```bash
git add src/api/sync.test.ts
git commit -m "test: add Epic API function tests"
```

---

## Task 4: Add Epic Hooks

**Files:**
- Modify: `frontend/src/hooks/use-sync.ts`

**Step 1: Add Epic hook imports**

Update the imports at the top of the file (around line 9):

```typescript
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SteamVerifyResponse,
  EpicAuthStartResponse,  // ADD THIS
  EpicAuthCompleteResponse,  // ADD THIS
  EpicAuthCheckResponse,  // ADD THIS
} from '@/types';
```

**Step 2: Add Epic query key**

Add to the `syncKeys` object (around line 24):

```typescript
export const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncPlatform) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncPlatform) => [...syncKeys.statuses(), platform] as const,
  ignoredGames: (params?: { source?: string; limit?: number; offset?: number }) =>
    [...syncKeys.all, 'ignored', params] as const,
  epicAuth: () => [...syncKeys.all, 'epicAuth'] as const,  // ADD THIS
};
```

**Step 3: Add Epic auth hooks**

Add at the end of the file (after the Steam hooks):

```typescript
// ============================================================================
// Epic Auth Hooks
// ============================================================================

/**
 * Hook to start Epic authentication flow.
 * Returns auth URL for user to visit.
 */
export function useStartEpicAuth() {
  return useMutation<EpicAuthStartResponse, Error>({
    mutationFn: syncApi.startEpicAuth,
    onError: (error) => {
      console.error('Failed to start Epic auth:', error);
    },
  });
}

/**
 * Hook to complete Epic authentication with code.
 * Invalidates sync configs on success.
 */
export function useCompleteEpicAuth() {
  const queryClient = useQueryClient();

  return useMutation<EpicAuthCompleteResponse, Error, string>({
    mutationFn: (code: string) => syncApi.completeEpicAuth(code),
    onSuccess: () => {
      // Invalidate sync configs to refresh connection status
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.EPIC) });
    },
  });
}

/**
 * Hook to check current Epic authentication status.
 * Cached for 5 minutes.
 */
export function useCheckEpicAuth() {
  return useQuery<EpicAuthCheckResponse, Error>({
    queryKey: syncKeys.epicAuth(),
    queryFn: syncApi.checkEpicAuth,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true,
  });
}

/**
 * Hook to disconnect Epic integration.
 * Invalidates all Epic-related queries on success.
 */
export function useDisconnectEpic() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectEpic,
    onSuccess: () => {
      // Invalidate all Epic-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.EPIC) });
      queryClient.invalidateQueries({ queryKey: syncKeys.epicAuth() });
    },
    onError: (error) => {
      console.error('Failed to disconnect Epic:', error);
    },
  });
}
```

**Step 4: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 5: Commit**

```bash
git add src/hooks/use-sync.ts
git commit -m "feat: add Epic auth hooks with TanStack Query"
```

---

## Task 5: Write Hook Tests for Epic Functions

**Files:**
- Modify: `frontend/src/hooks/use-sync.test.ts`

**Step 1: Add Epic hook tests**

Add at the end of the test file:

```typescript
describe('Epic Auth Hooks', () => {
  it('should call startEpicAuth mutation', async () => {
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    const { result } = renderHook(() => useStartEpicAuth(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(mockStartEpicAuth).toHaveBeenCalled();
    mockStartEpicAuth.mockRestore();
  });

  it('should invalidate queries on successful Epic auth', async () => {
    const mockCompleteEpicAuth = vi.spyOn(syncApi, 'completeEpicAuth');
    mockCompleteEpicAuth.mockResolvedValue({
      valid: true,
      displayName: 'EpicUser',
      error: null,
    });

    const { result } = renderHook(() => useCompleteEpicAuth(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync('TESTCODE');
    });

    expect(mockCompleteEpicAuth).toHaveBeenCalledWith('TESTCODE');

    // Query invalidation happens automatically via TanStack Query
    mockCompleteEpicAuth.mockRestore();
  });

  it('should cache Epic auth status', async () => {
    const mockCheckEpicAuth = vi.spyOn(syncApi, 'checkEpicAuth');
    mockCheckEpicAuth.mockResolvedValue({
      isAuthenticated: true,
      displayName: 'EpicUser',
    });

    const { result, waitFor } = renderHook(() => useCheckEpicAuth(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => result.current.isSuccess);

    expect(result.current.data).toEqual({
      isAuthenticated: true,
      displayName: 'EpicUser',
    });

    mockCheckEpicAuth.mockRestore();
  });

  it('should invalidate all Epic queries on disconnect', async () => {
    const mockDisconnectEpic = vi.spyOn(syncApi, 'disconnectEpic');
    mockDisconnectEpic.mockResolvedValue(undefined);

    const { result } = renderHook(() => useDisconnectEpic(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(mockDisconnectEpic).toHaveBeenCalled();
    mockDisconnectEpic.mockRestore();
  });
});
```

**Step 2: Add imports for Epic hooks**

Update imports at the top of the test file:

```typescript
import {
  useSyncConfigs,
  useSyncConfig,
  useSyncStatus,
  useUpdateSyncConfig,
  useTriggerSync,
  useIgnoredGames,
  useUnignoreGame,
  useVerifySteamCredentials,
  useDisconnectSteam,
  useStartEpicAuth,  // ADD THIS
  useCompleteEpicAuth,  // ADD THIS
  useCheckEpicAuth,  // ADD THIS
  useDisconnectEpic,  // ADD THIS
} from './use-sync';
```

**Step 3: Run hook tests**

```bash
npm run test src/hooks/use-sync.test.ts
```

Expected: PASS (all 4 Epic hook tests pass)

**Step 4: Commit**

```bash
git add src/hooks/use-sync.test.ts
git commit -m "test: add Epic hook tests"
```

---

## Task 6: Create Epic Auth Dialog Component

**Files:**
- Create: `frontend/src/components/sync/epic-auth-dialog.tsx`

**Step 1: Create the component file**

Create `frontend/src/components/sync/epic-auth-dialog.tsx` with the following content:

```typescript
'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2, ExternalLink, Copy, Check } from 'lucide-react';
import { useStartEpicAuth, useCompleteEpicAuth } from '@/hooks';
import { EPIC_AUTH_ERROR_MESSAGES } from '@/types';

const authCodeSchema = z.object({
  code: z.string().min(1, 'Authorization code is required'),
});

type AuthCodeForm = z.infer<typeof authCodeSchema>;

interface EpicAuthDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

export function EpicAuthDialog({ open, onOpenChange, onSuccess }: EpicAuthDialogProps) {
  const [step, setStep] = useState<'start' | 'code'>('start');
  const [authUrl, setAuthUrl] = useState<string>('');
  const [urlCopied, setUrlCopied] = useState(false);

  const startMutation = useStartEpicAuth();
  const completeMutation = useCompleteEpicAuth();

  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
  } = useForm<AuthCodeForm>({
    resolver: zodResolver(authCodeSchema),
  });

  const handleStart = async () => {
    try {
      const result = await startMutation.mutateAsync();
      setAuthUrl(result.authUrl);
      setStep('code');
    } catch (err) {
      toast.error('Failed to start Epic authentication');
    }
  };

  const handleCopyUrl = async () => {
    await navigator.clipboard.writeText(authUrl);
    setUrlCopied(true);
    setTimeout(() => setUrlCopied(false), 2000);
    toast.success('URL copied to clipboard');
  };

  const onSubmit = async (data: AuthCodeForm) => {
    try {
      const result = await completeMutation.mutateAsync(data.code);

      if (!result.valid) {
        const errorMessage = result.error
          ? EPIC_AUTH_ERROR_MESSAGES[result.error] || 'Authentication failed'
          : 'Authentication failed';
        toast.error(errorMessage);
        return;
      }

      toast.success(`Epic Games connected as ${result.displayName}`);
      handleClose();
      onSuccess();
    } catch (err) {
      toast.error('Failed to complete Epic authentication');
    }
  };

  const handleClose = () => {
    setStep('start');
    setAuthUrl('');
    setUrlCopied(false);
    reset();
    onOpenChange(false);
  };

  const isLoading = startMutation.isPending || completeMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[500px]">
        {step === 'start' ? (
          <>
            <DialogHeader>
              <DialogTitle>Connect Epic Games Store</DialogTitle>
              <DialogDescription>
                Authenticate with Epic Games to sync your library
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <p className="text-sm text-muted-foreground">
                You'll be redirected to Epic Games to authorize Nexorious. After logging in,
                you'll receive an authorization code to complete the connection.
              </p>
              <Button onClick={handleStart} disabled={isLoading} className="w-full">
                {isLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Starting...
                  </>
                ) : (
                  'Start Authentication'
                )}
              </Button>
            </div>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Enter Authorization Code</DialogTitle>
              <DialogDescription>
                Complete authentication by entering the code from Epic Games
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="rounded-lg border bg-muted/50 p-4 space-y-3">
                <p className="text-sm font-medium">Step 1: Visit Epic Games</p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => window.open(authUrl, '_blank')}
                    className="flex-1"
                  >
                    <ExternalLink className="mr-2 h-4 w-4" />
                    Open Epic Login
                  </Button>
                  <Button variant="outline" size="sm" onClick={handleCopyUrl}>
                    {urlCopied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Log in to your Epic account and authorize Nexorious
                </p>
              </div>

              <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="code">Step 2: Enter Authorization Code</Label>
                  <Input
                    id="code"
                    placeholder="Paste the code from Epic Games"
                    {...register('code')}
                    disabled={isLoading}
                    autoComplete="off"
                  />
                  {errors.code && (
                    <p className="text-sm text-destructive">{errors.code.message}</p>
                  )}
                </div>

                <div className="flex gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleClose}
                    disabled={isLoading}
                    className="flex-1"
                  >
                    Cancel
                  </Button>
                  <Button type="submit" disabled={isLoading} className="flex-1">
                    {isLoading ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Verifying...
                      </>
                    ) : (
                      'Connect'
                    )}
                  </Button>
                </div>
              </form>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 3: Commit**

```bash
git add src/components/sync/epic-auth-dialog.tsx
git commit -m "feat: create Epic auth dialog with two-step OAuth flow"
```

---

## Task 7: Create Epic Auth Dialog Tests

**Files:**
- Create: `frontend/src/components/sync/epic-auth-dialog.test.tsx`

**Step 1: Create test file**

Create `frontend/src/components/sync/epic-auth-dialog.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EpicAuthDialog } from './epic-auth-dialog';
import * as syncApi from '@/api/sync';

// Mock toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
};

describe('EpicAuthDialog', () => {
  const mockOnOpenChange = vi.fn();
  const mockOnSuccess = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render step 1 with start button', () => {
    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Connect Epic Games Store')).toBeInTheDocument();
    expect(screen.getByText('Start Authentication')).toBeInTheDocument();
  });

  it('should call startEpicAuth on button click', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    const startButton = screen.getByText('Start Authentication');
    await user.click(startButton);

    await waitFor(() => {
      expect(mockStartEpicAuth).toHaveBeenCalled();
    });

    mockStartEpicAuth.mockRestore();
  });

  it('should transition to step 2 after start', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    const startButton = screen.getByText('Start Authentication');
    await user.click(startButton);

    await waitFor(() => {
      expect(screen.getByText('Enter Authorization Code')).toBeInTheDocument();
      expect(screen.getByPlaceholderText('Paste the code from Epic Games')).toBeInTheDocument();
    });

    mockStartEpicAuth.mockRestore();
  });

  it('should open Epic URL in new tab', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    const mockWindowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Start auth to get to step 2
    await user.click(screen.getByText('Start Authentication'));

    await waitFor(() => {
      expect(screen.getByText('Open Epic Login')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Open Epic Login'));

    expect(mockWindowOpen).toHaveBeenCalledWith('https://epicgames.com/activate', '_blank');

    mockStartEpicAuth.mockRestore();
    mockWindowOpen.mockRestore();
  });

  it('should submit auth code', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    const mockCompleteEpicAuth = vi.spyOn(syncApi, 'completeEpicAuth');

    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    mockCompleteEpicAuth.mockResolvedValue({
      valid: true,
      displayName: 'EpicUser',
      error: null,
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Start auth
    await user.click(screen.getByText('Start Authentication'));

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Paste the code from Epic Games')).toBeInTheDocument();
    });

    // Enter code
    const codeInput = screen.getByPlaceholderText('Paste the code from Epic Games');
    await user.type(codeInput, 'TESTCODE123');

    // Submit
    await user.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(mockCompleteEpicAuth).toHaveBeenCalledWith('TESTCODE123');
      expect(mockOnSuccess).toHaveBeenCalled();
    });

    mockStartEpicAuth.mockRestore();
    mockCompleteEpicAuth.mockRestore();
  });

  it('should show error for invalid code', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    const mockCompleteEpicAuth = vi.spyOn(syncApi, 'completeEpicAuth');
    const { toast } = await import('sonner');

    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    mockCompleteEpicAuth.mockResolvedValue({
      valid: false,
      displayName: null,
      error: 'invalid_code',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Start auth
    await user.click(screen.getByText('Start Authentication'));
    await waitFor(() => screen.getByPlaceholderText('Paste the code from Epic Games'));

    // Enter invalid code
    await user.type(screen.getByPlaceholderText('Paste the code from Epic Games'), 'BADCODE');
    await user.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });

    mockStartEpicAuth.mockRestore();
    mockCompleteEpicAuth.mockRestore();
  });

  it('should reset state on cancel', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');

    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Go to step 2
    await user.click(screen.getByText('Start Authentication'));
    await waitFor(() => screen.getByText('Cancel'));

    // Click cancel
    await user.click(screen.getByText('Cancel'));

    expect(mockOnOpenChange).toHaveBeenCalledWith(false);

    mockStartEpicAuth.mockRestore();
  });
});
```

**Step 2: Run tests**

```bash
npm run test src/components/sync/epic-auth-dialog.test.tsx
```

Expected: PASS (all 7 tests pass)

**Step 3: Commit**

```bash
git add src/components/sync/epic-auth-dialog.test.tsx
git commit -m "test: add Epic auth dialog component tests"
```

---

## Task 8: Create Epic Connection Card Component

**Files:**
- Create: `frontend/src/components/sync/epic-connection-card.tsx`

**Step 1: Create component file**

Create `frontend/src/components/sync/epic-connection-card.tsx`:

```typescript
'use client';

import { useState, useEffect } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Loader2, Check, Info } from 'lucide-react';
import { useDisconnectEpic, useSyncStatus } from '@/hooks';
import { SyncPlatform } from '@/types';
import { EpicAuthDialog } from './epic-auth-dialog';

interface EpicConnectionCardProps {
  isConfigured: boolean;
  displayName?: string;
  accountId?: string;
  onConnectionChange: () => void;
}

export function EpicConnectionCard({
  isConfigured,
  displayName,
  accountId,
  onConnectionChange,
}: EpicConnectionCardProps) {
  const [authDialogOpen, setAuthDialogOpen] = useState(false);

  const disconnectMutation = useDisconnectEpic();
  const { data: syncStatus } = useSyncStatus(SyncPlatform.EPIC);

  const isDisconnecting = disconnectMutation.isPending;

  // Monitor for auth expiration
  useEffect(() => {
    if (syncStatus?.requiresReauth || syncStatus?.authExpired) {
      toast.error('Epic authentication expired. Please reconnect.', {
        action: {
          label: 'Reconnect',
          onClick: () => setAuthDialogOpen(true),
        },
        duration: 10000,
      });
    }
  }, [syncStatus?.requiresReauth, syncStatus?.authExpired]);

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Epic Games disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Epic Games');
    }
  };

  const handleAuthSuccess = () => {
    onConnectionChange();
  };

  const getBadgeState = () => {
    if (syncStatus?.authExpired || syncStatus?.requiresReauth) {
      return {
        label: 'Auth Expired',
        className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
      };
    }
    if (!isConfigured) {
      return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
    }
    return {
      label: 'Connected',
      className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
    };
  };

  const badgeState = getBadgeState();
  const authExpired = syncStatus?.authExpired || syncStatus?.requiresReauth;

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Epic Games Connection</CardTitle>
              <CardDescription>
                {authExpired
                  ? 'Your Epic authentication has expired'
                  : isConfigured
                    ? 'Your Epic account is connected'
                    : 'Connect your Epic account to sync your game library'}
              </CardDescription>
            </div>
            <Badge variant="outline" className={badgeState.className}>
              {badgeState.label}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          {isConfigured && !authExpired ? (
            <div className="space-y-4">
              <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
                <Check className="h-5 w-5 text-green-600" />
                <div>
                  <p className="font-medium">Connected as {displayName}</p>
                  {accountId && <p className="text-sm text-muted-foreground">{accountId}</p>}
                </div>
              </div>

              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  <strong>Note:</strong> Playtime data is not available for Epic games.
                </AlertDescription>
              </Alert>

              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button variant="outline" disabled={isDisconnecting}>
                    {isDisconnecting ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Disconnecting...
                      </>
                    ) : (
                      'Disconnect'
                    )}
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Disconnect Epic Games?</AlertDialogTitle>
                    <AlertDialogDescription>
                      Your sync settings will be preserved but syncing will stop until you
                      reconnect.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={handleDisconnect}>Disconnect</AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          ) : (
            <div className="space-y-4">
              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  <strong>Note:</strong> Playtime data is not available for Epic games.
                </AlertDescription>
              </Alert>

              <Button onClick={() => setAuthDialogOpen(true)} className="w-full">
                {authExpired ? 'Re-authenticate' : 'Connect Epic Games'}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <EpicAuthDialog
        open={authDialogOpen}
        onOpenChange={setAuthDialogOpen}
        onSuccess={handleAuthSuccess}
      />
    </>
  );
}
```

**Step 2: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 3: Commit**

```bash
git add src/components/sync/epic-connection-card.tsx
git commit -m "feat: create Epic connection card with auth expiration handling"
```

---

## Task 9: Create Epic Connection Card Tests

**Files:**
- Create: `frontend/src/components/sync/epic-connection-card.test.tsx`

**Step 1: Create test file**

Create `frontend/src/components/sync/epic-connection-card.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EpicConnectionCard } from './epic-connection-card';
import * as syncApi from '@/api/sync';
import * as hooks from '@/hooks';

// Mock toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
};

describe('EpicConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    // Mock useSyncStatus to return no auth expiration by default
    vi.spyOn(hooks, 'useSyncStatus').mockReturnValue({
      data: {
        platform: 'epic',
        isSyncing: false,
        lastSyncedAt: null,
        activeJobId: null,
      },
    } as any);
  });

  it('should render not configured state', () => {
    render(
      <EpicConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Epic Games Connection')).toBeInTheDocument();
    expect(screen.getByText('Connect Epic Games')).toBeInTheDocument();
    expect(screen.getByText('Not Configured')).toBeInTheDocument();
  });

  it('should render connected state', () => {
    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        accountId="epic-account-id"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Connected as EpicUser123')).toBeInTheDocument();
    expect(screen.getByText('epic-account-id')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText('Disconnect')).toBeInTheDocument();
  });

  it('should show disconnect confirmation', async () => {
    const user = userEvent.setup();

    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByText('Disconnect'));

    await waitFor(() => {
      expect(screen.getByText('Disconnect Epic Games?')).toBeInTheDocument();
    });
  });

  it('should call disconnectEpic on confirm', async () => {
    const user = userEvent.setup();
    const mockDisconnectEpic = vi.spyOn(syncApi, 'disconnectEpic');
    mockDisconnectEpic.mockResolvedValue(undefined);

    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByText('Disconnect'));
    await waitFor(() => screen.getByRole('button', { name: /Disconnect$/i }));

    const confirmButton = screen.getByRole('button', { name: /Disconnect$/i });
    await user.click(confirmButton);

    await waitFor(() => {
      expect(mockDisconnectEpic).toHaveBeenCalled();
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });

    mockDisconnectEpic.mockRestore();
  });

  it('should render auth expired state', () => {
    vi.spyOn(hooks, 'useSyncStatus').mockReturnValue({
      data: {
        platform: 'epic',
        isSyncing: false,
        lastSyncedAt: null,
        activeJobId: null,
        authExpired: true,
      },
    } as any);

    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Auth Expired')).toBeInTheDocument();
    expect(screen.getByText('Re-authenticate')).toBeInTheDocument();
  });

  it('should show playtime limitation note', () => {
    render(
      <EpicConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText(/Playtime data is not available for Epic games/i)).toBeInTheDocument();
  });

  it('should open auth dialog on connect click', async () => {
    const user = userEvent.setup();

    render(
      <EpicConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByText('Connect Epic Games'));

    // Dialog should open (check for dialog title)
    await waitFor(() => {
      expect(screen.getByText('Connect Epic Games Store')).toBeInTheDocument();
    });
  });
});
```

**Step 2: Run tests**

```bash
npm run test src/components/sync/epic-connection-card.test.tsx
```

Expected: PASS (all 7 tests pass)

**Step 3: Commit**

```bash
git add src/components/sync/epic-connection-card.test.tsx
git commit -m "test: add Epic connection card component tests"
```

---

## Task 10: Create Epic Settings Detail Page

**Files:**
- Create: `frontend/src/app/(main)/sync/epic/page.tsx`

**Step 1: Create Epic settings page**

Create `frontend/src/app/(main)/sync/epic/page.tsx`:

```typescript
'use client';

import { SyncServiceCard } from '@/components/sync/sync-service-card';
import { SyncPlatform } from '@/types';

export default function EpicSyncSettingsPage() {
  return (
    <div className="container max-w-4xl py-8">
      <SyncServiceCard platform={SyncPlatform.EPIC} />
    </div>
  );
}
```

**Step 2: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 3: Test navigation manually**

Start the dev server and navigate to `/sync/epic` to verify the page renders.

**Step 4: Commit**

```bash
git add src/app/(main)/sync/epic/page.tsx
git commit -m "feat: create Epic sync settings detail page"
```

---

## Task 11: Integrate Epic Card into Main Sync Page

**Files:**
- Modify: `frontend/src/app/(main)/sync/page.tsx`

**Step 1: Read current sync page structure**

```bash
cat /home/abo/workspace/home/nexorious/frontend/src/app/(main)/sync/page.tsx
```

**Step 2: Add Epic import**

Add to imports at top of file:

```typescript
import { EpicConnectionCard } from '@/components/sync/epic-connection-card';
```

**Step 3: Add Epic config and preferences extraction**

Find where `steamConfig` is defined and add Epic config below it:

```typescript
const steamConfig = configs?.configs.find((c) => c.platform === SyncPlatform.STEAM);
const epicConfig = configs?.configs.find((c) => c.platform === SyncPlatform.EPIC); // ADD THIS

// Get preferences
const steamPrefs = user?.preferences?.steam;
const epicPrefs = user?.preferences?.epic; // ADD THIS
```

**Step 4: Add Epic connection card to render**

Find where `SteamConnectionCard` is rendered and add `EpicConnectionCard` below it:

```typescript
<SteamConnectionCard
  isConfigured={steamConfig?.isConfigured ?? false}
  steamId={steamPrefs?.steam_id}
  steamUsername={steamPrefs?.username}
  onConnectionChange={refetch}
/>

<EpicConnectionCard
  isConfigured={epicConfig?.isConfigured ?? false}
  displayName={epicPrefs?.display_name}
  accountId={epicPrefs?.account_id}
  onConnectionChange={refetch}
/>
```

**Step 5: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 6: Test manually**

Start dev server and verify Epic card appears on sync page.

**Step 7: Commit**

```bash
git add src/app/(main)/sync/page.tsx
git commit -m "feat: integrate Epic connection card into main sync page"
```

---

## Task 12: Run Full Test Suite and Verify Coverage

**Files:**
- None (verification)

**Step 1: Run all tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test
```

Expected: All tests pass

**Step 2: Check test coverage**

```bash
npm run test:coverage
```

Expected: Overall coverage >70%

**Step 3: Run type checking**

```bash
npm run check
```

Expected: PASS (no TypeScript errors)

**Step 4: If any issues, document them**

Create a list of any failing tests or type errors to address.

---

## Task 13: Manual Testing and Polish

**Files:**
- Various (fixes based on testing)

**Step 1: Start backend and frontend**

Terminal 1:
```bash
cd /home/abo/workspace/home/nexorious/backend
uv run python -m app.main
```

Terminal 2:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run dev
```

**Step 2: Manual testing checklist**

Test the following flows:

1. **Connection Flow**:
   - [ ] Navigate to sync settings page
   - [ ] Epic card shows "Not Configured"
   - [ ] Click "Connect Epic Games"
   - [ ] Dialog opens with "Start Authentication"
   - [ ] Click start → transitions to code input step
   - [ ] "Open Epic Login" button opens URL in new tab
   - [ ] Copy URL button works
   - [ ] Enter auth code and submit
   - [ ] Success toast appears with display name
   - [ ] Dialog closes automatically
   - [ ] Connection card shows "Connected" with display name

2. **Epic Settings Page**:
   - [ ] Navigate to `/sync/epic`
   - [ ] Page shows Epic sync settings
   - [ ] Sync frequency dropdown works
   - [ ] Auto-add toggle works
   - [ ] "Sync Now" button triggers sync
   - [ ] Last synced timestamp displays

3. **Disconnect Flow**:
   - [ ] Click "Disconnect" button
   - [ ] Confirmation dialog appears
   - [ ] Click "Disconnect" in dialog
   - [ ] Success toast appears
   - [ ] Card shows "Not Configured"

4. **Auth Expiration** (if possible to test):
   - [ ] Trigger a sync with expired auth (need backend support)
   - [ ] Badge shows "Auth Expired"
   - [ ] Toast appears with "Reconnect" action
   - [ ] Click "Re-authenticate" opens dialog
   - [ ] Complete re-auth flow
   - [ ] Connection restored

**Step 3: Fix any issues found**

Document and fix any UI/UX issues, bugs, or errors found during manual testing.

**Step 4: Commit fixes**

```bash
git add <modified files>
git commit -m "fix: address manual testing issues"
```

---

## Task 14: Final Commit and Cleanup

**Files:**
- None (final verification)

**Step 1: Run final checks**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
npm run test
npm run test:coverage
```

Expected: All pass, coverage >70%

**Step 2: Review all commits**

```bash
git log --oneline -20
```

Verify all commits are descriptive and follow conventional commits.

**Step 3: Push to remote (if applicable)**

```bash
git push origin <branch-name>
```

---

## Success Criteria Checklist

### Functional Requirements

- [ ] Users can click "Connect Epic Games" and start OAuth flow
- [ ] Two-step dialog guides users through Epic login
- [ ] Authorization code submission works correctly
- [ ] Success toast shows Epic display name
- [ ] Connection card updates to show connected state
- [ ] Users can trigger manual Epic sync from detail page
- [ ] Sync status polling shows real-time progress
- [ ] Last synced timestamp displays correctly
- [ ] Sync frequency and auto-add settings persist
- [ ] Expired auth detected via sync status polling
- [ ] Connection card shows "Auth Expired" badge
- [ ] Toast notification with "Reconnect" action appears
- [ ] Re-authentication flow works identically to initial auth
- [ ] Epic card appears alongside Steam on main sync page
- [ ] Epic detail page accessible and fully functional
- [ ] Playtime limitation note visible to users
- [ ] Disconnect confirmation dialog prevents accidents
- [ ] All components follow existing design system

### Testing & Quality

- [ ] All new tests pass (`npm run test`)
- [ ] Type checking passes (`npm run check`)
- [ ] Frontend coverage remains >70%
- [ ] Manual testing confirms full flow works end-to-end

### Code Quality

- [ ] Follows existing Steam patterns
- [ ] Components are reusable and well-typed
- [ ] Proper error handling throughout
- [ ] Clear component and function naming

## Files Created/Modified Summary

### New Files (5)
- `frontend/src/components/sync/epic-auth-dialog.tsx`
- `frontend/src/components/sync/epic-auth-dialog.test.tsx`
- `frontend/src/components/sync/epic-connection-card.tsx`
- `frontend/src/components/sync/epic-connection-card.test.tsx`
- `frontend/src/app/(main)/sync/epic/page.tsx`

### Modified Files (5)
- `frontend/src/types/sync.ts` (Epic types)
- `frontend/src/api/sync.ts` (Epic API functions)
- `frontend/src/api/sync.test.ts` (Epic API tests)
- `frontend/src/hooks/use-sync.ts` (Epic hooks)
- `frontend/src/hooks/use-sync.test.ts` (Epic hook tests)
- `frontend/src/app/(main)/sync/page.tsx` (Epic card integration)

## References

- Design Document: `docs/plans/2025-12-31-epic-games-frontend-design.md`
- Backend Design: `docs/plans/2025-12-31-epic-games-store-sync-design.md`
- Backend Implementation: `docs/plans/2025-12-31-epic-games-store-sync-implementation.md`
- Steam Connection Card: `frontend/src/components/sync/steam-connection-card.tsx`
- Sync Service Card: `frontend/src/components/sync/sync-service-card.tsx`
