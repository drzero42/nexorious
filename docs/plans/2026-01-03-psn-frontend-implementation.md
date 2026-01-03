# PSN Frontend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build frontend UI for PlayStation Network sync integration following Steam/Epic patterns

**Architecture:** React components with TanStack Query for data fetching, reusing existing sync infrastructure patterns. PSN configuration card in settings, sync service card on sync page, with NPSSO token input flow.

**Tech Stack:** Next.js 16, React 19, TypeScript, TanStack Query, shadcn/ui, Tailwind CSS

---

## Prerequisites

Backend PSN implementation must be complete with these endpoints available:
- `POST /sync/psn/configure` - Configure PSN with NPSSO token
- `GET /sync/psn/status` - Get PSN connection status
- `DELETE /sync/psn/disconnect` - Disconnect PSN
- `POST /sync/{platform}` - Trigger sync (generic, already exists)
- `GET /sync/config/{platform}` - Get sync config (generic, already exists)
- `PUT /sync/config/{platform}` - Update sync config (generic, already exists)

---

## Task 1: Update TypeScript Types

Add PSN to sync types enum and add PSN-specific API types.

**Files:**
- Modify: [frontend/src/types/sync.ts](frontend/src/types/sync.ts:1-160)

**Step 1: Write failing test for PSN types**

Create test file: `frontend/src/types/sync.test.ts`

```typescript
import { describe, it, expect } from 'vitest';
import { SyncPlatform, SUPPORTED_SYNC_PLATFORMS, getPlatformDisplayInfo } from './sync';

describe('SyncPlatform - PSN', () => {
  it('should include PSN in SyncPlatform enum', () => {
    expect(SyncPlatform.PSN).toBe('psn');
  });

  it('should include PSN in SUPPORTED_SYNC_PLATFORMS', () => {
    expect(SUPPORTED_SYNC_PLATFORMS).toContain(SyncPlatform.PSN);
  });

  it('should return PSN display info', () => {
    const displayInfo = getPlatformDisplayInfo(SyncPlatform.PSN);
    expect(displayInfo.name).toBe('PlayStation Network');
    expect(displayInfo.color).toBeDefined();
    expect(displayInfo.bgColor).toBeDefined();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test sync.test.ts`

Expected: FAIL - "Property 'PSN' does not exist on type 'typeof SyncPlatform'"

**Step 3: Add PSN to SyncPlatform enum**

Edit [frontend/src/types/sync.ts](frontend/src/types/sync.ts:5-14):

```typescript
export enum SyncPlatform {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
  PSN = 'psn',
}

export const SUPPORTED_SYNC_PLATFORMS: SyncPlatform[] = [
  SyncPlatform.STEAM,
  SyncPlatform.EPIC,
  SyncPlatform.PSN,
];
```

**Step 4: Add PSN display info**

Edit [frontend/src/types/sync.ts](frontend/src/types/sync.ts:76-99):

```typescript
export function getPlatformDisplayInfo(platform: SyncPlatform): {
  name: string;
  color: string;
  bgColor: string;
} {
  const info: Record<SyncPlatform, { name: string; color: string; bgColor: string }> = {
    [SyncPlatform.STEAM]: {
      name: 'Steam',
      color: 'text-[#1b2838]',
      bgColor: 'bg-[#1b2838]/10 dark:bg-[#1b2838]/30',
    },
    [SyncPlatform.EPIC]: {
      name: 'Epic Games',
      color: 'text-gray-800 dark:text-gray-200',
      bgColor: 'bg-gray-100 dark:bg-gray-700',
    },
    [SyncPlatform.GOG]: {
      name: 'GOG',
      color: 'text-purple-700 dark:text-purple-400',
      bgColor: 'bg-purple-100 dark:bg-purple-900/30',
    },
    [SyncPlatform.PSN]: {
      name: 'PlayStation Network',
      color: 'text-[#003087]',
      bgColor: 'bg-[#003087]/10 dark:bg-[#003087]/30',
    },
  };
  return info[platform];
}
```

**Step 5: Add PSN API types**

Add to end of [frontend/src/types/sync.ts](frontend/src/types/sync.ts:160):

```typescript
// PSN Types
export interface PSNConfigureRequest {
  npssoToken: string;
}

export interface PSNConfigureResponse {
  success: boolean;
  onlineId: string;
  accountId: string;
  region: string;
  message: string;
}

export interface PSNStatusResponse {
  isConfigured: boolean;
  onlineId: string | null;
  accountId: string | null;
  region: string | null;
  tokenExpired: boolean;
}

export interface PSNConnectionInfo {
  configured: boolean;
  onlineId: string | null;
  accountId: string | null;
  region: string | null;
  tokenExpired: boolean;
}

// Error message mapping for PSN auth
export const PSN_CONFIG_ERROR_MESSAGES: Record<string, string> = {
  invalid_token: 'Invalid NPSSO token. Please obtain a new token and try again.',
  token_expired: 'NPSSO token has expired. Please obtain a new token.',
  network_error: 'Could not connect to PlayStation Network. Please try again.',
  invalid_token_format: 'NPSSO token must be exactly 64 characters.',
};
```

