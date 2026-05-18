import { api } from './client';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SyncPlatform,
  SyncFrequency,
  SteamVerifyResponse,
  EpicConnectResponse,
  EpicConnectionResponse,
  PSNConfigureResponse,
  PSNStatusResponse,
  ExternalGame,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface SyncConfigApiResponse {
  id: string;
  user_id: string;
  storefront: string;
  frequency: string;
  auto_add: boolean;
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
  storefront: string;
  is_syncing: boolean;
  last_synced_at: string | null;
  active_job_id: string | null;
}

interface ManualSyncApiResponse {
  message: string;
  job_id: string;
  storefront: string;
  status: string;
}

// ============================================================================
// Response Types
// ============================================================================

export interface SyncConfigsResponse {
  configs: SyncConfig[];
  total: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformSyncConfig(apiConfig: SyncConfigApiResponse): SyncConfig {
  return {
    id: apiConfig.id,
    userId: apiConfig.user_id,
    platform: apiConfig.storefront as SyncPlatform,
    frequency: apiConfig.frequency as SyncFrequency,
    autoAdd: apiConfig.auto_add,
    lastSyncedAt: apiConfig.last_synced_at,
    createdAt: apiConfig.created_at,
    updatedAt: apiConfig.updated_at,
    isConfigured: apiConfig.is_configured,
  };
}

function transformSyncStatus(apiStatus: SyncStatusApiResponse): SyncStatus {
  return {
    platform: apiStatus.storefront as SyncPlatform,
    isSyncing: apiStatus.is_syncing,
    lastSyncedAt: apiStatus.last_synced_at,
    activeJobId: apiStatus.active_job_id,
  };
}

function transformManualSyncResponse(apiResponse: ManualSyncApiResponse): ManualSyncResponse {
  return {
    message: apiResponse.message,
    jobId: apiResponse.job_id,
    platform: apiResponse.storefront,
    status: apiResponse.status,
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
// Epic Auth API Types
// ============================================================================

interface EpicConnectApiRequest {
  auth_code: string;
}

interface EpicConnectApiResponse {
  display_name: string;
  account_id: string;
}

interface EpicConnectionApiResponse {
  connected: boolean;
  disabled: boolean;
  display_name?: string;
  account_id?: string;
  reason?: string;
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

// ============================================================================
// Epic Auth Functions
// ============================================================================

/**
 * Connect Epic Games Store by exchanging the legendary auth code for an
 * access/refresh token. Backend runs `legendary auth --code <code>` and
 * persists the resulting state snapshot.
 */
export async function connectEpic(authCode: string): Promise<EpicConnectResponse> {
  const response = await api.post<EpicConnectApiResponse>('/sync/epic/connect', {
    auth_code: authCode,
  } as EpicConnectApiRequest);
  return {
    displayName: response.display_name,
    accountId: response.account_id,
  };
}

/**
 * Get current Epic Games Store connection status.
 */
export async function getEpicConnection(): Promise<EpicConnectionResponse> {
  const response = await api.get<EpicConnectionApiResponse>('/sync/epic/connection');
  return {
    connected: response.connected,
    disabled: response.disabled,
    displayName: response.display_name,
    accountId: response.account_id,
    reason: response.reason,
  };
}

/**
 * Disconnect Epic Games Store. Clears legendary state and per-user working dir.
 */
export async function disconnectEpic(): Promise<void> {
  await api.delete('/sync/epic/connection');
}

// ============================================================================
// PSN API Types
// ============================================================================

interface PSNConfigureApiRequest {
  npsso_token: string;
}

interface PSNConfigureApiResponse {
  success: boolean;
  online_id: string | null;
  account_id: string | null;
  region: string | null;
  message: string;
}

interface PSNStatusApiResponse {
  is_configured: boolean;
  online_id: string | null;
  account_id: string | null;
  region: string | null;
  token_expired: boolean;
}

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
    valid: response.success,
    accountId: response.account_id,
    onlineId: response.online_id,
    error: response.success ? null : response.message,
  };
}

/**
 * Get PSN connection status.
 */
export async function getPSNStatus(): Promise<PSNStatusResponse> {
  const response = await api.get<PSNStatusApiResponse>('/sync/psn/connection');

  return {
    configured: response.is_configured,
    accountId: response.account_id,
    onlineId: response.online_id,
    tokenExpired: response.token_expired,
  };
}

/**
 * Disconnect PSN integration.
 */
export async function disconnectPSN(): Promise<void> {
  await api.delete('/sync/psn/connection');
}

// ============================================================================
// External Games
// ============================================================================

export async function getExternalGames(platform: SyncPlatform): Promise<ExternalGame[]> {
  const response = await api.get<ExternalGame[]>(`/sync/${platform}/external-games`);
  return response;
}

export async function skipExternalGame(id: string): Promise<void> {
  await api.post(`/sync/ignored/${id}`);
}

export async function unskipExternalGame(id: string): Promise<void> {
  await api.delete(`/sync/ignored/${id}`);
}

export async function rematchExternalGame(
  id: string,
  igdbId: number,
  orphanAction?: 'keep' | 'remove',
): Promise<void> {
  await api.post(`/sync/external-games/${id}/rematch`, {
    igdb_id: igdbId,
    orphan_action: orphanAction ?? '',
  });
}

export async function resetSyncData(platform: SyncPlatform): Promise<void> {
  await api.delete(`/sync/${platform}/data`);
}
