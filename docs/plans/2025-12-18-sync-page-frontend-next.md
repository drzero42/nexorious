# Sync Page (frontend-next) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a Sync page in frontend-next that allows users to view, configure, and trigger platform sync operations (Steam, Epic, GOG).

**Architecture:** Create types for sync entities, API client module for sync endpoints, React Query hooks for data fetching/mutations, and a page component with service cards showing sync configuration and controls. The page displays connected services with settings (enabled, frequency, auto-add), allows triggering manual syncs, and shows recent sync jobs.

**Tech Stack:** Next.js 16, React 19, TypeScript, React Query v5, shadcn/ui, Tailwind CSS, lucide-react icons

---

## Phase 1: Types and API Layer

### Task 1.1: Create Sync Types

**Files:**
- Create: `frontend-next/src/types/sync.ts`
- Modify: `frontend-next/src/types/index.ts`

**Step 1: Create the sync types file**

```typescript
// frontend-next/src/types/sync.ts
/**
 * Types for sync configuration and status management.
 */

export enum SyncPlatform {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
}

export enum SyncFrequency {
  MANUAL = 'manual',
  HOURLY = 'hourly',
  DAILY = 'daily',
  WEEKLY = 'weekly',
}

export interface SyncConfig {
  id: string;
  userId: string;
  platform: SyncPlatform;
  frequency: SyncFrequency;
  autoAdd: boolean;
  enabled: boolean;
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SyncConfigUpdateData {
  frequency?: SyncFrequency;
  autoAdd?: boolean;
  enabled?: boolean;
}

export interface SyncStatus {
  platform: SyncPlatform;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
}

export interface ManualSyncResponse {
  message: string;
  jobId: string;
  platform: string;
  status: string;
}

export interface IgnoredGame {
  id: string;
  source: string;
  externalId: string;
  title: string;
  createdAt: string;
}

// Helper to get human-readable frequency label
export function getSyncFrequencyLabel(frequency: SyncFrequency): string {
  const labels: Record<SyncFrequency, string> = {
    [SyncFrequency.MANUAL]: 'Manual',
    [SyncFrequency.HOURLY]: 'Every hour',
    [SyncFrequency.DAILY]: 'Daily',
    [SyncFrequency.WEEKLY]: 'Weekly',
  };
  return labels[frequency];
}

// Helper to get platform display info
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
  };
  return info[platform];
}
```

**Step 2: Export from types index**

In `frontend-next/src/types/index.ts`, add:

```typescript
export * from './sync';
```

**Step 3: Run type check**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend-next/src/types/sync.ts frontend-next/src/types/index.ts
git commit -m "feat(types): add sync types for frontend-next"
```

---

### Task 1.2: Create Sync API Module

**Files:**
- Create: `frontend-next/src/api/sync.ts`
- Modify: `frontend-next/src/api/index.ts`

**Step 1: Create the sync API module**

```typescript
// frontend-next/src/api/sync.ts
import { api } from './client';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  IgnoredGame,
  SyncPlatform,
  SyncFrequency,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface SyncConfigApiResponse {
  id: string;
  user_id: string;
  platform: string;
  frequency: string;
  auto_add: boolean;
  enabled: boolean;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
}

interface SyncConfigListApiResponse {
  configs: SyncConfigApiResponse[];
  total: number;
}

interface SyncStatusApiResponse {
  platform: string;
  is_syncing: boolean;
  last_synced_at: string | null;
  active_job_id: string | null;
}

interface ManualSyncApiResponse {
  message: string;
  job_id: string;
  platform: string;
  status: string;
}

interface IgnoredGameApiResponse {
  id: string;
  source: string;
  external_id: string;
  title: string;
  created_at: string;
}

interface IgnoredGameListApiResponse {
  items: IgnoredGameApiResponse[];
  total: number;
}

// ============================================================================
// Response Types
// ============================================================================

export interface SyncConfigsResponse {
  configs: SyncConfig[];
  total: number;
}