**Step 6: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test sync.test.ts`

Expected: PASS - All PSN type tests pass

**Step 7: Run all type checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 8: Commit**

```bash
git add frontend/src/types/sync.ts frontend/src/types/sync.test.ts
git commit -m "feat(frontend): add PSN types to sync platform enum

- Add PSN to SyncPlatform enum and SUPPORTED_SYNC_PLATFORMS
- Add PSN display info with PlayStation blue colors
- Add PSN API types for configure/status/disconnect
- Add PSN error message mappings
- Add comprehensive tests for PSN types

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Add PSN API Client Functions

Add PSN API functions following Steam/Epic patterns.

**Files:**
- Modify: [frontend/src/api/sync.ts](frontend/src/api/sync.ts:1-328)

**Step 1: Write failing test for PSN API functions**

Create test file: `frontend/src/api/sync-psn.test.ts`

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { configurePSN, getPSNStatus, disconnectPSN } from './sync';
import * as client from './client';

vi.mock('./client');

describe('PSN API', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('configurePSN', () => {
    it('should configure PSN with valid NPSSO token', async () => {
      const mockResponse = {
        success: true,
        online_id: 'testuser',
        account_id: 'test-account-id',
        region: 'us',
        message: 'PSN configured successfully',
      };

      vi.mocked(client.api.post).mockResolvedValue(mockResponse);

      const result = await configurePSN('a'.repeat(64));

      expect(client.api.post).toHaveBeenCalledWith('/sync/psn/configure', {
        npsso_token: 'a'.repeat(64),
      });
      expect(result).toEqual({
        success: true,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        message: 'PSN configured successfully',
      });
    });
  });

  describe('getPSNStatus', () => {
    it('should get PSN connection status', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'testuser',
        account_id: 'test-account-id',
        region: 'us',
        token_expired: false,
      };

      vi.mocked(client.api.get).mockResolvedValue(mockResponse);

      const result = await getPSNStatus();

      expect(client.api.get).toHaveBeenCalledWith('/sync/psn/status');
      expect(result).toEqual({
        isConfigured: true,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        tokenExpired: false,
      });
    });
  });

  describe('disconnectPSN', () => {
    it('should disconnect PSN', async () => {
      vi.mocked(client.api.delete).mockResolvedValue({});

      await disconnectPSN();

      expect(client.api.delete).toHaveBeenCalledWith('/sync/psn/disconnect');
    });
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test sync-psn.test.ts`

Expected: FAIL - "configurePSN is not a function"

**Step 3: Add PSN API response types**

Add to [frontend/src/api/sync.ts](frontend/src/api/sync.ts:328) (end of file, after Epic functions):

```typescript
// ============================================================================
// PSN API Types
// ============================================================================

interface PSNConfigureApiRequest {
  npsso_token: string;
}

interface PSNConfigureApiResponse {
  success: boolean;
  online_id: string;
  account_id: string;
  region: string;
  message: string;
}

interface PSNStatusApiResponse {
  is_configured: boolean;
  online_id: string | null;
  account_id: string | null;
  region: string | null;
  token_expired: boolean;
}
```

**Step 4: Implement PSN API functions**

Add to [frontend/src/api/sync.ts](frontend/src/api/sync.ts:328) (after PSN API types):

```typescript
// ============================================================================
// PSN Functions
// ============================================================================

/**
 * Configure PSN with NPSSO token.
 */
export async function configurePSN(npssoToken: string): Promise<PSNConfigureResponse> {
  const response = await api.post<PSNConfigureApiResponse>('/sync/psn/configure', {
    npsso_token: npssoToken,
  } as PSNConfigureApiRequest);

  return {
    success: response.success,
    onlineId: response.online_id,
    accountId: response.account_id,
    region: response.region,
    message: response.message,
  };
}

/**
 * Get PSN connection status.
 */
export async function getPSNStatus(): Promise<PSNStatusResponse> {
  const response = await api.get<PSNStatusApiResponse>('/sync/psn/status');

  return {
    isConfigured: response.is_configured,
    onlineId: response.online_id,
    accountId: response.account_id,
    region: response.region,
    tokenExpired: response.token_expired,
  };
}

/**
 * Disconnect PSN integration.
 */
export async function disconnectPSN(): Promise<void> {
  await api.delete('/sync/psn/disconnect');
}
```

**Step 5: Update imports in sync.ts**

Add PSN types to import statement at top of [frontend/src/api/sync.ts](frontend/src/api/sync.ts:1-14):

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
  EpicAuthStartResponse,
  EpicAuthCompleteResponse,
  EpicAuthCheckResponse,
  PSNConfigureResponse,
  PSNStatusResponse,
} from '@/types';
```

**Step 6: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test sync-psn.test.ts`

Expected: PASS - All PSN API tests pass

**Step 7: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 8: Commit**

```bash
git add frontend/src/api/sync.ts frontend/src/api/sync-psn.test.ts
git commit -m "feat(frontend): add PSN API client functions

- Add configurePSN for NPSSO token verification
- Add getPSNStatus for connection status
- Add disconnectPSN for disconnection
- Follow Steam/Epic API patterns
- Add comprehensive API tests with mocks

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Add PSN React Query Hooks

Add PSN hooks to use-sync.ts following Steam/Epic patterns.

**Files:**
- Modify: [frontend/src/hooks/use-sync.ts](frontend/src/hooks/use-sync.ts:1-257)

**Step 1: Write failing test for PSN hooks**

Create test file: `frontend/src/hooks/use-sync-psn.test.ts`

```typescript
import { describe, it, expect, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useConfigurePSN, usePSNStatus, useDisconnectPSN } from './use-sync';
import * as syncApi from '@/api/sync';

vi.mock('@/api/sync');

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe('PSN Hooks', () => {
  describe('useConfigurePSN', () => {
    it('should configure PSN with NPSSO token', async () => {
      const mockResponse = {
        success: true,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        message: 'PSN configured successfully',
      };

      vi.mocked(syncApi.configurePSN).mockResolvedValue(mockResponse);

      const { result } = renderHook(() => useConfigurePSN(), {
        wrapper: createWrapper(),
      });

      result.current.mutate('a'.repeat(64));

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockResponse);
    });
  });

  describe('usePSNStatus', () => {
    it('should fetch PSN status', async () => {
      const mockStatus = {
        isConfigured: true,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        tokenExpired: false,
      };

      vi.mocked(syncApi.getPSNStatus).mockResolvedValue(mockStatus);

      const { result } = renderHook(() => usePSNStatus(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(mockStatus);
    });
  });

  describe('useDisconnectPSN', () => {
    it('should disconnect PSN', async () => {
      vi.mocked(syncApi.disconnectPSN).mockResolvedValue();

      const { result } = renderHook(() => useDisconnectPSN(), {
        wrapper: createWrapper(),
      });

      result.current.mutate();

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
    });
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test use-sync-psn.test.ts`

Expected: FAIL - "useConfigurePSN is not a function"

**Step 3: Add PSN query key**

Edit [frontend/src/hooks/use-sync.ts](frontend/src/hooks/use-sync.ts:19-28):

```typescript
export const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncPlatform) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncPlatform) => [...syncKeys.statuses(), platform] as const,
  ignoredGames: (params?: { source?: string; limit?: number; offset?: number }) =>
    [...syncKeys.all, 'ignored', params] as const,
  epicAuth: () => [...syncKeys.all, 'epicAuth'] as const,
  psnStatus: () => [...syncKeys.all, 'psnStatus'] as const,
};
```

**Step 4: Add PSN imports**

Edit [frontend/src/hooks/use-sync.ts](frontend/src/hooks/use-sync.ts:1-13):

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as syncApi from '@/api/sync';
import { SyncPlatform } from '@/types';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SteamVerifyResponse,
  EpicAuthStartResponse,
  EpicAuthCompleteResponse,
  EpicAuthCheckResponse,
  PSNConfigureResponse,
  PSNStatusResponse,
} from '@/types';
```

**Step 5: Implement PSN hooks**

Add to end of [frontend/src/hooks/use-sync.ts](frontend/src/hooks/use-sync.ts:257) (after Epic hooks):

```typescript
// ============================================================================
// PSN Hooks
// ============================================================================

/**
 * Hook to configure PSN with NPSSO token.
 * Invalidates sync configs on success.
 */
export function useConfigurePSN() {
  const queryClient = useQueryClient();

  return useMutation<PSNConfigureResponse, Error, string>({
    mutationFn: (npssoToken: string) => syncApi.configurePSN(npssoToken),
    onSuccess: () => {
      // Invalidate sync configs to refresh connection status
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.PSN) });
      queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
    },
    onError: (error) => {
      console.error('Failed to configure PSN:', error);
    },
  });
}

