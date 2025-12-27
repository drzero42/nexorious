import { api } from './client';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  IgnoredGame,
  SyncPlatform,
  SyncFrequency,
  SteamVerifyResponse,
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
  is_configured: boolean;
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
    isConfigured: apiConfig.is_configured,
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

// ============================================================================
// Steam Verification Types
// ============================================================================

interface SteamVerifyApiRequest {
  steam_id: string;
  web_api_key: string;
}

interface SteamVerifyApiResponse {
  valid: boolean;
  steam_username: string | null;
  error: string | null;
}

// ============================================================================
// Steam Verification Functions
// ============================================================================

/**
 * Verify Steam credentials before saving.
 */
export async function verifySteamCredentials(
  steamId: string,
  webApiKey: string
): Promise<SteamVerifyResponse> {
  const response = await api.post<SteamVerifyApiResponse>('/sync/steam/verify', {
    steam_id: steamId,
    web_api_key: webApiKey,
  } as SteamVerifyApiRequest);

  return {
    valid: response.valid,
    steamUsername: response.steam_username,
    error: response.error,
  };
}

/**
 * Disconnect Steam integration.
 */
export async function disconnectSteam(): Promise<void> {
  await api.delete('/sync/steam/connection');
}