export interface IgnoredGamesResponse {
  items: IgnoredGame[];
  total: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformSyncConfig(apiConfig: SyncConfigApiResponse): SyncConfig {
  return {
    id: apiConfig.id,
    userId: apiConfig.user_id,
    platform: apiConfig.platform as SyncPlatform,
    frequency: apiConfig.frequency as SyncFrequency,
    autoAdd: apiConfig.auto_add,
    enabled: apiConfig.enabled,
    lastSyncedAt: apiConfig.last_synced_at,
    createdAt: apiConfig.created_at,
    updatedAt: apiConfig.updated_at,
  };
}

function transformSyncStatus(apiStatus: SyncStatusApiResponse): SyncStatus {
  return {
    platform: apiStatus.platform as SyncPlatform,
    isSyncing: apiStatus.is_syncing,
    lastSyncedAt: apiStatus.last_synced_at,
    activeJobId: apiStatus.active_job_id,
  };
}

function transformManualSyncResponse(apiResponse: ManualSyncApiResponse): ManualSyncResponse {
  return {
    message: apiResponse.message,
    jobId: apiResponse.job_id,
    platform: apiResponse.platform,
    status: apiResponse.status,
  };
}

function transformIgnoredGame(apiGame: IgnoredGameApiResponse): IgnoredGame {
  return {
    id: apiGame.id,
    source: apiGame.source,
    externalId: apiGame.external_id,
    title: apiGame.title,
    createdAt: apiGame.created_at,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get all sync configurations for the current user.
 */
export async function getSyncConfigs(): Promise<SyncConfigsResponse> {
  const response = await api.get<SyncConfigListApiResponse>('/sync/config');
  return {
    configs: response.configs.map(transformSyncConfig),
    total: response.total,
  };
}

/**
 * Get sync configuration for a specific platform.
 */
export async function getSyncConfig(platform: SyncPlatform): Promise<SyncConfig> {
  const response = await api.get<SyncConfigApiResponse>(`/sync/config/${platform}`);
  return transformSyncConfig(response);
}

/**
 * Update sync configuration for a specific platform.
 */
export async function updateSyncConfig(
  platform: SyncPlatform,
  data: SyncConfigUpdateData
): Promise<SyncConfig> {
  const requestBody: Record<string, unknown> = {};

  if (data.frequency !== undefined) {
    requestBody.frequency = data.frequency;
  }
  if (data.autoAdd !== undefined) {
    requestBody.auto_add = data.autoAdd;
  }
  if (data.enabled !== undefined) {
    requestBody.enabled = data.enabled;
  }

  const response = await api.put<SyncConfigApiResponse>(
    `/sync/config/${platform}`,
    requestBody
  );
  return transformSyncConfig(response);
}

/**
 * Trigger a manual sync for a specific platform.
 */
export async function triggerSync(platform: SyncPlatform): Promise<ManualSyncResponse> {
  const response = await api.post<ManualSyncApiResponse>(`/sync/${platform}`);
  return transformManualSyncResponse(response);
}

/**
 * Get the current sync status for a platform.
 */
export async function getSyncStatus(platform: SyncPlatform): Promise<SyncStatus> {
  const response = await api.get<SyncStatusApiResponse>(`/sync/${platform}/status`);
  return transformSyncStatus(response);
}

/**
 * Get ignored games list with optional filtering.
 */
export async function getIgnoredGames(params?: {
  source?: string;
  limit?: number;
  offset?: number;
}): Promise<IgnoredGamesResponse> {
  const queryParams: Record<string, string | number> = {};
  if (params?.source) queryParams.source = params.source;
  if (params?.limit) queryParams.limit = params.limit;
  if (params?.offset) queryParams.offset = params.offset;

  const response = await api.get<IgnoredGameListApiResponse>('/sync/ignored', {
    params: queryParams,
  });

  return {
    items: response.items.map(transformIgnoredGame),
    total: response.total,
  };
}

/**
 * Remove a game from the ignored list.
 */
export async function unignoreGame(id: string): Promise<void> {
  await api.delete(`/sync/ignored/${id}`);
}
```

**Step 2: Export from api index**

In `frontend-next/src/api/index.ts`, add:

```typescript
export * as syncApi from './sync';
```

**Step 3: Run type check**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend-next/src/api/sync.ts frontend-next/src/api/index.ts
git commit -m "feat(api): add sync API module for frontend-next"
```

---

### Task 1.3: Create Sync API Tests

**Files:**
- Create: `frontend-next/src/api/sync.test.ts`

**Step 1: Create the test file**

```typescript
// frontend-next/src/api/sync.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as syncApi from './sync';
import { api } from './client';
import { SyncPlatform, SyncFrequency } from '@/types';

vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    put: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('syncApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getSyncConfigs', () => {
    it('should fetch and transform sync configs', async () => {
      const mockResponse = {
        configs: [
          {
            id: '1',
            user_id: 'user-1',
            platform: 'steam',
            frequency: 'daily',
            auto_add: true,
            enabled: true,
            last_synced_at: '2025-01-01T00:00:00Z',
            created_at: '2025-01-01T00:00:00Z',
            updated_at: '2025-01-01T00:00:00Z',
          },
        ],
        total: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncConfigs();

      expect(api.get).toHaveBeenCalledWith('/sync/config');
      expect(result.configs[0].platform).toBe(SyncPlatform.STEAM);
      expect(result.configs[0].frequency).toBe(SyncFrequency.DAILY);
      expect(result.configs[0].autoAdd).toBe(true);
      expect(result.configs[0].userId).toBe('user-1');
    });
  });

  describe('updateSyncConfig', () => {
    it('should update sync config with correct snake_case params', async () => {
      const mockResponse = {
        id: '1',
        user_id: 'user-1',
        platform: 'steam',
        frequency: 'weekly',
        auto_add: false,
        enabled: true,
        last_synced_at: null,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.updateSyncConfig(SyncPlatform.STEAM, {
        frequency: SyncFrequency.WEEKLY,
        autoAdd: false,
      });

      expect(api.put).toHaveBeenCalledWith('/sync/config/steam', {
        frequency: 'weekly',
        auto_add: false,
      });
      expect(result.frequency).toBe(SyncFrequency.WEEKLY);
      expect(result.autoAdd).toBe(false);
    });
  });

  describe('triggerSync', () => {
    it('should trigger sync and return job info', async () => {
      const mockResponse = {
        message: 'Sync started',
        job_id: 'job-123',
        platform: 'steam',
        status: 'queued',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.triggerSync(SyncPlatform.STEAM);

      expect(api.post).toHaveBeenCalledWith('/sync/steam');
      expect(result.jobId).toBe('job-123');
      expect(result.platform).toBe('steam');
    });
  });

  describe('getSyncStatus', () => {
    it('should fetch and transform sync status', async () => {
      const mockResponse = {
        platform: 'steam',
        is_syncing: true,
        last_synced_at: '2025-01-01T00:00:00Z',
        active_job_id: 'job-123',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncStatus(SyncPlatform.STEAM);

      expect(api.get).toHaveBeenCalledWith('/sync/steam/status');
      expect(result.isSyncing).toBe(true);
      expect(result.activeJobId).toBe('job-123');
    });
  });

  describe('getIgnoredGames', () => {
    it('should fetch ignored games with filters', async () => {
      const mockResponse = {
        items: [
          {
            id: 'ignored-1',
            source: 'STEAM',
            external_id: '12345',
            title: 'Some Game',
            created_at: '2025-01-01T00:00:00Z',
          },
        ],
        total: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getIgnoredGames({ source: 'STEAM', limit: 10 });

      expect(api.get).toHaveBeenCalledWith('/sync/ignored', {
        params: { source: 'STEAM', limit: 10 },
      });
      expect(result.items[0].externalId).toBe('12345');
      expect(result.total).toBe(1);
    });
  });

  describe('unignoreGame', () => {
    it('should delete ignored game', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await syncApi.unignoreGame('ignored-1');

      expect(api.delete).toHaveBeenCalledWith('/sync/ignored/ignored-1');
    });
  });
});
```

**Step 2: Run tests**

Run: `cd frontend-next && npm run test -- src/api/sync.test.ts`
Expected: All tests pass

**Step 3: Commit**

```bash
git add frontend-next/src/api/sync.test.ts
git commit -m "test(api): add sync API tests"
```

---

## Phase 2: React Query Hooks

### Task 2.1: Create Sync Hooks

**Files:**
- Create: `frontend-next/src/hooks/use-sync.ts`
- Modify: `frontend-next/src/hooks/index.ts`

**Step 1: Create the sync hooks file**

```typescript
// frontend-next/src/hooks/use-sync.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as syncApi from '@/api/sync';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SyncPlatform,
} from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncPlatform) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncPlatform) => [...syncKeys.statuses(), platform] as const,
  ignoredGames: (params?: { source?: string }) => [...syncKeys.all, 'ignored', params] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch all sync configurations for the current user.
 */
export function useSyncConfigs() {
  return useQuery({
    queryKey: syncKeys.configs(),
    queryFn: () => syncApi.getSyncConfigs(),
  });
}

/**
 * Hook to fetch sync configuration for a specific platform.
 */
export function useSyncConfig(platform: SyncPlatform) {
  return useQuery({
    queryKey: syncKeys.config(platform),
    queryFn: () => syncApi.getSyncConfig(platform),
  });
}

/**
 * Hook to fetch sync status for a specific platform.
 */
export function useSyncStatus(platform: SyncPlatform) {
  return useQuery({
    queryKey: syncKeys.status(platform),
    queryFn: () => syncApi.getSyncStatus(platform),
    refetchInterval: (query) => {
      // Poll every 5 seconds if syncing is in progress
      const data = query.state.data as SyncStatus | undefined;
      return data?.isSyncing ? 5000 : false;
    },
  });
}

/**
 * Hook to fetch all sync statuses for all platforms.
 * Returns a map of platform -> status for easy lookup.
 */
export function useSyncStatuses() {
  const platforms = Object.values(SyncPlatform) as SyncPlatform[];

  const queries = platforms.map(platform => ({
    queryKey: syncKeys.status(platform),
    queryFn: () => syncApi.getSyncStatus(platform),
    refetchInterval: 10000, // Poll every 10 seconds
  }));

  // Return individual queries for each platform
  return {
    steam: useQuery(queries[0]),
    epic: useQuery(queries[1]),
    gog: useQuery(queries[2]),
  };
}

/**
 * Hook to fetch ignored games list.
 */
export function useIgnoredGames(params?: { source?: string; limit?: number; offset?: number }) {
  return useQuery({
    queryKey: syncKeys.ignoredGames(params),
    queryFn: () => syncApi.getIgnoredGames(params),
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to update sync configuration for a platform.
 */
export function useUpdateSyncConfig() {
  const queryClient = useQueryClient();

  return useMutation<
    SyncConfig,
    Error,
    { platform: SyncPlatform; data: SyncConfigUpdateData }
  >({
    mutationFn: ({ platform, data }) => syncApi.updateSyncConfig(platform, data),
    onSuccess: (updatedConfig, { platform }) => {
      // Update the specific config in cache
      queryClient.setQueryData(syncKeys.config(platform), updatedConfig);
      // Invalidate the configs list to refetch
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
    },
  });
}

/**
 * Hook to trigger a manual sync for a platform.
 */
export function useTriggerSync() {
  const queryClient = useQueryClient();

  return useMutation<ManualSyncResponse, Error, SyncPlatform>({
    mutationFn: (platform) => syncApi.triggerSync(platform),
    onSuccess: (_result, platform) => {
      // Invalidate status to show syncing state
      queryClient.invalidateQueries({ queryKey: syncKeys.status(platform) });
    },
  });
}

/**
 * Hook to remove a game from the ignored list.
 */
export function useUnignoreGame() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, string>({
    mutationFn: (id) => syncApi.unignoreGame(id),
    onSuccess: () => {
      // Invalidate ignored games list
      queryClient.invalidateQueries({ queryKey: syncKeys.ignoredGames() });
    },
  });
}
```

**Step 2: Export from hooks index**

In `frontend-next/src/hooks/index.ts`, add:

```typescript
export * from './use-sync';
```

**Step 3: Run type check**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend-next/src/hooks/use-sync.ts frontend-next/src/hooks/index.ts
git commit -m "feat(hooks): add sync React Query hooks"
```

---

### Task 2.2: Create Sync Hooks Tests

**Files:**
- Create: `frontend-next/src/hooks/use-sync.test.ts`

**Step 1: Create the test file**

```typescript
// frontend-next/src/hooks/use-sync.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useSyncConfigs, useUpdateSyncConfig, useTriggerSync, syncKeys } from './use-sync';
import * as syncApi from '@/api/sync';
import { SyncPlatform, SyncFrequency } from '@/types';
import type { ReactNode } from 'react';

vi.mock('@/api/sync');

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    );
  };
}