/**
 * Hook to check current PSN connection status.
 * Cached for 5 minutes.
 */
export function usePSNStatus() {
  return useQuery<PSNStatusResponse, Error>({
    queryKey: syncKeys.psnStatus(),
    queryFn: syncApi.getPSNStatus,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true,
  });
}

/**
 * Hook to disconnect PSN integration.
 * Invalidates all PSN-related queries on success.
 */
export function useDisconnectPSN() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectPSN,
    onSuccess: () => {
      // Invalidate all PSN-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.PSN) });
      queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
    },
    onError: (error) => {
      console.error('Failed to disconnect PSN:', error);
    },
  });
}
```

**Step 6: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test use-sync-psn.test.ts`

Expected: PASS - All PSN hook tests pass

**Step 7: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 8: Commit**

```bash
git add frontend/src/hooks/use-sync.ts frontend/src/hooks/use-sync-psn.test.ts
git commit -m "feat(frontend): add PSN React Query hooks

- Add useConfigurePSN for token verification
- Add usePSNStatus for connection status with caching
- Add useDisconnectPSN for disconnection
- Add PSN query keys to syncKeys
- Follow Steam/Epic hook patterns
- Add comprehensive hook tests

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Create PSN Connection Card Component

Create PSN connection card following Steam pattern with NPSSO token input.

**Files:**
- Create: `frontend/src/components/sync/psn-connection-card.tsx`
- Create: `frontend/src/components/sync/psn-connection-card.test.tsx`
- Modify: `frontend/src/components/sync/index.ts`

**Step 1: Write failing test for PSN connection card**

Create test file: `frontend/src/components/sync/psn-connection-card.test.tsx`

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { PSNConnectionCard } from './psn-connection-card';
import * as hooks from '@/hooks';

vi.mock('@/hooks');

describe('PSNConnectionCard', () => {
  it('should render not configured state', () => {
    vi.mocked(hooks.usePSNStatus).mockReturnValue({
      data: {
        isConfigured: false,
        onlineId: null,
        accountId: null,
        region: null,
        tokenExpired: false,
      },
      isLoading: false,
      error: null,
    } as any);

    vi.mocked(hooks.useConfigurePSN).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as any);

    render(<PSNConnectionCard onConnectionChange={vi.fn()} />);

    expect(screen.getByText('PlayStation Network')).toBeInTheDocument();
    expect(screen.getByText('Not Configured')).toBeInTheDocument();
    expect(screen.getByLabelText('NPSSO Token')).toBeInTheDocument();
  });

  it('should render configured state', () => {
    vi.mocked(hooks.usePSNStatus).mockReturnValue({
      data: {
        isConfigured: true,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        tokenExpired: false,
      },
      isLoading: false,
      error: null,
    } as any);

    vi.mocked(hooks.useDisconnectPSN).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as any);

    render(<PSNConnectionCard onConnectionChange={vi.fn()} />);

    expect(screen.getByText('Connected as testuser')).toBeInTheDocument();
    expect(screen.getByText('test-account-id')).toBeInTheDocument();
    expect(screen.getByText('Disconnect')).toBeInTheDocument();
  });

  it('should handle NPSSO token configuration', async () => {
    const user = userEvent.setup();
    const mockMutate = vi.fn();
    const mockOnConnectionChange = vi.fn();

    vi.mocked(hooks.usePSNStatus).mockReturnValue({
      data: {
        isConfigured: false,
        onlineId: null,
        accountId: null,
        region: null,
        tokenExpired: false,
      },
      isLoading: false,
      error: null,
    } as any);

    vi.mocked(hooks.useConfigurePSN).mockReturnValue({
      mutate: mockMutate,
      mutateAsync: vi.fn().mockResolvedValue({
        success: true,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        message: 'PSN configured successfully',
      }),
      isPending: false,
    } as any);

    render(<PSNConnectionCard onConnectionChange={mockOnConnectionChange} />);

    const tokenInput = screen.getByLabelText('NPSSO Token');
    await user.type(tokenInput, 'a'.repeat(64));

    const submitButton = screen.getByRole('button', { name: /verify & connect/i });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });
  });

  it('should show token expired warning', () => {
    vi.mocked(hooks.usePSNStatus).mockReturnValue({
      data: {
        isConfigured: false,
        onlineId: 'testuser',
        accountId: 'test-account-id',
        region: 'us',
        tokenExpired: true,
      },
      isLoading: false,
      error: null,
    } as any);

    vi.mocked(hooks.useConfigurePSN).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as any);

    render(<PSNConnectionCard onConnectionChange={vi.fn()} />);

    expect(screen.getByText(/token.*expired/i)).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test psn-connection-card.test.tsx`

Expected: FAIL - "Cannot find module './psn-connection-card'"

**Step 3: Create PSN connection card component**

Create file: `frontend/src/components/sync/psn-connection-card.tsx`

```typescript
'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
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
import { Loader2, Check, ExternalLink, AlertTriangle } from 'lucide-react';
import { useConfigurePSN, useDisconnectPSN, usePSNStatus } from '@/hooks';
import { PSN_CONFIG_ERROR_MESSAGES } from '@/types';

const psnCredentialsSchema = z.object({
  npssoToken: z
    .string()
    .length(64, 'NPSSO token must be exactly 64 characters')
    .regex(/^[A-Za-z0-9]+$/, 'Invalid NPSSO token format'),
});

type PSNCredentialsForm = z.infer<typeof psnCredentialsSchema>;

interface PSNConnectionCardProps {
  onConnectionChange: () => void;
}

export function PSNConnectionCard({ onConnectionChange }: PSNConnectionCardProps) {
  const [verifiedOnlineId, setVerifiedOnlineId] = useState<string | null>(null);

  const { data: status, isLoading: isLoadingStatus } = usePSNStatus();
  const configureMutation = useConfigurePSN();
  const disconnectMutation = useDisconnectPSN();

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<PSNCredentialsForm>({
    resolver: zodResolver(psnCredentialsSchema),
  });

  const isConfiguring = configureMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;
  const isConfigured = status?.isConfigured ?? false;
  const tokenExpired = status?.tokenExpired ?? false;

  const onSubmit = async (data: PSNCredentialsForm) => {
    try {
      const result = await configureMutation.mutateAsync(data.npssoToken);

      if (!result.success) {
        toast.error('Failed to configure PSN');
        return;
      }

      setVerifiedOnlineId(result.onlineId);
      toast.success('PlayStation Network connected successfully');
      onConnectionChange();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to connect PSN';

      // Check for specific error types
      if (errorMessage.includes('64 characters')) {
        setError('npssoToken', { message: PSN_CONFIG_ERROR_MESSAGES.invalid_token_format });
      } else if (errorMessage.includes('Invalid') || errorMessage.includes('invalid')) {
        setError('npssoToken', { message: PSN_CONFIG_ERROR_MESSAGES.invalid_token });
      } else {
        toast.error(errorMessage);
      }
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('PlayStation Network disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect PSN');
    }
  };

  const getBadgeState = () => {
    if (tokenExpired) {
      return { label: 'Token Expired', className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' };
    }
    if (!isConfigured) {
      return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
    }
    return { label: 'Connected', className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' };
  };

  const badgeState = getBadgeState();

  if (isLoadingStatus) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>PlayStation Network</CardTitle>
              <CardDescription>Loading...</CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>PlayStation Network</CardTitle>
            <CardDescription>
              {isConfigured
                ? 'Your PlayStation Network account is connected'
                : 'Connect your PlayStation Network account to sync your game library'}
            </CardDescription>
          </div>
          <Badge variant="outline" className={badgeState.className}>
            {badgeState.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        {tokenExpired && (
          <Alert variant="destructive" className="mb-4">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              Your NPSSO token has expired. Please enter a new token to continue syncing.
            </AlertDescription>
          </Alert>
        )}

        {isConfigured && !tokenExpired ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
              <Check className="h-5 w-5 text-green-600" />
              <div>
                <p className="font-medium">
                  Connected as {status?.onlineId || verifiedOnlineId}
                </p>
                {status?.accountId && (
                  <p className="text-sm text-muted-foreground">{status.accountId}</p>
                )}
                {status?.region && (
                  <p className="text-sm text-muted-foreground">Region: {status.region}</p>
                )}
              </div>
            </div>

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
                  <AlertDialogTitle>Disconnect PlayStation Network?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Your sync settings will be preserved but syncing will stop until you reconnect.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDisconnect}>
                    Disconnect
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="npssoToken">NPSSO Token</Label>
              <Input
                id="npssoToken"
                type="password"
                placeholder="64-character NPSSO token"
                {...register('npssoToken')}
                disabled={isConfiguring}
              />
              {errors.npssoToken && (
                <p className="text-sm text-destructive">{errors.npssoToken.message}</p>
              )}

              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="npsso-help" className="border-none">
                  <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                    How do I get my NPSSO token?
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground">
                    <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                      <p className="font-medium text-foreground">
                        Your NPSSO token is a 64-character authentication token from PlayStation.
                      </p>
                      <ol className="list-inside list-decimal space-y-1">
                        <li>Sign in to{' '}
                          <a
                            href="https://www.playstation.com"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            PlayStation.com <ExternalLink className="inline h-3 w-3" />
                          </a>
                          {' '}with your PSN account
                        </li>
                        <li>
                          Visit{' '}
                          <a
                            href="https://ca.account.sony.com/api/v1/ssocookie"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            this page <ExternalLink className="inline h-3 w-3" />
                          </a>
                        </li>
                        <li>Copy the 64-character <code>npsso</code> value</li>
                        <li>Paste it into the field above</li>
                      </ol>
                      <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                        <strong>Important:</strong> Treat your NPSSO token like a password. It grants access to your PSN account.
                        <br />
                        <strong>Token Lifespan:</strong> NPSSO tokens expire after about 2 months. You&apos;ll need to update it when it expires.
                      </div>
                      <div className="mt-2 rounded border border-blue-200 bg-blue-50 p-2 text-blue-800 dark:border-blue-800 dark:bg-blue-900/20 dark:text-blue-200">
                        <strong>Note:</strong> Only PS4 and PS5 games will be synced. PS3 games cannot be synced automatically due to PSN API limitations, but you can add them manually.
                      </div>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>

            <Button type="submit" disabled={isConfiguring} className="w-full">
              {isConfiguring ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Verifying...
                </>
              ) : (
                'Verify & Connect'
              )}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
```