describe('useSyncConfigs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch sync configs', async () => {
    const mockConfigs = {
      configs: [
        {
          id: '1',
          userId: 'user-1',
          platform: SyncPlatform.STEAM,
          frequency: SyncFrequency.DAILY,
          autoAdd: true,
          enabled: true,
          lastSyncedAt: null,
          createdAt: '2025-01-01T00:00:00Z',
          updatedAt: '2025-01-01T00:00:00Z',
        },
      ],
      total: 1,
    };

    vi.mocked(syncApi.getSyncConfigs).mockResolvedValueOnce(mockConfigs);

    const { result } = renderHook(() => useSyncConfigs(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.configs).toHaveLength(1);
    expect(result.current.data?.configs[0].platform).toBe(SyncPlatform.STEAM);
  });
});

describe('useUpdateSyncConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should update sync config', async () => {
    const mockUpdatedConfig = {
      id: '1',
      userId: 'user-1',
      platform: SyncPlatform.STEAM,
      frequency: SyncFrequency.WEEKLY,
      autoAdd: false,
      enabled: true,
      lastSyncedAt: null,
      createdAt: '2025-01-01T00:00:00Z',
      updatedAt: '2025-01-01T00:00:00Z',
    };

    vi.mocked(syncApi.updateSyncConfig).mockResolvedValueOnce(mockUpdatedConfig);

    const { result } = renderHook(() => useUpdateSyncConfig(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({
      platform: SyncPlatform.STEAM,
      data: { frequency: SyncFrequency.WEEKLY },
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(syncApi.updateSyncConfig).toHaveBeenCalledWith(SyncPlatform.STEAM, {
      frequency: SyncFrequency.WEEKLY,
    });
  });
});

describe('useTriggerSync', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should trigger sync', async () => {
    const mockResponse = {
      message: 'Sync started',
      jobId: 'job-123',
      platform: 'steam',
      status: 'queued',
    };

    vi.mocked(syncApi.triggerSync).mockResolvedValueOnce(mockResponse);

    const { result } = renderHook(() => useTriggerSync(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(SyncPlatform.STEAM);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(syncApi.triggerSync).toHaveBeenCalledWith(SyncPlatform.STEAM);
    expect(result.current.data?.jobId).toBe('job-123');
  });
});

describe('syncKeys', () => {
  it('should generate correct query keys', () => {
    expect(syncKeys.all).toEqual(['sync']);
    expect(syncKeys.configs()).toEqual(['sync', 'configs']);
    expect(syncKeys.config(SyncPlatform.STEAM)).toEqual(['sync', 'configs', 'steam']);
    expect(syncKeys.status(SyncPlatform.GOG)).toEqual(['sync', 'statuses', 'gog']);
  });
});
```

**Step 2: Run tests**

Run: `cd frontend-next && npm run test -- src/hooks/use-sync.test.ts`
Expected: All tests pass

**Step 3: Commit**

```bash
git add frontend-next/src/hooks/use-sync.test.ts
git commit -m "test(hooks): add sync hooks tests"
```

---

## Phase 3: UI Components

### Task 3.1: Create SyncServiceCard Component

**Files:**
- Create: `frontend-next/src/components/sync/sync-service-card.tsx`

**Step 1: Create the component**

```typescript
// frontend-next/src/components/sync/sync-service-card.tsx
'use client';

import { useState } from 'react';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Loader2, RefreshCw, History } from 'lucide-react';
import Link from 'next/link';
import type { SyncConfig, SyncStatus, SyncConfigUpdateData } from '@/types';
import { SyncFrequency, SyncPlatform, getSyncFrequencyLabel, getPlatformDisplayInfo } from '@/types';