**Step 4: Export PSN connection card from index**

Edit [frontend/src/components/sync/index.ts](frontend/src/components/sync/index.ts):

```typescript
export { SteamConnectionCard } from './steam-connection-card';
export { EpicConnectionCard } from './epic-connection-card';
export { PSNConnectionCard } from './psn-connection-card';
export { SyncServiceCard } from './sync-service-card';
export { RecentActivity } from './recent-activity';
```

**Step 5: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test psn-connection-card.test.tsx`

Expected: PASS - All PSN connection card tests pass

**Step 6: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 7: Commit**

```bash
git add frontend/src/components/sync/psn-connection-card.tsx frontend/src/components/sync/psn-connection-card.test.tsx frontend/src/components/sync/index.ts
git commit -m "feat(frontend): add PSN connection card component

- Create PSNConnectionCard following Steam pattern
- Add NPSSO token input with validation (64 chars)
- Add configuration/connected/expired states
- Add comprehensive help text with PSN limitations
- Add token expiration warning
- Add disconnect confirmation dialog
- Add comprehensive component tests
- Export from sync components index

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Add PSN Card to Settings Page

Integrate PSN connection card into settings page alongside Steam and Epic.

**Files:**
- Find and modify: Settings page component (need to locate first)

**Step 1: Locate settings page**

Run: `find /home/abo/workspace/home/nexorious/frontend/src -name "*settings*" -o -name "*profile*" | grep -E "\.(tsx|ts)$"`

Expected: Find settings page file path

**Step 2: Read settings page to understand structure**

Read the settings page file found in step 1

**Step 3: Write test for PSN card in settings**

Add test case to existing settings page test file (or create if doesn't exist):

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
// Import settings page component

vi.mock('@/hooks');

describe('Settings Page - PSN Integration', () => {
  it('should render PSN connection card', () => {
    const queryClient = new QueryClient();

    vi.mocked(hooks.usePSNStatus).mockReturnValue({
      data: {
        isConfigured: false,
        onlineId: null,
        accountId: null,
        region: null,
        tokenExpired: false,
      },
      isLoading: false,
      error: null,
    } as any);

    render(
      <QueryClientProvider client={queryClient}>
        {/* Settings page component */}
      </QueryClientProvider>
    );

    expect(screen.getByText('PlayStation Network')).toBeInTheDocument();
  });
});
```

**Step 4: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test <settings-test-file>`

Expected: FAIL - PSN card not found

**Step 5: Add PSN card to settings page**

Import PSNConnectionCard and add it alongside Steam and Epic cards:

```typescript
import { PSNConnectionCard } from '@/components/sync';

// In the settings page component, add PSN card:
<PSNConnectionCard onConnectionChange={() => {
  // Refresh sync configs or status as needed
}} />
```

**Step 6: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test <settings-test-file>`

Expected: PASS - PSN card renders in settings

**Step 7: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 8: Commit**

```bash
git add <settings-page-files>
git commit -m "feat(frontend): add PSN card to settings page

- Add PSNConnectionCard to settings page
- Position alongside Steam and Epic cards
- Wire up connection change handler
- Add settings page test for PSN card

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 6: Update Sync Page for PSN

Ensure PSN sync config appears on sync page automatically (should work via SUPPORTED_SYNC_PLATFORMS).

**Files:**
- Verify: [frontend/src/app/(main)/sync/page.tsx](frontend/src/app/(main)/sync/page.tsx:1-203)

**Step 1: Write test for PSN on sync page**

Add test to [frontend/src/app/(main)/sync/page.test.tsx](frontend/src/app/(main)/sync/page.test.tsx):

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import SyncPage from './page';
import * as hooks from '@/hooks';
import { SyncPlatform, SyncFrequency } from '@/types';

vi.mock('@/hooks');
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}));