// Platform icons as SVG paths
const PLATFORM_ICONS: Record<SyncPlatform, string> = {
  [SyncPlatform.STEAM]:
    'M12 2C6.477 2 2 6.477 2 12c0 4.991 3.657 9.128 8.438 9.879V14.89h-2.54V12h2.54V9.797c0-2.506 1.492-3.89 3.777-3.89 1.094 0 2.238.195 2.238.195v2.46h-1.26c-1.243 0-1.63.771-1.63 1.562V12h2.773l-.443 2.89h-2.33v6.989C18.343 21.129 22 16.99 22 12c0-5.523-4.477-10-10-10z',
  [SyncPlatform.EPIC]:
    'M3 3h18v18H3V3zm2 2v14h14V5H5zm3 3h2v8H8V8zm4 0h4v2h-4v2h3v2h-3v2h4v2H12V8z',
  [SyncPlatform.GOG]:
    'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm-2-8c0-1.1.9-2 2-2s2 .9 2 2-.9 2-2 2-2-.9-2-2z',
};

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
}

function formatLastSync(dateStr: string | null): string {
  if (!dateStr) return 'Never';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

export function SyncServiceCard({
  config,
  status,
  onUpdate,
  onTriggerSync,
  isUpdating = false,
  isSyncing = false,
}: SyncServiceCardProps) {
  const [localEnabled, setLocalEnabled] = useState(config.enabled);
  const [localFrequency, setLocalFrequency] = useState(config.frequency);
  const [localAutoAdd, setLocalAutoAdd] = useState(config.autoAdd);

  const platformInfo = getPlatformDisplayInfo(config.platform);
  const isCurrentlySyncing = isSyncing || status?.isSyncing;

  const handleEnabledChange = async (enabled: boolean) => {
    setLocalEnabled(enabled);
    await onUpdate({ enabled });
  };

  const handleFrequencyChange = async (frequency: SyncFrequency) => {
    setLocalFrequency(frequency);
    await onUpdate({ frequency });
  };

  const handleAutoAddChange = async (autoAdd: boolean) => {
    setLocalAutoAdd(autoAdd);
    await onUpdate({ autoAdd });
  };

  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className={`flex h-12 w-12 items-center justify-center rounded-lg ${platformInfo.bgColor}`}
            >
              <svg
                className={`h-7 w-7 ${platformInfo.color}`}
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <path d={PLATFORM_ICONS[config.platform]} />
              </svg>
            </div>
            <div>
              <CardTitle className="text-lg">{platformInfo.name}</CardTitle>
              <p className="text-sm text-muted-foreground">
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </p>
            </div>
          </div>
          <Badge variant={localEnabled ? 'default' : 'secondary'}>
            {localEnabled ? 'Connected' : 'Disconnected'}
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Enable Toggle */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Enable sync</span>
          <Switch
            checked={localEnabled}
            onCheckedChange={handleEnabledChange}
            disabled={isUpdating}
          />
        </div>

        {/* Frequency Select */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Sync frequency</span>
          <Select
            value={localFrequency}
            onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
            disabled={!localEnabled || isUpdating}
          >
            <SelectTrigger className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.values(SyncFrequency).map((freq) => (
                <SelectItem key={freq} value={freq}>
                  {getSyncFrequencyLabel(freq)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Auto-add Toggle */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Auto-add games</span>
          <Switch
            checked={localAutoAdd}
            onCheckedChange={handleAutoAddChange}
            disabled={!localEnabled || isUpdating}
          />
        </div>
      </CardContent>

      <CardFooter className="flex items-center justify-between border-t bg-muted/50 px-6 py-4">
        <Link
          href={`/jobs?source=${config.platform}&job_type=sync`}
          className="flex items-center gap-1 text-sm text-primary hover:underline"
        >
          <History className="h-4 w-4" />
          View history
        </Link>
        <Button
          onClick={onTriggerSync}
          disabled={!localEnabled || isCurrentlySyncing}
          size="sm"
        >
          {isCurrentlySyncing ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Syncing...
            </>
          ) : (
            <>
              <RefreshCw className="mr-2 h-4 w-4" />
              Sync Now
            </>
          )}
        </Button>
      </CardFooter>
    </Card>
  );
}
```

**Step 2: Run type check**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend-next/src/components/sync/sync-service-card.tsx
git commit -m "feat(components): add SyncServiceCard component"
```

---

### Task 3.2: Create Sync Components Index

**Files:**
- Create: `frontend-next/src/components/sync/index.ts`

**Step 1: Create the index file**

```typescript
// frontend-next/src/components/sync/index.ts
export { SyncServiceCard } from './sync-service-card';
```

**Step 2: Commit**

```bash
git add frontend-next/src/components/sync/index.ts
git commit -m "feat(components): add sync components index"
```

---

## Phase 4: Sync Page

### Task 4.1: Create Sync Page

**Files:**
- Create: `frontend-next/src/app/(main)/sync/page.tsx`

**Step 1: Create the sync page**

```typescript
// frontend-next/src/app/(main)/sync/page.tsx
'use client';

import { useSyncConfigs, useUpdateSyncConfig, useTriggerSync, useSyncStatus } from '@/hooks';
import { SyncServiceCard } from '@/components/sync';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { AlertCircle, Info, ExternalLink, RefreshCw, Upload, Gamepad2 } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import type { SyncConfig, SyncConfigUpdateData } from '@/types';
import { SyncPlatform } from '@/types';

function SyncPageSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <Card key={i}>
            <CardHeader>
              <div className="flex items-center gap-3">
                <Skeleton className="h-12 w-12 rounded-lg" />
                <div>
                  <Skeleton className="mb-2 h-5 w-24" />
                  <Skeleton className="h-4 w-32" />
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {Array.from({ length: 3 }).map((_, j) => (
                <div key={j} className="flex items-center justify-between">
                  <Skeleton className="h-4 w-24" />
                  <Skeleton className="h-6 w-12" />
                </div>
              ))}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

function SyncServiceCardWithStatus({
  config,
  onUpdate,
  onTriggerSync,
  isUpdating,
}: {
  config: SyncConfig;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  isUpdating: boolean;
}) {
  const { data: status, isLoading: statusLoading } = useSyncStatus(config.platform);

  return (
    <SyncServiceCard
      config={config}
      status={status}
      onUpdate={onUpdate}
      onTriggerSync={onTriggerSync}
      isUpdating={isUpdating}
      isSyncing={statusLoading ? false : status?.isSyncing}
    />
  );
}

export default function SyncPage() {
  const router = useRouter();
  const { data: configsData, isLoading, error } = useSyncConfigs();
  const updateConfig = useUpdateSyncConfig();
  const triggerSync = useTriggerSync();

  const handleUpdateConfig = async (
    platform: SyncPlatform,
    data: SyncConfigUpdateData
  ) => {
    try {
      await updateConfig.mutateAsync({ platform, data });
      toast.success('Settings updated');
    } catch (err) {
      toast.error('Failed to update settings');
      throw err;
    }
  };

  const handleTriggerSync = async (platform: SyncPlatform) => {
    try {
      const result = await triggerSync.mutateAsync(platform);
      toast.success(`Sync started for ${platform}`);
      // Navigate to job details page
      router.push(`/jobs/${result.jobId}`);
    } catch (err) {
      const error = err as Error;
      if (error.message?.includes('409')) {
        toast.error('A sync is already in progress for this platform');
      } else {
        toast.error('Failed to start sync');
      }
    }
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <nav className="mb-4 flex text-sm text-muted-foreground" aria-label="Breadcrumb">
          <ol className="inline-flex items-center space-x-1 md:space-x-3">
            <li>
              <Link href="/dashboard" className="hover:text-foreground">
                Dashboard
              </Link>
            </li>
            <li>
              <span className="mx-2">›</span>
            </li>
            <li>
              <span className="font-medium text-foreground">Sync</span>
            </li>
          </ol>
        </nav>
        <h1 className="text-2xl font-bold">Sync</h1>
        <p className="text-muted-foreground">
          Connect and synchronize your game libraries from Steam, Epic, GOG, and other platforms.
        </p>
      </div>

      {/* Error State */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            Failed to load sync configurations. Please try again later.
          </AlertDescription>
        </Alert>
      )}

      {/* Loading State */}
      {isLoading && <SyncPageSkeleton />}

      {/* Connected Services Grid */}
      {configsData && (
        <section>
          <h2 className="mb-4 text-lg font-semibold">Connected Services</h2>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {configsData.configs.map((config) => (
              <SyncServiceCardWithStatus
                key={config.platform}
                config={config}
                onUpdate={(data) => handleUpdateConfig(config.platform, data)}
                onTriggerSync={() => handleTriggerSync(config.platform)}
                isUpdating={updateConfig.isPending}
              />
            ))}
          </div>
        </section>
      )}

      {/* Info Card */}
      <Alert>
        <Info className="h-4 w-4" />
        <AlertTitle>About Platform Syncing</AlertTitle>
        <AlertDescription>
          Platform syncing keeps your Nexorious collection in sync with your game libraries.
          When you acquire new games on Steam, Epic, or GOG, they&apos;ll automatically be
          detected and either added to your collection or queued for review.
        </AlertDescription>
      </Alert>

      {/* Quick Links */}
      <div className="flex flex-wrap gap-4 border-t pt-6 text-sm">
        <Link
          href="/review?source=sync"
          className="flex items-center gap-1 text-primary hover:underline"
        >
          <RefreshCw className="h-4 w-4" />
          Review Sync Items
        </Link>
        <Link
          href="/import"
          className="flex items-center gap-1 text-primary hover:underline"
        >
          <Upload className="h-4 w-4" />
          Import / Export
        </Link>
        <Link
          href="/games"
          className="flex items-center gap-1 text-primary hover:underline"
        >
          <Gamepad2 className="h-4 w-4" />
          View Collection
        </Link>
      </div>
    </div>
  );
}
```

**Step 2: Run type check and lint**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend-next/src/app/\(main\)/sync/page.tsx
git commit -m "feat(pages): add sync page"
```

---

### Task 4.2: Add Sync Page to Navigation

**Files:**
- Modify: `frontend-next/src/app/(main)/layout.tsx` (if navigation is there)

**Step 1: Check the layout file for navigation**

Read the layout file to understand where navigation links are defined.

**Step 2: Add sync link to navigation**

In the navigation section of the layout, add a link for Sync:

```typescript
// Add to navigation items array
{ href: '/sync', label: 'Sync', icon: RefreshCw },
```

Import the icon:
```typescript
import { RefreshCw } from 'lucide-react';
```

**Step 3: Run type check**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend-next/src/app/\(main\)/layout.tsx
git commit -m "feat(nav): add sync page to navigation"
```

---

## Phase 5: Testing

### Task 5.1: Create Sync Page Tests

**Files:**
- Create: `frontend-next/src/app/(main)/sync/page.test.tsx`

**Step 1: Create the test file**

```typescript
// frontend-next/src/app/(main)/sync/page.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import SyncPage from './page';
import * as syncApi from '@/api/sync';
import { SyncPlatform, SyncFrequency } from '@/types';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}));

// Mock sonner
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

// Mock API
vi.mock('@/api/sync');

const mockConfigs = {
  configs: [
    {
      id: '1',
      userId: 'user-1',
      platform: SyncPlatform.STEAM,
      frequency: SyncFrequency.DAILY,
      autoAdd: true,
      enabled: true,
      lastSyncedAt: '2025-01-01T00:00:00Z',
      createdAt: '2025-01-01T00:00:00Z',
      updatedAt: '2025-01-01T00:00:00Z',
    },
    {
      id: '2',
      userId: 'user-1',
      platform: SyncPlatform.EPIC,
      frequency: SyncFrequency.MANUAL,
      autoAdd: false,
      enabled: false,
      lastSyncedAt: null,
      createdAt: '2025-01-01T00:00:00Z',
      updatedAt: '2025-01-01T00:00:00Z',
    },
    {
      id: '3',
      userId: 'user-1',
      platform: SyncPlatform.GOG,
      frequency: SyncFrequency.WEEKLY,
      autoAdd: false,
      enabled: true,
      lastSyncedAt: null,
      createdAt: '2025-01-01T00:00:00Z',
      updatedAt: '2025-01-01T00:00:00Z',
    },
  ],
  total: 3,
};

const mockStatus = {
  platform: SyncPlatform.STEAM,
  isSyncing: false,
  lastSyncedAt: '2025-01-01T00:00:00Z',
  activeJobId: null,
};

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  );
}

describe('SyncPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(syncApi.getSyncConfigs).mockResolvedValue(mockConfigs);
    vi.mocked(syncApi.getSyncStatus).mockResolvedValue(mockStatus);
  });

  it('should render sync page header', async () => {
    renderWithProviders(<SyncPage />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Sync' })).toBeInTheDocument();
    });

    expect(
      screen.getByText(/Connect and synchronize your game libraries/)
    ).toBeInTheDocument();
  });

  it('should display all platform cards', async () => {
    renderWithProviders(<SyncPage />);

    await waitFor(() => {
      expect(screen.getByText('Steam')).toBeInTheDocument();
    });

    expect(screen.getByText('Epic Games')).toBeInTheDocument();
    expect(screen.getByText('GOG')).toBeInTheDocument();
  });

  it('should show loading skeleton initially', () => {
    vi.mocked(syncApi.getSyncConfigs).mockImplementation(
      () => new Promise(() => {}) // Never resolves
    );

    renderWithProviders(<SyncPage />);

    // Should show loading skeletons
    expect(screen.queryByText('Steam')).not.toBeInTheDocument();
  });

  it('should show connected badge for enabled services', async () => {
    renderWithProviders(<SyncPage />);

    await waitFor(() => {
      expect(screen.getByText('Steam')).toBeInTheDocument();
    });

    // Steam and GOG are enabled, Epic is not
    const connectedBadges = screen.getAllByText('Connected');
    const disconnectedBadges = screen.getAllByText('Disconnected');

    expect(connectedBadges).toHaveLength(2);
    expect(disconnectedBadges).toHaveLength(1);
  });

  it('should show info alert about platform syncing', async () => {
    renderWithProviders(<SyncPage />);

    await waitFor(() => {
      expect(screen.getByText('About Platform Syncing')).toBeInTheDocument();
    });
  });

  it('should show quick links', async () => {
    renderWithProviders(<SyncPage />);

    await waitFor(() => {
      expect(screen.getByText('Review Sync Items')).toBeInTheDocument();
    });

    expect(screen.getByText('Import / Export')).toBeInTheDocument();
    expect(screen.getByText('View Collection')).toBeInTheDocument();
  });
});
```

**Step 2: Run tests**

Run: `cd frontend-next && npm run test -- src/app/\(main\)/sync/page.test.tsx`
Expected: All tests pass

**Step 3: Commit**

```bash
git add frontend-next/src/app/\(main\)/sync/page.test.tsx
git commit -m "test(pages): add sync page tests"
```

---

### Task 5.2: Create SyncServiceCard Tests

**Files:**
- Create: `frontend-next/src/components/sync/sync-service-card.test.tsx`

**Step 1: Create the test file**

```typescript
// frontend-next/src/components/sync/sync-service-card.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SyncServiceCard } from './sync-service-card';
import { SyncPlatform, SyncFrequency } from '@/types';
import type { SyncConfig, SyncStatus } from '@/types';

const mockConfig: SyncConfig = {
  id: '1',
  userId: 'user-1',
  platform: SyncPlatform.STEAM,
  frequency: SyncFrequency.DAILY,
  autoAdd: true,
  enabled: true,
  lastSyncedAt: new Date().toISOString(),
  createdAt: '2025-01-01T00:00:00Z',
  updatedAt: '2025-01-01T00:00:00Z',
};

const mockStatus: SyncStatus = {
  platform: SyncPlatform.STEAM,
  isSyncing: false,
  lastSyncedAt: new Date().toISOString(),
  activeJobId: null,
};

describe('SyncServiceCard', () => {
  it('should render platform name and badge', () => {
    render(
      <SyncServiceCard
        config={mockConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
      />
    );

    expect(screen.getByText('Steam')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
  });

  it('should show disconnected badge when not enabled', () => {
    const disabledConfig = { ...mockConfig, enabled: false };

    render(
      <SyncServiceCard
        config={disabledConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
      />
    );

    expect(screen.getByText('Disconnected')).toBeInTheDocument();
  });

  it('should call onUpdate when enable toggle is changed', async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn().mockResolvedValue(undefined);

    render(
      <SyncServiceCard
        config={mockConfig}
        status={mockStatus}
        onUpdate={onUpdate}
        onTriggerSync={vi.fn()}
      />
    );

    // Find the enable sync switch (first switch on the page)
    const switches = screen.getAllByRole('switch');
    await user.click(switches[0]);

    expect(onUpdate).toHaveBeenCalledWith({ enabled: false });
  });

  it('should call onTriggerSync when sync button is clicked', async () => {
    const user = userEvent.setup();
    const onTriggerSync = vi.fn().mockResolvedValue(undefined);

    render(
      <SyncServiceCard
        config={mockConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={onTriggerSync}
      />
    );

    const syncButton = screen.getByRole('button', { name: /sync now/i });
    await user.click(syncButton);

    expect(onTriggerSync).toHaveBeenCalled();
  });

  it('should show syncing state when isSyncing is true', () => {
    const syncingStatus = { ...mockStatus, isSyncing: true };

    render(
      <SyncServiceCard
        config={mockConfig}
        status={syncingStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
        isSyncing={true}
      />
    );

    expect(screen.getByText('Syncing...')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /syncing/i })).toBeDisabled();
  });

  it('should disable sync button when not enabled', () => {
    const disabledConfig = { ...mockConfig, enabled: false };

    render(
      <SyncServiceCard
        config={disabledConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
      />
    );

    expect(screen.getByRole('button', { name: /sync now/i })).toBeDisabled();
  });

  it('should show view history link', () => {
    render(
      <SyncServiceCard
        config={mockConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
      />
    );

    expect(screen.getByText('View history')).toBeInTheDocument();
  });

  it('should format last sync time correctly', () => {
    const recentConfig = {
      ...mockConfig,
      lastSyncedAt: new Date(Date.now() - 5 * 60 * 1000).toISOString(), // 5 minutes ago
    };

    render(
      <SyncServiceCard
        config={recentConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
      />
    );

    expect(screen.getByText(/5m ago/)).toBeInTheDocument();
  });

  it('should show "Never" when never synced', () => {
    const neverSyncedConfig = { ...mockConfig, lastSyncedAt: null };

    render(
      <SyncServiceCard
        config={neverSyncedConfig}
        status={mockStatus}
        onUpdate={vi.fn()}
        onTriggerSync={vi.fn()}
      />
    );

    expect(screen.getByText(/Never/)).toBeInTheDocument();
  });
});
```

**Step 2: Run tests**

Run: `cd frontend-next && npm run test -- src/components/sync/sync-service-card.test.tsx`
Expected: All tests pass

**Step 3: Commit**

```bash
git add frontend-next/src/components/sync/sync-service-card.test.tsx
git commit -m "test(components): add SyncServiceCard tests"
```

---

## Phase 6: Final Verification

### Task 6.1: Run Full Test Suite

**Step 1: Run all frontend-next tests**

Run: `cd frontend-next && npm run test`
Expected: All tests pass

**Step 2: Run type checking**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Run lint**

Run: `cd frontend-next && npm run lint`
Expected: No errors or warnings

**Step 4: Build the project**

Run: `cd frontend-next && npm run build`
Expected: Build completes successfully

---

### Task 6.2: Manual Verification (Optional)

**Step 1: Start the development server**

Run: `cd frontend-next && npm run dev`

**Step 2: Navigate to /sync**

Open http://localhost:3000/sync in a browser

**Step 3: Verify the page displays**

- Header with "Sync" title and description
- Three service cards (Steam, Epic, GOG)
- Each card shows enable toggle, frequency dropdown, auto-add toggle
- Sync Now buttons
- Info alert about platform syncing
- Quick links at bottom

---

### Task 6.3: Final Commit and Issue Closure

**Step 1: Run beads sync**

```bash
bd sync
```

**Step 2: Update issue status**

```bash
bd update nexorious-pw50 --status in_progress
```

**Step 3: Close the issue when complete**

```bash
bd close nexorious-pw50 --reason="Implemented sync page in frontend-next with platform service cards, sync configuration, and manual sync triggering"
```

**Step 4: Final beads sync**

```bash
bd sync
```

---

## Summary

This plan creates a complete Sync page for frontend-next with:

1. **Types** (`src/types/sync.ts`): SyncPlatform, SyncFrequency enums, SyncConfig, SyncStatus interfaces
2. **API Module** (`src/api/sync.ts`): Functions for getting/updating configs, triggering syncs, managing ignored games
3. **React Query Hooks** (`src/hooks/use-sync.ts`): useSyncConfigs, useUpdateSyncConfig, useTriggerSync, etc.
4. **Components** (`src/components/sync/`): SyncServiceCard with platform icons, settings toggles, sync button
5. **Page** (`src/app/(main)/sync/page.tsx`): Full page with service cards grid, info section, quick links
6. **Tests**: Unit tests for API, hooks, components, and page

The implementation follows existing patterns in the codebase for API calls (snake_case transformation), React Query hooks (query keys, mutations with cache invalidation), and component structure (shadcn/ui components, Tailwind styling).