describe('SyncPage - PSN', () => {
  it('should render PSN sync card when configured', () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    vi.mocked(hooks.useSyncConfigs).mockReturnValue({
      data: {
        configs: [
          {
            id: 'psn-config-id',
            userId: 'user-id',
            platform: SyncPlatform.PSN,
            frequency: SyncFrequency.MANUAL,
            autoAdd: false,
            lastSyncedAt: null,
            createdAt: '2026-01-03T00:00:00Z',
            updatedAt: '2026-01-03T00:00:00Z',
            isConfigured: true,
          },
        ],
        total: 1,
      },
      isLoading: false,
      error: null,
    } as any);

    vi.mocked(hooks.useSyncStatus).mockReturnValue({
      data: {
        platform: SyncPlatform.PSN,
        isSyncing: false,
        lastSyncedAt: null,
        activeJobId: null,
      },
      isLoading: false,
      error: null,
    } as any);

    vi.mocked(hooks.usePendingReviewCount).mockReturnValue({
      data: { countsBySource: {} },
    } as any);

    vi.mocked(hooks.useUpdateSyncConfig).mockReturnValue({
      isPending: false,
    } as any);

    vi.mocked(hooks.useTriggerSync).mockReturnValue({
      isPending: false,
    } as any);

    render(
      <QueryClientProvider client={queryClient}>
        <SyncPage />
      </QueryClientProvider>
    );

    expect(screen.getByText(/PlayStation Network/i)).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify current behavior**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test sync/page.test.tsx`

Expected: Should PASS if SUPPORTED_SYNC_PLATFORMS already includes PSN (from Task 1)

**Step 3: Verify sync page doesn't need changes**

The sync page at [frontend/src/app/(main)/sync/page.tsx](frontend/src/app/(main)/sync/page.tsx:135-147) uses `SUPPORTED_SYNC_PLATFORMS` to filter configs, so PSN should automatically appear once it's added to the enum.

No code changes needed if Task 1 is complete.

**Step 4: Update page description to mention PSN**

Edit [frontend/src/app/(main)/sync/page.tsx](frontend/src/app/(main)/sync/page.tsx:117-121):

```typescript
<p className="text-muted-foreground">
  Sync your Steam, Epic Games, and PlayStation Network libraries with Nexorious.
</p>
```

**Step 5: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test sync/page.test.tsx`

Expected: PASS - PSN sync card renders

**Step 6: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 7: Commit**

```bash
git add frontend/src/app/\(main\)/sync/page.tsx frontend/src/app/\(main\)/sync/page.test.tsx
git commit -m "feat(frontend): add PSN support to sync page

- Update sync page description to mention PSN
- PSN automatically appears via SUPPORTED_SYNC_PLATFORMS
- Add test for PSN sync card rendering
- No structural changes needed (generic platform handling)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 7: Export PSN Hooks from Index

Add PSN hooks to hooks barrel export for easy importing.

**Files:**
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Locate hooks index file**

Run: `find /home/abo/workspace/home/nexorious/frontend/src/hooks -name "index.ts"`

Expected: Find hooks index file

**Step 2: Read current hooks index**

Read the hooks index file to see export pattern

**Step 3: Add PSN hooks to exports**

Edit `frontend/src/hooks/index.ts`:

```typescript
// ... existing exports
export {
  // ... existing exports
  useConfigurePSN,
  usePSNStatus,
  useDisconnectPSN,
} from './use-sync';
```

**Step 4: Verify imports work**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 5: Commit**

```bash
git add frontend/src/hooks/index.ts
git commit -m "feat(frontend): export PSN hooks from hooks index

- Export useConfigurePSN, usePSNStatus, useDisconnectPSN
- Enables convenient importing: import { useConfigurePSN } from '@/hooks'

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 8: Run All Tests and Type Checks

Verify all frontend tests pass and no type errors exist.

**Step 1: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

Expected: PASS - All tests pass

**Step 2: Check test coverage**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test:coverage`

Expected: >70% coverage maintained

**Step 3: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS - No TypeScript errors

**Step 4: Run linting**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run lint`

Expected: PASS - No linting errors (or only warnings)

**Step 5: Build frontend**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run build`

Expected: SUCCESS - Build completes without errors

**Step 6: Document results**

Create summary of test results and coverage

**Step 7: Commit if any fixes were needed**

```bash
# Only if fixes were needed
git add <fixed-files>
git commit -m "fix(frontend): resolve test/type issues for PSN

- Fix any failing tests
- Resolve type errors
- Address linting issues

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 9: Integration Testing Documentation

Document manual testing steps for PSN integration.

**Files:**
- Create: `docs/testing/psn-frontend-integration-testing.md`

**Step 1: Create testing documentation**

Create file: `docs/testing/psn-frontend-integration-testing.md`

```markdown
# PSN Frontend Integration Testing

## Test Environment Setup

1. Ensure backend PSN implementation is deployed and running
2. Ensure you have a valid PSN account
3. Obtain a fresh NPSSO token from https://ca.account.sony.com/api/v1/ssocookie

## Test Cases

### TC1: PSN Configuration - Happy Path

**Steps:**
1. Navigate to Settings page
2. Locate PlayStation Network card
3. Verify card shows "Not Configured" badge
4. Click "How do I get my NPSSO token?" accordion
5. Follow instructions to obtain NPSSO token
6. Paste token into input field
7. Click "Verify & Connect"

**Expected:**
- Loading spinner appears during verification
- Success toast: "PlayStation Network connected successfully"
- Card updates to show "Connected" badge
- Connected state displays PSN Online ID, Account ID, and Region
- Disconnect button appears

### TC2: PSN Configuration - Invalid Token

**Steps:**
1. Navigate to Settings page
2. Enter invalid token (e.g., too short, wrong format)
3. Click "Verify & Connect"

**Expected:**
- Error message appears below input: "NPSSO token must be exactly 64 characters" or "Invalid NPSSO token format"
- Card remains in not configured state

### TC3: PSN Configuration - Token Expired

**Prerequisite:** PSN configured with expired token (backend mock or wait 2 months)

**Steps:**
1. Navigate to Settings page
2. Observe PSN card

**Expected:**
- Card shows "Token Expired" badge (yellow)
- Warning alert: "Your NPSSO token has expired. Please enter a new token to continue syncing."
- Input field available to enter new token
- Previous account info still visible

### TC4: PSN Sync - Manual Trigger

**Prerequisite:** PSN configured successfully

**Steps:**
1. Navigate to Sync page (/sync)
2. Locate PlayStation Network sync card
3. Click "Sync Now" button

**Expected:**
- Sync starts immediately
- Redirects to /sync/psn page
- Job progress card appears
- Games begin appearing in collection with PS4/PS5 platforms

### TC5: PSN Sync - Update Settings

**Prerequisite:** PSN configured successfully

**Steps:**
1. Navigate to Sync page (/sync)
2. Locate PlayStation Network sync card
3. Change frequency from "Manual" to "Daily"
4. Toggle "Auto-add" to ON
5. Observe sync settings

**Expected:**
- Settings update successfully
- Success toast: "Sync settings updated successfully"
- Card reflects new settings immediately

### TC6: PSN Disconnect

**Prerequisite:** PSN configured successfully

**Steps:**
1. Navigate to Settings page
2. Click "Disconnect" button on PSN card
3. Confirm disconnect in dialog

**Expected:**
- Confirmation dialog appears with warning
- After confirming, success toast: "PlayStation Network disconnected"
- Card returns to "Not Configured" state
- Sync settings preserved (still visible on sync page)

### TC7: Multi-Platform Games

**Prerequisite:** PSN configured with games that have both PS4 and PS5 versions

**Steps:**
1. Trigger PSN sync
2. Wait for sync to complete
3. Navigate to game collection
4. Find a cross-gen game (e.g., Spider-Man)
5. View game details

**Expected:**
- Game appears with BOTH "PlayStation 4" and "PlayStation 5" platform tags
- Separate entries for each platform in platforms list

### TC8: Type Safety Verification

**Steps:**
1. Open browser developer console
2. Navigate through PSN settings/sync pages
3. Trigger PSN operations
4. Monitor console for errors

**Expected:**
- Zero console errors
- Zero TypeScript errors
- Zero React errors
- All network requests succeed (or fail gracefully with user messages)

## Performance Testing

### PT1: Load Time

**Steps:**
1. Navigate to Settings page
2. Measure time to render PSN card

**Expected:**
- Card renders in <500ms
- No layout shift after hydration

### PT2: API Response Handling

**Steps:**
1. Configure PSN (network tab open)
2. Observe API response times

**Expected:**
- /sync/psn/configure responds in <2s
- /sync/psn/status responds in <500ms
- Loading states shown during requests

## Accessibility Testing

### AT1: Keyboard Navigation

**Steps:**
1. Navigate to Settings page using only keyboard
2. Tab through PSN card elements
3. Use Enter/Space to interact

**Expected:**
- All interactive elements focusable
- Focus visible (outline or highlight)
- Can complete configuration flow with keyboard only

### AT2: Screen Reader Testing

**Steps:**
1. Enable screen reader (NVDA, JAWS, or VoiceOver)
2. Navigate PSN card
3. Interact with form elements

**Expected:**
- Labels announced correctly
- Error messages announced
- Button states announced (loading, disabled)

## Edge Cases

### EC1: Network Failure During Configuration

**Steps:**
1. Disconnect internet
2. Attempt PSN configuration
3. Reconnect internet

**Expected:**
- Error toast: "Could not connect to PlayStation Network. Please try again."
- Form remains in error state
- User can retry after reconnecting

### EC2: Concurrent Configuration Attempts

**Steps:**
1. Open settings in two browser tabs
2. Configure PSN in first tab
3. Attempt to configure in second tab

**Expected:**
- Second tab detects first configuration
- Both tabs show connected state after refresh
- No duplicate configurations created

## Browser Compatibility

Test on:
- [ ] Chrome (latest)
- [ ] Firefox (latest)
- [ ] Safari (latest)
- [ ] Edge (latest)

## Mobile Testing

Test on:
- [ ] iOS Safari
- [ ] Android Chrome
- [ ] Responsive design (320px - 1920px)
```

**Step 2: Commit testing documentation**

```bash
git add docs/testing/psn-frontend-integration-testing.md
git commit -m "docs: add PSN frontend integration testing guide

- Add comprehensive test cases for PSN configuration
- Add sync testing scenarios
- Add multi-platform game testing
- Add performance and accessibility tests
- Add edge case scenarios
- Add browser compatibility checklist

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 10: Update Project Documentation

Update README and PRD with PSN frontend implementation status.

**Files:**
- Modify: `docs/PRD.md` (if exists)
- Modify: `README.md` or `frontend/README.md` (if exists)

**Step 1: Check for documentation files**

Run: `ls -la /home/abo/workspace/home/nexorious/docs/ | grep -E "(PRD|README)"`

Expected: Find PRD.md or similar docs

**Step 2: Update PRD with PSN frontend status**

Edit PRD to mark PSN frontend as implemented:

```markdown
## 6.1 Enhanced Storefront Integration

### PlayStation Network Sync
- [x] Backend implementation complete
- [x] Frontend implementation complete
- [x] NPSSO token authentication
- [x] PS4/PS5 game sync with multi-platform support
- [x] Token expiration handling
- [ ] Manual testing complete
- [ ] Production deployment
```

**Step 3: Update README with PSN feature**

Add PSN to features list in README:

```markdown
### Sync Integrations
- **Steam**: Sync your Steam library automatically
- **Epic Games**: Sync your Epic Games library
- **PlayStation Network**: Sync your PSN library (PS4/PS5 games)
```

**Step 4: Commit documentation updates**

```bash
git add docs/PRD.md README.md
git commit -m "docs: update PRD and README with PSN frontend status

- Mark PSN frontend implementation as complete
- Add PSN to features list
- Update sync integrations documentation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Success Criteria

- [ ] PSN added to SyncPlatform enum and SUPPORTED_SYNC_PLATFORMS
- [ ] PSN API functions implemented with snake_case to camelCase transformation
- [ ] PSN React Query hooks implemented with proper caching
- [ ] PSN connection card component created following Steam pattern
- [ ] PSN card integrated into settings page
- [ ] PSN automatically appears on sync page
- [ ] All TypeScript types properly defined
- [ ] All tests pass (>70% coverage maintained)
- [ ] No TypeScript errors (`npm run check` passes)
- [ ] Frontend builds successfully (`npm run build` passes)
- [ ] Token expiration handled gracefully in UI
- [ ] NPSSO token help text provides clear instructions
- [ ] PS3 limitation documented in help text
- [ ] Comprehensive testing documentation created
- [ ] Project documentation updated with PSN status

## Notes

- Follow existing Steam/Epic patterns exactly for consistency
- NPSSO token is 64 characters (alphanumeric)
- Token expires after ~2 months
- PS3 games cannot be synced (API limitation) - document this clearly
- Multi-platform games (PS4+PS5) handled automatically by backend
- Use PlayStation blue color: `#003087` for PSN branding
- All API responses use snake_case, must transform to camelCase
- Sync page already generic, should work with PSN via SUPPORTED_SYNC_PLATFORMS
